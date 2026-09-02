package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"handicap-service/internal/model"
)

// AB 版模块配置数据访问（ab_module / ab_module_item，见 PLAN-ab-module.md）。
// versions 列库内为逗号串（'mass,data'），扫描到局部 string 再 splitVersions 塞入行模型。

// ============ 模块（父级） ============

// ExistsAbModuleKey 模块标识唯一性检查。
// excludeID 编辑时排除自身，新增传 0（id <> 0 对所有行恒真，同 ExistsAdminUserByUsername 先例）
func (r *Repository) ExistsAbModuleKey(ctx context.Context, moduleKey string, excludeID int64) (bool, error) {
	const q = `SELECT 1 FROM ab_module WHERE module_key = ? AND id <> ? LIMIT 1`
	var one int
	err := r.db.QueryRowContext(ctx, q, moduleKey, excludeID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("exists ab_module %s: %w", moduleKey, err)
	}
	return true, nil
}

// GetAbModuleByID 按主键查模块。不存在返回 (nil, nil)，404 判断留给 service
func (r *Repository) GetAbModuleByID(ctx context.Context, id int64) (*model.AbModule, error) {
	const q = `SELECT id, module_key, module_name, sort_no, created_at, updated_at
	           FROM ab_module WHERE id = ?`
	var m model.AbModule
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&m.ID, &m.ModuleKey, &m.ModuleName, &m.SortNo, &m.CreatedAt, &m.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get ab_module %d: %w", id, err)
	}
	return &m, nil
}

// ListAbModules 按标识/名称模糊分页查询模块。item_count 子查询带出（管理台删模块前
// 直观看到有无子项）；排序 sort_no,id 升序——配置域按 sort_no 稳定展示，区别于业务表 id 倒序
func (r *Repository) ListAbModules(ctx context.Context, f model.AbModuleListFilter) ([]model.AbModule, int, error) {
	where, args := abModuleWhere(f)

	// 1. 总数（无需子查询）
	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM ab_module WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count ab_module: %w", err)
	}

	// 2. 当前页（LIMIT/OFFSET 只能拼常量，参数放最后）
	const query = `SELECT id, module_key, module_name, sort_no,
	                      (SELECT COUNT(*) FROM ab_module_item i WHERE i.module_id = ab_module.id) AS item_count,
	                      created_at, updated_at
	               FROM ab_module
	               WHERE %s
	               ORDER BY sort_no, id
	               LIMIT ? OFFSET ?`

	list := make([]model.AbModule, 0, f.Limit)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(query, where), append(args, f.Limit, f.Offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query ab_module list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var m model.AbModule
		if err := rows.Scan(
			&m.ID, &m.ModuleKey, &m.ModuleName, &m.SortNo, &m.ItemCount, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan ab_module row: %w", err)
		}
		list = append(list, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate ab_module rows: %w", err)
	}
	return list, count, nil
}

// abModuleWhere 拼接模块查询条件（均 LIKE 模糊），返回 WHERE 片段（不含 WHERE 关键字）与参数
func abModuleWhere(f model.AbModuleListFilter) (string, []any) {
	where := "1 = 1"
	var args []any
	if f.ModuleKey != "" {
		where += " AND module_key LIKE CONCAT('%', ?, '%')"
		args = append(args, f.ModuleKey)
	}
	if f.ModuleName != "" {
		where += " AND module_name LIKE CONCAT('%', ?, '%')"
		args = append(args, f.ModuleName)
	}
	return where, args
}

// AbModuleOptions 全量模块下拉（配置项管理页选父模块，sort_no 排序与列表一致）
func (r *Repository) AbModuleOptions(ctx context.Context) ([]model.AbModuleOption, error) {
	const q = `SELECT id, module_key, module_name FROM ab_module ORDER BY sort_no, id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query ab_module options: %w", err)
	}
	defer rows.Close()

	list := make([]model.AbModuleOption, 0)
	for rows.Next() {
		var o model.AbModuleOption
		if err := rows.Scan(&o.ID, &o.ModuleKey, &o.ModuleName); err != nil {
			return nil, fmt.Errorf("scan ab_module option: %w", err)
		}
		list = append(list, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ab_module options: %w", err)
	}
	return list, nil
}

// CreateAbModule 新增模块（module_key 唯一性由 service 先查 + uk_module_key 兜底）
func (r *Repository) CreateAbModule(ctx context.Context, m *model.AbModule) error {
	const q = `INSERT INTO ab_module (module_key, module_name, sort_no, created_at, updated_at)
	           VALUES (?, ?, ?, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, q, m.ModuleKey, m.ModuleName, m.SortNo)
	return err
}

// UpdateAbModule 更新模块。不含 module_key：业务标识创建后不可改（编辑契约层杜绝）
func (r *Repository) UpdateAbModule(ctx context.Context, id int64, moduleName string, sortNo int) error {
	const q = `UPDATE ab_module SET module_name = ?, sort_no = ?, updated_at = NOW() WHERE id = ?`
	_, err := r.db.ExecContext(ctx, q, moduleName, sortNo, id)
	return err
}

// DeleteAbModule 删除模块（存在性与子项拦截由 service 先查）
func (r *Repository) DeleteAbModule(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM ab_module WHERE id = ?", id)
	return err
}

// CountAbModuleItems 数模块下配置项（删除父模块的拦截判断）
func (r *Repository) CountAbModuleItems(ctx context.Context, moduleID int64) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM ab_module_item WHERE module_id = ?", moduleID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count ab_module_item by module %d: %w", moduleID, err)
	}
	return count, nil
}

// ============ 配置项（子级） ============

// ListAbModuleItems 按所属模块（精确）/标识（模糊）分页查询配置项。
// JOIN ab_module 带出 module_key 供管理台展示；排序 sort_no,id 升序同模块列表
func (r *Repository) ListAbModuleItems(ctx context.Context, f model.AbModuleItemListFilter) ([]model.AbModuleItem, int, error) {
	where, args := abItemWhere(f)

	// 1. 总数
	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM ab_module_item WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count ab_module_item: %w", err)
	}

	// 2. 当前页
	const query = `SELECT i.id, i.module_id, m.module_key, i.item_key, i.item_name, i.versions, i.sort_no, i.created_at, i.updated_at
	               FROM ab_module_item i
	               JOIN ab_module m ON i.module_id = m.id
	               WHERE %s
	               ORDER BY i.sort_no, i.id
	               LIMIT ? OFFSET ?`

	list := make([]model.AbModuleItem, 0, f.Limit)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(query, where), append(args, f.Limit, f.Offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query ab_module_item list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var it model.AbModuleItem
		var versions string
		if err := rows.Scan(
			&it.ID, &it.ModuleID, &it.ModuleKey, &it.ItemKey, &it.ItemName,
			&versions, &it.SortNo, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan ab_module_item row: %w", err)
		}
		it.Versions = splitVersions(versions)
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate ab_module_item rows: %w", err)
	}
	return list, count, nil
}

// abItemWhere 拼接配置项查询条件：module_id 精确（值域从 1 起，0=不过滤）、item_key LIKE 模糊
func abItemWhere(f model.AbModuleItemListFilter) (string, []any) {
	where := "1 = 1"
	var args []any
	if f.ModuleID > 0 {
		where += " AND module_id = ?"
		args = append(args, f.ModuleID)
	}
	if f.ItemKey != "" {
		where += " AND item_key LIKE CONCAT('%', ?, '%')"
		args = append(args, f.ItemKey)
	}
	return where, args
}

// GetAbModuleItemByID 按主键查配置项。不存在返回 (nil, nil)
func (r *Repository) GetAbModuleItemByID(ctx context.Context, id int64) (*model.AbModuleItem, error) {
	const q = `SELECT i.id, i.module_id, m.module_key, i.item_key, i.item_name, i.versions, i.sort_no, i.created_at, i.updated_at
	           FROM ab_module_item i
	           JOIN ab_module m ON i.module_id = m.id
	           WHERE i.id = ?`
	var it model.AbModuleItem
	var versions string
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&it.ID, &it.ModuleID, &it.ModuleKey, &it.ItemKey, &it.ItemName,
		&versions, &it.SortNo, &it.CreatedAt, &it.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get ab_module_item %d: %w", id, err)
	}
	it.Versions = splitVersions(versions)
	return &it, nil
}

// ExistsAbItemKey 同模块内配置项标识唯一性检查（excludeID 编辑时排除自身）
func (r *Repository) ExistsAbItemKey(ctx context.Context, moduleID int64, itemKey string, excludeID int64) (bool, error) {
	const q = `SELECT 1 FROM ab_module_item WHERE module_id = ? AND item_key = ? AND id <> ? LIMIT 1`
	var one int
	err := r.db.QueryRowContext(ctx, q, moduleID, itemKey, excludeID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("exists ab_module_item %s in module %d: %w", itemKey, moduleID, err)
	}
	return true, nil
}

// CreateAbModuleItem 新增配置项。versions 已由 service 归一化为定序逗号串
func (r *Repository) CreateAbModuleItem(ctx context.Context, it *model.AbModuleItem, versions string) error {
	const q = `INSERT INTO ab_module_item (module_id, item_key, item_name, versions, sort_no, created_at, updated_at)
	           VALUES (?, ?, ?, ?, ?, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, q, it.ModuleID, it.ItemKey, it.ItemName, versions, it.SortNo)
	return err
}

// UpdateAbModuleItem 更新配置项。不含 item_key（业务标识不可改）；module_id 可改（挪模块）
func (r *Repository) UpdateAbModuleItem(ctx context.Context, id, moduleID int64, itemName, versions string, sortNo int) error {
	const q = `UPDATE ab_module_item
	           SET module_id = ?, item_name = ?, versions = ?, sort_no = ?, updated_at = NOW()
	           WHERE id = ?`
	_, err := r.db.ExecContext(ctx, q, moduleID, itemName, versions, sortNo, id)
	return err
}

// DeleteAbModuleItem 删除配置项（存在性由 service 先查）
func (r *Repository) DeleteAbModuleItem(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM ab_module_item WHERE id = ?", id)
	return err
}

// ============ H5 聚合（全量，配置数据量个位数量级） ============

// ListAllAbModules 聚合接口用：全量模块。只消费 ID/ModuleKey，其余字段顺带扫出
func (r *Repository) ListAllAbModules(ctx context.Context) ([]model.AbModule, error) {
	const q = `SELECT id, module_key, module_name, sort_no, created_at, updated_at
	           FROM ab_module ORDER BY sort_no, id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query all ab_module: %w", err)
	}
	defer rows.Close()

	list := make([]model.AbModule, 0)
	for rows.Next() {
		var m model.AbModule
		if err := rows.Scan(
			&m.ID, &m.ModuleKey, &m.ModuleName, &m.SortNo, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ab_module row: %w", err)
		}
		list = append(list, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ab_module rows: %w", err)
	}
	return list, nil
}

// ListAllAbModuleItems 聚合接口用：全量配置项。只消费 ModuleID/ItemKey/Versions，
// 不 JOIN（module_key 由 service 按 module_id 映射补全，省一次全表连接）
func (r *Repository) ListAllAbModuleItems(ctx context.Context) ([]model.AbModuleItem, error) {
	const q = `SELECT id, module_id, item_key, versions FROM ab_module_item ORDER BY sort_no, id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query all ab_module_item: %w", err)
	}
	defer rows.Close()

	list := make([]model.AbModuleItem, 0)
	for rows.Next() {
		var it model.AbModuleItem
		var versions string
		if err := rows.Scan(&it.ID, &it.ModuleID, &it.ItemKey, &versions); err != nil {
			return nil, fmt.Errorf("scan ab_module_item row: %w", err)
		}
		it.Versions = splitVersions(versions)
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ab_module_item rows: %w", err)
	}
	return list, nil
}

// splitVersions 库内逗号串转版本数组；空串返回空切片（避免 JSON 输出 null）
func splitVersions(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}
