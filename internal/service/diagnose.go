package service

import (
	"context"
	"errors"
	"strings"

	"handicap-service/internal/model"
	"handicap-service/internal/sanitize"
)

// diagnose 业务错误定义，handler 用 errors.Is 判断并映射 HTTP 状态码
var (
	ErrDiagnoseNotFound        = errors.New("diagnose record not found")
	ErrInvalidStatusFilter     = errors.New("status must be 1-6")
	ErrReportContentRequired   = errors.New("reportContent must not be empty")
	ErrInvalidStatusTransition = errors.New("current status does not allow this operation")
	ErrInvalidAuditType        = errors.New("auditType must be professional or compliance")
	ErrInvalidAuditResult      = errors.New("result must be pass or reject")
	ErrRejectReasonRequired    = errors.New("rejectReason is required when result is reject")
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

// auditType/result 枚举（前端 diagnose.js 约定）
var (
	validAuditTypes  = []string{"professional", "compliance"}
	validAuditResults = []string{"pass", "reject"}
)

// 审核操作人固定串（无登录态，对齐 resign operator="admin" 先例）
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

// AuditDiagnose 审核诊股报告：professional 管 2→4/3，compliance 管 4→6/5。
// 白名单拒绝而非静默纠正（对齐 rating/transferContent 风格）。
func (s *Service) AuditDiagnose(ctx context.Context, req model.DiagnoseAuditReq) error {
	// 1. 参数白名单与必填校验
	if !contains(validAuditTypes, req.AuditType) {
		return ErrInvalidAuditType
	}
	if !contains(validAuditResults, req.Result) {
		return ErrInvalidAuditResult
	}
	reject := req.Result == "reject"
	if reject {
		// 驳回原因是富文本，落库前白名单净化（先于判空，同 submitReport）
		req.RejectReason = sanitize.RichText(req.RejectReason)
		if strings.TrimSpace(stripHTML(req.RejectReason)) == "" {
			return ErrRejectReasonRequired
		}
	}

	// 2. 写前查询：判 404；顺带校验当前状态与审核类型匹配（终判仍由条件 UPDATE 保证）
	it, err := s.repo.GetDiagnose(ctx, req.ID)
	if err != nil {
		return err
	}
	if it == nil {
		return ErrDiagnoseNotFound
	}

	var fromStatus, toStatus int
	var logType, operator, remark string
	if req.AuditType == "professional" {
		fromStatus, toStatus, logType, operator = DiagnoseStatusAwaitProAudit, 0, logTypeProAudit, proAuditOperator
		if reject {
			toStatus, remark = DiagnoseStatusProRejected, req.RejectReason
		} else {
			toStatus, remark = DiagnoseStatusAwaitCompAudit, proPassRemark
		}
	} else {
		fromStatus, toStatus, logType, operator = DiagnoseStatusAwaitCompAudit, 0, logTypeCompAudit, compAuditOperator
		if reject {
			toStatus, remark = DiagnoseStatusCompRejected, req.RejectReason
		} else {
			toStatus, remark = DiagnoseStatusCompPassed, compPassRemark
		}
	}
	result := logResultPassed
	if reject {
		result = logResultRejected
	}

	ok, err := s.repo.AuditDiagnose(ctx, req.ID, fromStatus, toStatus, model.DiagnoseAuditLogInsert{
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

// contains 切片包含判断（slices.Contains 的薄封装，语义更贴业务）
func contains(list []string, v string) bool {
	for _, it := range list {
		if it == v {
			return true
		}
	}
	return false
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
