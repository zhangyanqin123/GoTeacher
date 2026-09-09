package model

// 前端监控告警（mon_alert 表，见 PLAN-frontend-monitor.md）。
// 由服务内 ticker 三规则写入（PROBE_FAIL / CHUNK_LOAD_SURGE / ERROR_SURGE），
// uk_dedup_pending 保证同规则+环境仅一条 pending，持续触发滚动 UPDATE，人工 ack 收口。

// MonAlert 告警行模型。Detail 为聚合摘要 JSON 串（Top 机型/cv/src/msg、样例 sid），前端解析展示
type MonAlert struct {
	ID          int64          `json:"id"           db:"id"`
	Project     string         `json:"project"      db:"project"`
	RuleCode    string         `json:"rule_code"    db:"rule_code"`
	Level       string         `json:"level"        db:"level"`
	Env         string         `json:"env"          db:"env"`
	Ver         string         `json:"ver"          db:"ver"`
	WindowStart DateTimeString `json:"window_start" db:"window_start"`
	WindowEnd   DateTimeString `json:"window_end"   db:"window_end"`
	MetricValue int            `json:"metric_value" db:"metric_value"`
	Threshold   int            `json:"threshold"    db:"threshold"`
	Detail      string         `json:"detail"       db:"detail"`
	Status      string         `json:"status"       db:"status"`
	AckUser     string         `json:"ack_user"     db:"ack_user"`
	AckNote     string         `json:"ack_note"     db:"ack_note"`
	AckedAt     DateTimeString `json:"acked_at"     db:"acked_at"`
	CreatedAt   DateTimeString `json:"created_at"   db:"created_at"`
	UpdatedAt   DateTimeString `json:"updated_at"   db:"updated_at"`
}

// MonAlertListReq 告警列表查询（status 缺省 pending；时间匹配 created_at）
type MonAlertListReq struct {
	Status    string `json:"status"     example:"pending"`
	Project   string `json:"project"    example:"personalCenter"`
	RuleCode  string `json:"rule_code"  example:"PROBE_FAIL"`
	Level     string `json:"level"      example:"P0"`
	Env       string `json:"env"        example:"production"`
	Begin     string `json:"begin"      example:"2026-09-07 00:00:00"`
	End       string `json:"end"        example:"2026-09-07 23:59:59"`
	PageIndex int    `json:"page_index" example:"1"`
	PageSize  int    `json:"page_size"  example:"10"`
}

// MonAlertListFilter service 归一化后传 repository
type MonAlertListFilter struct {
	Status   string
	Project  string
	RuleCode string
	Level    string
	Env      string
	Begin    string
	End      string
	Offset   int
	Limit    int
}

// MonAlertAckReq 告警确认。仅 pending 状态可确认；ack_user 取鉴权上下文
type MonAlertAckReq struct {
	ID      int64  `json:"id"       binding:"required,gt=0"   example:"1"`
	AckNote string `json:"ack_note" binding:"required,max=255" example:"XWEB 老内核语法不兼容，已回滚"`
}
