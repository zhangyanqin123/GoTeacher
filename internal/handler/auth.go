package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/model"
	"handicap-service/internal/response"
	"handicap-service/internal/service"
)

// AuthHandler 鉴权 HTTP 层（login/logout/getinfo，见 PLAN-auth.md）
type AuthHandler struct {
	svc *service.Service
}

func NewAuth(svc *service.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Login POST /api/v1/login
//
// 本接口响应是全局约定的特例（见 PLAN-auth.md）：
//
//   - 失败也返回 HTTP 200 + body code 400——gyz-admin 登录页 handleError 对 reject
//     值直接调 error.includes('密码')，HTTP 4xx 时 reject 的是 Error 对象会抛
//     TypeError，登录按钮永久转圈
//
//   - token 在 body 根而非 data 内——前端 store 从响应根解构 {token, expire, passwd_expired}
//
//     @Summary		登录
//     @Description	账号密码登录，签发 Bearer token（Redis 白名单单设备模式：重新登录即互踢旧设备）。phone_code/uuid 等多余字段忽略不校验
//     @Tags			鉴权
//     @Accept			json
//     @Produce		json
//     @Param			body body model.LoginReq true "登录凭据"
//     @Success		200 {object} model.LoginResp
//     @Router			/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Debug("bind login request failed", "err", err)
		c.JSON(200, model.LoginResp{Code: 400, Msg: "用户名或密码错误"}) // 与凭据错误同文案，走前端表单定位路径
		return
	}

	token, expire, err := h.svc.Login(c.Request.Context(), req.Username, req.Password, c.ClientIP())
	switch {
	case err == nil:
		c.JSON(200, model.LoginResp{
			Code:          200,
			Msg:           "登录成功",
			Token:         token,
			Expire:        expire,
			PasswdExpired: false,
		})
	case errors.Is(err, service.ErrInvalidCredentials), errors.Is(err, service.ErrUserDisabled):
		c.JSON(200, model.LoginResp{Code: 400, Msg: err.Error()})
	default:
		slog.Error("login failed", "username", req.Username, "err", err)
		c.JSON(200, model.LoginResp{Code: 500, Msg: "服务器内部错误"})
	}
}

// Logout POST /api/v1/logout
//
//	@Summary		退出登录
//	@Description	删除 Redis 白名单使 token 立即失效（幂等）。需登录态
//	@Tags			鉴权
//	@Produce		json
//	@Success		200 {object} model.ActionResp "msg 为 退出成功"
//	@Failure		401 {object} response.Response "登录已过期，请重新登录"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	claims := currentClaims(c)
	if err := h.svc.Logout(c.Request.Context(), claims); err != nil {
		slog.Error("logout failed", "user_id", claims.UserID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
		return
	}
	response.OKMsg(c, "退出成功", nil)
}

// GetInfo GET /api/v1/getinfo
//
//	@Summary		当前用户信息
//	@Description	返回角色/昵称等（gyz-admin 动态路由依赖 roles 非空）。需登录态
//	@Tags			鉴权
//	@Produce		json
//	@Success		200 {object} model.GetInfoResp
//	@Failure		401 {object} response.Response "登录已过期，请重新登录"
//	@Security		ApiKeyAuth
//	@Router			/getinfo [get]
func (h *AuthHandler) GetInfo(c *gin.Context) {
	userID := c.GetInt64(model.CtxKeyUserID)

	info, err := h.svc.GetUserInfo(c.Request.Context(), userID)
	switch {
	case err == nil:
		response.OKMsg(c, "success", info)
	case errors.Is(err, service.ErrUnauthorized):
		response.Fail(c, 401, 401, err.Error())
	default:
		slog.Error("get user info failed", "user_id", userID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// currentClaims 从 gin.Context 还原 claims（中间件验签通过后才有值）。
// logout 只需要 UserID 定位白名单 key，其余字段不还原，避免 interface{} 断言整套字段。
func currentClaims(c *gin.Context) *service.AccessClaims {
	return &service.AccessClaims{
		UserID:   c.GetInt64(model.CtxKeyUserID),
		Username: c.GetString(model.CtxKeyUsername),
	}
}
