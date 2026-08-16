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

// List GET /api/v1/dxsf/chatSys/teacher/list
//
// 错误映射：数字参数格式错误 → 400；其他 → 500（真实原因只进日志）
func (h *TeacherHandler) List(c *gin.Context) {
	var f model.TeacherListFilter

	for _, num := range []struct {
		key   string
		dst   *int64
		label string
	}{
		{"deptId", &f.DeptID, "deptId"},
		{"id", &f.ID, "id"},
	} {
		s := c.Query(num.key)
		if s == "" {
			continue
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			response.Fail(c, 400, 400, num.label+" must be an integer")
			return
		}
		*num.dst = v
	}
	if s := c.Query("bindSalesCount"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			response.Fail(c, 400, 400, "bindSalesCount must be an integer")
			return
		}
		f.BindSalesCount = &v // 指针：区分"未传"与 0
	}
	f.Account = c.Query("account")
	f.Nickname = c.Query("nickname")
	f.Name = c.Query("name")
	f.Title = c.Query("title")
	f.Qualification = c.Query("qualification")
	f.Status = c.Query("status")
	f.UpdateBy = c.Query("updateBy")
	f.UpdateBeginTime = c.Query("updateBeginTime")
	f.UpdateEndTime = c.Query("updateEndTime")
	f.PageIndex, f.PageSize = queryPage(c) // 默认在 service 层兜底

	result, err := h.svc.ListTeachers(c.Request.Context(), f)
	switch {
	case err != nil:
		slog.Error("list teachers failed", "err", err)
		response.Fail(c, 500, 500, "internal server error")
	default:
		response.OKMsg(c, "success", result)
	}
}

// Options GET /api/v1/dxsf/chatSys/teacher/options
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

// Update PUT /api/v1/dxsf/chatSys/teacher/update
//
// 错误映射：body 绑定失败/非法 rating/签名超长 → 400；老师不存在 → 404；其他 → 500
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

// SalesList GET /api/v1/dxsf/chatSys/teacher/bindSales/list
func (h *TeacherHandler) SalesList(c *gin.Context) {
	teacherID, err := strconv.ParseInt(c.Query("teacherId"), 10, 64)
	if err != nil || teacherID <= 0 {
		response.Fail(c, 400, 400, "teacherId is required")
		return
	}
	pageIndex, pageSize := queryPage(c)

	result, err := h.svc.ListTeacherSales(c.Request.Context(), teacherID, pageIndex, pageSize)
	switch {
	case err != nil:
		slog.Error("list teacher sales failed", "teacherId", teacherID, "err", err)
		response.Fail(c, 500, 500, "internal server error")
	default:
		response.OKMsg(c, "success", result)
	}
}

// Bind POST /api/v1/dxsf/chatSys/teacher/bindSales
//
// 错误映射：body 绑定失败 → 400；老师不存在 → 404；userIds 含不存在的 → 400；其他 → 500
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
	case errors.Is(err, service.ErrSalesNotFound):
		response.Fail(c, 400, 400, "some userIds do not exist")
	default:
		slog.Error("bind teacher sales failed", "teacherId", req.TeacherID, "err", err)
		response.Fail(c, 500, 500, "internal server error")
	}
}

// queryPage 取分页参数，未传返回 0（由 service 层补默认值）
func queryPage(c *gin.Context) (pageIndex, pageSize int) {
	pageIndex, _ = strconv.Atoi(c.Query("pageIndex"))
	pageSize, _ = strconv.Atoi(c.Query("pageSize"))
	return pageIndex, pageSize
}
