package model

// Teacher 老师列表行（对应前端 teacher.js mock 的字段，全量输出）
//
// 兼容约定（前端 teacherQuery.vue 直接比较，勿改）：
//   - Status 用 string：库里 TINYINT 1/0，接口必须输出 "1"/"0"（el-switch active-value="1"）
//   - qualification 存中文 '已认证'/'未认证'（el-tag 直接展示）
//   - CreatedAt/UpdatedAt 用 string：DATETIME 扫成 "2006-01-02 15:04:05"，
//     用 time.Time 会序列化成 RFC3339（带 T），前端直接展示会破格式
type Teacher struct {
	ID             int64          `json:"id"               db:"id"`
	Account        string         `json:"account"          db:"account"`
	Name           string         `json:"name"             db:"name"`
	Nickname       string         `json:"nickname"         db:"nickname"`
	Title          string         `json:"title"            db:"title"`
	Qualification  string         `json:"qualification"    db:"qualification"`
	BindSalesCount int            `json:"bindSalesCount"   db:"-"` // 相关子查询带出，不落列
	DeptID         int64          `json:"deptId"           db:"dept_id"`
	DeptName       string         `json:"deptName"         db:"dept_name"`
	Phone          string         `json:"phone"            db:"phone"`
	WorkNo         string         `json:"workNo"           db:"work_no"`
	Status         string         `json:"status"           db:"status"` // 输出 "1"/"0"
	Rating         int            `json:"rating"           db:"rating"` // 种子 1-5，编辑后 0/1/2
	Avatar         string         `json:"avatar"           db:"avatar"`
	Signature      string         `json:"signature"        db:"signature"`
	CreatedAt      DateTimeString `json:"createdAt"        db:"created_at"`
	UpdatedAt      DateTimeString `json:"updatedAt"        db:"updated_at"`
	UpdateBy       string         `json:"updateBy"         db:"update_by"`
}

// TeacherOption 老师下拉选项（含停用，离职转移弹窗用）
type TeacherOption struct {
	ID       int64  `json:"id"        db:"id"`
	Name     string `json:"name"      db:"name"`
	DeptName string `json:"deptName"  db:"dept_name"`
}

// TeacherSalesRow 老师绑定业务员行（详情弹窗用）
type TeacherSalesRow struct {
	Phone    string         `json:"phone"     db:"phone"`
	Nickname string         `json:"nickname"  db:"nickname"`
	DeptName string         `json:"deptName"  db:"dept_name"`
	BindTime DateTimeString `json:"bindTime"  db:"bind_time"`
}

// TeacherSalesBoundItem 已绑定业务员关系对（人员树过滤 + 提交合并用）
type TeacherSalesBoundItem struct {
	TeacherID int64 `json:"teacherId" db:"teacher_id"`
	UserID    int64 `json:"userId"    db:"user_id"`
}

// TeacherUpdateReq 编辑老师请求体（PUT /teacher/update）
type TeacherUpdateReq struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Rating    int    `json:"rating"` // 0 无 / 1 初级 / 2 高级
	Avatar    string `json:"avatar"`
	Signature string `json:"signature"`
}

// TeacherBindReq 绑定业务员请求体（POST /teacher/bindSales）
type TeacherBindReq struct {
	TeacherID int64   `json:"teacherId"`
	UserIDs   []int64 `json:"userIds"` // 全量替换语义，空数组 = 解绑全部
}

// TeacherListFilter 老师列表查询条件（零值字段不参与过滤）
type TeacherListFilter struct {
	DeptID          int64
	ID              int64
	Account         string // 模糊
	Nickname        string // 模糊
	Name            string // 模糊
	Title           string // 模糊
	Qualification   string // 精确（中文）
	BindSalesCount  *int   // 精确（指针区分"未传"与 0）
	Status          string // 精确 "1"/"0"
	UpdateBy        string // 模糊
	UpdateBeginTime string // yyyy-MM-dd，与 EndTime 成对生效
	UpdateEndTime   string
	PageIndex       int
	PageSize        int
}

// PageResult 分页结果（前端约定 data.list / data.count）
type PageResult struct {
	List  any `json:"list"`
	Count int `json:"count"`
}
