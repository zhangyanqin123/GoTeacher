package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gyz-service/internal/model"
)

// 前端监控告警业务层（mon_alert + 服务内 ticker 扫描，见 PLAN-frontend-monitor.md）。
// 项目无 cron 框架：复用 cmd/server/main.go 的 signal ctx + goroutine ticker 模式，
// StartMonAlertJob 为阻塞函数，由 main 以 go 调用；单实例部署，多实例需先加分布式锁（PLAN 风险项）。

// MonAlertConfig 告警任务配置。由 router 从 config 构造传入（service 不依赖 config 包，测试易构造）
type MonAlertConfig struct {
	Enabled            bool
	IntervalMin        int      // 扫描间隔
	WindowMin          int      // 统计滚动窗口
	Envs               []string // 参与告警的环境（默认仅 production）
	ProbeFailThreshold int      // PROBE_FAIL 触发阈值（窗口内 > 阈值，默认 0 即 >0）
	ChunkThreshold     int      // CHUNK_LOAD_SURGE 阈值
	ErrorThreshold     int      // ERROR_SURGE 阈值（win+vue+unhandled 合计）
	RetentionDays      int      // mon_event 保留天数（每日分批清理）
	WebhookURL         string   // 告警 webhook（空=仅落表不推送）
}

// 告警规则常量（规则就三条且语义稳定，不做规则 CRUD——见 PLAN 决策）
const (
	MonRuleProbeFail      = "PROBE_FAIL"       // P0：能力探测失败（老内核设备出现）
	MonRuleChunkSurge     = "CHUNK_LOAD_SURGE" // P0：页面 chunk 加载失败激增（白屏事故）
	MonRuleErrorSurge     = "ERROR_SURGE"      // P1：JS 错误激增（win+vue+unhandled 合计）
)

// monErrorTypes ERROR_SURGE 的统计口径（不含 chunk_load_error——它有专属规则；不含 probe_fail/boot）
var monErrorTypes = []string{"win_error", "vue_error", "unhandled_rejection"}

// monAlertDetail 告警聚合摘要（落 detail 列的 JSON 结构，前端解析展示）
type monAlertDetail struct {
	TopMdl    []model.MonGroupRow `json:"top_mdl,omitempty"`  // Top3 机型（判机型兼容）
	TopSrc    []model.MonGroupRow `json:"top_src,omitempty"`  // Top3 资源 URL（判部署 404）
	TopMsg    []model.MonGroupRow `json:"top_msg,omitempty"`  // Top3 错误信息（判业务 bug）
	TopVer    []model.MonGroupRow `json:"top_ver,omitempty"`  // Top1 版本（判发版引入）
	ByType    map[string]int      `json:"by_type,omitempty"`
	SampleSid string              `json:"sample_sid,omitempty"` // 会话回放入口
}

var ErrMonAlertNotPending = errors.New("告警不存在或已确认")

// StartMonAlertJob 告警扫描主循环（阻塞；main 以 go 调用并传 signal ctx）。
// 启动即扫一次（部署后尽快发现存量故障），之后按间隔滚动；单次扫描 panic 由 recover 兜底
func (s *Service) StartMonAlertJob(ctx context.Context) {
	if !s.mon.Enabled {
		slog.Info("mon alert job disabled")
		return
	}
	interval := time.Duration(s.mon.IntervalMin) * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	slog.Info("mon alert job started",
		"interval_min", s.mon.IntervalMin, "window_min", s.mon.WindowMin, "envs", s.mon.Envs)

	s.runMonAlertOnce(ctx)
	lastCleanupDay := time.Now().Day()
	for {
		select {
		case <-ctx.Done():
			slog.Info("mon alert job stopped")
			return
		case <-ticker.C:
			safeRunMonAlert(func() { s.runMonAlertOnce(ctx) })
			// 每日一次保留期清理（跨天即触发一次，多次触发由 Delete 返回 0 自然收敛）
			if d := time.Now().Day(); d != lastCleanupDay {
				lastCleanupDay = d
				safeRunMonAlert(func() { s.cleanupMonEvents(ctx) })
			}
		}
	}
}

// safeRunMonAlert 单次任务 panic 兜底：任务挂掉不能带崩整个进程
func safeRunMonAlert(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("mon alert job panic", "recover", r)
		}
	}()
	fn()
}

// runMonAlertOnce 对每个告警环境执行三规则扫描 + upsert pending
func (s *Service) runMonAlertOnce(ctx context.Context) {
	for _, env := range s.mon.Envs {
		s.checkMonRule(ctx, env, MonRuleProbeFail, "P0", []string{"probe_fail"}, s.mon.ProbeFailThreshold)
		s.checkMonRule(ctx, env, MonRuleChunkSurge, "P0", []string{"chunk_load_error"}, s.mon.ChunkThreshold)
		s.checkMonRule(ctx, env, MonRuleErrorSurge, "P1", monErrorTypes, s.mon.ErrorThreshold)
	}
}

// checkMonRule 单规则：窗口计数 → 超阈值组装 detail → upsert（pending 存在则滚动更新）
func (s *Service) checkMonRule(ctx context.Context, env, ruleCode, level string, types []string, threshold int) {
	now := time.Now()
	windowStart := now.Add(-time.Duration(s.mon.WindowMin) * time.Minute).Format(monTimeLayout)
	windowEnd := now.Format(monTimeLayout)

	count, err := s.repo.CountMonEventsWindow(ctx, env, types, windowStart)
	if err != nil {
		slog.Error("mon alert count failed", "rule", ruleCode, "env", env, "err", err)
		return
	}
	if count <= threshold {
		return
	}

	detail := s.buildMonAlertDetail(ctx, env, types, ruleCode, windowStart)
	detailJSON, _ := json.Marshal(detail)
	if len(detailJSON) > 1024 { // detail 列宽保护：截断保表
		detailJSON = detailJSON[:1000]
	}
	ver := ""
	if len(detail.TopVer) > 0 {
		ver = detail.TopVer[0].Key
	}

	pending, err := s.repo.FindPendingMonAlertByRule(ctx, ruleCode, env)
	if err != nil {
		slog.Error("mon alert find pending failed", "rule", ruleCode, "env", env, "err", err)
		return
	}
	if pending != nil {
		// 持续触发：滚动更新指标与窗口（同故障不刷屏，uk_dedup_pending 兜底）
		if err := s.repo.UpdateMonAlertMetric(ctx, pending.ID, ver, windowStart, windowEnd,
			count, threshold, string(detailJSON)); err != nil {
			slog.Error("mon alert update metric failed", "id", pending.ID, "err", err)
		}
		return
	}
	if err := s.repo.InsertMonAlert(ctx, &model.MonAlert{
		RuleCode: ruleCode, Level: level, Env: env, Ver: ver,
		WindowStart: model.DateTimeString(windowStart), WindowEnd: model.DateTimeString(windowEnd),
		MetricValue: count, Threshold: threshold, Detail: string(detailJSON),
	}, ruleCode+"|"+env); err != nil {
		slog.Error("mon alert insert failed", "rule", ruleCode, "env", env, "err", err)
		return
	}
	slog.Warn("mon alert triggered", "rule", ruleCode, "env", env, "count", count, "threshold", threshold)
	s.postMonWebhook(ruleCode, level, env, count, threshold)
}

// buildMonAlertDetail 组装规则各自的聚合摘要（判型线索：机型/资源/信息/版本/样例会话）
func (s *Service) buildMonAlertDetail(ctx context.Context, env string, types []string,
	ruleCode, windowStart string) monAlertDetail {
	d := monAlertDetail{ByType: map[string]int{}}
	begin, end := windowStart, time.Now().Format(monTimeLayout)
	d.TopMdl, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, types, "device_model", 3, false)
	d.TopVer, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, types, "ver", 1, false)
	d.SampleSid, _ = s.repo.FirstMonSidInWindow(ctx, env, types, windowStart)
	switch ruleCode {
	case MonRuleChunkSurge:
		d.TopSrc, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, types, "src", 3, false)
	case MonRuleErrorSurge:
		d.TopMsg, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, types, "msg", 3, false)
	}
	return d
}

// postMonWebhook 告警外推（钉钉/企微自定义机器人通用 text JSON；失败仅记日志不重试）
func (s *Service) postMonWebhook(ruleCode, level, env string, count, threshold int) {
	if s.mon.WebhookURL == "" {
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	body, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": strings.Join([]string{
				"[前端监控告警]", level, ruleCode,
				"环境: " + env,
				"窗口事件数: " + itoa(count) + "（阈值 " + itoa(threshold) + "）",
			}, " "),
		},
	})
	resp, err := client.Post(s.mon.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("mon alert webhook failed", "err", err)
		return
	}
	resp.Body.Close()
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// cleanupMonEvents 保留期清理：分批 DELETE（LIMIT 5000）循环至删完，避免长事务锁表
func (s *Service) cleanupMonEvents(ctx context.Context) {
	before := time.Now().AddDate(0, 0, -s.mon.RetentionDays).Format(monTimeLayout)
	var total int64
	for {
		n, err := s.repo.DeleteMonEventsBefore(ctx, before)
		if err != nil {
			slog.Error("mon cleanup failed", "err", err)
			return
		}
		total += n
		if n < 5000 {
			break
		}
	}
	if total > 0 {
		slog.Info("mon cleanup done", "deleted", total, "before", before)
	}
}

// ListMonAlerts 告警列表查询（默认最新在前；status 缺省 pending 由前端显式传）
func (s *Service) ListMonAlerts(ctx context.Context, req model.MonAlertListReq) (model.PageResult, error) {
	pageIndex, pageSize := normalizePage(req.PageIndex, req.PageSize, 10)
	list, count, err := s.repo.ListMonAlerts(ctx, model.MonAlertListFilter{
		Status: req.Status, RuleCode: req.RuleCode, Level: req.Level, Env: req.Env,
		Begin: req.Begin, End: req.End,
		Offset: (pageIndex - 1) * pageSize, Limit: pageSize,
	})
	if err != nil {
		return model.PageResult{}, err
	}
	return model.PageResult{List: list, Count: count}, nil
}

// AckMonAlert 确认告警。ack_user 取鉴权上下文；并发重复确认按 404 语义（哨兵错误）
func (s *Service) AckMonAlert(ctx context.Context, id int64, ackUser, ackNote string) error {
	found, err := s.repo.UpdateMonAlertAck(ctx, id, ackUser, ackNote)
	if err != nil {
		return err
	}
	if !found {
		return ErrMonAlertNotPending
	}
	return nil
}
