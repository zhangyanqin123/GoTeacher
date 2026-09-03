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

// DiagnoseHandler 诊股记录 HTTP 层
type DiagnoseHandler struct {
	svc *service.Service
}

func NewDiagnose(svc *service.Service) *DiagnoseHandler {
	return &DiagnoseHandler{svc: svc}
}

// List POST /api/v1/dxsf/teacher/diagnose/list
//
// 错误映射：body 绑定失败（含数值字段非法串）/status 越界 → 400；其他 → 500（真实原因只进日志）
//
//	@Summary		诊股记录列表
//	@Description	分页查询诊股记录（筛选条件走 JSON body），零值/未传筛选字段不参与过滤；昵称/姓名/股票代码/股票名/老师模糊匹配，ID/买入价/持仓数/状态精确匹配
//	@Tags			诊股记录
//	@Accept			json
//	@Produce		json
//	@Param			body body model.DiagnoseListReq true "查询条件（姓名/股票类模糊匹配，ID/买入价/持仓数/状态精确；数值字段必须是 JSON number，传字符串/空串一律 400，未填传 null 或缺省）"
//	@Success		200 {object} model.DiagnoseListResp
//	@Failure		400 {object} response.Response "请求体非法 / status 越界（非 1-6）"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/diagnose/list [post]
func (h *DiagnoseHandler) List(c *gin.Context) {
	var req model.DiagnoseListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind diagnose list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	var f model.DiagnoseListFilter
	f.ID = req.ID
	f.UserNickName = req.UserNickName
	f.UserName = req.UserName
	f.StockCode = req.StockCode
	f.StockName = req.StockName
	f.BuyPrice = req.BuyPrice
	f.BuyNum = req.BuyNum
	f.TeacherName = req.TeacherName
	f.Status = req.Status
	f.SubmitBeginTime = req.SubmitBeginTime
	f.SubmitEndTime = req.SubmitEndTime
	f.ReportBeginTime = req.ReportBeginTime
	f.ReportEndTime = req.ReportEndTime
	f.PageIndex, f.PageSize = req.PageIndex, req.PageSize // 默认在 service 层兜底

	switch result, err := h.svc.ListDiagnoses(c.Request.Context(), f); {
	case err == nil:
		response.OKMsg(c, "success", result)
	case errors.Is(err, service.ErrInvalidStatusFilter):
		response.Fail(c, 400, 400, err.Error())
	default:
		slog.Error("list diagnoses failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// Detail GET /api/v1/dxsf/teacher/diagnose/detail
//
// 错误映射：缺 id/格式错误 → 400；记录不存在 → 404；其他 → 500
//
//	@Summary		诊股记录详情
//	@Description	诊股记录全字段 + 审核流程日志（audit_logs）
//	@Tags			诊股记录
//	@Produce		json
//	@Param			id query integer true "诊股记录 ID"
//	@Success		200 {object} model.DiagnoseDetailResp
//	@Failure		400 {object} response.Response "id 缺失或非整数"
//	@Failure		404 {object} response.Response "诊股记录不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/diagnose/detail [get]
func (h *DiagnoseHandler) Detail(c *gin.Context) {
	s := c.Query("id")
	if s == "" {
		response.Fail(c, 400, 400, "参数 id 不能为空")
		return
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		response.Fail(c, 400, 400, "参数 id 必须是整数")
		return
	}

	switch result, err := h.svc.GetDiagnoseDetail(c.Request.Context(), id); {
	case err == nil:
		response.OKMsg(c, "success", result)
	case errors.Is(err, service.ErrDiagnoseNotFound):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("get diagnose detail failed", "id", id, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// SubmitReport POST /api/v1/dxsf/teacher/diagnose/submit/report
//
// 错误映射：body 绑定失败/空内容/状态不允许 → 400；记录不存在 → 404；其他 → 500
//
//	@Summary		提交诊股报告
//	@Description	状态 1/3/5 可提交（首次编写 / 重新提审），提交后状态统一回落 2；report_content 为富文本 HTML
//	@Tags			诊股记录
//	@Accept			json
//	@Produce		json
//	@Param			body body model.DiagnoseSubmitReportReq true "提交报告请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「提交成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 报告内容为空 / 当前状态不允许提交"
//	@Failure		404 {object} response.Response "诊股记录不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/diagnose/submit/report [post]
func (h *DiagnoseHandler) SubmitReport(c *gin.Context) {
	var req model.DiagnoseSubmitReportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind request body failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
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
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// Audit POST /api/v1/dxsf/teacher/diagnose/audit
//
// 错误映射：body 绑定失败/status 白名单/驳回原因必填/状态不允许 → 400；记录不存在 → 404；其他 → 500
//
//	@Summary		审核诊股报告
//	@Description	status 为前端按状态机换算的目标状态，后端白名单校验后直接落库：2→3 专业驳回 / 2→4 专业通过转待合规 / 4→5 合规驳回 / 4→6 合规通过（终态）；status 为 3/5（驳回）时 reject_reason 必填，富文本 HTML
//	@Tags			诊股记录
//	@Accept			json
//	@Produce		json
//	@Param			body body model.DiagnoseAuditReq true "审核请求体（status 为目标状态 3/4/5/6，前端换算）"
//	@Success		200 {object} model.ActionResp "msg 为「审核通过」（status 4/6）或「已驳回」（status 3/5），data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / status 非 3-6 / 驳回时原因必填 / 当前状态不允许审核"
//	@Failure		404 {object} response.Response "诊股记录不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/diagnose/audit [post]
func (h *DiagnoseHandler) Audit(c *gin.Context) {
	var req model.DiagnoseAuditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind request body failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.AuditDiagnose(c.Request.Context(), req); {
	case err == nil:
		msg := "审核通过"
		if req.Status == service.DiagnoseStatusProRejected || req.Status == service.DiagnoseStatusCompRejected {
			msg = "已驳回" // mock 约定 msg
		}
		response.OKMsg(c, msg, nil)
	case errors.Is(err, service.ErrDiagnoseNotFound):
		response.Fail(c, 404, 404, err.Error())
	case errors.Is(err, service.ErrInvalidAuditStatus),
		errors.Is(err, service.ErrRejectReasonRequired),
		errors.Is(err, service.ErrInvalidStatusTransition):
		response.Fail(c, 400, 400, err.Error())
	default:
		slog.Error("audit diagnose failed", "id", req.ID, "status", req.Status, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}
