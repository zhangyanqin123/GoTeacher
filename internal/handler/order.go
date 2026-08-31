package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/model"
	"handicap-service/internal/response"
	"handicap-service/internal/service"
)

// OrderHandler 订单系统 HTTP 层（Gin → MySQL → RabbitMQ，见 PLAN-order.md）
type OrderHandler struct {
	svc *service.Service
}

func NewOrder(svc *service.Service) *OrderHandler {
	return &OrderHandler{svc: svc}
}

// Create POST /api/v1/orders
//
// 错误映射：body 绑定失败/数量非法/库存不足 → 400；商品不存在 → 404；其他 → 500
//
//	@Summary		创建订单
//	@Description	下单落库后发布 order.created 事件（RabbitMQ fanout），库存/积分/通知由消费者异步处理；商品快照与金额由后端回查计算，user_id 取登录态
//	@Tags			订单
//	@Accept			json
//	@Produce		json
//	@Param			body body model.OrderCreateReq true "创建订单请求体"
//	@Success		200 {object} model.OrderCreateResp "msg 固定「下单成功」，data 为订单（status=1 处理中，三个 *_status=0 待处理）"
//	@Failure		400 {object} response.Response "请求体非法 / 购买数量必须为 1-999 / 库存不足"
//	@Failure		404 {object} response.Response "商品不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/orders [post]
func (h *OrderHandler) Create(c *gin.Context) {
	var req model.OrderCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind order create request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	order, err := h.svc.CreateOrder(c.Request.Context(), req, c.GetInt64(model.CtxKeyUserID))
	switch {
	case err == nil:
		response.OKMsg(c, "下单成功", order)
	case errors.Is(err, service.ErrProductNotFound):
		response.Fail(c, 404, 404, err.Error())
	case errors.Is(err, service.ErrInvalidQuantity),
		errors.Is(err, service.ErrInsufficientStock):
		response.Fail(c, 400, 400, err.Error())
	default:
		slog.Error("create order failed",
			"product_id", req.ProductID, "quantity", req.Quantity, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// List POST /api/v1/orders/list
//
// 错误映射：body 绑定失败/status 非法 → 400；其他 → 500
//
//	@Summary		订单列表
//	@Description	分页查询订单（order_no 精确、product_name 模糊、status 精确 1/2/3；含三个异步处理步骤的状态列）
//	@Tags			订单
//	@Accept			json
//	@Produce		json
//	@Param			body body model.OrderListReq true "查询条件（空值表示未填，status 传 null 不过滤）"
//	@Success		200 {object} model.OrderListResp "查询类 msg 为 success"
//	@Failure		400 {object} response.Response "请求体非法 / 状态必须是 1/2/3"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/orders/list [post]
func (h *OrderHandler) List(c *gin.Context) {
	var req model.OrderListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind order list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}
	// status 白名单 {1,2,3}：0 与越界值直接拒（对齐 diagnose 域数值严格模式）
	if req.Status != nil && (*req.Status < 1 || *req.Status > 3) {
		response.Fail(c, 400, 400, "状态必须是 1/2/3")
		return
	}

	var f model.OrderListFilter
	f.OrderNo = req.OrderNo
	f.ProductName = req.ProductName
	if req.Status != nil {
		f.Status = *req.Status
	}
	f.PageIndex, f.PageSize = req.PageIndex, req.PageSize // 默认在 service 层兜底

	result, err := h.svc.ListOrders(c.Request.Context(), f)
	if err != nil {
		slog.Error("list orders failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
		return
	}
	response.OKMsg(c, "success", result)
}

// Products GET /api/v1/orders/products
//
//	@Summary		商品下拉
//	@Description	全量商品（含价格/库存），创建订单页选择用；data 直接为数组不分页
//	@Tags			订单
//	@Produce		json
//	@Success		200 {object} model.ProductListResp "查询类 msg 为 success"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/orders/products [get]
func (h *OrderHandler) Products(c *gin.Context) {
	list, err := h.svc.ListProducts(c.Request.Context())
	if err != nil {
		slog.Error("list products failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
		return
	}
	response.OKMsg(c, "success", list)
}

// PointsList POST /api/v1/points/list
//
//	@Summary		积分列表
//	@Description	分页查询积分流水（order.points 消费者异步写入；order_no 精确匹配快照）
//	@Tags			订单
//	@Accept			json
//	@Produce		json
//	@Param			body body model.PointsListReq true "查询条件（空值表示未填）"
//	@Success		200 {object} model.PointsListResp "查询类 msg 为 success"
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/points/list [post]
func (h *OrderHandler) PointsList(c *gin.Context) {
	var req model.PointsListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind points list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	f := model.PointsListFilter{
		OrderNo:   req.OrderNo,
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
	}
	result, err := h.svc.ListPoints(c.Request.Context(), f)
	if err != nil {
		slog.Error("list points failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
		return
	}
	response.OKMsg(c, "success", result)
}

// NotificationsList POST /api/v1/notifications/list
//
//	@Summary		通知列表
//	@Description	分页查询通知记录（order.notify 消费者异步写入；title 模糊匹配）
//	@Tags			订单
//	@Accept			json
//	@Produce		json
//	@Param			body body model.NotificationListReq true "查询条件（空值表示未填）"
//	@Success		200 {object} model.NotificationListResp "查询类 msg 为 success"
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/notifications/list [post]
func (h *OrderHandler) NotificationsList(c *gin.Context) {
	var req model.NotificationListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind notifications list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	f := model.NotificationListFilter{
		Title:     req.Title,
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
	}
	result, err := h.svc.ListNotifications(c.Request.Context(), f)
	if err != nil {
		slog.Error("list notifications failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
		return
	}
	response.OKMsg(c, "success", result)
}

// ---------- 商品管理 CRUD（POST /products/*，见 PLAN-product-crud.md） ----------

// ProductList POST /api/v1/products/list
//
// 错误映射：body 绑定失败 → 400；其他 → 500
//
//	@Summary		商品列表
//	@Description	分页查询商品（product_name 模糊匹配，传空串/null 表示未填；商品管理 tab 用，区别于 /orders/products 全量下拉）
//	@Tags			商品管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.ProductListReq true "查询条件（product_name 模糊匹配，传空串/null 表示未填）"
//	@Success		200 {object} model.ProductManageListResp
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/products/list [post]
func (h *OrderHandler) ProductList(c *gin.Context) {
	var req model.ProductListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind product list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	result, err := h.svc.ListManagedProducts(c.Request.Context(), req)
	if err != nil {
		slog.Error("list products failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
		return
	}
	response.OKMsg(c, "success", result)
}

// ProductAdd POST /api/v1/products/add
//
// 错误映射：body 绑定失败 → 400；其他 → 500
//
//	@Summary		新增商品
//	@Description	新增商品（商品名 ≤100 字符、价格 >0、库存 ≥0；商品名不做唯一性限制）
//	@Tags			商品管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.ProductAddReq true "新增商品请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「新增成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/products/add [post]
func (h *OrderHandler) ProductAdd(c *gin.Context) {
	var req model.ProductAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind product add request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	if err := h.svc.CreateProduct(c.Request.Context(), req); err != nil {
		slog.Error("create product failed", "name", req.ProductName, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
		return
	}
	response.OKMsg(c, "新增成功", nil)
}

// ProductEdit POST /api/v1/products/edit
//
// 错误映射：body 绑定失败 → 400；商品不存在 → 404；其他 → 500
//
//	@Summary		编辑商品
//	@Description	编辑商品信息（名称/价格/库存全覆盖；库存编辑与消费者异步扣减存在覆盖竞态，人工改数场景可接受）
//	@Tags			商品管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.ProductEditReq true "编辑商品请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「编辑成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		404 {object} response.Response "商品不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/products/edit [post]
func (h *OrderHandler) ProductEdit(c *gin.Context) {
	var req model.ProductEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind product edit request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.UpdateProduct(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "编辑成功", nil)
	case errors.Is(err, service.ErrProductNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("update product failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// ProductDelete POST /api/v1/products/delete
//
// 错误映射：body 绑定失败 → 400；商品不存在 → 404；其他 → 500
//
//	@Summary		删除商品
//	@Description	物理删除商品。orders.product_name 冗余快照且无外键，历史订单不受影响；删除后该商品不可下单（404 商品不存在）
//	@Tags			商品管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.ProductDeleteReq true "删除商品请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「删除成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		404 {object} response.Response "商品不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/products/delete [post]
func (h *OrderHandler) ProductDelete(c *gin.Context) {
	var req model.ProductDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind product delete request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.DeleteProduct(c.Request.Context(), req.ID); {
	case err == nil:
		response.OKMsg(c, "删除成功", nil)
	case errors.Is(err, service.ErrProductNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("delete product failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}
