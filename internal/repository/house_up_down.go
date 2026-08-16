package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"handicap-service/internal/model"
)

// Repository 数据访问层（对应 Spring Boot 的 DAO/Mapper）
type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// FindByMarketAndRange 按市场代码 + 统计区间查询最新一条统计。
// 无数据时返回 (nil, nil)，由调用方决定业务含义。
func (r *Repository) FindByMarketAndRange(ctx context.Context, market, rng string) (*model.HouseUpDown, error) {
	const query = `SELECT secu_market, stat_range, above7, between5_7, between3_5, between0_3,
	                      equal0, between_n3_0, between_n5_n3, between_n7_n5, below_n7,
	                      total, up_count, down_count, flat_count, stat_date
	               FROM house_up_down_stats
	               WHERE secu_market = ? AND stat_range = ?
	               ORDER BY stat_date DESC LIMIT 1`

	var m model.HouseUpDown
	err := r.db.QueryRowContext(ctx, query, market, rng).Scan(
		&m.SecuMarket, &m.Range,
		&m.Above7, &m.Between5_7, &m.Between3_5, &m.Between0_3,
		&m.Equal0, &m.BetweenN3_0, &m.BetweenN5_N3, &m.BetweenN7_N5, &m.BelowN7,
		&m.Total, &m.UpCount, &m.DownCount, &m.FlatCount,
		&m.StatDate,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query house_up_down_stats: %w", err)
	}
	return &m, nil
}