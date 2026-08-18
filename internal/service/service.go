package service

import (
	"handicap-service/internal/repository"
)

// Service 业务逻辑层（对应 Spring Boot 的 Service）
type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}
