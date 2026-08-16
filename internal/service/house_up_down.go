package service

import (
	"context"
	"errors"
	"slices"

	"handicap-service/internal/model"
	"handicap-service/internal/repository"
)

// 业务错误定义，handler 用 errors.Is 判断并映射 HTTP 状态码
var (
	ErrMissingMarket = errors.New("secuMarket is required")
	ErrInvalidRange  = errors.New("range must be one of today/week/month")
)

var validRanges = []string{"today", "week", "month"}

// Service 业务逻辑层（对应 Spring Boot 的 Service）
type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// GetHouseUpDown 校验参数后查询统计；无数据时返回 (nil, nil)。
func (s *Service) GetHouseUpDown(ctx context.Context, market, rng string) (*model.HouseUpDown, error) {
	if market == "" {
		return nil, ErrMissingMarket
	}
	if rng == "" {
		rng = "today"
	}
	if !slices.Contains(validRanges, rng) {
		return nil, ErrInvalidRange
	}
	return s.repo.FindByMarketAndRange(ctx, market, rng)
}