package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"gyz-service/internal/config"
)

// ConnectRedis 建立 Redis 连接并探活（鉴权白名单存储，见 PLAN-auth.md）。
// 启动期 fail-fast：Ping 失败直接返回错误，与 Connect(MySQL) 行为一致。
func ConnectRedis(cfg *config.Config) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis %s db %d, err: %w", cfg.RedisAddr, cfg.RedisDB, err)
	}
	return rdb, nil
}
