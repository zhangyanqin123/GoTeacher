package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/model"
	"handicap-service/internal/response"
	"handicap-service/internal/service"
)

// AdminUserHandler 用户管理 HTTP 层（登录账号 CRUD，见 PLAN-admin-user.md）
type AdminUserHandler struct {
	svc *service.Service
}

func NewAdminUser(svc *service.Service) *AdminUserHandler {
	return &AdminUserHandler{svc: svc}
}

// List POST /api/v1/admin/user/list
//
// 错误映射：body 绑定失败 → 400；其他 → 500（真实原因只进日志）
//
//	@Summary		用户列表
//	@Description	分页查询登录账号（username 模糊匹配，传空串表示未填）。密码永不返回（password 列不查，json:"-" 双保险）
//	@Tags			用户管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AdminUserListReq true "查询条件（username 模糊匹配，传空串/null 表示未填）"
//	@Success		200 {object} model.AdminUserListResp
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/admin/user/list [post]
func (h *AdminUserHandler) List(c *gin.Context) {
	var req model.AdminUserListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind admin user list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch result, err := h.svc.ListAdminUsers(c.Request.Context(), req); {
	case err != nil:
		slog.Error("list admin users failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", result)
	}
}

// Add POST /api/v1/admin/user/add
//
// 错误映射：body 绑定失败/用户名已存在 → 400；其他 → 500
//
//	@Summary		新增用户
//	@Description	新增登录账号（仅用户名+密码；密码 bcrypt 哈希落库，nickname 取 username 兜底，role 固定 admin）
//	@Tags			用户管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AdminUserAddReq true "新增用户请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「新增成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 用户名已存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/admin/user/add [post]
func (h *AdminUserHandler) Add(c *gin.Context) {
	var req model.AdminUserAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind admin user add request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.CreateAdminUser(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "新增成功", nil)
	case errors.Is(err, service.ErrUsernameExists):
		response.Fail(c, 400, 400, err.Error())
	default:
		slog.Error("create admin user failed", "username", req.Username, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// Edit POST /api/v1/admin/user/edit
//
// 错误映射：body 绑定失败/用户名已存在 → 400；用户不存在 → 404；其他 → 500
//
//	@Summary		编辑用户
//	@Description	编辑登录账号（password 传空串表示不修改密码；改密码且目标非操作者本人时，目标账号立即被踢下线）
//	@Tags			用户管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AdminUserEditReq true "编辑用户请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「编辑成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 用户名已存在"
//	@Failure		404 {object} response.Response "用户不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/admin/user/edit [post]
func (h *AdminUserHandler) Edit(c *gin.Context) {
	var req model.AdminUserEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind admin user edit request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	operatorID := c.GetInt64(model.CtxKeyUserID)
	switch err := h.svc.UpdateAdminUser(c.Request.Context(), operatorID, req); {
	case err == nil:
		response.OKMsg(c, "编辑成功", nil)
	case errors.Is(err, service.ErrUsernameExists):
		response.Fail(c, 400, 400, err.Error())
	case errors.Is(err, service.ErrAdminUserNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("update admin user failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// Delete POST /api/v1/admin/user/delete
//
// 错误映射：body 绑定失败/不能删除自己 → 400；用户不存在 → 404；其他 → 500
//
//	@Summary		删除用户
//	@Description	删除登录账号并立即踢下线（DEL Redis 白名单，该账号当前 token 立即失效）；不能删除当前登录账号
//	@Tags			用户管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AdminUserDeleteReq true "删除用户请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「删除成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 不能删除当前登录账号"
//	@Failure		404 {object} response.Response "用户不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/admin/user/delete [post]
func (h *AdminUserHandler) Delete(c *gin.Context) {
	var req model.AdminUserDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind admin user delete request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	operatorID := c.GetInt64(model.CtxKeyUserID)
	switch err := h.svc.DeleteAdminUser(c.Request.Context(), operatorID, req.ID); {
	case err == nil:
		response.OKMsg(c, "删除成功", nil)
	case errors.Is(err, service.ErrCannotDeleteSelf):
		response.Fail(c, 400, 400, err.Error())
	case errors.Is(err, service.ErrAdminUserNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("delete admin user failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}
