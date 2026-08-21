package service

import (
	"context"
	"errors"
	"strings"

	"handicap-service/internal/model"
	"handicap-service/internal/sanitize"
)

// diagnose 业务错误定义，handler 用 errors.Is 判断并映射 HTTP 状态码
// （文本即 API 契约：中文可展示文案，handler 透传 err.Error() 给前端）
var (
	ErrDiagnoseNotFound        = errors.New("诊股记录不存在")
	ErrInvalidStatusFilter     = errors.New("状态筛选必须是 1-6")
	ErrReportContentRequired   = errors.New("报告内容不能为空")
	ErrInvalidStatusTransition = errors.New("当前状态不允许此操作")
	ErrInvalidAuditStatus      = errors.New("审核状态必须是 3/4/5/6")
	ErrRejectReasonRequired    = errors.New("驳回时必须填写驳回原因")
)

// 诊股状态枚举（前端 diagnoseQuery.vue statusOptions 一致，勿改）
const (
	DiagnoseStatusAwaitReport  = 1 // 待诊股
	DiagnoseStatusAwaitProAudit = 2 // 待专业审核
	DiagnoseStatusProRejected   = 3 // 专业审核不通过
	DiagnoseStatusAwaitCompAudit = 4 // 待合规审核
	DiagnoseStatusCompRejected  = 5 // 合规审核不通过
	DiagnoseStatusCompPassed    = 6 // 合规审核通过（终态）
)

// 审核操作人固定串（无登录态，对齐 resign operator="admin" 先例）
// TODO: 接入登录态后改取 handler 传入的 c.GetString(model.CtxKeyUsername)
const (
	proAuditOperator  = "专业审核员"
	compAuditOperator = "合规审核员"
)

// 审核通过时的固定日志文案（种子 SQL 硬编码同值，勿单独改动）
const (
	proPassRemark  = "报告专业、结论合理"
	compPassRemark = "内容合规"
)

// auditLogType 日志类型枚举（存中文展示串，对齐 teacher.qualification 先例）
const (
	logTypeUserSubmit  = "用户提交"
	logTypeReport      = "诊股报告"
	logTypeProAudit    = "专业审核"
	logTypeCompAudit   = "合规审核"
)

// logResultSubmitted / logResultPassed / logResultRejected 日志结果枚举（中文展示串）
const (
	logResultSubmitted = "已提交"
	logResultPassed    = "通过"
	logResultRejected  = "不通过"
)

// ListDiagnoses 诊股列表（分页 + 多条件筛选，默认 pageSize=10 对齐 mock）。
// remark 由 C 端写入（本服务无写入口），读路径净化兜底；reportContent 本服务写入
// 已在写路径净化，列表不做二次扫描（每页 10 行大 HTML 逐行词法分析收益趋零）。
func (s *Service) ListDiagnoses(ctx context.Context, f model.DiagnoseListFilter) (*model.PageResult, error) {
	if f.Status != nil && (*f.Status < 1 || *f.Status > 6) {
		return nil, ErrInvalidStatusFilter
	}
	f.PageIndex, f.PageSize = normalizePage(f.PageIndex, f.PageSize, defaultListPageSize)
	list, count, err := s.repo.ListDiagnoses(ctx, f)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Remark = sanitize.RichText(list[i].Remark)
	}
	return &model.PageResult{List: list, Count: count}, nil
}

// GetDiagnoseDetail 诊股详情 = 主表全字段 + 审核流程日志（按 id 正序）。
// remark / 日志 remark 统一读路径净化：'用户提交'类日志与主表 remark 来自 C 端
// （未经本服务写路径净化）；其余日志已净化，bluemonday 幂等，二次净化无畸变。
func (s *Service) GetDiagnoseDetail(ctx context.Context, id int64) (*model.DiagnoseDetail, error) {
	it, err := s.repo.GetDiagnose(ctx, id)
	if err != nil {
		return nil, err
	}
	if it == nil {
		return nil, ErrDiagnoseNotFound
	}
	logs, err := s.repo.ListDiagnoseAuditLogs(ctx, id)
	if err != nil {
		return nil, err
	}
	it.Remark = sanitize.RichText(it.Remark)
	for i := range logs {
		logs[i].Remark = sanitize.RichText(logs[i].Remark)
	}
	return &model.DiagnoseDetail{Diagnose: *it, AuditLogs: logs}, nil
}

// SubmitDiagnoseReport 提交诊股报告（首次编写 / 重新提审共用，统一回落状态 2）。
// 状态 2/4/6 严格拒绝：在途重提使审核对象错位、审计链断裂；6 为终态。
func (s *Service) SubmitDiagnoseReport(ctx context.Context, req model.DiagnoseSubmitReportReq) error {
	// 1. 白名单净化（先于判空：纯恶意标签内容净化后为空，收紧为 400 而非落库空串）
	req.ReportContent = sanitize.RichText(req.ReportContent)
	// 2. 富文本判空：去标签后全空白视为未填写（前端 isRichEmpty 同构，后端兜底）
	if strings.TrimSpace(stripHTML(req.ReportContent)) == "" {
		return ErrReportContentRequired
	}

	// 2. 写前查询：判 404 并取 teacher_name 供日志（并发守卫仍由条件 UPDATE 保证）
	it, err := s.repo.GetDiagnose(ctx, req.ID)
	if err != nil {
		return err
	}
	if it == nil {
		return ErrDiagnoseNotFound
	}
	teacher := it.TeacherName
	if teacher == "" {
		teacher = "诊股老师" // mock buildAuditLogs 同构兜底
	}

	ok, err := s.repo.SubmitDiagnoseReport(ctx, req.ID, req.ReportContent, model.DiagnoseAuditLogInsert{
		DiagnoseID: req.ID,
		LogType:    logTypeReport,
		Operator:   teacher,
		Result:     logResultSubmitted,
		Remark:     "老师编写诊股报告", // mock 固定文案
	})
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidStatusTransition // 状态已流转（2/4/6），条件 UPDATE 未命中
	}
	return nil
}

// AuditDiagnose 审核诊股报告：status 为前端按状态机换算的目标状态，白名单校验后直接落库
// （2026-08-21 由 audit_type+result 后端推导改为前端直传）：2→3 专业驳回 / 2→4 专业通过 /
// 4→5 合规驳回 / 4→6 合规通过（终态）。白名单拒绝而非静默纠正（对齐 rating/transferContent 风格）。
func (s *Service) AuditDiagnose(ctx context.Context, req model.DiagnoseAuditReq) error {
	// 1. 目标状态白名单与驳回必填校验
	reject := req.Status == DiagnoseStatusProRejected || req.Status == DiagnoseStatusCompRejected
	switch req.Status {
	case DiagnoseStatusProRejected, DiagnoseStatusAwaitCompAudit, DiagnoseStatusCompRejected, DiagnoseStatusCompPassed:
	default:
		return ErrInvalidAuditStatus
	}
	if reject {
		// 驳回原因是富文本，落库前白名单净化（先于判空，同 submitReport）
		req.RejectReason = sanitize.RichText(req.RejectReason)
		if strings.TrimSpace(stripHTML(req.RejectReason)) == "" {
			return ErrRejectReasonRequired
		}
	}

	// 2. 写前查询：判 404（迁移合法性终判仍由条件 UPDATE 保证）
	it, err := s.repo.GetDiagnose(ctx, req.ID)
	if err != nil {
		return err
	}
	if it == nil {
		return ErrDiagnoseNotFound
	}

	// 3. 按目标状态推导来源状态与日志素材（审核日志/操作人契约不变，仅换算入口前移）
	var fromStatus int
	var logType, operator, remark string
	result := logResultPassed
	switch req.Status {
	case DiagnoseStatusProRejected: // 2→3 专业驳回
		fromStatus, logType, operator, remark, result = DiagnoseStatusAwaitProAudit, logTypeProAudit, proAuditOperator, req.RejectReason, logResultRejected
	case DiagnoseStatusAwaitCompAudit: // 2→4 专业通过
		fromStatus, logType, operator, remark = DiagnoseStatusAwaitProAudit, logTypeProAudit, proAuditOperator, proPassRemark
	case DiagnoseStatusCompRejected: // 4→5 合规驳回
		fromStatus, logType, operator, remark, result = DiagnoseStatusAwaitCompAudit, logTypeCompAudit, compAuditOperator, req.RejectReason, logResultRejected
	default: // 4→6 合规通过（终态）
		fromStatus, logType, operator, remark = DiagnoseStatusAwaitCompAudit, logTypeCompAudit, compAuditOperator, compPassRemark
	}

	ok, err := s.repo.AuditDiagnose(ctx, req.ID, fromStatus, req.Status, model.DiagnoseAuditLogInsert{
		DiagnoseID: req.ID,
		LogType:    logType,
		Operator:   operator,
		Result:     result,
		Remark:     remark,
	})
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidStatusTransition // 状态非 fromStatus（顺序错位/已审/未提审）
	}
	return nil
}

// stripHTML 去除富文本 HTML 标签与常见空白实体，用于内容判空（前端 isRichEmpty 同构）
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.ReplaceAll(b.String(), "&nbsp;", " ")
}
