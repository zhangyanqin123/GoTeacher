package model

// 本文件全部类型仅供 Swag 注解引用（@Success 的响应 schema），运行时不使用：
// 实际响应由 response.OKMsg/Fail 动态组装，结构与此处声明保持一致。
//
// 之所以单独声明：PageResult.List 是 any，Swag 无法展开分页记录的字段；
// 文档专用类型给出 data 的具体形状，Swagger UI 即可展示每条记录的 schema。

// ActionResp 写操作通用响应（update/bind/resign add/submitReport/audit）
type ActionResp struct {
	Code int    `json:"code" example:"200"`
	Msg  string `json:"msg"  example:"编辑成功"`
	Data any    `json:"data"` // 恒为 null
}

// TeacherListResp 老师列表响应（GET /chatSys/teacher/list）
type TeacherListResp struct {
	Code int    `json:"code" example:"200"`
	Msg  string `json:"msg"  example:"success"`
	Data struct {
		List  []Teacher `json:"list"`
		Count int       `json:"count" example:"42"`
	} `json:"data"`
}

// TeacherOptionsResp 老师下拉选项响应（GET /chatSys/teacher/options）
type TeacherOptionsResp struct {
	Code int             `json:"code" example:"200"`
	Msg  string          `json:"msg"  example:"success"`
	Data []TeacherOption `json:"data"`
}

// TeacherDetailResp 老师详情响应（GET /teacher/detail）
type TeacherDetailResp struct {
	Code int           `json:"code" example:"200"`
	Msg  string        `json:"msg"  example:"success"`
	Data TeacherDetail `json:"data"`
}

// TeacherSalesListResp 老师绑定业务员列表响应（GET /teacher/bind/salesman/list）。
// data 回显 pageIndex/pageSize（驼峰为本接口前端约定，snake_case 全链路约束的例外）。
type TeacherSalesListResp struct {
	Code int    `json:"code" example:"200"`
	Msg  string `json:"msg"  example:"success"`
	Data struct {
		List      []TeacherSalesRow `json:"list"`
		Count     int               `json:"count" example:"3"`
		PageIndex int               `json:"pageIndex" example:"1"`
		PageSize  int               `json:"pageSize" example:"5"`
	} `json:"data"`
}

// TeacherBoundResp 全量已绑定业务员响应（GET /chatSys/teacher/bindSales/boundUserIds）
type TeacherBoundResp struct {
	Code int     `json:"code" example:"200"`
	Msg  string  `json:"msg"  example:"success"`
	Data []int64 `json:"data" example:"1"`
}

// ResignListResp 离职转移列表响应（POST /teacher/resign/list）
type ResignListResp struct {
	Code int    `json:"code" example:"200"`
	Msg  string `json:"msg"  example:"success"`
	Data struct {
		List  []Resign `json:"list"`
		Count int      `json:"count" example:"7"`
	} `json:"data"`
}

// AdminUserListResp 用户管理列表响应（POST /admin/user/list）
type AdminUserListResp struct {
	Code int    `json:"code" example:"200"`
	Msg  string `json:"msg"  example:"success"`
	Data struct {
		List  []AdminUser `json:"list"`
		Count int         `json:"count" example:"2"`
	} `json:"data"`
}

// DiagnoseListResp 诊股列表响应（GET /diagnose/list）
type DiagnoseListResp struct {
	Code int    `json:"code" example:"200"`
	Msg  string `json:"msg"  example:"success"`
	Data struct {
		List  []Diagnose `json:"list"`
		Count int        `json:"count" example:"15"`
	} `json:"data"`
}

// DiagnoseDetailResp 诊股详情响应（GET /diagnose/detail）
type DiagnoseDetailResp struct {
	Code int            `json:"code" example:"200"`
	Msg  string         `json:"msg"  example:"success"`
	Data DiagnoseDetail `json:"data"`
}

// XeLoginURLResp 小鹅通登录链接响应（GET /guyuzhoudb/live/get_login_url，透传 xe.login.url/1.0.0）。
// 注意：该接口路径不在 /api/v1 下，Swagger UI 因全局 BasePath 会显示成 /api/v1/guyuzhoudb/...（已知瑕疵，见 PLAN-live.md）。
type XeLoginURLResp struct {
	Code int    `json:"code" example:"200"`
	Msg  string `json:"msg"  example:"success"`
	Data struct {
		LoginURL            string `json:"login_url" example:"https://h5.xiaoe-tech.com/platform/login_cooperate/h5_login?token=xxx&app_id=appxxx"`
		PermissionDeniedURL string `json:"permission_denied_url" example:""`
	} `json:"data"`
}

// XeRegisterUserResp 注册小鹅通用户响应（GET /guyuzhoudb/live/register_user，透传 xe.user.register/1.0.0）。
// 同 XeLoginURLResp 的 BasePath 显示瑕疵（实际路径 /guyuzhoudb/live/register_user）。
type XeRegisterUserResp struct {
	Code int    `json:"code" example:"200"`
	Msg  string `json:"msg"  example:"success"`
	Data struct {
		UserID     string `json:"user_id" example:"u_api_6a8beb36e8fc5_AhdVFUMFJQ"`
		UserExists int    `json:"user_exists" example:"1"`
	} `json:"data"`
}
