package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"gyz-service/internal/model"
)

// 前端监控事件业务层（mon_event，见 PLAN-frontend-monitor.md）。

// monEventTypes 探针事件类型白名单：未知类型（探针升级/伪造流量）静默丢弃仅记日志
var monEventTypes = map[string]bool{
	"probe_fail":           true,
	"chunk_load_error":     true,
	"win_error":            true,
	"resource_error":       true,
	"unhandled_rejection":  true,
	"vue_error":            true,
	"boot":                 true,
	"device":               true, // App 启动 enrich 后补发（带机型/系统，弥补 boot 上报过早无机型）
}

// 各字段入库截断上限（与表列宽一致；探针端已截一次，此处二次防御伪造超长）
const (
	monCapMax    = 512
	monCapbadMax = 128
	monUaMax     = 300
	monMsgMax    = 500
	monStackMax  = 800
	monSrcMax    = 255
	monOtherMax  = 128 // ver/wv/net/mdl/os/rt/sid 等短字段统一上限（列宽 16~128，按最宽截）
)

// IngestEvents 清洗并批量写入：类型白名单 → 逐字段 rune 截断 → cap_syntax 派生。
// 单条失败不拖垮整批（整批一次 INSERT，清洗阶段已剔除非法条目，rejected 计数返回）。
//
// 注意：msg/stack 不过 sanitize.RichText——bluemonday 会把堆栈中的 "<anonymous>" 等
// 片段当 HTML 标签剥掉破坏诊断信息；监控字段不是富文本，XSS 防线由前端 React 默认转义承担
func (s *Service) IngestEvents(ctx context.Context, events []model.MonEventIngest, ip string) (accepted, rejected int) {
	rows := make([]model.MonEventRow, 0, len(events))
	for _, e := range events {
		if !monEventTypes[e.T] {
			rejected++
			slog.Warn("mon ingest drop unknown event_type", "t", e.T, "ip", ip)
			continue
		}
		capbad := truncRunes(e.Capbad, monCapbadMax)
		rows = append(rows, model.MonEventRow{
			EventType:   e.T,
			Env:         truncRunes(e.Env, 16),
			Ver:         truncRunes(e.Ver, 32),
			ChromeVer:   e.Cv,
			Osv:         truncRunes(e.Osv, 16),
			Webview:     truncRunes(e.Wv, monOtherMax),
			NetType:     truncRunes(e.Net, 16),
			DeviceModel: truncRunes(e.Mdl, 64),
			Os:          truncRunes(e.Os, 64),
			Route:       truncRunes(e.Rt, monOtherMax),
			SessionID:   truncRunes(e.Sid, 40),
			Seq:         e.Seq,
			Capbad:      capbad,
			CapSyntax:   boolToInt(strings.Contains(capbad, "syntax")),
			Cap:         truncRunes(string(e.Cap), monCapMax),
			Ua:          truncRunes(e.Ua, monUaMax),
			Msg:         truncRunes(e.Msg, monMsgMax),
			Stack:       truncRunes(e.Stack, monStackMax),
			Src:         truncRunes(e.Src, monSrcMax),
			Ip:          truncRunes(ip, 45),
		})
	}
	if len(rows) == 0 {
		return 0, rejected
	}
	if err := s.repo.InsertMonEvents(ctx, rows); err != nil {
		slog.Error("mon ingest insert failed", "rows", len(rows), "err", err)
		return 0, len(events) // 库失败时全部计为 rejected，GET 通道仍回 204（探针无重试语义）
	}
	return len(rows), rejected
}

// truncRunes 按 rune 截断（中文友好，UTF-8 不产生半个字符）
func truncRunes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s // 字节数超但字符数未超（列宽按字符计）
	}
	return string(r[:max])
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------- 查询/概览（管理台） ----------

// 哨兵错误（中文文案即前端展示契约）
var (
	ErrMonTimeRange      = errors.New("时间范围非法（不可为空且跨度不超过 31 天）")
	ErrMonOverviewRange  = errors.New("时间范围非法（跨度不超过 7 天）")
	ErrMonEventNotFound  = errors.New("事件不存在")
)

const monTimeLayout = "2006-01-02 15:04:05"

// normalizeMonRange 校验/补全时间范围：缺省端点回退 [now-maxSpan, now]；非法格式或跨度超限返回哨兵错误
func normalizeMonRange(begin, end string, maxSpan time.Duration) (string, string, error) {
	now := time.Now()
	if end == "" {
		end = now.Format(monTimeLayout)
	}
	if begin == "" {
		begin = now.Add(-maxSpan).Format(monTimeLayout)
	}
	bt, err1 := time.ParseInLocation(monTimeLayout, begin, time.Local)
	et, err2 := time.ParseInLocation(monTimeLayout, end, time.Local)
	if err1 != nil || err2 != nil || et.Before(bt) {
		return "", "", ErrMonTimeRange
	}
	if et.Sub(bt) > maxSpan {
		return "", "", ErrMonTimeRange
	}
	return begin, end, nil
}

// ListMonEvents 事件多口径分页查询（PageResult 同项目惯例：{list, count}）
func (s *Service) ListMonEvents(ctx context.Context, req model.MonEventListReq) (model.PageResult, error) {
	begin, end, err := normalizeMonRange(req.Begin, req.End, 31*24*time.Hour)
	if err != nil {
		return model.PageResult{}, err
	}
	pageIndex, pageSize := normalizePage(req.PageIndex, req.PageSize, 20)

	list, count, err := s.repo.ListMonEvents(ctx, model.MonEventListFilter{
		Begin: begin, End: end,
		EventTypes: req.EventTypes, Env: req.Env, VerPrefix: req.Ver,
		ChromeVer: req.ChromeVer, SessionID: req.SessionID, DeviceModel: req.DeviceModel,
		Capbads: req.Capbads, CapSyntax: req.CapSyntax, Route: req.Route,
		Msg: req.Msg, Src: req.Src,
		Offset: (pageIndex - 1) * pageSize, Limit: pageSize,
	})
	if err != nil {
		return model.PageResult{}, err
	}
	return model.PageResult{List: list, Count: count}, nil
}

// GetMonEvent 事件详情（全字段：stack/cap/ua 全文）
func (s *Service) GetMonEvent(ctx context.Context, id int64) (*model.MonEventRow, error) {
	row, err := s.repo.GetMonEventByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrMonEventNotFound
	}
	return row, nil
}

// MonOverview 概览聚合：指标卡 + 五个 Top（默认近 24h，≤7 天）
func (s *Service) MonOverview(ctx context.Context, req model.MonOverviewReq) (*model.MonOverviewResp, error) {
	begin, end, err := normalizeMonRange(req.Begin, req.End, 7*24*time.Hour)
	if err != nil {
		return nil, ErrMonOverviewRange
	}
	env := req.Env

	sum, err := s.repo.SumMonEvents(ctx, begin, end, env)
	if err != nil {
		return nil, err
	}
	byType, err := s.repo.GroupMonEventsByType(ctx, begin, end, env)
	if err != nil {
		return nil, err
	}
	byVer, err := s.repo.GroupMonEventsTop(ctx, begin, end, env, nil, "ver", 10, false)
	if err != nil {
		return nil, err
	}
	byMdl, err := s.repo.GroupMonEventsTop(ctx, begin, end, env, nil, "device_model", 10, true)
	if err != nil {
		return nil, err
	}
	byCv, err := s.repo.GroupMonEventsByCvAsc(ctx, begin, end, env, 15)
	if err != nil {
		return nil, err
	}
	byRoute, err := s.repo.GroupMonEventsTop(ctx, begin, end, env, nil, "route", 10, false)
	if err != nil {
		return nil, err
	}
	// by_src 只统计 resource_error（404 部署问题的定位入口）
	bySrc, err := s.repo.GroupMonEventsTop(ctx, begin, end, env, []string{"resource_error"}, "src", 10, false)
	if err != nil {
		return nil, err
	}
	if byMdl == nil {
		byMdl = []model.MonGroupRow{}
	}
	if byVer == nil {
		byVer = []model.MonGroupRow{}
	}
	if byCv == nil {
		byCv = []model.MonGroupRow{}
	}
	if byRoute == nil {
		byRoute = []model.MonGroupRow{}
	}
	if bySrc == nil {
		bySrc = []model.MonGroupRow{}
	}
	return &model.MonOverviewResp{
		Summary: model.MonOverviewSummary{
			Total: sum.Total, Errors: sum.Errors, Sessions: sum.Sessions,
			Devices: sum.Devices, Syntax: sum.Syntax, ByType: byType,
		},
		ByVer: byVer, ByMdl: byMdl, ByCv: byCv, ByRoute: byRoute, BySrc: bySrc,
	}, nil
}
