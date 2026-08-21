package model

// Diagnose 诊股记录行（对应前端 diagnose.js mock 的字段，全量输出）
//
// 兼容约定（前端 diagnoseQuery.vue 直接展示，勿改）：
//   - 昵称/姓名/股票名/老师为冗余快照
//   - BuyPrice DECIMAL 扫 float64，接口输出 1680.5
//   - SubmitTime/ReportSubmitTime 用 DateTimeString："2006-01-02 15:04:05"；
//     report_submit_time 可空，NULL → 空串（空串=未提交，与 mock 同构）
//   - Status 数字枚举 1-6，前端 el-tag 按值配色
type Diagnose struct {
	ID               int64          `json:"id"                 db:"id"`
	UserNickName     string         `json:"user_nick_name"     db:"user_nick_name"`
	UserName         string         `json:"user_name"          db:"user_name"`
	StockCode        string         `json:"stock_code"         db:"stock_code"`
	StockName        string         `json:"stock_name"         db:"stock_name"`
	BuyPrice         float64        `json:"buy_price"          db:"buy_price"`
	BuyNum           int64          `json:"buy_num"            db:"buy_num"`
	TeacherName      string         `json:"teacher_name"       db:"teacher_name"`
	SubmitTime       DateTimeString `json:"submit_time"        db:"submit_time"`
	ReportContent    string         `json:"report_content"     db:"report_content"`
	ReportSubmitTime DateTimeString `json:"report_submit_time" db:"report_submit_time"` // NULL→""
	Status           int            `json:"status"             db:"status"`
	Remark           string         `json:"remark"             db:"remark"`
}

// DiagnoseListFilter 诊股列表查询条件（指针/零值字段不参与过滤）
type DiagnoseListFilter struct {
	ID           *int64  // 精确
	UserNickName string  // 模糊
	UserName     string  // 模糊
	StockCode    string  // 模糊
	StockName    string  // 模糊
	BuyPrice     *float64 // 精确数值
	BuyNum       *int64  // 精确数值
	TeacherName  string  // 模糊
	Status       *int    // 精确 1-6
	SubmitBeginTime string // yyyy-MM-dd，与 EndTime 成对生效（闭合区间）
	SubmitEndTime   string
	ReportBeginTime string
	ReportEndTime   string
	PageIndex    int
	PageSize     int
}

// DiagnoseListReq 诊股列表查询请求体（POST /teacher/diagnose/list）。
// id/buy_price/buy_num/status 只接受 JSON 数值（2026-08-21 收紧：数值字符串 "6" 一律 400，
// 与 resign 的 FlexInt64 宽容策略相反，前端负责把 el-input 产出转成数字再发）；
// 指针 + null/缺省 = 不过滤，传 0 是有效过滤值；昵称/姓名/股票代码/股票名/老师模糊匹配；
// status 1-6 白名单校验在 service 层兜底。
type DiagnoseListReq struct {
	ID               *int64  `json:"id" swaggertype:"integer"` // 精确
	UserNickName     string  `json:"user_nick_name"`           // 模糊
	UserName         string  `json:"user_name"`                // 模糊
	StockCode        string  `json:"stock_code"`               // 模糊
	StockName        string  `json:"stock_name"`               // 模糊
	BuyPrice         *float64 `json:"buy_price"`               // 精确，DECIMAL 等值匹配
	BuyNum           *int64  `json:"buy_num" swaggertype:"integer"` // 精确
	TeacherName      string  `json:"teacher_name"`             // 模糊
	Status           *int    `json:"status" swaggertype:"integer"`  // 精确 1-6
	SubmitBeginTime  string  `json:"submit_begin_time"`        // yyyy-MM-dd，与 EndTime 成对生效（闭合区间）
	SubmitEndTime    string  `json:"submit_end_time"`
	ReportBeginTime  string  `json:"report_begin_time"`
	ReportEndTime    string  `json:"report_end_time"`
	PageIndex        int     `json:"page_index"` // 默认 1（service 层兜底）
	PageSize         int     `json:"page_size"`  // 默认 10，上限 100
}

// DiagnoseSubmitReportReq 提交诊股报告请求体（POST /diagnose/submitReport）。
// 状态 1/3/5 可提交（首次编写 / 重新提审），提交后统一回落状态 2。
type DiagnoseSubmitReportReq struct {
	ID            int64  `json:"id"`
	ReportContent string `json:"report_content"` // 富文本 HTML
}

// DiagnoseAuditReq 审核诊股报告请求体（POST /diagnose/audit）。
// status 为前端按状态机换算的目标状态，后端校验白名单后直接落库
// （2026-08-21 由 audit_type+result 后端推导改为前端直传，换算逻辑迁至前端）：
// 3 专业驳回 / 4 专业通过转待合规 / 5 合规驳回 / 6 合规通过（终态）；
// RejectReason 仅 status 为 3/5（驳回）时必填，富文本 HTML。
type DiagnoseAuditReq struct {
	ID           int64  `json:"id"`
	Status       int    `json:"status"` // 目标状态，白名单 3/4/5/6
	RejectReason string `json:"reject_reason"` // 富文本 HTML，驳回（3/5）必填
}

// DiagnoseAuditLog 审核流程日志行（详情弹窗表格直接展示）。
// log_type/result 存中文展示串（对齐 teacher.qualification 存中文先例，读取零转换）。
type DiagnoseAuditLog struct {
	Time     DateTimeString `json:"time"     db:"log_time"`
	Type     string         `json:"type"     db:"log_type"`
	Operator string         `json:"operator" db:"operator"`
	Result   string         `json:"result"   db:"result"`
	Remark   string         `json:"remark"   db:"remark"`
}

// DiagnoseAuditLogInsert 流程日志落库参数（log_time 库端 NOW()）
type DiagnoseAuditLogInsert struct {
	DiagnoseID int64
	LogType    string
	Operator   string
	Result     string
	Remark     string
}

// DiagnoseDetail 诊股详情 = 主表全字段 + 审核流程记录
type DiagnoseDetail struct {
	Diagnose
	AuditLogs []DiagnoseAuditLog `json:"audit_logs"`
}
