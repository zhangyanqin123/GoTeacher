package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"gyz-service/internal/model"
)

// 鉴权哨兵错误（文本即前端展示文案，见 PLAN-auth.md）。
var (
	// ErrInvalidCredentials 文案含「密码」关键词：gyz-admin 登录页 handleError
	// 按 error.includes('密码') 定位到密码输入框报错，勿改措辞。
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserDisabled       = errors.New("账号已停用，请联系管理员")
	// ErrUnauthorized 验签失败/过期/白名单不命中统一文案，不区分原因（防探测）；
	// 具体原因打 debug 日志。
	ErrUnauthorized = errors.New("登录已过期，请重新登录")
)

// tokenKey 白名单 key：auth:token:{user_id} → jti。
// 单设备登录：重新登录 SET 覆盖旧 jti 即互踢；DEL 即主动踢人。
func tokenKey(userID int64) string {
	return "auth:token:" + strconv.FormatInt(userID, 10)
}

// AccessClaims JWT 载荷：业务字段 + 标准声明（ID 即 jti）
type AccessClaims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Login 校验账号密码，签发 JWT 并写入 Redis 白名单。
func (s *Service) Login(ctx context.Context, username, password, ip string) (token string, expire int64, err error) {
	u, err := s.repo.GetAdminUserByUsername(ctx, username)
	if err != nil {
		return "", 0, fmt.Errorf("query admin user: %w", err)
	}
	// 用户不存在与密码错误同文案，防用户名枚举
	if u == nil {
		return "", 0, ErrInvalidCredentials
	}
	if err := bcryptCompare(u.Password, password); err != nil {
		return "", 0, ErrInvalidCredentials
	}
	if u.Status != 1 {
		return "", 0, ErrUserDisabled
	}

	now := time.Now()
	expiry := now.Add(s.jwtTTL)
	claims := &AccessClaims{
		UserID:   u.ID,
		Username: u.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        newJTI(),
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		return "", 0, fmt.Errorf("sign token: %w", err)
	}

	// 白名单写入失败则整体登录失败（fail-closed：不放行无白名单的 token）
	if err := s.rdb.Set(ctx, tokenKey(u.ID), claims.ID, s.jwtTTL).Err(); err != nil {
		return "", 0, fmt.Errorf("redis set whitelist: %w", err)
	}
	if err := s.repo.TouchAdminUserLogin(ctx, u.ID, ip); err != nil {
		// 最近登录时间/IP 仅为审计信息，失败不阻断登录，记录日志即可
		slog.Warn("touch admin login failed", "user_id", u.ID, "err", err)
	}
	return token, expiry.Unix(), nil
}

// VerifyAccessToken 验签 + 白名单比对。未授权返 ErrUnauthorized；
// Redis 基础设施错误原样返回（中间件映 500，fail-closed）。
func (s *Service) VerifyAccessToken(ctx context.Context, rawToken string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	_, err := jwt.ParseWithClaims(rawToken, claims, func(t *jwt.Token) (any, error) {
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, ErrUnauthorized
	}

	jti, err := s.rdb.Get(ctx, tokenKey(claims.UserID)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrUnauthorized // 白名单无此用户（登出/被踢/过期）
	}
	if err != nil {
		return nil, err // Redis 故障，非未授权；调用方映 500
	}
	if jti != claims.ID {
		return nil, ErrUnauthorized // 旧设备 token（已被新登录覆盖）
	}
	return claims, nil
}

// Logout 删除白名单（幂等：key 已不存在也视为成功）。
func (s *Service) Logout(ctx context.Context, claims *AccessClaims) error {
	if err := s.rdb.Del(ctx, tokenKey(claims.UserID)).Err(); err != nil {
		return fmt.Errorf("redis del whitelist: %w", err)
	}
	return nil
}

// GetUserInfo 回查账号组装 getinfo 数据（gyz-admin 动态路由依赖 roles）。
func (s *Service) GetUserInfo(ctx context.Context, userID int64) (*model.UserInfo, error) {
	u, err := s.repo.GetAdminUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("query admin user: %w", err)
	}
	// 账号已被删除：强制重登
	if u == nil {
		return nil, ErrUnauthorized
	}
	return &model.UserInfo{
		Roles:        []string{u.Role},
		Name:         u.Nickname,
		Avatar:       u.Avatar,
		Introduction: "",
		Permissions:  []string{"*:*:*"},
	}, nil
}

// newJTI 生成 token 唯一标识（crypto/rand 16 字节 hex，不引 uuid 库）
func newJTI() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 读失败属系统级异常，直接 panic 让进程退出（与密钥缺失同等处理）
		panic(fmt.Sprintf("read crypto/rand: %v", err))
	}
	return hex.EncodeToString(b)
}
