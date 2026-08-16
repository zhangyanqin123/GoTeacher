package main

import (
	"log/slog"
	"os"

	"handicap-service/internal/config"
	"handicap-service/internal/database"
	"handicap-service/internal/router"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg := config.Load()

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