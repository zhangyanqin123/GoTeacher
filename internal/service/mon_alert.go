package service

import (
	"bytes"
	"fmt"
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

// 告警级别（语义：P0 = 用户已实际受害；P1 = 错误量异常需关注）
const (
	MonLevelP0 = "P0"
	MonLevelP1 = "P1"
)

// 推送企微的级别集合：仅 P0（新告警 INSERT 时推一次）；P1 只落表+看板。扩展新推送级别往此集合加
var monWebhookLevels = map[string]bool{MonLevelP0: true}

// monErrorTypes ERROR_SURGE 的统计口径（不含 chunk_load_error——它有专属规则；不含 probe_fail/boot）
var monErrorTypes = []string{"win_error", "vue_error", "unhandled_rejection"}

// monAlertDetail 告警聚合摘要（落 detail 列的 JSON 结构，前端解析展示）
type monAlertDetail struct {
	Project   string              `json:"project,omitempty"`  // 项目标识（企微卡片/看板展示；空=存量未知）
	TopMdl    []model.MonGroupRow `json:"top_mdl,omitempty"`  // Top3 机型（判机型兼容）
	TopSrc    []model.MonGroupRow `json:"top_src,omitempty"`  // Top3 资源 URL（判部署 404）
	TopMsg    []model.MonGroupRow `json:"top_msg,omitempty"`  // Top3 错误信息（判业务 bug）
	TopVer    []model.MonGroupRow `json:"top_ver,omitempty"`  // Top1 版本（判发版引入）
	TopCv     []model.MonGroupRow `json:"top_cv,omitempty"`   // Top3 内核版本（判老内核）
	TopRoute  []model.MonGroupRow `json:"top_route,omitempty"` // Top3 访问路由（判页面）
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

// runMonAlertOnce 对每个（环境 × 项目）执行三规则扫描 + upsert pending。
// 多项目监控：不同项目（personalCenter/f10/lbh/information）的同规则故障各自独立告警，
// 窗口内出现过事件的 distinct project 逐个检查（含空串——存量数据归为未知项目）
func (s *Service) runMonAlertOnce(ctx context.Context) {
	for _, env := range s.mon.Envs {
		windowStart := time.Now().Add(-time.Duration(s.mon.WindowMin) * time.Minute).Format(monTimeLayout)
		projects, err := s.repo.ListMonProjectsInWindow(ctx, env, windowStart)
		if err != nil {
			slog.Error("mon alert list projects failed", "env", env, "err", err)
			continue
		}
		for _, project := range projects {
			s.checkMonRule(ctx, env, project, MonRuleProbeFail, MonLevelP0, []string{"probe_fail"}, s.mon.ProbeFailThreshold)
			s.checkMonRule(ctx, env, project, MonRuleChunkSurge, MonLevelP0, []string{"chunk_load_error"}, s.mon.ChunkThreshold)
			s.checkMonRule(ctx, env, project, MonRuleErrorSurge, MonLevelP1, monErrorTypes, s.mon.ErrorThreshold)
		}
	}
}

// checkMonRule 单规则：窗口计数 → 超阈值组装 detail → upsert（pending 存在则滚动更新）。
// project 维度独立去重（dedup_key = rule|env|project）
func (s *Service) checkMonRule(ctx context.Context, env, project, ruleCode, level string, types []string, threshold int) {
	now := time.Now()
	windowStart := now.Add(-time.Duration(s.mon.WindowMin) * time.Minute).Format(monTimeLayout)
	windowEnd := now.Format(monTimeLayout)

	count, err := s.repo.CountMonEventsWindow(ctx, env, project, types, windowStart)
	if err != nil {
		slog.Error("mon alert count failed", "rule", ruleCode, "env", env, "err", err)
		return
	}
	if count <= threshold {
		return
	}

	detail := s.buildMonAlertDetail(ctx, env, project, types, ruleCode, windowStart)
	detailJSON, _ := json.Marshal(detail)
	if len(detailJSON) > 1024 { // detail 列宽保护：截断保表
		detailJSON = detailJSON[:1000]
	}
	ver := ""
	if len(detail.TopVer) > 0 {
		ver = detail.TopVer[0].Key
	}

	pending, err := s.repo.FindPendingMonAlertByRule(ctx, ruleCode, env, project)
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
		Project: project, RuleCode: ruleCode, Level: level, Env: env, Ver: ver,
		WindowStart: model.DateTimeString(windowStart), WindowEnd: model.DateTimeString(windowEnd),
		MetricValue: count, Threshold: threshold, Detail: string(detailJSON),
	}, ruleCode+"|"+env+"|"+project); err != nil {
		slog.Error("mon alert insert failed", "rule", ruleCode, "env", env, "err", err)
		return
	}
	slog.Warn("mon alert triggered", "rule", ruleCode, "env", env, "count", count, "threshold", threshold)
	if monWebhookLevels[level] { // 仅 P0 推企微；滚动 UPDATE 分支不推（新告警才推一次，不刷屏）
		s.postMonWebhook(ruleCode, level, env, project, count, threshold, detail)
	}
}

// buildMonAlertDetail 组装规则各自的聚合摘要（判型线索：机型/资源/信息/版本/样例会话）
func (s *Service) buildMonAlertDetail(ctx context.Context, env, project string, types []string,
	ruleCode, windowStart string) monAlertDetail {
	d := monAlertDetail{Project: project, ByType: map[string]int{}}
	begin, end := windowStart, time.Now().Format(monTimeLayout)
	d.TopMdl, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, project, types, "device_model", 3, false)
	for i := range d.TopMdl { // 品牌前缀（OPPO PKL110），webhook/看板摘要共用
		d.TopMdl[i].Brand = monDeviceBrand(d.TopMdl[i].Key)
	}
	d.TopVer, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, project, types, "ver", 1, false)
	d.TopCv, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, project, types, "chrome_ver", 3, false)
	d.TopRoute, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, project, types, "route", 3, false)
	d.SampleSid, _ = s.repo.FirstMonSidInWindow(ctx, env, project, types, windowStart)
	switch ruleCode {
	case MonRuleChunkSurge:
		d.TopSrc, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, project, types, "src", 3, false)
	case MonRuleErrorSurge:
		d.TopMsg, _ = s.repo.GroupMonEventsTop(ctx, begin, end, env, project, types, "msg", 3, false)
	}
	return d
}

// postMonWebhook 告警外推企微群机器人（markdown 卡片，含判型线索；失败仅记日志不重试）。
// 企微 webhook 协议：POST {"msgtype":"markdown","markdown":{"content":...}}，
// content ≤4096 字节（Top3 截断已保证），机器人限频 20 条/分钟（告警去重天然满足）
func (s *Service) postMonWebhook(ruleCode, level, env, project string, _ /*count*/, _ /*threshold*/ int, detail monAlertDetail) {
	if s.mon.WebhookURL == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**[前端监控告警]** <font color=\"warning\">%s · %s</font>\n", level, monRuleName(ruleCode))
	fmt.Fprintf(&b, "> 项目: %s ｜ 环境: %s\n", monProjectName(project), env)
	if len(detail.TopMdl) > 0 {
		fmt.Fprintf(&b, "> 机型: %s\n", monGroupText(detail.TopMdl))
	}
	if len(detail.TopCv) > 0 {
		fmt.Fprintf(&b, "> 内核: %s\n", monCvGroupText(detail.TopCv))
	}
	if len(detail.TopRoute) > 0 {
		fmt.Fprintf(&b, "> 路由: %s\n", monGroupText(detail.TopRoute))
	}
	if len(detail.TopSrc) > 0 {
		fmt.Fprintf(&b, "> 资源: %s\n", monGroupText(detail.TopSrc))
	}
	if len(detail.TopMsg) > 0 {
		fmt.Fprintf(&b, "> 消息: %s\n", monGroupText(detail.TopMsg))
	}
	if detail.SampleSid != "" {
		fmt.Fprintf(&b, "> 会话ID: %s", detail.SampleSid)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	body, _ := json.Marshal(map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{"content": b.String()},
	})
	resp, err := client.Post(s.mon.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("mon alert webhook failed", "err", err)
		return
	}
	resp.Body.Close()
	slog.Info("mon alert webhook sent", "rule", ruleCode, "env", env, "project", project, "http", resp.StatusCode)
}

// monGroupText 聚合行转「OPPO PKL110、小米/Redmi 2211133C」（不带次数）；key 超 60 字截断（资源/消息列）
func monGroupText(rows []model.MonGroupRow) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		k := r.Key
		if cut := len(k); cut > 60 {
			k = k[:60]
		}
		if r.Brand != "" {
			k = r.Brand + " " + k
		}
		parts = append(parts, k)
	}
	return strings.Join(parts, "、")
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// monRuleName 规则码中文显示名（与看板 MON_ALERT_RULES 字典语义一致），企微卡片用
func monRuleName(code string) string {
	switch code {
	case MonRuleProbeFail:
		return "有兼容性问题"
	case MonRuleChunkSurge:
		return "页面加载失败激增"
	case MonRuleErrorSurge:
		return "JS 错误激增"
	}
	return code
}

// monProjectName 项目标识显示名；空串=存量数据（接入 project 字段前）
func monProjectName(p string) string {
	if p == "" {
		return "未知（存量）"
	}
	return p
}

// monCvGroupText 内核聚合行转可读文本：数字 → Chrome N；0 → iOS/未知（与看板 chromeLabel 语义一致）
func monCvGroupText(rows []model.MonGroupRow) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		label := r.Key
		if n, err := strconv.Atoi(strings.TrimSpace(r.Key)); err == nil {
			label = "iOS/未知"
			if n > 0 {
				label = "Chrome " + itoa(n)
			}
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "、")
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
		Status: req.Status, Project: req.Project, RuleCode: req.RuleCode, Level: req.Level, Env: req.Env,
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
