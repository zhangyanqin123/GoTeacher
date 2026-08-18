package main

import (
	"log/slog"
	"os"

	"handicap-service/internal/config"
	"handicap-service/internal/database"
	"handicap-service/internal/router"
)

func main() {
	// 先加载配置再初始化日志：级别（LOG_LEVEL，debug 时输出 SQL 日志）来自配置
	cfg := config.Load()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout,
		&slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)})))

	// 1. 连接 MySQL
	db, err := database.Connect(cfg)
	if err != nil {
		slog.Error("connect mysql failed",
			"addr", cfg.DBHost+":"+cfg.DBPort,
			"user", cfg.DBUser,
			"db", cfg.DBName,
			"err", err,
		)
		os.Exit(1)
	}
	defer db.Close()

	// 2. 自动建表（幂等）
	if err := database.Migrate(db); err != nil {
		slog.Error("migrate failed", "err", err)
		os.Exit(1)
	}

	// 3. 表空时插入种子数据
	if err := database.Seed(db); err != nil {
		slog.Error("seed failed", "err", err)
		os.Exit(1)
	}

	// 4. 启动 HTTP 服务
	r := router.New(db)
	slog.Info("server started", "port", cfg.ServerPort)
	if err := r.Run(":" + cfg.ServerPort); err != nil {
		slog.Error("server exit", "err", err)
	}
}

// parseLevel 解析日志级别（debug/info/warn/error，亦支持 "debug+2" 形式），非法值回落 info
func parseLevel(s string) slog.Level {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return lv
}