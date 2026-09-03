package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"gyz-service/internal/model"
)

// AB 版模块配置业务错误定义，handler 用 errors.Is 判断并映射 HTTP 状态码。
// 文本即 API 契约：中文可展示文案，handler 透传 err.Error() 给前端。
var (
	ErrAbModuleKeyExists  = errors.New("模块标识已存在")
	ErrAbModuleNotFound   = errors.New("模块不存在")
	ErrAbModuleHasItems   = errors.New("模块下存在配置项，请先删除全部配置项")
	ErrAbModuleKeyInvalid = errors.New("模块标识仅支持小写字母开头，可含小写字母、数字与下划线")
	ErrAbItemKeyExists    = errors.New("该模块下配置项标识已存在")
	ErrAbItemNotFound     = errors.New("配置项不存在")
	ErrAbItemKeyInvalid   = errors.New("配置项标识仅支持英文字母开头，可含英文字母与数字（camelCase，如 topBanner）")
	ErrAbVersionsInvalid  = errors.New("可见版本仅支持 mass 或 data")
)

// abVersionOrder AB 版值域，兼做库内存储定序（mass,data）——归一化后库内形态恒定
var abVersionOrder = []string{"mass", "data"}

// 业务标识格式：module_key 对齐 H5 页面域命名（小写），item_key 对齐 H5 TS 常量名（camelCase）
var (
	abModuleKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)
	abItemKeyRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,49}$`)
)

// ============ 模块（父级） ============

// ListAbModules 模块列表（分页 + 标识/名称模糊，默认 pageSize=10 对齐其他列表）
func (s *Service) ListAbModules(ctx context.Context, req model.AbModuleListReq) (*model.PageResult, error) {
	pageIndex, pageSize := normalizePage(req.PageIndex, req.PageSize, defaultListPageSize)
	list, count, err := s.repo.ListAbModules(ctx, model.AbModuleListFilter{
		ModuleKey:  strings.TrimSpace(req.ModuleKey),
		ModuleName: strings.TrimSpace(req.ModuleName),
		Offset:     (pageIndex - 1) * pageSize,
		Limit:      pageSize,
	})
	if err != nil {
		return nil, err
	}
	return &model.PageResult{List: list, Count: count}, nil
}

// AbModuleOptions 全量模块下拉（配置项管理页选父模块）
func (s *Service) AbModuleOptions(ctx context.Context) ([]model.AbModuleOption, error) {
	return s.repo.AbModuleOptions(ctx)
}

// CreateAbModule 新增模块：格式校验 → 重名校验 → 落库。
// module_key 创建后不可改（H5 代码按此引用），要换 key 走删旧建新
func (s *Service) CreateAbModule(ctx context.Context, req model.AbModuleAddReq) error {
	moduleKey := strings.TrimSpace(req.ModuleKey)
	if !abModuleKeyRe.MatchString(moduleKey) {
		return ErrAbModuleKeyInvalid
	}
	exists, err := s.repo.ExistsAbModuleKey(ctx, moduleKey, 0)
	if err != nil {
		return err
	}
	if exists {
		return ErrAbModuleKeyExists
	}
	return s.repo.CreateAbModule(ctx, &model.AbModule{
		ModuleKey:  moduleKey,
		ModuleName: strings.TrimSpace(req.ModuleName),
		SortNo:     req.SortNo,
	})
}

// UpdateAbModule 编辑模块：存在性 → 落库（不含 module_key）
func (s *Service) UpdateAbModule(ctx context.Context, req model.AbModuleEditReq) error {
	m, err := s.repo.GetAbModuleByID(ctx, req.ID)
	if err != nil {
		return err
	}
	if m == nil {
		return ErrAbModuleNotFound
	}
	return s.repo.UpdateAbModule(ctx, req.ID, strings.TrimSpace(req.ModuleName), req.SortNo)
}

// DeleteAbModule 删除模块：存在性 → 子项拦截（不级联删，级联会让 H5 整模块 UI 瞬间消失）→ 删行
func (s *Service) DeleteAbModule(ctx context.Context, id int64) error {
	m, err := s.repo.GetAbModuleByID(ctx, id)
	if err != nil {
		return err
	}
	if m == nil {
		return ErrAbModuleNotFound
	}
	count, err := s.repo.CountAbModuleItems(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrAbModuleHasItems
	}
	return s.repo.DeleteAbModule(ctx, id)
}

// ============ 配置项（子级） ============

// ListAbModuleItems 配置项列表（分页 + 所属模块精确 + 标识模糊）
func (s *Service) ListAbModuleItems(ctx context.Context, req model.AbModuleItemListReq) (*model.PageResult, error) {
	pageIndex, pageSize := normalizePage(req.PageIndex, req.PageSize, defaultListPageSize)
	list, count, err := s.repo.ListAbModuleItems(ctx, model.AbModuleItemListFilter{
		ModuleID: req.ModuleID,
		ItemKey:  strings.TrimSpace(req.ItemKey),
		Offset:   (pageIndex - 1) * pageSize,
		Limit:    pageSize,
	})
	if err != nil {
		return nil, err
	}
	return &model.PageResult{List: list, Count: count}, nil
}

// CreateAbModuleItem 新增配置项：模块存在性 → item_key 格式 → versions 归一化 → 同模块重名 → 落库
func (s *Service) CreateAbModuleItem(ctx context.Context, req model.AbModuleItemAddReq) error {
	m, err := s.repo.GetAbModuleByID(ctx, req.ModuleID)
	if err != nil {
		return err
	}
	if m == nil {
		return ErrAbModuleNotFound
	}

	itemKey := strings.TrimSpace(req.ItemKey)
	if !abItemKeyRe.MatchString(itemKey) {
		return ErrAbItemKeyInvalid
	}
	versions, err := normalizeVersions(req.Versions)
	if err != nil {
		return err
	}
	exists, err := s.repo.ExistsAbItemKey(ctx, req.ModuleID, itemKey, 0)
	if err != nil {
		return err
	}
	if exists {
		return ErrAbItemKeyExists
	}
	return s.repo.CreateAbModuleItem(ctx, &model.AbModuleItem{
		ModuleID:  req.ModuleID,
		ItemKey:   itemKey,
		ItemName:  strings.TrimSpace(req.ItemName),
		SortNo:    req.SortNo,
	}, versions)
}

// UpdateAbModuleItem 编辑配置项：存在性 → 目标模块存在性（module_id 可改=挪模块）→
// versions 归一化 → 目标模块内重名（排除自身）→ 落库（不含 item_key）
func (s *Service) UpdateAbModuleItem(ctx context.Context, req model.AbModuleItemEditReq) error {
	it, err := s.repo.GetAbModuleItemByID(ctx, req.ID)
	if err != nil {
		return err
	}
	if it == nil {
		return ErrAbItemNotFound
	}

	m, err := s.repo.GetAbModuleByID(ctx, req.ModuleID)
	if err != nil {
		return err
	}
	if m == nil {
		return ErrAbModuleNotFound
	}

	versions, err := normalizeVersions(req.Versions)
	if err != nil {
		return err
	}
	exists, err := s.repo.ExistsAbItemKey(ctx, req.ModuleID, it.ItemKey, req.ID)
	if err != nil {
		return err
	}
	if exists {
		return ErrAbItemKeyExists
	}
	return s.repo.UpdateAbModuleItem(ctx, req.ID, req.ModuleID, strings.TrimSpace(req.ItemName), versions, req.SortNo)
}

// DeleteAbModuleItem 删除配置项：存在性 → 删行。
// 删除即该配置项在 H5 对应模块 UI 隐藏（显性产品行为）
func (s *Service) DeleteAbModuleItem(ctx context.Context, id int64) error {
	it, err := s.repo.GetAbModuleItemByID(ctx, id)
	if err != nil {
		return err
	}
	if it == nil {
		return ErrAbItemNotFound
	}
	return s.repo.DeleteAbModuleItem(ctx, id)
}

// ============ H5 聚合 ============

// AbConfig H5 聚合配置（GET /api/v1/ab/config，免鉴权）：
// 两级 map 模块标识 → 配置项标识 → 可见版本数组。
// moduleKey 非空时只返回该模块；模块不存在返回空 map——快照过滤语义（与列表筛选空结果
// 一致），H5 无需处理错误分支。空模块输出空对象（不省略 key），H5 端 cfg[moduleKey]?.[itemKey] 无需判空特殊处理。
func (s *Service) AbConfig(ctx context.Context, moduleKey string) (map[string]map[string][]string, error) {
	modules, err := s.repo.ListAllAbModules(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListAllAbModuleItems(ctx)
	if err != nil {
		return nil, err
	}

	// idToKey 只收录入选模块，items 循环天然跳过非目标模块的配置项
	// （配置数据量个位数量级，全量查询 + 内存过滤，无需按模块单查）
	cfg := make(map[string]map[string][]string, len(modules))
	idToKey := make(map[int64]string, len(modules))
	for _, m := range modules {
		if moduleKey != "" && m.ModuleKey != moduleKey {
			continue
		}
		cfg[m.ModuleKey] = make(map[string][]string)
		idToKey[m.ID] = m.ModuleKey
	}
	for _, it := range items {
		if key, ok := idToKey[it.ModuleID]; ok {
			cfg[key][it.ItemKey] = it.Versions
		}
	}
	return cfg, nil
}

// normalizeVersions 校验并归一化可见版本：值域校验（mass/data）→ 去重 →
// 按 abVersionOrder 固定顺序输出逗号串（库内形态恒定：'mass,data' 或 'data'）
func normalizeVersions(in []string) (string, error) {
	valid := make(map[string]bool, len(abVersionOrder))
	for _, v := range abVersionOrder {
		valid[v] = true
	}
	picked := make(map[string]struct{}, len(in))
	for _, v := range in {
		if !valid[v] {
			return "", ErrAbVersionsInvalid
		}
		picked[v] = struct{}{}
	}
	if len(picked) == 0 {
		return "", ErrAbVersionsInvalid
	}
	out := make([]string, 0, len(picked))
	for _, v := range abVersionOrder {
		if _, ok := picked[v]; ok {
			out = append(out, v)
		}
	}
	return strings.Join(out, ","), nil
}
