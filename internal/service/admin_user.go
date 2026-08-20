package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"handicap-service/internal/model"
)

// 用户管理业务错误定义，handler 用 errors.Is 判断并映射 HTTP 状态码。
// 文本即 API 契约：中文可展示文案，handler 透传 err.Error() 给前端。
var (
	ErrUsernameExists    = errors.New("用户名已存在")
	ErrAdminUserNotFound = errors.New("用户不存在")
	ErrCannotDeleteSelf  = errors.New("不能删除当前登录账号")
)

// ListAdminUsers 登录账号列表（分页 + 用户名模糊，默认 pageSize=10 对齐其他列表）
func (s *Service) ListAdminUsers(ctx context.Context, req model.AdminUserListReq) (*model.PageResult, error) {
	pageIndex, pageSize := normalizePage(req.PageIndex, req.PageSize, defaultListPageSize)
	list, count, err := s.repo.ListAdminUsers(ctx, model.AdminUserListFilter{
		Username: strings.TrimSpace(req.Username),
		Offset:   (pageIndex - 1) * pageSize,
		Limit:    pageSize,
	})
	if err != nil {
		return nil, err
	}
	return &model.PageResult{List: list, Count: count}, nil
}

// CreateAdminUser 新增登录账号：重名校验 → bcrypt 哈希 → 落库。
// nickname 取 username 兜底（getinfo 的 name 有值），其余默认值由 repository 写入。
func (s *Service) CreateAdminUser(ctx context.Context, req model.AdminUserAddReq) error {
	username := strings.TrimSpace(req.Username)
	exists, err := s.repo.ExistsAdminUserByUsername(ctx, username, 0)
	if err != nil {
		return err
	}
	if exists {
		return ErrUsernameExists
	}

	hash, err := bcryptGenerate(req.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return s.repo.CreateAdminUser(ctx, &model.AdminUser{Username: username, Password: hash, Nickname: username})
}

// UpdateAdminUser 编辑登录账号：存在性 → 重名（排除自身）→ 落库 → 必要时踢下线。
// 改密码且目标非本人时 DEL 白名单使其 token 立即失效；仅改用户名不踢
// （token 与 Redis key 均以 userID 为准，getinfo 实时查库无陈旧问题）。
func (s *Service) UpdateAdminUser(ctx context.Context, operatorID int64, req model.AdminUserEditReq) error {
	u, err := s.repo.GetAdminUserByID(ctx, req.ID)
	if err != nil {
		return err
	}
	if u == nil {
		return ErrAdminUserNotFound
	}

	username := strings.TrimSpace(req.Username)
	exists, err := s.repo.ExistsAdminUserByUsername(ctx, username, req.ID)
	if err != nil {
		return err
	}
	if exists {
		return ErrUsernameExists
	}

	newHash := ""
	if req.Password != "" {
		newHash, err = bcryptGenerate(req.Password)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
	}

	if err := s.repo.UpdateAdminUser(ctx, req.ID, username, newHash); err != nil {
		return err
	}
	if req.Password != "" && req.ID != operatorID {
		return s.rdb.Del(ctx, tokenKey(req.ID)).Err()
	}
	return nil
}

// DeleteAdminUser 删除登录账号：不能删自己 → 存在性 → 删行 → 必踢下线。
// 踢下线是硬要求：不删白名单 key 的话，已删账号的 JWT 在 TTL 内仍可通过鉴权。
func (s *Service) DeleteAdminUser(ctx context.Context, operatorID, id int64) error {
	if id == operatorID {
		return ErrCannotDeleteSelf
	}
	u, err := s.repo.GetAdminUserByID(ctx, id)
	if err != nil {
		return err
	}
	if u == nil {
		return ErrAdminUserNotFound
	}

	if err := s.repo.DeleteAdminUser(ctx, id); err != nil {
		return err
	}
	return s.rdb.Del(ctx, tokenKey(id)).Err()
}
