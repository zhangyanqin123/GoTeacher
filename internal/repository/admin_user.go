package repository

import (
	"context"
	"database/sql"
	"fmt"

	"gyz-service/internal/model"
)

// ExistsAdminUserByUsername 用户名唯一性检查。
// excludeID 用于编辑时排除自身；新增时传 0（id <> 0 对所有行恒真）。
// 存在性检查用 SELECT 1 ... LIMIT 1，不用 RowsAffected 判断（同 ExistsTeacher 先例）。
func (r *Repository) ExistsAdminUserByUsername(ctx context.Context, username string, excludeID int64) (bool, error) {
	const q = `SELECT 1 FROM admin_user WHERE username = ? AND id <> ? LIMIT 1`
	var one int
	err := r.db.QueryRowContext(ctx, q, username, excludeID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("exists admin_user %s: %w", username, err)
	}
	return true, nil
}

// ListAdminUsers 按用户名模糊条件分页查询登录账号。
// SELECT 列表不含 password（json:"-" 双保险）；last_login_at 可空，
// 依赖 model.DateTimeString 扫 NULL 为空串。排序 id 倒序（新账号在上）。
func (r *Repository) ListAdminUsers(ctx context.Context, f model.AdminUserListFilter) ([]model.AdminUser, int, error) {
	where, args := adminUserWhere(f.Username)

	// 1. 总数
	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM admin_user WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count admin_user: %w", err)
	}

	// 2. 当前页（LIMIT/OFFSET 只能拼常量，参数放最后）
	const query = `SELECT id, username, nickname, role, status, last_login_at, last_login_ip, created_at, updated_at
	               FROM admin_user
	               WHERE %s
	               ORDER BY id DESC
	               LIMIT ? OFFSET ?`

	list := make([]model.AdminUser, 0, f.Limit)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(query, where), append(args, f.Limit, f.Offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin_user list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var it model.AdminUser
		if err := rows.Scan(
			&it.ID, &it.Username, &it.Nickname, &it.Role, &it.Status,
			&it.LastLoginAt, &it.LastLoginIP, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan admin_user row: %w", err)
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate admin_user rows: %w", err)
	}
	return list, count, nil
}

// adminUserWhere 拼接用户名模糊条件，返回 WHERE 片段（不含 WHERE 关键字）与参数
func adminUserWhere(username string) (string, []any) {
	if username == "" {
		return "1 = 1", nil
	}
	return "username LIKE CONCAT('%', ?, '%')", []any{username}
}

// CreateAdminUser 新增登录账号。表单外字段显式写默认值（不依赖 schema DEFAULT，
// 便于审计）：nickname 取 username 兜底、role 固定 admin、avatar 空串、status 启用；
// last_login_at/last_login_ip 不写保持 NULL（列表显示空）。
func (r *Repository) CreateAdminUser(ctx context.Context, u *model.AdminUser) error {
	const q = `INSERT INTO admin_user (username, password, nickname, role, avatar, status, created_at, updated_at)
	           VALUES (?, ?, ?, 'admin', '', 1, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, q, u.Username, u.Password, u.Nickname)
	return err
}

// UpdateAdminUser 更新登录账号。newHash 空串表示不改密码（两条 SQL 二选一，单条 UPDATE 原子）
func (r *Repository) UpdateAdminUser(ctx context.Context, id int64, username, newHash string) error {
	if newHash == "" {
		_, err := r.db.ExecContext(ctx,
			"UPDATE admin_user SET username = ?, updated_at = NOW() WHERE id = ?", username, id)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		"UPDATE admin_user SET username = ?, password = ?, updated_at = NOW() WHERE id = ?", username, newHash, id)
	return err
}

// DeleteAdminUser 删除登录账号（存在性由 service 层先查，不用 RowsAffected 推断 404）
func (r *Repository) DeleteAdminUser(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM admin_user WHERE id = ?", id)
	return err
}
