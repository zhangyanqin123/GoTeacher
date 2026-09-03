package service

import (
	"context"
	"errors"
	"slices"
	"unicode/utf8"

	"gyz-service/internal/model"
)

// teacher 业务错误定义，handler 用 errors.Is 判断并映射 HTTP 状态码
// （文本即 API 契约：中文可展示文案，handler 透传 err.Error() 给前端）
var (
	ErrTeacherNotFound  = errors.New("老师不存在")
	ErrInvalidLevel     = errors.New("评级必须是 0/3/5")
	ErrSignatureTooLong = errors.New("签名不能超过 200 字符")
)

// 编辑弹窗评级枚举（前端 teacherQuery.vue editForm 下拉一致：0 无 / 3 初级 / 5 高级）
var validLevels = []int{0, 3, 5}

const (
	maxPageSize          = 100
	maxSignatureUTF8     = 200
	defaultListPageSize  = 10
	defaultSalesPageSize = 5
)

// ListTeachers 老师列表（分页 + 多条件筛选，默认 pageSize=10 对齐 mock）
func (s *Service) ListTeachers(ctx context.Context, f model.TeacherListFilter) (*model.PageResult, error) {
	f.PageIndex, f.PageSize = normalizePage(f.PageIndex, f.PageSize, defaultListPageSize)
	list, count, err := s.repo.ListTeachers(ctx, f)
	if err != nil {
		return nil, err
	}
	return &model.PageResult{List: list, Count: count}, nil
}

// ListTeacherOptions 老师下拉选项（含停用）
func (s *Service) ListTeacherOptions(ctx context.Context) ([]model.TeacherOption, error) {
	return s.repo.ListTeacherOptions(ctx)
}

// GetTeacherDetail 老师详情（编辑弹窗回显），不存在返回 nil（handler 映射 404）
func (s *Service) GetTeacherDetail(ctx context.Context, id int64) (*model.TeacherDetail, error) {
	return s.repo.GetTeacherDetailByID(ctx, id)
}

// ListAllTeacherSales 全量已绑定业务员 userId（绑定弹窗人员树过滤用）
func (s *Service) ListAllTeacherSales(ctx context.Context) ([]int64, error) {
	return s.repo.ListAllTeacherSales(ctx)
}

// UpdateTeacher 编辑老师：校验评级白名单与签名长度，老师必须存在
func (s *Service) UpdateTeacher(ctx context.Context, req model.TeacherUpdateReq) error {
	if req.ID <= 0 {
		return ErrTeacherNotFound
	}
	if !slices.Contains(validLevels, req.Level) {
		return ErrInvalidLevel
	}
	if utf8.RuneCountInString(req.Sign) > maxSignatureUTF8 {
		return ErrSignatureTooLong
	}

	ok, err := s.repo.ExistsTeacher(ctx, req.ID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeacherNotFound
	}
	// 无登录态，更新人固定 admin（与 mock 一致）
	// TODO: 接入登录态后改取 handler 传入的 c.GetString(model.CtxKeyUsername)
	return s.repo.UpdateTeacher(ctx, req, "admin")
}

// ListTeacherSales 老师绑定业务员分页列表。
// 老师不存在时返回空列表（对齐 mock：find 不到 → count 0 → list []），不视为错误。
// 默认 pageSize=5（对齐 mock 的 query.pageSize || 5，与列表接口的 10 不同）。
// 返回 SalesPageResult：data 回显 pageIndex/pageSize（该接口前端约定的返回结构）。
func (s *Service) ListTeacherSales(ctx context.Context, teacherID int64, pageIndex, pageSize int) (*model.SalesPageResult, error) {
	pageIndex, pageSize = normalizePage(pageIndex, pageSize, defaultSalesPageSize)
	list, count, err := s.repo.ListTeacherSalesByTeacher(ctx, teacherID, pageSize, (pageIndex-1)*pageSize)
	if err != nil {
		return nil, err
	}
	return &model.SalesPageResult{List: list, Count: count, PageIndex: pageIndex, PageSize: pageSize}, nil
}

// BindTeacherSales 追加绑定业务员：仅新增绑定，已存在的不动（INSERT IGNORE 幂等）
func (s *Service) BindTeacherSales(ctx context.Context, req model.TeacherBindReq) error {
	if req.TeacherID <= 0 {
		return ErrTeacherNotFound
	}

	ok, err := s.repo.ExistsTeacher(ctx, req.TeacherID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeacherNotFound
	}

	userIDs := slices.Compact(slices.Clone(req.UserIDs)) // 去重（前端已按 userId 去重，双保险）
	return s.repo.AddTeacherSales(ctx, req.TeacherID, userIDs)
}

// normalizePage 分页默认值与上限：pageSize 未传（<1）用 defaultPageSize，显式传入则钳制到 maxPageSize
func normalizePage(pageIndex, pageSize, defaultPageSize int) (int, int) {
	if pageIndex < 1 {
		pageIndex = 1
	}
	switch {
	case pageSize < 1:
		pageSize = defaultPageSize
	case pageSize > maxPageSize:
		pageSize = maxPageSize
	}
	return pageIndex, pageSize
}
