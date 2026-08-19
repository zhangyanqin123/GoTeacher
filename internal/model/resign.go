package model

// Resign 离职转移记录行（对应前端 resign.js mock 的字段，全量输出）
//
// 兼容约定（前端 teacherQuery.vue 直接展示，勿改）：
//   - 姓名/部门为冗余快照（离职后老师可能被改/删，记录保留转移当时值）
//   - SalesmanName/SalesmanDept 为原老师全部绑定业务员，多个逗号分隔
//   - TransferContent 为转移内容自由文本（2026-08-19 由 remark 改名，如「首席投顾」）
//   - TransferTime 等用 DateTimeString：扫描点格式化为 "2006-01-02 15:04:05"
type Resign struct {
	ID                    int64          `json:"id"                      db:"id"`
	OriginalTeacherID     int64          `json:"original_teacher_id"      db:"original_teacher_id"`
	OriginalTeacherName   string         `json:"original_teacher_name"    db:"original_teacher_name"`
	OriginalTeacherDeptID int64          `json:"original_teacher_dept_id" db:"original_teacher_dept_id"`
	OriginalTeacherDept   string         `json:"original_teacher_dept"    db:"original_teacher_dept"`
	ReplaceTeacherID      int64          `json:"replace_teacher_id"       db:"replace_teacher_id"`
	ReplaceTeacherName    string         `json:"replace_teacher_name"     db:"replace_teacher_name"`
	ReplaceTeacherDept    string         `json:"replace_teacher_dept"     db:"replace_teacher_dept"`
	SalesmanName          string         `json:"salesman_name"            db:"salesman_name"`
	SalesmanDept          string         `json:"salesman_dept"            db:"salesman_dept"`
	GroupCount            int            `json:"group_count"              db:"group_count"` // 原老师绑定业务员数（一业务员一群，后端计算）
	Operator              string         `json:"operator"                 db:"operator"`
	OperateIP             string         `json:"operate_ip"               db:"operate_ip"`
	TransferTime          DateTimeString `json:"transfer_time"            db:"transfer_time"`
	TransferContent       string         `json:"transfer_content"         db:"transfer_content"`
	CreatedAt             DateTimeString `json:"created_at"               db:"created_at"`
	UpdatedAt             DateTimeString `json:"updated_at"               db:"updated_at"`
}

// ResignInsert 新增离职转移落库参数（service 组装完冗余快照后传给 repository）
type ResignInsert struct {
	OriginalTeacherID     int64
	OriginalTeacherName   string
	OriginalTeacherDeptID int64
	OriginalTeacherDept   string
	ReplaceTeacherID      int64
	ReplaceTeacherName    string
	ReplaceTeacherDept    string
	SalesmanName          string
	SalesmanDept          string
	GroupCount            int
	Operator              string
	OperateIP             string
	TransferContent       string
}

// ResignAddReq 新增离职转移请求体（POST /teacher/resign/add）。
// 前端可能多传 original_teacher_name 等冗余字段，后端不声明（gin 忽略未知键），
// 一律从 teacher 表回查（单一事实来源）；group_count 由后端按原老师绑定业务员数计算，不接收前端传值。
// transfer_content 为转移内容自由文本（2026-08-19 由原 remark 改名而来，如「首席投顾」）。
type ResignAddReq struct {
	OriginalTeacherID int64  `json:"original_teacher_id"`
	ReplaceTeacherID  int64  `json:"replace_teacher_id"`
	TransferContent   string `json:"transfer_content"` // 自由文本，≤200 字符
}

// ResignListReq 离职转移列表查询请求体（POST /teacher/resign/list）。
// 数值字段用 FlexInt64：前端 el-select 清空产出 ''/null，统一宽容归一为未设置
//（同 TeacherListReq），''/null/缺省不过滤，非法串仍报错。
type ResignListReq struct {
	DeptID            FlexInt64 `json:"dept_id" swaggertype:"integer"`            // 精确，匹配原老师部门
	OriginalTeacherID FlexInt64 `json:"original_teacher_id" swaggertype:"integer"` // 精确
	ReplaceTeacherID  FlexInt64 `json:"replace_teacher_id" swaggertype:"integer"`  // 精确
	SalesmanName      string    `json:"salesman_name"`       // 模糊
	TransferBeginTime string    `json:"transfer_begin_time"` // yyyy-MM-dd，与 EndTime 成对生效
	TransferEndTime   string    `json:"transfer_end_time"`
	PageIndex         int       `json:"page_index"` // 默认 1（service 层兜底）
	PageSize          int       `json:"page_size"`  // 默认 10，上限 100
}

// ResignListFilter 离职转移列表查询条件（零值字段不参与过滤）
type ResignListFilter struct {
	DeptID            int64  // 匹配 original_teacher_dept_id（mock 同构）
	OriginalTeacherID int64  // 精确
	ReplaceTeacherID  int64  // 精确
	SalesmanName      string // 模糊
	TransferBeginTime string // yyyy-MM-dd，与 EndTime 成对生效
	TransferEndTime   string
	PageIndex         int
	PageSize          int
}

// TeacherBrief 老师简要信息（add 回查姓名/部门用，避免污染 teacher.go）
type TeacherBrief struct {
	ID       int64  `json:"id"         db:"id"`
	Name     string `json:"name"       db:"name"`
	DeptID   int64  `json:"dept_id"    db:"dept_id"`
	DeptName string `json:"dept_name"  db:"dept_name"`
}

// TeacherSalesmanBrief 业务员简要信息（add 回查原老师绑定业务员用）
type TeacherSalesmanBrief struct {
	Nickname string `json:"nickname"   db:"nickname"`
	DeptName string `json:"dept_name"  db:"dept_name"`
}
