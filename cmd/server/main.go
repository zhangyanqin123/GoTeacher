// @title			im系统诊股 API
// @version		1.0
// @description	chatSys（老师管理/绑定业务员/离职转移）+ 诊股记录接口。
// @description	统一响应结构 {code, msg, data}；写操作 msg 为约定中文，查询类为 "success"。
// @description	业务接口需 Bearer token（JWT + Redis 白名单，见 PLAN-auth.md）。
// @schemes		http
// @BasePath		/api/v1
// @securityDefinitions.apikey	ApiKeyAuth
// @in							header
// @name						Authorization
// @description				值格式：Bearer {token}，登录接口签发
package main

import (
	"log/slog"
	"os"

	"handicap-service/internal/config"
	"handicap-service/internal/database"
	"handicap-service/internal/router"

	_ "handicap-service/docs" // swag 生成物（swag init -g cmd/server/main.go -o docs）
)

func main() {
	// 先加载配置再初始化日志：级别（LOG_LEVEL，debug 时输出 SQL 日志）来自配置
	cfg := config.Load()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout,
		&slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)})))

	// JWT 密钥必填：空值直接退出，不给默认弱密钥（见 PLAN-auth.md）
	if cfg.JWTSecret == "" {
		slog.Error("JWT_SECRET is empty, set it in .env (e.g. openssl rand -hex 32)")
		os.Exit(1)
	}

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

	// 2. 连接 Redis（鉴权白名单，启动 fail-fast 对齐 MySQL）
	rdb, err := database.ConnectRedis(cfg)
	if err != nil {
		slog.Error("connect redis failed",
			"addr", cfg.RedisAddr,
			"db", cfg.RedisDB,
			"err", err,
		)
		os.Exit(1)
	}
	defer rdb.Close()

	// 3. 自动建表（幂等）
	if err := database.Migrate(db); err != nil {
		slog.Error("migrate failed", "err", err)
		os.Exit(1)
	}

	// 4. 表空时插入种子数据
	if err := database.Seed(db); err != nil {
		slog.Error("seed failed", "err", err)
		os.Exit(1)
	}

	// 5. 启动 HTTP 服务
	r := router.New(db, rdb, cfg)
	slog.Info("server started", "port", cfg.ServerPort, "jwt_ttl_hours", cfg.JWTTTLHours)
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
