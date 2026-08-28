package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"handicap-service/internal/model"
)

// order 业务错误定义，handler 用 errors.Is 判断并映射 HTTP 状态码
// （文本即 API 契约：中文可展示文案，handler 透传 err.Error() 给前端）
var (
	ErrProductNotFound   = errors.New("商品不存在")
	ErrInvalidQuantity   = errors.New("购买数量必须为 1-999")
	ErrInsufficientStock = errors.New("库存不足")
)

const maxOrderQuantity = 999 // 对齐下单数量上限（防一次买空库存刷事件）

// ListProducts 商品下拉（创建订单页）
func (s *Service) ListProducts(ctx context.Context) ([]model.Product, error) {
	return s.repo.ListProducts(ctx)
}

// CreateOrder 创建订单：校验 → 查商品（快照 + 库存预检）→ 落库 → 发 order.created。
// 金额由后端按 price×quantity 计算（两位小数回规），忽略前端一切冗余字段；
// 库存预检只是友好拦截（400 提前失败），真正的并发守卫在消费者条件 UPDATE。
// user_id 取登录态（handler 从 ctx 传入）。
//
// 发布取舍（Demo）：落库与发消息非原子——INSERT 提交后再 publish，
// 失败仅记日志不回滚（订单已可见，事件丢失靠 outbox 表补偿，见 PLAN-order.md 后续项）。
func (s *Service) CreateOrder(ctx context.Context, req model.OrderCreateReq, userID int64) (*model.Order, error) {
	// 1. 基础校验
	if req.Quantity < 1 || req.Quantity > maxOrderQuantity {
		return nil, ErrInvalidQuantity
	}
	if req.ProductID <= 0 {
		return nil, ErrProductNotFound
	}

	// 2. 回查商品：存在性 + 库存预检 + 快照（名称/单价以库为准）
	p, err := s.repo.GetProduct(ctx, req.ProductID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrProductNotFound
	}
	if p.Stock < req.Quantity {
		return nil, ErrInsufficientStock
	}

	// 3. 落库（status=1 处理中；product_name 冗余快照保留下单当时值）
	rec := model.OrderInsert{
		OrderNo:     genOrderNo(),
		UserID:      userID,
		ProductID:   p.ID,
		ProductName: p.ProductName,
		Quantity:    req.Quantity,
		Amount:      roundAmount(p.Price * float64(req.Quantity)),
	}
	id, err := s.repo.InsertOrder(ctx, rec)
	if err != nil {
		return nil, err
	}

	// 4. 事务已提交（单条 INSERT 自动提交），发布事件广播给库存/积分/通知三队列
	ev := model.OrderCreatedEvent{
		OrderID:     id,
		OrderNo:     rec.OrderNo,
		UserID:      rec.UserID,
		ProductID:   rec.ProductID,
		ProductName: rec.ProductName,
		Quantity:    rec.Quantity,
		Amount:      rec.Amount,
	}
	if s.publisher != nil {
		if err := s.publisher.PublishOrderCreated(ctx, ev); err != nil {
			slog.Error("publish order.created failed (order persisted, event lost)",
				"order_id", id, "order_no", rec.OrderNo, "err", err)
		}
	}

	return &model.Order{
		ID:           id,
		OrderNo:      rec.OrderNo,
		UserID:       rec.UserID,
		ProductID:    rec.ProductID,
		ProductName:  rec.ProductName,
		Quantity:     rec.Quantity,
		Amount:       rec.Amount,
		Status:       "1",
		StockStatus:  "0",
		PointsStatus: "0",
		NotifyStatus: "0",
		CreatedAt:    model.DateTimeString(time.Now().Format("2006-01-02 15:04:05")),
		UpdatedAt:    model.DateTimeString(time.Now().Format("2006-01-02 15:04:05")),
	}, nil
}

// ListOrders 订单列表（分页 + 筛选，默认 pageSize=10）
func (s *Service) ListOrders(ctx context.Context, f model.OrderListFilter) (*model.PageResult, error) {
	f.PageIndex, f.PageSize = normalizePage(f.PageIndex, f.PageSize, defaultListPageSize)
	list, count, err := s.repo.ListOrders(ctx, f)
	if err != nil {
		return nil, err
	}
	return &model.PageResult{List: list, Count: count}, nil
}

// ListPoints 积分列表（分页，默认 pageSize=10）
func (s *Service) ListPoints(ctx context.Context, f model.PointsListFilter) (*model.PageResult, error) {
	f.PageIndex, f.PageSize = normalizePage(f.PageIndex, f.PageSize, defaultListPageSize)
	list, count, err := s.repo.ListPoints(ctx, f)
	if err != nil {
		return nil, err
	}
	return &model.PageResult{List: list, Count: count}, nil
}

// ListNotifications 通知列表（分页，默认 pageSize=10）
func (s *Service) ListNotifications(ctx context.Context, f model.NotificationListFilter) (*model.PageResult, error) {
	f.PageIndex, f.PageSize = normalizePage(f.PageIndex, f.PageSize, defaultListPageSize)
	list, count, err := s.repo.ListNotifications(ctx, f)
	if err != nil {
		return nil, err
	}
	return &model.PageResult{List: list, Count: count}, nil
}

// DeductStock 库存消费者（order.created → 扣库存）：
// 幂等预检（步骤已回写/订单已取消直接跳过）→ 条件 UPDATE 扣库存 →
// 不足则订单置 3 已取消（预检与消费间隙的并发缺口兜底）。
func (s *Service) DeductStock(ctx context.Context, ev model.OrderCreatedEvent) error {
	skip, err := s.orderStepDone(ctx, ev.OrderID, model.OrderStepStock)
	if err != nil || skip {
		return err
	}
	ok, err := s.repo.DeductProductStock(ctx, ev.ProductID, ev.Quantity)
	if err != nil {
		return err
	}
	if !ok {
		// 补偿模式：并行消费下积分/通知可能已先落库，取消事务内回滚（见 repo 注释）
		slog.Warn("insufficient stock, order cancelled (points/notification rolled back)",
			"order_no", ev.OrderNo, "product_id", ev.ProductID)
		return s.repo.MarkOrderStockFailed(ctx, ev.OrderID)
	}
	slog.Info("stock deducted", "order_no", ev.OrderNo, "product_id", ev.ProductID, "quantity", ev.Quantity)
	return s.repo.MarkOrderStepDone(ctx, ev.OrderID, model.OrderStepStock)
}

// AddPoints 积分消费者（order.created → 加积分）：1 元 1 分按订单金额向下取整。
// 幂等靠 points_record.uk_order + INSERT IGNORE（重复消息不重复加分）；
// 「订单已取消不加分」靠 INSERT...SELECT...WHERE status=1 在库端原子守卫——
// 三消费者并行时取消写入与积分预检存在竞态窗口，不能只依赖 service 预检。
func (s *Service) AddPoints(ctx context.Context, ev model.OrderCreatedEvent) error {
	skip, err := s.orderStepDone(ctx, ev.OrderID, model.OrderStepPoints)
	if err != nil || skip {
		return err
	}
	points := CalcPoints(ev.Amount)
	if err := s.repo.InsertPointsRecord(ctx, model.PointsInsert{
		UserID:  ev.UserID,
		OrderID: ev.OrderID,
		OrderNo: ev.OrderNo,
		Points:  points,
		Remark:  fmt.Sprintf("订单 %s 消费 ￥%.2f", ev.OrderNo, ev.Amount),
	}); err != nil {
		return err
	}
	slog.Info("points added", "order_no", ev.OrderNo, "points", points)
	return s.repo.MarkOrderStepDone(ctx, ev.OrderID, model.OrderStepPoints)
}

// SendNotification 通知消费者（order.created → 发通知）：
// Demo 渠道为站内记录表（不接短信/推送）；幂等与「已取消不发」守卫同 AddPoints
// （uk_order + INSERT IGNORE + INSERT...SELECT...WHERE status=1）。
func (s *Service) SendNotification(ctx context.Context, ev model.OrderCreatedEvent) error {
	skip, err := s.orderStepDone(ctx, ev.OrderID, model.OrderStepNotify)
	if err != nil || skip {
		return err
	}
	if err := s.repo.InsertNotification(ctx, model.NotificationInsert{
		UserID:  ev.UserID,
		OrderID: ev.OrderID,
		Title:   "下单成功",
		Content: fmt.Sprintf("您的订单 %s（%s x%d，￥%.2f）已创建，积分与库存处理中",
			ev.OrderNo, ev.ProductName, ev.Quantity, ev.Amount),
	}); err != nil {
		return err
	}
	slog.Info("notification sent", "order_no", ev.OrderNo)
	return s.repo.MarkOrderStepDone(ctx, ev.OrderID, model.OrderStepNotify)
}

// orderStepDone 消费幂等预检（省一次无谓写入，非正确性守卫）：订单不存在（异常事件）、
// 该步骤已回写（重复投递）、订单已取消（积分/通知不处理）三种情况均跳过，返回 true。
// 取消判断存在并行竞态窗口（库存消费者可能尚未写入取消），真正的守卫在
// repository 的 INSERT...SELECT...WHERE status = 1（库端原子）。
func (s *Service) orderStepDone(ctx context.Context, orderID int64, step string) (bool, error) {
	o, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return false, err
	}
	if o == nil {
		return true, nil // 事件引用的订单不存在：丢弃（nack 无意义，ack 走人）
	}
	if o.Status == "3" { // 已取消：跳过积分/通知（库存消费者自身的失败回写也不重做）
		return true, nil
	}
	var stepVal string
	switch step {
	case model.OrderStepStock:
		stepVal = o.StockStatus
	case model.OrderStepPoints:
		stepVal = o.PointsStatus
	case model.OrderStepNotify:
		stepVal = o.NotifyStatus
	}
	return stepVal != "0", nil // 1 成功 / 2 失败 都算已处理，重复消息直接跳过
}

// CalcPoints 积分计算（纯函数供测试）：1 元 1 分，按订单金额向下取整。
// 1999.0 → 1999；399.5 → 399。
func CalcPoints(amount float64) int {
	return int(amount)
}

// roundAmount 金额两位小数回规（DECIMAL(10,2) 落库，规避浮点乘的尾差）
func roundAmount(v float64) float64 {
	return math.Round(v*100) / 100
}

// genOrderNo 订单号：yyyyMMddHHmmss + 4 位随机（demo 唯一性足够，uk_order_no 兜底冲突）
func genOrderNo() string {
	return time.Now().Format("20060102150405") + fmt.Sprintf("%04d", rand.Intn(10000))
}
