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
	ID               int64          `json:"id"                db:"id"`
	UserNickName     string         `json:"userNickName"      db:"user_nick_name"`
	UserName         string         `json:"userName"          db:"user_name"`
	StockCode        string         `json:"stockCode"         db:"stock_code"`
	StockName        string         `json:"stockName"         db:"stock_name"`
	BuyPrice         float64        `json:"buyPrice"          db:"buy_price"`
	BuyNum           int64          `json:"buyNum"            db:"buy_num"`
	TeacherName      string         `json:"teacherName"       db:"teacher_name"`
	SubmitTime       DateTimeString `json:"submitTime"        db:"submit_time"`
	ReportContent    string         `json:"reportContent"     db:"report_content"`
	ReportSubmitTime DateTimeString `json:"reportSubmitTime" db:"report_submit_time"` // NULL→""
	Status           int            `json:"status"            db:"status"`
	Remark           string         `json:"remark"            db:"remark"`
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

// DiagnoseSubmitReportReq 提交诊股报告请求体（POST /diagnose/submitReport）。
// 状态 1/3/5 可提交（首次编写 / 重新提审），提交后统一回落状态 2。
type DiagnoseSubmitReportReq struct {
	ID            int64  `json:"id"`
	ReportContent string `json:"reportContent"` // 富文本 HTML
}

// DiagnoseAuditReq 审核诊股报告请求体（POST /diagnose/audit）。
// AuditType: professional 专业审核（状态 2）/ compliance 合规审核（状态 4）；
// RejectReason 仅 result=reject 时必填，富文本 HTML。
type DiagnoseAuditReq struct {
	ID           int64  `json:"id"`
	AuditType    string `json:"auditType"`
	Result       string `json:"result"` // pass / reject
	RejectReason string `json:"rejectReason"`
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
	AuditLogs []DiagnoseAuditLog `json:"auditLogs"`
}
