package model

// Resign 离职转移记录行（对应前端 resign.js mock 的字段，全量输出）
//
// 兼容约定（前端 teacherQuery.vue 直接展示，勿改）：
//   - 姓名/部门为冗余快照（离职后老师可能被改/删，记录保留转移当时值）
//   - TransferContent 用 StringSlice：库存逗号串 'group'，接口输出 ["group"]
//   - SalesmanName/SalesmanDept 为原老师全部绑定业务员，多个逗号分隔
//   - TransferTime 等用 DateTimeString：扫描点格式化为 "2006-01-02 15:04:05"
type Resign struct {
	ID                    int64          `json:"id"                      db:"id"`
	OriginalTeacherID     int64          `json:"originalTeacherId"       db:"original_teacher_id"`
	OriginalTeacherName   string         `json:"originalTeacherName"     db:"original_teacher_name"`
	OriginalTeacherDeptID int64          `json:"originalTeacherDeptId"   db:"original_teacher_dept_id"`
	OriginalTeacherDept   string         `json:"originalTeacherDept"     db:"original_teacher_dept"`
	ReplaceTeacherID      int64          `json:"replaceTeacherId"        db:"replace_teacher_id"`
	ReplaceTeacherName    string         `json:"replaceTeacherName"      db:"replace_teacher_name"`
	ReplaceTeacherDept    string         `json:"replaceTeacherDept"      db:"replace_teacher_dept"`
	SalesmanName          string         `json:"salesmanName"            db:"salesman_name"`
	SalesmanDept          string         `json:"salesmanDept"            db:"salesman_dept"`
	TransferContent       StringSlice    `json:"transferContent"         db:"transfer_content"`
	GroupCount            int            `json:"groupCount"              db:"group_count"` // 原老师绑定业务员数（一业务员一群，后端计算）
	Operator              string         `json:"operator"                db:"operator"`
	OperateIP             string         `json:"operateIp"               db:"operate_ip"`
	TransferTime          DateTimeString `json:"transferTime"            db:"transfer_time"`
	Remark                string         `json:"remark"                  db:"remark"`
	CreatedAt             DateTimeString `json:"createdAt"               db:"created_at"`
	UpdatedAt             DateTimeString `json:"updatedAt"               db:"updated_at"`
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
	TransferContent       StringSlice
	GroupCount            int
	Operator              string
	OperateIP             string
	Remark                string
}

// ResignAddReq 新增离职转移请求体（POST /resign/add）。
// 前端还会传 originalTeacherName 等 4 个冗余字段，后端不声明（gin 忽略未知键），
// 一律从 teacher 表回查（单一事实来源）；groupCount 由后端按原老师绑定业务员数计算，不接收前端传值。
type ResignAddReq struct {
	OriginalTeacherID int64    `json:"originalTeacherId"`
	ReplaceTeacherID  int64    `json:"replaceTeacherId"`
	TransferContent   []string `json:"transferContent"` // 白名单 group，非空（传 friend 400）
	Remark            string   `json:"remark"`
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
	ID       int64  `json:"id"        db:"id"`
	Name     string `json:"name"      db:"name"`
	DeptID   int64  `json:"deptId"    db:"dept_id"`
	DeptName string `json:"deptName"  db:"dept_name"`
}

// TeacherSalesmanBrief 业务员简要信息（add 回查原老师绑定业务员用）
type TeacherSalesmanBrief struct {
	Nickname string `json:"nickname"  db:"nickname"`
	DeptName string `json:"deptName"  db:"dept_name"`
}
