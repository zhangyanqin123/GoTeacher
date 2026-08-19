package repository

import (
	"context"
	"database/sql"

	"handicap-service/internal/model"
)

// GetAdminUserByUsername 按用户名查管理员。ErrNoRows 返回 (nil, nil)
// （对齐 GetTeacherDetailByID 先例），service 判 nil 走凭据错误分支。
func (r *Repository) GetAdminUserByUsername(ctx context.Context, username string) (*model.AdminUser, error) {
	const q = `SELECT id, username, password, nickname, role, avatar, status,
	                  last_login_at, last_login_ip, created_at, updated_at
	           FROM admin_user WHERE username = ?`
	var u model.AdminUser
	err := r.db.QueryRowContext(ctx, q, username).Scan(
		&u.ID, &u.Username, &u.Password, &u.Nickname, &u.Role, &u.Avatar, &u.Status,
		&u.LastLoginAt, &u.LastLoginIP, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetAdminUserByID 按主键查管理员（getinfo 回查）。ErrNoRows 同样返回 (nil, nil)。
func (r *Repository) GetAdminUserByID(ctx context.Context, id int64) (*model.AdminUser, error) {
	const q = `SELECT id, username, password, nickname, role, avatar, status,
	                  last_login_at, last_login_ip, created_at, updated_at
	           FROM admin_user WHERE id = ?`
	var u model.AdminUser
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&u.ID, &u.Username, &u.Password, &u.Nickname, &u.Role, &u.Avatar, &u.Status,
		&u.LastLoginAt, &u.LastLoginIP, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// TouchAdminUserLogin 登录成功后记录最近登录时间/IP（失败不影响登录主流程返回值语义，由调用方决定）
func (r *Repository) TouchAdminUserLogin(ctx context.Context, id int64, ip string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE admin_user SET last_login_at = NOW(), last_login_ip = ? WHERE id = ?", ip, id)
	return err
}
