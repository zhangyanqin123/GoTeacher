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

// DiagnoseHandler 诊股记录 HTTP 层
type DiagnoseHandler struct {
	svc *service.Service
}

func NewDiagnose(svc *service.Service) *DiagnoseHandler {
	return &DiagnoseHandler{svc: svc}
}

// List GET /api/v1/dxsf/diagnose/list
//
// 错误映射：数值参数格式错误 → 400；status 越界 → 400；其他 → 500（真实原因只进日志）
func (h *DiagnoseHandler) List(c *gin.Context) {
	var f model.DiagnoseListFilter

	// 整数类筛选：非空才解析，失败即 400
	for _, num := range []struct {
		key   string
		dst   **int64
		label string
	}{
		{"id", &f.ID, "id"},
		{"buyNum", &f.BuyNum, "buyNum"},
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
		*num.dst = &v
	}

	// buyPrice → DECIMAL 精确匹配
	if s := c.Query("buyPrice"); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			response.Fail(c, 400, 400, "buyPrice must be a number")
			return
		}
		f.BuyPrice = &v
	}

	// status → 1-6 枚举（越界在 service 白名单兜底，这里只管格式）
	if s := c.Query("status"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			response.Fail(c, 400, 400, "status must be an integer")
			return
		}
		f.Status = &v
	}

	f.UserNickName = c.Query("userNickName")
	f.UserName = c.Query("userName")
	f.StockCode = c.Query("stockCode")
	f.StockName = c.Query("stockName")
	f.TeacherName = c.Query("teacherName")
	f.SubmitBeginTime = c.Query("submitBeginTime")
	f.SubmitEndTime = c.Query("submitEndTime")
	f.ReportBeginTime = c.Query("reportBeginTime")
	f.ReportEndTime = c.Query("reportEndTime")
	f.PageIndex, f.PageSize = queryPage(c) // 默认在 service 层兜底

	switch result, err := h.svc.ListDiagnoses(c.Request.Context(), f); {
	case err == nil:
		response.OKMsg(c, "success", result)
	case errors.Is(err, service.ErrInvalidStatusFilter):
		response.Fail(c, 400, 400, err.Error())
	default:
		slog.Error("list diagnoses failed", "err", err)
		response.Fail(c, 500, 500, "internal server error")
	}
}

// Detail GET /api/v1/dxsf/diagnose/detail
//
// 错误映射：缺 id/格式错误 → 400；记录不存在 → 404；其他 → 500
func (h *DiagnoseHandler) Detail(c *gin.Context) {
	s := c.Query("id")
	if s == "" {
		response.Fail(c, 400, 400, "id is required")
		return
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		response.Fail(c, 400, 400, "id must be an integer")
		return
	}

	switch result, err := h.svc.GetDiagnoseDetail(c.Request.Context(), id); {
	case err == nil:
		response.OKMsg(c, "success", result)
	case errors.Is(err, service.ErrDiagnoseNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("get diagnose detail failed", "id", id, "err", err)
		response.Fail(c, 500, 500, "internal server error")
	}
}

// SubmitReport POST /api/v1/dxsf/diagnose/submitReport
//
// 错误映射：body 绑定失败/空内容/状态不允许 → 400；记录不存在 → 404；其他 → 500
func (h *DiagnoseHandler) SubmitReport(c *gin.Context) {
	var req model.DiagnoseSubmitReportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "invalid request body: "+err.Error())
		return
	}

	switch err := h.svc.SubmitDiagnoseReport(c.Request.Context(), req); {
	case err == nil:
		response.OKMsg(c, "提交成功", nil) // mock 约定 msg
	case errors.Is(err, service.ErrDiagnoseNotFound):
		response.Fail(c, 404, 404, err.Error())
	case errors.Is(err, service.ErrReportContentRequired),
		errors.Is(err, service.ErrInvalidStatusTransition):
		response.Fail(c, 400, 400, err.Error())
	default:
		slog.Error("submit diagnose report failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "internal server error")
	}
}

// Audit POST /api/v1/dxsf/diagnose/audit
//
// 错误映射：body 绑定失败/白名单/驳回原因必填/状态不允许 → 400；记录不存在 → 404；其他 → 500
func (h *DiagnoseHandler) Audit(c *gin.Context) {
	var req model.DiagnoseAuditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "invalid request body: "+err.Error())
		return
	}

	switch err := h.svc.AuditDiagnose(c.Request.Context(), req); {
	case err == nil:
		msg := "审核通过"
		if req.Result == "reject" {
			msg = "已驳回" // mock 约定 msg
		}
		response.OKMsg(c, msg, nil)
	case errors.Is(err, service.ErrDiagnoseNotFound):
		response.Fail(c, 404, 404, err.Error())
	case errors.Is(err, service.ErrInvalidAuditType),
		errors.Is(err, service.ErrInvalidAuditResult),
		errors.Is(err, service.ErrRejectReasonRequired),
		errors.Is(err, service.ErrInvalidStatusTransition):
		response.Fail(c, 400, 400, err.Error())
	default:
		slog.Error("audit diagnose failed", "id", req.ID, "auditType", req.AuditType, "err", err)
		response.Fail(c, 500, 500, "internal server error")
	}
}
