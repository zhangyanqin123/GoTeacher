package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"handicap-service/internal/model"
)

// resign 业务错误定义，handler 用 errors.Is 判断并映射 HTTP 状态码
// （ErrTeacherNotFound 复用 teacher.go 的，同包共享，勿重复声明；
// 文本即 API 契约：中文可展示文案，handler 透传 err.Error() 给前端）
var (
	ErrSameTeacher               = errors.New("原老师和接替老师不能相同")
	ErrInvalidTransferContent    = errors.New("转移内容必须为非空 group 子集")
	ErrRemarkTooLong             = errors.New("备注不能超过 200 字符")
	ErrOriginalTeacherNoSalesman = errors.New("原老师无绑定业务员")
)

// 转移内容枚举（前端 resign.js / teacherQuery.vue 转移弹窗勾选一致；好友概念已移除）
var validTransferContents = []string{"group"}

const maxResignRemarkUTF8 = 200 // 对齐 remark 列宽与前端 maxlength

// ListResigns 离职转移列表（分页 + 多条件筛选，默认 pageSize=10 对齐 mock）
func (s *Service) ListResigns(ctx context.Context, f model.ResignListFilter) (*model.PageResult, error) {
	f.PageIndex, f.PageSize = normalizePage(f.PageIndex, f.PageSize, defaultListPageSize)
	list, count, err := s.repo.ListResigns(ctx, f)
	if err != nil {
		return nil, err
	}
	return &model.PageResult{List: list, Count: count}, nil
}

// AddResign 新增离职转移：校验 → 从 teacher 表回查冗余快照 → 落库。
// 前端传的姓名/部门冗余字段一律忽略，以库为准；operateIP 由 handler 传入（c.ClientIP()）；
// operator 无登录态固定 "admin"（对齐 UpdateTeacher 的 update_by）。
func (s *Service) AddResign(ctx context.Context, req model.ResignAddReq, operateIP string) error {
	// 1. 基础校验
	if req.OriginalTeacherID <= 0 || req.ReplaceTeacherID <= 0 {
		return ErrTeacherNotFound
	}
	if req.OriginalTeacherID == req.ReplaceTeacherID {
		return ErrSameTeacher // 前端已有同校验，后端兜底
	}
	content := normalizeTransferContent(req.TransferContent)
	if len(content) == 0 {
		return ErrInvalidTransferContent
	}
	if utf8.RuneCountInString(req.Remark) > maxResignRemarkUTF8 {
		return ErrRemarkTooLong
	}

	// 2. 回查老师（一次 IN 查询拿两个），id/姓名/部门以库为准
	teachers, err := s.repo.GetTeachersByIDs(ctx, []int64{req.OriginalTeacherID, req.ReplaceTeacherID})
	if err != nil {
		return err
	}
	byID := make(map[int64]model.TeacherBrief, len(teachers))
	for _, t := range teachers {
		byID[t.ID] = t
	}
	original, okOriginal := byID[req.OriginalTeacherID]
	replace, okReplace := byID[req.ReplaceTeacherID]
	if !okOriginal || !okReplace {
		return ErrTeacherNotFound
	}

	// 3. 业务员快照：原老师全部绑定业务员，多个逗号拼接（与姓名一一对应）；
	//    空绑定是无效转移（无可转内容，快照/计数全空），直接 400 拦截不入库
	salesmen, err := s.repo.ListTeacherSalesmen(ctx, req.OriginalTeacherID)
	if err != nil {
		return err
	}
	if len(salesmen) == 0 {
		return ErrOriginalTeacherNoSalesman
	}
	names := make([]string, 0, len(salesmen))
	depts := make([]string, 0, len(salesmen))
	for _, sm := range salesmen {
		names = append(names, sm.Nickname)
		depts = append(depts, sm.DeptName)
	}

	// 4. 同事务落快照并真实转移绑定（原老师绑定移给接替老师，计数子查询随之生效）：
	//    重叠绑定由 repository 去重合并（保留接替老师现有行的 bind_time）；
	//    groupCount/salesman 快照取自上方转移前的查询，快照语义不变（可能大于实际移动数）
	return s.repo.TransferResign(ctx, model.ResignInsert{
		OriginalTeacherID:     original.ID,
		OriginalTeacherName:   original.Name,
		OriginalTeacherDeptID: original.DeptID,
		OriginalTeacherDept:   original.DeptName,
		ReplaceTeacherID:      replace.ID,
		ReplaceTeacherName:    replace.Name,
		ReplaceTeacherDept:    replace.DeptName,
		SalesmanName:          strings.Join(names, ","),
		SalesmanDept:          strings.Join(depts, ","),
		TransferContent:       model.StringSlice(content),
		GroupCount:            len(salesmen),
		Operator:              "admin",
		OperateIP:             operateIP,
		Remark:                req.Remark,
	})
}

// normalizeTransferContent 白名单收敛：非法值直接判空触发 ErrInvalidTransferContent
// （对齐 rating 白名单"拒绝而非静默纠正"的风格），合法值去重保序。
func normalizeTransferContent(content []string) []string {
	var out []string
	for _, v := range content {
		if !slices.Contains(validTransferContents, v) {
			return nil
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
