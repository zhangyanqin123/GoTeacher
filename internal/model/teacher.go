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
	BindSalesCount int            `json:"bind_sales_count" db:"-"` // 相关子查询带出，不落列
	DeptID         int64          `json:"dept_id"          db:"dept_id"`
	DeptName       string         `json:"dept_name"        db:"dept_name"`
	Phone          string         `json:"phone"            db:"phone"`
	WorkNo         string         `json:"work_no"          db:"work_no"`
	Status         string         `json:"status"           db:"status"` // 输出 "1"/"0"
	Rating         int            `json:"rating"           db:"rating"` // 种子 1-5，编辑后 0/1/2
	Avatar         string         `json:"avatar"           db:"avatar"`
	Signature      string         `json:"signature"        db:"signature"`
	CreatedAt      DateTimeString `json:"created_at"       db:"created_at"`
	UpdatedAt      DateTimeString `json:"updated_at"       db:"updated_at"`
	UpdateBy       string         `json:"update_by"        db:"update_by"`
}

// TeacherOption 老师下拉选项（含停用，离职转移弹窗用）
type TeacherOption struct {
	ID       int64  `json:"id"        db:"id"`
	Name     string `json:"name"      db:"name"`
	DeptName string `json:"dept_name"  db:"dept_name"`
}

// TeacherSalesRow 老师绑定业务员行（详情弹窗用）
type TeacherSalesRow struct {
	Phone    string         `json:"phone"      db:"phone"`
	Nickname string         `json:"nickname"   db:"nickname"`
	DeptName string         `json:"dept_name"  db:"dept_name"`
	BindTime DateTimeString `json:"bind_time"  db:"bind_time"`
}

// TeacherUpdateReq 编辑老师请求体（POST /teacher/edit）
type TeacherUpdateReq struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Rating    int    `json:"rating"` // 0 无 / 1 初级 / 2 高级
	Avatar    string `json:"avatar"`
	Signature string `json:"signature"`
}

// TeacherBindReq 绑定业务员请求体（POST /teacher/bindSales）
type TeacherBindReq struct {
	TeacherID int64   `json:"teacher_id"`
	UserIDs   []int64 `json:"user_ids"` // 追加语义，仅新增绑定
}

// TeacherListReq 老师列表查询请求体（POST /teacher/list）。
// 数值字段用 string：前端筛选框（el-input/el-select-number）传空串表示未填，
// 非空才由 handler 解析为整数（空 = 不过滤），避免空串反序列化 int 报错。
type TeacherListReq struct {
	DeptID          string `json:"dept_id"`          // 精确，空串不参与过滤
	ID              string `json:"id"`               // 精确，空串不参与过滤
	Account         string `json:"account"`          // 模糊
	Nickname        string `json:"nickname"`         // 模糊
	Name            string `json:"name"`             // 模糊
	Title           string `json:"title"`            // 模糊
	Qualification   string `json:"qualification"`    // 精确（中文）
	BindSalesCount  string `json:"bind_sales_count"` // 精确，空串不参与过滤；"0" 是有效过滤值
	Status          string `json:"status"`           // 精确 "1"/"0"
	UpdateBy        string `json:"update_by"`        // 模糊
	UpdateBeginTime string `json:"update_begin_time"` // yyyy-MM-dd，与 EndTime 成对生效
	UpdateEndTime   string `json:"update_end_time"`
	PageIndex       int    `json:"page_index"`       // 默认 1（service 层兜底）
	PageSize        int    `json:"page_size"`        // 默认 10，上限 100
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
