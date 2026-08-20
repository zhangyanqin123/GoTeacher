package model

// Resign 离职转移记录行（对应前端 resign.js mock 的字段，除审计列外全量输出）
//
// 兼容约定（前端 teacherQuery.vue 直接展示，勿改）：
//   - 姓名/部门为冗余快照（离职后老师可能被改/删，记录保留转移当时值）
//   - SalesmanName/SalesmanDept 为原老师全部绑定业务员，多个逗号分隔
//   - TransferContent 为转移内容自由文本（2026-08-19 由 remark 改名，如「首席投顾」）
//   - TransferTime 等用 DateTimeString：扫描点格式化为 "2006-01-02 15:04:05"
//   - operate_ip 仅审计落库不查询返回（2026-08-20 列表移除「操作ip」展示，见 ResignInsert.OperateIP）
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
	OperateIP             string // 审计留痕：handler 取 c.ClientIP() 落库，列表接口不返回（2026-08-20 移除展示，列与数据保留）
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
// 三个姓名筛选均为模糊匹配（前端为文本输入框而非下拉，2026-08-19 对齐真实接口契约）：
// 姓名/业务员按落库冗余快照列匹配（离职后老师可能被改/删，快照保留转移当时值），
// 空串不过滤。dept_id 精确匹配原老师部门快照（FlexInt64 宽容解析，同 TeacherListReq）。
type ResignListReq struct {
	DeptID            FlexInt64 `json:"dept_id" swaggertype:"integer"` // 精确，匹配 original_teacher_dept_id 快照，''/null/缺省不过滤
	OriginalTeacher   string    `json:"original_teacher"`              // 模糊，匹配 original_teacher_name
	ReplaceTeacher    string    `json:"replace_teacher"`              // 模糊，匹配 replace_teacher_name
	Salesman          string    `json:"salesman"`                     // 模糊，匹配 salesman_name
	TransferBeginTime string    `json:"transfer_begin_time"`          // yyyy-MM-dd，与 EndTime 成对生效
	TransferEndTime   string    `json:"transfer_end_time"`
	PageIndex         int       `json:"page_index"` // 默认 1（service 层兜底）
	PageSize          int       `json:"page_size"`  // 默认 10，上限 100
}

// ResignListFilter 离职转移列表查询条件（零值字段不参与过滤）
type ResignListFilter struct {
	DeptID            int64  // 精确，匹配 original_teacher_dept_id 快照
	OriginalTeacher   string // 模糊，匹配 original_teacher_name 快照
	ReplaceTeacher    string // 模糊，匹配 replace_teacher_name 快照
	Salesman          string // 模糊，匹配 salesman_name 快照
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
