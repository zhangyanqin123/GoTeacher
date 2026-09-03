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
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gyz-service/internal/config"
	"gyz-service/internal/database"
	"gyz-service/internal/mq"
	"gyz-service/internal/router"

	_ "gyz-service/docs" // swag 生成物（swag init -g cmd/server/main.go -o docs）
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

	// 3. 连接 RabbitMQ（订单事件 order.created 发布端，见 PLAN-order.md）——
	//    fail-fast 对齐 MySQL/Redis：MQ 不可用时创建订单必丢事件，起服务无意义
	mqConn, err := mq.Connect(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("connect rabbitmq failed", "url", cfg.RabbitMQURL, "err", err)
		os.Exit(1)
	}
	defer mqConn.Close()
	mqCh, err := mq.Channel(mqConn)
	if err != nil {
		slog.Error("open rabbitmq channel failed", "err", err)
		os.Exit(1)
	}

	// 4. 自动建表（幂等）
	if err := database.Migrate(db); err != nil {
		slog.Error("migrate failed", "err", err)
		os.Exit(1)
	}

	// 5. 表空时插入种子数据
	if err := database.Seed(db); err != nil {
		slog.Error("seed failed", "err", err)
		os.Exit(1)
	}

	// 6. 启动 HTTP 服务：http.Server 显式超时 + 优雅退出（信号风格对齐 cmd/consumer/main.go）
	r := router.New(db, rdb, cfg, mq.NewPublisher(mqCh))
	srv := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second, // slowloris 防护
		IdleTimeout:       60 * time.Second,
		// 不设 WriteTimeout：接口均短响应，设了反而在慢客户端场景截断写回
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	slog.Info("server started", "port", cfg.ServerPort, "jwt_ttl_hours", cfg.JWTTTLHours)
	select {
	case err := <-errCh:
		// 非 Shutdown 触发的退出（典型：宿主机 8080 被占）——保持既有 fail-fast 风格
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server exit", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining in-flight requests...")
	}

	// 排空在途请求；10s 必须小于 compose stop_grace_period(15s)，否则被 SIGKILL 截断
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}
	slog.Info("server stopped")
	// main 自然返回 → defer 依次关闭 mqConn/rdb/db（与启动顺序相反）
}

// parseLevel 解析日志级别（debug/info/warn/error，亦支持 "debug+2" 形式），非法值回落 info
func parseLevel(s string) slog.Level {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return lv
}
