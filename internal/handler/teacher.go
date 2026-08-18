package handler

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/model"
	"handicap-service/internal/response"
	"handicap-service/internal/service"
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
//	@Description	分页查询老师（筛选条件走 JSON body），零值筛选字段不参与过滤；姓名/账号/昵称/头衔/操作人模糊匹配，部门/ID/认证/状态精确匹配
//	@Tags			老师管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.TeacherListReq true "查询条件（数值字段传空串表示未填）"
//	@Success		200 {object} model.TeacherListResp
//	@Failure		400 {object} response.Response "请求体非法 / 数字参数格式错误"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/teacher/list [post]
func (h *TeacherHandler) List(c *gin.Context) {
	var req model.TeacherListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "invalid request body: "+err.Error())
		return
	}

	var f model.TeacherListFilter
	for _, num := range []struct {
		raw   string
		dst   *int64
		label string
	}{
		{req.DeptID, &f.DeptID, "dept_id"},
		{req.ID, &f.ID, "id"},
	} {
		if num.raw == "" {
			continue
		}
		v, err := strconv.ParseInt(num.raw, 10, 64)
		if err != nil {
			response.Fail(c, 400, 400, num.label+" must be an integer")
			return
		}
		*num.dst = v
	}
	if req.BindSalesCount != "" {
		v, err := strconv.Atoi(req.BindSalesCount)
		if err != nil {
			response.Fail(c, 400, 400, "bind_sales_count must be an integer")
			return
		}
		f.BindSalesCount = &v // 指针：区分"未传"与 0
	}
	f.Account = req.Account
	f.Nickname = req.Nickname
	f.Name = req.Name
	f.Title = req.Title
	f.Qualification = req.Qualification
	f.Status = req.Status
	f.UpdateBy = req.UpdateBy
	f.UpdateBeginTime = req.UpdateBeginTime
	f.UpdateEndTime = req.UpdateEndTime
	f.PageIndex, f.PageSize = req.PageIndex, req.PageSize // 默认在 service 层兜底

	result, err := h.svc.ListTeachers(c.Request.Context(), f)
	switch {
	case err != nil:
		slog.Error("list teachers failed", "err", err)
		response.Fail(c, 500, 500, "internal server error")
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
//	@Router			/teacher/options [get]
func (h *TeacherHandler) Options(c *gin.Context) {
	list, err := h.svc.ListTeacherOptions(c.Request.Context())
	switch {
	case err != nil:
		slog.Error("list teacher options failed", "err", err)
		response.Fail(c, 500, 500, "internal server error")
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
//	@Router			/teacher/bind/salesman/users [get]
func (h *TeacherHandler) BoundUserIds(c *gin.Context) {
	list, err := h.svc.ListAllTeacherSales(c.Request.Context())
	switch {
	case err != nil:
		slog.Error("list bound user ids failed", "err", err)
		response.Fail(c, 500, 500, "internal server error")
	default:
		response.OKMsg(c, "success", list)
	}
}

// Update POST /api/v1/dxsf/teacher/edit
//
// 错误映射：body 绑定失败/非法 rating/签名超长 → 400；老师不存在 → 404；其他 → 500
//
//	@Summary		编辑老师
//	@Description	编辑头衔/评级/头像/签名（仅这 4 个字段可改，冗余快照字段一律忽略）
//	@Tags			老师管理
//	@Accept			json
//	@Produce		json
//	@Param			body body model.TeacherUpdateReq true "编辑老师请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「编辑成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / rating 非 0/1/2 / 签名超过 200 字符"
//	@Failure		404 {object} response.Response "老师不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/teacher/edit [post]
func (h *TeacherHandler) Update(c *gin.Context) {
	var req model.TeacherUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "invalid request body: "+err.Error())
		return
	}

	switch err := h.svc.UpdateTeacher(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "编辑成功", nil)
	case errors.Is(err, service.ErrInvalidRating):
		response.Fail(c, 400, 400, "rating must be 0/1/2")
	case errors.Is(err, service.ErrSignatureTooLong):
		response.Fail(c, 400, 400, "signature must be at most 200 characters")
	case errors.Is(err, service.ErrTeacherNotFound):
		response.Fail(c, 404, 404, "teacher not found")
	default:
		slog.Error("update teacher failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "internal server error")
	}
}

// SalesList GET /api/v1/dxsf/teacher/bind/salesman/list
//
//	@Summary		老师绑定业务员列表
//	@Description	分页查询指定老师已绑定的业务员（手机号/昵称/部门/绑定时间）
//	@Tags			绑定业务员
//	@Produce		json
//	@Param			teacher_id query integer true "老师 ID"
//	@Param			page_index query integer false "页码（默认 1）"
//	@Param			page_size query integer false "页大小（默认 5，上限 100）"
//	@Success		200 {object} model.TeacherSalesListResp
//	@Failure		400 {object} response.Response "teacher_id 缺失或非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/teacher/bind/salesman/list [get]
func (h *TeacherHandler) SalesList(c *gin.Context) {
	teacherID, err := strconv.ParseInt(c.Query("teacher_id"), 10, 64)
	if err != nil || teacherID <= 0 {
		response.Fail(c, 400, 400, "teacher_id is required")
		return
	}
	pageIndex, pageSize := queryPage(c)

	result, err := h.svc.ListTeacherSales(c.Request.Context(), teacherID, pageIndex, pageSize)
	switch {
	case err != nil:
		slog.Error("list teacher sales failed", "teacher_id", teacherID, "err", err)
		response.Fail(c, 500, 500, "internal server error")
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
//	@Router			/teacher/bind/salesman [post]
func (h *TeacherHandler) Bind(c *gin.Context) {
	var req model.TeacherBindReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "invalid request body: "+err.Error())
		return
	}

	switch err := h.svc.BindTeacherSales(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "绑定成功", nil)
	case errors.Is(err, service.ErrTeacherNotFound):
		response.Fail(c, 404, 404, "teacher not found")
	default:
		slog.Error("bind teacher sales failed", "teacher_id", req.TeacherID, "err", err)
		response.Fail(c, 500, 500, "internal server error")
	}
}

// queryPage 取分页参数，未传返回 0（由 service 层补默认值）
func queryPage(c *gin.Context) (pageIndex, pageSize int) {
	pageIndex, _ = strconv.Atoi(c.Query("page_index"))
	pageSize, _ = strconv.Atoi(c.Query("page_size"))
	return pageIndex, pageSize
}
