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

// ResignHandler 老师离职转移 HTTP 层
type ResignHandler struct {
	svc *service.Service
}

func NewResign(svc *service.Service) *ResignHandler {
	return &ResignHandler{svc: svc}
}

// List GET /api/v1/dxsf/chatSys/resign/list
//
// 错误映射：数字参数格式错误 → 400；其他 → 500（真实原因只进日志）
func (h *ResignHandler) List(c *gin.Context) {
	var f model.ResignListFilter

	for _, num := range []struct {
		key   string
		dst   *int64
		label string
	}{
		{"deptId", &f.DeptID, "deptId"},
		{"originalTeacherId", &f.OriginalTeacherID, "originalTeacherId"},
		{"replaceTeacherId", &f.ReplaceTeacherID, "replaceTeacherId"},
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
	f.SalesmanName = c.Query("salesmanName")
	f.TransferBeginTime = c.Query("transferBeginTime")
	f.TransferEndTime = c.Query("transferEndTime")
	f.PageIndex, f.PageSize = queryPage(c) // 默认在 service 层兜底

	result, err := h.svc.ListResigns(c.Request.Context(), f)
	switch {
	case err != nil:
		slog.Error("list resigns failed", "err", err)
		response.Fail(c, 500, 500, "internal server error")
	default:
		response.OKMsg(c, "success", result)
	}
}

// Add POST /api/v1/dxsf/chatSys/resign/add
//
// 错误映射：body 绑定失败/同人/内容白名单/remark 超长 → 400；
// 老师不存在 → 404；其他 → 500
func (h *ResignHandler) Add(c *gin.Context) {
	var req model.ResignAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "invalid request body: "+err.Error())
		return
	}

	switch err := h.svc.AddResign(c.Request.Context(), req, c.ClientIP()); {
	case err == nil:
		response.OKMsg(c, "转移成功", nil) // mock 约定 msg
	case errors.Is(err, service.ErrTeacherNotFound):
		response.Fail(c, 404, 404, "teacher not found")
	case errors.Is(err, service.ErrSameTeacher):
		response.Fail(c, 400, 400, "original teacher and replace teacher must differ")
	case errors.Is(err, service.ErrInvalidTransferContent):
		response.Fail(c, 400, 400, "transferContent must be a non-empty subset of [group]")
	case errors.Is(err, service.ErrRemarkTooLong):
		response.Fail(c, 400, 400, "remark must be at most 200 characters")
	default:
		slog.Error("add resign failed", "originalTeacherId", req.OriginalTeacherID, "replaceTeacherId", req.ReplaceTeacherID, "err", err)
		response.Fail(c, 500, 500, "internal server error")
	}
}
