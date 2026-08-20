package model

// 登录账号管理请求模型（admin_user 表 CRUD，见 PLAN-admin-user.md）。
// 用户信息仅用户名+密码：nickname/role 等表内字段不由前端指定，
// 新增时 nickname 取 username 兜底、role 固定 admin。

// AdminUserAddReq 新增登录账号
type AdminUserAddReq struct {
	Username string `json:"username" binding:"required,max=50" example:"tom"`
	Password string `json:"password" binding:"required,min=6,max=64" example:"123456"`
}

// AdminUserEditReq 编辑登录账号；Password 为空表示不修改密码
type AdminUserEditReq struct {
	ID       int64  `json:"id"       binding:"required,gt=0" example:"2"`
	Username string `json:"username" binding:"required,max=50" example:"tom"`
	Password string `json:"password" binding:"omitempty,min=6,max=64" example:"newpass123"`
}

// AdminUserDeleteReq 删除登录账号
type AdminUserDeleteReq struct {
	ID int64 `json:"id" binding:"required,gt=0" example:"2"`
}

// AdminUserListReq 用户列表查询（POST body，同 resign 先例）
type AdminUserListReq struct {
	Username  string `json:"username"  example:"tom"`
	PageIndex int    `json:"page_index" example:"1"`
	PageSize  int    `json:"page_size"  example:"10"`
}

// AdminUserListFilter service 归一化分页后传给 repository
type AdminUserListFilter struct {
	Username string
	Offset   int
	Limit    int
}
