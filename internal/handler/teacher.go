package handler

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"gyz-service/internal/model"
	"gyz-service/internal/response"
	"gyz-service/internal/service"
)

// TeacherHandler 老师管理 HTTP 层
type TeacherHandler struct {
	svc *service.Service
}

func NewTeacher(svc *service.Service) *TeacherHandler {
	return &TeacherHandler{svc: svc}
}

// List POST /api/v1/dxsf/teacher/list
//
// 错误映射：body 绑定失败/数字参数格式错误 → 400；其他 → 500（真实原因只进日志）
//
//	@Summary		老师列表
//	@Description	分页查询老师（筛选条件走 JSON body），未传/null 字段不参与过滤；姓名/账号/昵称/头衔/操作人模糊匹配，部门/ID/认证/状态精确匹配
//	@Tags			老师管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.TeacherListReq true "查询条件（数值字段传 null 表示未填）"
//	@Success		200 {object} model.TeacherListResp
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/list [post]
func (h *TeacherHandler) List(c *gin.Context) {
	var req model.TeacherListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind teacher list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	var f model.TeacherListFilter
	f.DeptID = derefOrZero(req.DeptID.Ptr())
	f.ID = derefOrZero(req.ID.Ptr())
	if p := req.BindSalesCount.Ptr(); p != nil { // 0 是有效过滤值，指针区分"未传"
		v := int(*p)
		f.BindSalesCount = &v
	}
	if p := req.Status.Ptr(); p != nil && *p != -1 { // 前端下拉 -1 全部；0 停用是有效过滤值
		v := int(*p)
		f.StatusFilter = &v
	}
	f.Account = req.Account
	f.Nickname = req.Nickname
	f.Name = req.Name
	f.Title = req.Title
	f.Qualification = req.Qualification
	f.UpdateBy = req.UpdateBy
	f.UpdateBeginTime = req.UpdateBeginTime
	f.UpdateEndTime = req.UpdateEndTime
	f.PageIndex, f.PageSize = req.PageIndex, req.PageSize // 默认在 service 层兜底

	result, err := h.svc.ListTeachers(c.Request.Context(), f)
	switch {
	case err != nil:
		slog.Error("list teachers failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", result)
	}
}

// Options GET /api/v1/dxsf/teacher/options
//
//	@Summary		老师下拉选项
//	@Description	全量老师选项（含停用，离职转移弹窗用）
//	@Tags			老师管理
//	@Produce		json
//	@Success		200 {object} model.TeacherOptionsResp
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/options [get]
func (h *TeacherHandler) Options(c *gin.Context) {
	list, err := h.svc.ListTeacherOptions(c.Request.Context())
	switch {
	case err != nil:
		slog.Error("list teacher options failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", list)
	}
}

// BoundUserIds GET /api/v1/dxsf/teacher/bind/salesman/users
//
// 全量已绑定业务员 userId（前端人员树过滤用）
//
//	@Summary		全量已绑定业务员
//	@Description	返回全部已绑定业务员 userId（去重平铺数组），供人员树过滤已绑定人员
//	@Tags			绑定业务员
//	@Produce		json
//	@Success		200 {object} model.TeacherBoundResp
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/bind/salesman/users [get]
func (h *TeacherHandler) BoundUserIds(c *gin.Context) {
	list, err := h.svc.ListAllTeacherSales(c.Request.Context())
	switch {
	case err != nil:
		slog.Error("list bound user ids failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", list)
	}
}

// Detail GET /api/v1/dxsf/teacher/detail
//
// 错误映射：id 缺失/非法 → 400；老师不存在 → 404；其他 → 500
//
//	@Summary		老师详情
//	@Description	编辑弹窗回显（昵称/头衔/评级/头像/签名；列 rating/signature 映射接口 level/sign）
//	@Tags			老师管理
//	@Produce		json
//	@Param			id query integer true "老师 ID"
//	@Success		200 {object} model.TeacherDetailResp
//	@Failure		400 {object} response.Response "id 缺失或非法"
//	@Failure		404 {object} response.Response "老师不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/detail [get]
func (h *TeacherHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Fail(c, 400, 400, "参数 id 不能为空")
		return
	}

	detail, err := h.svc.GetTeacherDetail(c.Request.Context(), id)
	switch {
	case err != nil:
		slog.Error("get teacher detail failed", "id", id, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	case detail == nil:
		response.Fail(c, 404, 404, service.ErrTeacherNotFound.Error())
	default:
		response.OKMsg(c, "success", detail)
	}
}

// Update POST /api/v1/dxsf/teacher/edit
//
// 错误映射：body 绑定失败/非法 level/签名超长 → 400；老师不存在 → 404；其他 → 500
//
//	@Summary		编辑老师
//	@Description	编辑头衔/评级/头像/签名（仅这 4 个字段可改，冗余快照字段一律忽略；level 0 无 / 3 初级 / 5 高级）
//	@Tags			老师管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.TeacherUpdateReq true "编辑老师请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「编辑成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / level 非 0/3/5 / 签名超过 200 字符"
//	@Failure		404 {object} response.Response "老师不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/edit [post]
func (h *TeacherHandler) Update(c *gin.Context) {
	var req model.TeacherUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind teacher update request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.UpdateTeacher(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "编辑成功", nil)
	case errors.Is(err, service.ErrInvalidLevel):
		response.Fail(c, 400, 400, err.Error())
	case errors.Is(err, service.ErrSignatureTooLong):
		response.Fail(c, 400, 400, err.Error())
	case errors.Is(err, service.ErrTeacherNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("update teacher failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// SalesList GET /api/v1/dxsf/teacher/bind/salesman/list
//
//	@Summary		老师绑定业务员列表
//	@Description	分页查询指定老师已绑定的业务员（id/用户名/昵称/部门/绑定时间）；data 回显 pageIndex/pageSize
//	@Tags			绑定业务员
//	@Produce		json
//	@Param			id query integer true "老师 ID"
//	@Param			page_index query integer false "页码（默认 1）"
//	@Param			page_size query integer false "页大小（默认 5，上限 100）"
//	@Success		200 {object} model.TeacherSalesListResp
//	@Failure		400 {object} response.Response "id 缺失或非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/bind/salesman/list [get]
func (h *TeacherHandler) SalesList(c *gin.Context) {
	teacherID, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || teacherID <= 0 {
		response.Fail(c, 400, 400, "参数 id 不能为空")
		return
	}
	pageIndex, pageSize := queryPage(c)

	result, err := h.svc.ListTeacherSales(c.Request.Context(), teacherID, pageIndex, pageSize)
	switch {
	case err != nil:
		slog.Error("list teacher sales failed", "teacher_id", teacherID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", result)
	}
}

// Bind POST /api/v1/dxsf/teacher/bind/salesman
//
// 错误映射：body 绑定失败 → 400；老师不存在 → 404；其他 → 500
//
//	@Summary		绑定业务员
//	@Description	追加语义：仅新增绑定，已存在的绑定保持不变；重复绑定幂等
//	@Tags			绑定业务员
//	@Accept			json
//	@Produce		json
//	@Param			body body model.TeacherBindReq true "绑定业务员请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「绑定成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		404 {object} response.Response "老师不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/bind/salesman [post]
func (h *TeacherHandler) Bind(c *gin.Context) {
	var req model.TeacherBindReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind teacher bind request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.BindTeacherSales(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "绑定成功", nil)
	case errors.Is(err, service.ErrTeacherNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("bind teacher sales failed", "teacher_id", req.TeacherID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// queryPage 取分页参数，未传返回 0（由 service 层补默认值）
func queryPage(c *gin.Context) (pageIndex, pageSize int) {
	pageIndex, _ = strconv.Atoi(c.Query("page_index"))
	pageSize, _ = strconv.Atoi(c.Query("page_size"))
	return pageIndex, pageSize
}

// derefOrZero 指针取值，nil 返回 0（0 在 dept_id/id 语义里本就不过滤）
func derefOrZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
