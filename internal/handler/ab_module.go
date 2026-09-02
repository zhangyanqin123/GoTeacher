package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/model"
	"handicap-service/internal/response"
	"handicap-service/internal/service"
)

// AbModuleHandler AB 版模块配置 HTTP 层（模块/配置项 CRUD + H5 聚合，见 PLAN-ab-module.md）
type AbModuleHandler struct {
	svc *service.Service
}

func NewAbModule(svc *service.Service) *AbModuleHandler {
	return &AbModuleHandler{svc: svc}
}

// ModuleList POST /api/v1/ab/modules/list
//
// 错误映射：body 绑定失败 → 400；其他 → 500
//
//	@Summary		模块列表
//	@Description	分页查询 AB 版模块（module_key/module_name 均模糊匹配，传空串表示未填），list 项含 item_count（模块下配置项数）
//	@Tags			AB 模块配置
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AbModuleListReq true "查询条件（module_key/module_name 模糊匹配）"
//	@Success		200 {object} model.AbModuleListResp
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/ab/modules/list [post]
func (h *AbModuleHandler) ModuleList(c *gin.Context) {
	var req model.AbModuleListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind ab module list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch result, err := h.svc.ListAbModules(c.Request.Context(), req); {
	case err != nil:
		slog.Error("list ab modules failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", result)
	}
}

// ModuleOptions GET /api/v1/ab/modules/options
//
//	@Summary		模块下拉选项
//	@Description	全量模块下拉（id/module_key/module_name，配置项管理页选父模块用），不分页
//	@Tags			AB 模块配置
//	@Produce		json
//	@Success		200 {object} model.AbModuleOptionsResp
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/ab/modules/options [get]
func (h *AbModuleHandler) ModuleOptions(c *gin.Context) {
	switch list, err := h.svc.AbModuleOptions(c.Request.Context()); {
	case err != nil:
		slog.Error("list ab module options failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", list)
	}
}

// ModuleAdd POST /api/v1/ab/modules/add
//
// 错误映射：body 绑定失败/标识格式非法/标识已存在 → 400；其他 → 500
//
//	@Summary		新增模块
//	@Description	新增 AB 版模块（module_key 小写字母开头，可含小写字母/数字/下划线，创建后不可改）
//	@Tags			AB 模块配置
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AbModuleAddReq true "新增模块请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「新增成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 标识格式非法 / 模块标识已存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/ab/modules/add [post]
func (h *AbModuleHandler) ModuleAdd(c *gin.Context) {
	var req model.AbModuleAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind ab module add request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.CreateAbModule(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "新增成功", nil)
	case errors.Is(err, service.ErrAbModuleKeyInvalid), errors.Is(err, service.ErrAbModuleKeyExists):
		response.Fail(c, 400, 400, err.Error())
	default:
		slog.Error("create ab module failed", "module_key", req.ModuleKey, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// ModuleEdit POST /api/v1/ab/modules/edit
//
// 错误映射：body 绑定失败 → 400；模块不存在 → 404；其他 → 500
//
//	@Summary		编辑模块
//	@Description	编辑 AB 版模块（module_key 不可改：请求体不含该字段，业务标识被 H5 代码引用）
//	@Tags			AB 模块配置
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AbModuleEditReq true "编辑模块请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「编辑成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		404 {object} response.Response "模块不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/ab/modules/edit [post]
func (h *AbModuleHandler) ModuleEdit(c *gin.Context) {
	var req model.AbModuleEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind ab module edit request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.UpdateAbModule(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "编辑成功", nil)
	case errors.Is(err, service.ErrAbModuleNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("update ab module failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// ModuleDelete POST /api/v1/ab/modules/delete
//
// 错误映射：body 绑定失败/存在子项 → 400；模块不存在 → 404；其他 → 500
//
//	@Summary		删除模块
//	@Description	删除 AB 版模块；模块下存在配置项时拒绝删除（不级联删，先删全部配置项再删模块）
//	@Tags			AB 模块配置
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AbModuleDeleteReq true "删除模块请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「删除成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 模块下存在配置项，请先删除全部配置项"
//	@Failure		404 {object} response.Response "模块不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/ab/modules/delete [post]
func (h *AbModuleHandler) ModuleDelete(c *gin.Context) {
	var req model.AbModuleDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind ab module delete request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.DeleteAbModule(c.Request.Context(), req.ID); {
	case err == nil:
		response.OKMsg(c, "删除成功", nil)
	case errors.Is(err, service.ErrAbModuleHasItems):
		response.Fail(c, 400, 400, err.Error())
	case errors.Is(err, service.ErrAbModuleNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("delete ab module failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// ItemList POST /api/v1/ab/items/list
//
// 错误映射：body 绑定失败 → 400；其他 → 500
//
//	@Summary		配置项列表
//	@Description	分页查询配置项（module_id 精确过滤，传 0/不传 = 不过滤；item_key 模糊匹配），项含所属模块 module_key 与可见版本数组
//	@Tags			AB 模块配置
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AbModuleItemListReq true "查询条件"
//	@Success		200 {object} model.AbModuleItemListResp
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/ab/items/list [post]
func (h *AbModuleHandler) ItemList(c *gin.Context) {
	var req model.AbModuleItemListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind ab item list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch result, err := h.svc.ListAbModuleItems(c.Request.Context(), req); {
	case err != nil:
		slog.Error("list ab items failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", result)
	}
}

// ItemAdd POST /api/v1/ab/items/add
//
// 错误映射：body 绑定失败/标识格式非法/版本值域非法/重名 → 400；模块不存在 → 404；其他 → 500
//
//	@Summary		新增配置项
//	@Description	在模块下新增配置项（item_key 为 H5 代码 camelCase 常量原文如 topBanner，创建后不可改；versions 值域 mass/data）
//	@Tags			AB 模块配置
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AbModuleItemAddReq true "新增配置项请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「新增成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 标识格式非法 / 可见版本仅支持 mass 或 data / 该模块下配置项标识已存在"
//	@Failure		404 {object} response.Response "模块不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/ab/items/add [post]
func (h *AbModuleHandler) ItemAdd(c *gin.Context) {
	var req model.AbModuleItemAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind ab item add request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.CreateAbModuleItem(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "新增成功", nil)
	case errors.Is(err, service.ErrAbItemKeyInvalid),
		errors.Is(err, service.ErrAbVersionsInvalid),
		errors.Is(err, service.ErrAbItemKeyExists):
		response.Fail(c, 400, 400, err.Error())
	case errors.Is(err, service.ErrAbModuleNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("create ab item failed", "module_id", req.ModuleID, "item_key", req.ItemKey, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// ItemEdit POST /api/v1/ab/items/edit
//
// 错误映射：body 绑定失败/版本值域非法/重名 → 400；配置项或模块不存在 → 404；其他 → 500
//
//	@Summary		编辑配置项
//	@Description	编辑配置项（item_key 不可改：请求体不含该字段；module_id 可改 = 挪模块；versions 值域 mass/data）
//	@Tags			AB 模块配置
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AbModuleItemEditReq true "编辑配置项请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「编辑成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 可见版本仅支持 mass 或 data / 该模块下配置项标识已存在"
//	@Failure		404 {object} response.Response "配置项不存在 / 模块不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/ab/items/edit [post]
func (h *AbModuleHandler) ItemEdit(c *gin.Context) {
	var req model.AbModuleItemEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind ab item edit request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.UpdateAbModuleItem(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "编辑成功", nil)
	case errors.Is(err, service.ErrAbVersionsInvalid),
		errors.Is(err, service.ErrAbItemKeyExists):
		response.Fail(c, 400, 400, err.Error())
	case errors.Is(err, service.ErrAbItemNotFound),
		errors.Is(err, service.ErrAbModuleNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("update ab item failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// ItemDelete POST /api/v1/ab/items/delete
//
// 错误映射：body 绑定失败 → 400；配置项不存在 → 404；其他 → 500
//
//	@Summary		删除配置项
//	@Description	删除配置项（该配置项对应模块 UI 在 H5 全版本隐藏）
//	@Tags			AB 模块配置
//	@Accept			json
//	@Produce		json
//	@Param			body body model.AbModuleItemDeleteReq true "删除配置项请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「删除成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		404 {object} response.Response "配置项不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/ab/items/delete [post]
func (h *AbModuleHandler) ItemDelete(c *gin.Context) {
	var req model.AbModuleItemDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind ab item delete request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.DeleteAbModuleItem(c.Request.Context(), req.ID); {
	case err == nil:
		response.OKMsg(c, "删除成功", nil)
	case errors.Is(err, service.ErrAbItemNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("delete ab item failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// AbConfig GET /api/v1/ab/config（免鉴权，公开不挂 Auth：H5 无本服务登录态，公网域名直访）
//
//	@Summary		H5 聚合配置
//	@Description	返回全部模块的 AB 版显隐配置两级 map：模块标识 → 配置项标识（camelCase 原文，如 topBanner）→ 可见版本数组（mass 大众版 / data 数据版）。data 无 list 包装，空模块输出空对象。公开接口无鉴权
//	@Tags			AB 模块配置
//	@Produce		json
//	@Success		200 {object} model.AbConfigResp
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/ab/config [get]
func (h *AbModuleHandler) AbConfig(c *gin.Context) {
	switch cfg, err := h.svc.AbConfig(c.Request.Context()); {
	case err != nil:
		slog.Error("get ab config failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", cfg)
	}
}
