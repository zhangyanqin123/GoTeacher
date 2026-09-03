// 订单事件消费者（订单系统 Demo，见 PLAN-order.md）：
// 连 MySQL + RabbitMQ，三条队列（order.stock/order.points/order.notify）各一 goroutine 消费，
// prefetch=1 手动 ack，与 cmd/server 完全独立部署（贴合 Gin→MQ→Consumer 的解耦拓扑）。
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"

	"gyz-service/internal/config"
	"gyz-service/internal/database"
	"gyz-service/internal/model"
	"gyz-service/internal/mq"
	"gyz-service/internal/repository"
	"gyz-service/internal/service"
)

func main() {
	cfg := config.Load()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout,
		&slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)})))

	// 1. 连接 MySQL（消费者直接走 repo 写库存/积分/通知，不走 HTTP）
	db, err := database.Connect(cfg)
	if err != nil {
		slog.Error("connect mysql failed",
			"addr", cfg.DBHost+":"+cfg.DBPort, "user", cfg.DBUser, "db", cfg.DBName, "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// 2. 连接 RabbitMQ（声明拓扑幂等：先于 server 启动也能建齐 exchange/queue）
	conn, err := mq.Connect(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("connect rabbitmq failed", "url", cfg.RabbitMQURL, "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	// 3. 组装业务（rdb/jwt/xiaoe/publisher 均不触达：消费者不鉴权不发消息，传零值/nil）
	repo := repository.New(db)
	svc := service.New(repo, nil, "", 0, "", nil)

	// 4. SIGINT/SIGTERM 优雅停机：cancel 后各消费者退出投递循环（在途消息处理完再退出）
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	consumers := []struct {
		queue   string
		handler func(ctx context.Context, ev model.OrderCreatedEvent) error
	}{
		{mq.QueueStock, svc.DeductStock},
		{mq.QueuePoints, svc.AddPoints},
		{mq.QueueNotify, svc.SendNotification},
	}

	var wg sync.WaitGroup
	for _, c := range consumers {
		// 每队列独立 channel：AMQP channel 非线程安全，多 goroutine 共用会串方法帧
		ch, err := mq.Channel(conn)
		if err != nil {
			slog.Error("open channel failed", "queue", c.queue, "err", err)
			os.Exit(1)
		}
		wg.Add(1)
		go func(queue string, ch *amqp.Channel, fn func(ctx context.Context, ev model.OrderCreatedEvent) error) {
			defer wg.Done()
			defer ch.Close()
			if err := mq.Consume(ctx, ch, queue, mq.OrderCreatedHandler(fn)); err != nil {
				slog.Error("consume failed", "queue", queue, "err", err)
				os.Exit(1) // channel 级致命错误：进程退出交给外部重启（Demo 手动）
			}
		}(c.queue, ch, c.handler)
	}

	slog.Info("order consumers started",
		"queues", []string{mq.QueueStock, mq.QueuePoints, mq.QueueNotify})
	wg.Wait()
	slog.Info("order consumers stopped")
}

// parseLevel 解析日志级别（与 cmd/server/main.go 同款，非法值回落 info）
func parseLevel(s string) slog.Level {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return lv
}
