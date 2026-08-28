// Package mq RabbitMQ 接入（订单系统 Demo，见 PLAN-order.md）：
// fanout exchange order.created 广播给三条队列，分别由库存/积分/通知消费者消费。
// 拓扑声明幂等（durable），server 与 consumer 谁先启动都能把 exchange/queue 建齐。
package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

	"handicap-service/internal/model"
)

// 拓扑常量：一个事件广播给三个消费者（库存/积分/通知）
const (
	OrderCreatedExchange = "order.created" // fanout：订单已创建事件
	QueueStock           = "order.stock"   // 库存消费者队列
	QueuePoints          = "order.points"  // 积分消费者队列
	QueueNotify          = "order.notify"  // 通知消费者队列
)

// orderQueues 三条消费者队列（declareTopology 与 cmd/consumer 共用顺序）
var orderQueues = []string{QueueStock, QueuePoints, QueueNotify}

// Connect 建立 RabbitMQ 连接并声明拓扑。
// 返回 *amqp.Connection 由调用方持有（连接可开多个 channel）；失败 fail-fast 由上层退出。
func Connect(url string) (*amqp.Connection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	if _, err := Channel(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// Channel 开新 channel 并幂等声明拓扑。
// 每个消费者独立 channel：AMQP channel 非线程安全，多 goroutine 共用会串方法帧。
func Channel(conn *amqp.Connection) (*amqp.Channel, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	if err := declareTopology(ch); err != nil {
		_ = ch.Close()
		return nil, err
	}
	return ch, nil
}

// declareTopology 幂等声明 exchange/queue/bind（fanout 无路由键，bind key 固定空串）
func declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(OrderCreatedExchange, amqp.ExchangeFanout,
		true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %s: %w", OrderCreatedExchange, err)
	}
	for _, q := range orderQueues {
		if _, err := ch.QueueDeclare(q, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare queue %s: %w", q, err)
		}
		if err := ch.QueueBind(q, "", OrderCreatedExchange, false, nil); err != nil {
			return fmt.Errorf("bind queue %s: %w", q, err)
		}
	}
	return nil
}

// Publisher 订单事件发布接口。
// service 依赖此抽象而非 amqp 实现（测试可传 nil 跳过发布），真实现由 cmd/server 组装。
type Publisher interface {
	PublishOrderCreated(ctx context.Context, ev model.OrderCreatedEvent) error
}

// AMQPPublisher Publisher 的 amqp 实现（复用 cmd/server 生命周期的 channel）
type AMQPPublisher struct {
	ch *amqp.Channel
}

func NewPublisher(ch *amqp.Channel) *AMQPPublisher {
	return &AMQPPublisher{ch: ch}
}

// PublishOrderCreated 发布订单已创建事件（persistent 落盘，broker 重启不丢）
func (p *AMQPPublisher) PublishOrderCreated(_ context.Context, ev model.OrderCreatedEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal order.created event: %w", err)
	}
	if err := p.ch.PublishWithContext(context.Background(), OrderCreatedExchange, "",
		false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		}); err != nil {
		return fmt.Errorf("publish order.created: %w", err)
	}
	return nil
}

// Consume 阻塞消费一条队列：prefetch=1 逐条处理 + 手动 ack。
// handler 返回 nil → ack；返回 error → nack 丢弃（requeue=false）并记日志——
// Demo 取舍：不重试不进死信，失败即丢（真实系统应配 DLX 供人工重放，见 PLAN-order.md 后续项）。
// ctx 取消时退出循环，返回 nil（上层优雅停机）。
func Consume(ctx context.Context, ch *amqp.Channel, queue string, handler func(ctx context.Context, body []byte) error) error {
	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set qos for %s: %w", queue, err)
	}
	msgs, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", queue, err)
	}
	slog.Info("consumer started", "queue", queue)
	for {
		select {
		case <-ctx.Done():
			slog.Info("consumer stopped", "queue", queue)
			return nil
		case d, ok := <-msgs:
			if !ok { // channel 被 broker 关闭，视为致命错误让进程退出重启
				return fmt.Errorf("queue %s delivery channel closed", queue)
			}
			if err := handler(ctx, d.Body); err != nil {
				slog.Error("handle message failed, discard (no requeue)",
					"queue", queue, "err", err, "body", string(d.Body))
				if err := d.Nack(false, false); err != nil {
					return fmt.Errorf("nack %s: %w", queue, err)
				}
				continue
			}
			if err := d.Ack(false); err != nil {
				return fmt.Errorf("ack %s: %w", queue, err)
			}
		}
	}
}

// OrderCreatedHandler 把 []byte 反序列化为事件再交给业务回调（cmd/consumer 装配用）。
// 收 []byte 而非事件类型：mq 包不依赖 service，避免 service → mq → service 环。
func OrderCreatedHandler(fn func(ctx context.Context, ev model.OrderCreatedEvent) error) func(ctx context.Context, body []byte) error {
	return func(ctx context.Context, body []byte) error {
		var ev model.OrderCreatedEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			return fmt.Errorf("unmarshal order.created: %w", err)
		}
		return fn(ctx, ev)
	}
}
