package repository

import (
	"database/sql"
)

// Repository 数据访问层（对应 Spring Boot 的 DAO/Mapper）
type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}
