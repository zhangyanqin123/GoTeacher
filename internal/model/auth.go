package model

// 鉴权相关模型（见 PLAN-auth.md）
//
// context key 常量放 model 的原因：handler（读用户信息）与 router 中间件（写入）
// 双向都要用，router 不允许被 import；model 是零依赖的公共层，两边都引它。

// gin.Context 键：中间件验签通过后写入，handler 用 c.Get/CtxKeyUserID 读取
const (
	CtxKeyUserID   = "authUserID"
	CtxKeyUsername = "authUsername"
)

// AdminUser 管理员账号（对应 admin_user 表）
type AdminUser struct {
	ID          int64          `db:"id"             json:"id"`
	Username    string         `db:"username"       json:"username"`
	Password    string         `db:"password"       json:"-"` // bcrypt 哈希，永不输出
	Nickname    string         `db:"nickname"       json:"nickname"`
	Role        string         `db:"role"           json:"role"`
	Avatar      string         `db:"avatar"         json:"avatar"`
	Status      int8           `db:"status"         json:"status"` // 1 启用 / 0 停用
	LastLoginAt DateTimeString `db:"last_login_at"  json:"last_login_at"`
	LastLoginIP string         `db:"last_login_ip"  json:"last_login_ip"`
	CreatedAt   DateTimeString `db:"created_at"     json:"created_at"`
	UpdatedAt   DateTimeString `db:"updated_at"     json:"updated_at"`
}

// LoginReq 登录请求。
// 前端 gyz-admin 登录表单还传 phone_code/uuid/rememberMe，
// gin 绑定时静默忽略未知字段，后端不校验短信验证码（见 PLAN-auth.md）。
type LoginReq struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"admin123"`
}

// LoginResp 登录响应。token 在 body 根而非 data 内——gyz-admin 的 user store
// 从响应根解构 {token, passwd_expired, expire}，此形状为前端契约，勿改。
type LoginResp struct {
	Code          int    `json:"code"           example:"200"`
	Msg           string `json:"msg"            example:"登录成功"`
	Token         string `json:"token"          example:"eyJhbGciOiJIUzI1NiIs..."`
	Expire        int64  `json:"expire"         example:"1724054400"` // JWT exp（Unix 秒）
	PasswdExpired bool   `json:"passwd_expired" example:"false"`      // 密码是否过期（暂恒 false）
	Data          any    `json:"data"`                                // 恒为 null
}

// UserInfo getinfo 返回的当前用户信息。
// roles 必须非空数组：gyz-admin 的 getInfo action 校验 roles 非空，空数组会 reject。
type UserInfo struct {
	Roles        []string `json:"roles"        example:"admin"`
	Name         string   `json:"name"         example:"系统管理员"`
	Avatar       string   `json:"avatar"       example:""`
	Introduction string   `json:"introduction" example:""`
	Permissions  []string `json:"permissions"  example:"*:*:*"`
}

// GetInfoResp getinfo 响应（文档展开用；运行时经 response.OKMsg 组装同构结果）
type GetInfoResp struct {
	Code int      `json:"code" example:"200"`
	Msg  string   `json:"msg"  example:"success"`
	Data UserInfo `json:"data"`
}
