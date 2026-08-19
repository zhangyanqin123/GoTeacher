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
