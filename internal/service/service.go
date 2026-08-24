package service

import (
	"time"

	"github.com/redis/go-redis/v9"

	"handicap-service/internal/repository"
)

// Service 业务逻辑层（对应 Spring Boot 的 Service）
type Service struct {
	repo         *repository.Repository
	rdb          *redis.Client // 鉴权白名单（auth:token:{uid} → jti，见 PLAN-auth.md）
	jwtSecret    []byte        // HS256 签发/验签密钥
	jwtTTL       time.Duration // JWT 有效期，同时是白名单 TTL
	xiaoeAPIBase string        // 小鹅通开放平台 API 域名（直播登录链接透传上游，见 PLAN-live.md）
}

// New 显式收 jwt/xiaoe 参数而非整个 *config.Config：service 不依赖 config 包，测试更易构造。
func New(repo *repository.Repository, rdb *redis.Client, jwtSecret string, jwtTTL time.Duration, xiaoeAPIBase string) *Service {
	return &Service{
		repo:         repo,
		rdb:          rdb,
		jwtSecret:    []byte(jwtSecret),
		jwtTTL:       jwtTTL,
		xiaoeAPIBase: xiaoeAPIBase,
	}
}
