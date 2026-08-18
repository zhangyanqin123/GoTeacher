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
//
//	@Summary		诊股记录列表
//	@Description	分页查询诊股记录，零值/未传筛选字段不参与过滤；昵称/姓名/股票代码/股票名/老师模糊匹配，ID/买入价/持仓数/状态精确匹配
//	@Tags			诊股记录
//	@Accept			json
//	@Produce		json
//	@Param			id query integer false "诊股记录 ID（精确）"
//	@Param			userNickName query string false "用户昵称（模糊）"
//	@Param			userName query string false "用户姓名（模糊）"
//	@Param			stockCode query string false "股票代码（模糊）"
//	@Param			stockName query string false "股票名称（模糊）"
//	@Param			buyPrice query number false "买入价（精确，DECIMAL 等值匹配）"
//	@Param			buyNum query integer false "持仓数（精确）"
//	@Param			teacherName query string false "老师姓名（模糊）"
//	@Param			status query integer false "状态（精确 1-6：1 待提交 2 待审核 3 已驳回 4 待合规 5 合规驳回 6 已通过）" Enums(1,2,3,4,5,6)
//	@Param			submitBeginTime query string false "提交时间起（yyyy-MM-dd，与 submitEndTime 成对生效，闭合区间）"
//	@Param			submitEndTime query string false "提交时间止（yyyy-MM-dd）"
//	@Param			reportBeginTime query string false "报告提交时间起（yyyy-MM-dd，与 reportEndTime 成对生效）"
//	@Param			reportEndTime query string false "报告提交时间止（yyyy-MM-dd）"
//	@Param			pageIndex query integer false "页码（默认 1）"
//	@Param			pageSize query integer false "页大小（默认 10，上限 100）"
//	@Success		200 {object} model.DiagnoseListResp
//	@Failure		400 {object} response.Response "数值参数格式错误 / status 越界（非 1-6）"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/diagnose/list [get]
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
//
//	@Summary		诊股记录详情
//	@Description	诊股记录全字段 + 审核流程日志（auditLogs）
//	@Tags			诊股记录
//	@Produce		json
//	@Param			id query integer true "诊股记录 ID"
//	@Success		200 {object} model.DiagnoseDetailResp
//	@Failure		400 {object} response.Response "id 缺失或非整数"
//	@Failure		404 {object} response.Response "诊股记录不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/diagnose/detail [get]
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
//
//	@Summary		提交诊股报告
//	@Description	状态 1/3/5 可提交（首次编写 / 重新提审），提交后状态统一回落 2；reportContent 为富文本 HTML
//	@Tags			诊股记录
//	@Accept			json
//	@Produce		json
//	@Param			body body model.DiagnoseSubmitReportReq true "提交报告请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「提交成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 报告内容为空 / 当前状态不允许提交"
//	@Failure		404 {object} response.Response "诊股记录不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/diagnose/submitReport [post]
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
//
//	@Summary		审核诊股报告
//	@Description	professional 专业审核（状态 2）/ compliance 合规审核（状态 4）；通过 → 3/6，驳回 → 5/4（驳回时 rejectReason 必填，富文本 HTML）
//	@Tags			诊股记录
//	@Accept			json
//	@Produce		json
//	@Param			body body model.DiagnoseAuditReq true "审核请求体"
//	@Success		200 {object} model.ActionResp "msg 为「审核通过」或「已驳回」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / auditType 或 result 非法 / 驳回时原因必填 / 当前状态不允许审核"
//	@Failure		404 {object} response.Response "诊股记录不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/diagnose/audit [post]
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
