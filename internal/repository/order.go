package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"handicap-service/internal/model"
)

// GetProduct 按ID查商品（创建订单回查快照/存在性校验）。查无返回 nil，不算错误。
func (r *Repository) GetProduct(ctx context.Context, id int64) (*model.Product, error) {
	const query = `SELECT id, product_name, price, stock, created_at, updated_at
	               FROM product WHERE id = ?`
	var p model.Product
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.ProductName, &p.Price, &p.Stock, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query product %d: %w", id, err)
	}
	return &p, nil
}

// ListProducts 全量商品（创建订单下拉用，种子仅 4 个不分页，按 id 正序）
func (r *Repository) ListProducts(ctx context.Context) ([]model.Product, error) {
	const query = `SELECT id, product_name, price, stock, created_at, updated_at
	               FROM product ORDER BY id`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query product list: %w", err)
	}
	defer rows.Close()

	list := make([]model.Product, 0, 8)
	for rows.Next() {
		var p model.Product
		if err := rows.Scan(&p.ID, &p.ProductName, &p.Price, &p.Stock, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan product row: %w", err)
		}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product rows: %w", err)
	}
	return list, nil
}

// InsertOrder 落一条订单（status=1 处理中，三步骤列默认 0 待处理）。单条 INSERT 自身原子。
func (r *Repository) InsertOrder(ctx context.Context, rec model.OrderInsert) (int64, error) {
	const query = `INSERT INTO orders
	               (order_no, user_id, product_id, product_name, quantity, amount)
	               VALUES (?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, query,
		rec.OrderNo, rec.UserID, rec.ProductID, rec.ProductName, rec.Quantity, rec.Amount,
	)
	if err != nil {
		return 0, fmt.Errorf("insert orders: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// GetOrder 按ID查订单（消费者幂等预检：步骤列已回写或订单已取消则跳过）。查无返回 nil。
func (r *Repository) GetOrder(ctx context.Context, id int64) (*model.Order, error) {
	const query = `SELECT id, order_no, user_id, product_id, product_name, quantity, amount,
	                      status, stock_status, points_status, notify_status, created_at, updated_at
	               FROM orders WHERE id = ?`
	var o model.Order
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&o.ID, &o.OrderNo, &o.UserID, &o.ProductID, &o.ProductName, &o.Quantity, &o.Amount,
		&o.Status, &o.StockStatus, &o.PointsStatus, &o.NotifyStatus, &o.CreatedAt, &o.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query order %d: %w", id, err)
	}
	return &o, nil
}

// DeductProductStock 条件 UPDATE 扣库存（并发守卫：stock >= qty 才扣）。
// 返回 false = 库存不足（含并发抢空），调用方据此取消订单。
func (r *Repository) DeductProductStock(ctx context.Context, productID int64, quantity int) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		"UPDATE product SET stock = stock - ? WHERE id = ? AND stock >= ?",
		quantity, productID, quantity,
	)
	if err != nil {
		return false, fmt.Errorf("deduct product stock %d: %w", productID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n > 0, nil
}

// orderSteps 步骤列白名单：MarkOrderStepDone/GetOrderStep 的 col 只允许这三个值（拼 SQL 防注入）
var orderSteps = map[string]bool{
	model.OrderStepStock:  true,
	model.OrderStepPoints: true,
	model.OrderStepNotify: true,
}

// MarkOrderStepDone 消费者成功回写步骤列，并在「未取消且三列全 1」时将 status 置 2 已完成。
//
// MySQL 单表 UPDATE 的 SET 按书写顺序求值、后读可见前写：col 恒在 CASE 之前，
// 故 CASE 里读 col 得新值 1（本步骤刚完成）、读其余两列得旧值——判断「本步 + 其余两步」
// 是否全部完成，语义正确且多消费者并发回写无竞态（谁最后补齐谁翻状态）。
// CASE 前置 status = 1：已取消（3）的订单即便三列被并发写成全 1 也不翻完成态（防御）。
func (r *Repository) MarkOrderStepDone(ctx context.Context, orderID int64, col string) error {
	if !orderSteps[col] {
		return fmt.Errorf("unknown order step column %q", col)
	}
	const doneCase = `status = CASE WHEN status = 1 AND stock_status = 1 AND points_status = 1 AND notify_status = 1
	                  THEN 2 ELSE status END`
	query := "UPDATE orders SET " + col + " = 1, " + doneCase + " WHERE id = ?"
	if _, err := r.db.ExecContext(ctx, query, orderID); err != nil {
		return fmt.Errorf("mark orders.%s done %d: %w", col, orderID, err)
	}
	return nil
}

// MarkOrderStockFailed 库存消费者失败路径（补偿模式，同一事务三步）：
//  1. UPDATE orders SET stock_status=2, status=3——先置取消，封住后续
//     INSERT...SELECT...WHERE status=1 的积分/通知插入（并行消费无全序，
//     积分可能已先于取消落库，靠下面两步 DELETE 补偿，最终一致）
//  2. DELETE points_record——回滚已加积分
//  3. DELETE notification——撤回已发通知
//
// 顺序承重：必须先 UPDATE 再 DELETE——反序时 UPDATE 前的间隙里
// INSERT...SELECT 仍见 status=1 可插入，留下删不干净的孤儿流水。
func (r *Repository) MarkOrderStockFailed(ctx context.Context, orderID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mark stock failed begin: %w", err)
	}
	defer tx.Rollback() // 已提交时为 no-op

	const cancel = `UPDATE orders SET stock_status = 2, status = 3 WHERE id = ?`
	if _, err := tx.ExecContext(ctx, cancel, orderID); err != nil {
		return fmt.Errorf("cancel order %d: %w", orderID, err)
	}
	const delPoints = `DELETE FROM points_record WHERE order_id = ?`
	if _, err := tx.ExecContext(ctx, delPoints, orderID); err != nil {
		return fmt.Errorf("rollback points of order %d: %w", orderID, err)
	}
	const delNotify = `DELETE FROM notification WHERE order_id = ?`
	if _, err := tx.ExecContext(ctx, delNotify, orderID); err != nil {
		return fmt.Errorf("rollback notification of order %d: %w", orderID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mark stock failed commit: %w", err)
	}
	return nil
}

// InsertPointsRecord 落积分流水，原子守卫「订单未取消」：
// INSERT ... SELECT ... WHERE status = 1 在库端判取消，规避三消费者并发的预检竞态
// （积分消费者预检读到 status=1、库存消费者随后才写入取消的窗口）。
// INSERT IGNORE + uk_order 唯一键 = 消息幂等：重投/重复消费不重复加分。
// RowsAffected == 0 的两种成因（uk 冲突或订单已取消/不存在）都视为「无需加分」。
func (r *Repository) InsertPointsRecord(ctx context.Context, rec model.PointsInsert) error {
	const query = `INSERT IGNORE INTO points_record (user_id, order_id, order_no, points, remark)
	               SELECT ?, ?, ?, ?, ? FROM orders WHERE id = ? AND status = 1`
	if _, err := r.db.ExecContext(ctx, query,
		rec.UserID, rec.OrderID, rec.OrderNo, rec.Points, rec.Remark, rec.OrderID,
	); err != nil {
		return fmt.Errorf("insert points_record: %w", err)
	}
	return nil
}

// InsertNotification 落通知记录，原子守卫与幂等语义同 InsertPointsRecord。
func (r *Repository) InsertNotification(ctx context.Context, rec model.NotificationInsert) error {
	const query = `INSERT IGNORE INTO notification (user_id, order_id, title, content)
	               SELECT ?, ?, ?, ? FROM orders WHERE id = ? AND status = 1`
	if _, err := r.db.ExecContext(ctx, query,
		rec.UserID, rec.OrderID, rec.Title, rec.Content, rec.OrderID,
	); err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// ListOrders 按筛选条件分页查询订单（动态 WHERE：零值不拼接；模糊走 LIKE CONCAT；id 倒序最新在上）
func (r *Repository) ListOrders(ctx context.Context, f model.OrderListFilter) ([]model.Order, int, error) {
	where, args := orderWhere(f)

	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM orders o WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
	}

	const query = `SELECT o.id, o.order_no, o.user_id, o.product_id, o.product_name, o.quantity, o.amount,
	                      o.status, o.stock_status, o.points_status, o.notify_status, o.created_at, o.updated_at
	               FROM orders o
	               WHERE %s
	               ORDER BY o.id DESC
	               LIMIT ? OFFSET ?`
	list := make([]model.Order, 0, f.PageSize)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(query, where), append(args, f.PageSize, (f.PageIndex-1)*f.PageSize)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query orders list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var it model.Order
		if err := rows.Scan(
			&it.ID, &it.OrderNo, &it.UserID, &it.ProductID, &it.ProductName, &it.Quantity, &it.Amount,
			&it.Status, &it.StockStatus, &it.PointsStatus, &it.NotifyStatus, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan orders row: %w", err)
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate orders rows: %w", err)
	}
	return list, count, nil
}

// orderWhere 拼接订单筛选条件（不含 WHERE 关键字），返回片段与参数
func orderWhere(f model.OrderListFilter) (string, []any) {
	var conds []string
	var args []any

	if f.OrderNo != "" {
		conds = append(conds, "o.order_no = ?")
		args = append(args, f.OrderNo)
	}
	if f.ProductName != "" {
		conds = append(conds, "o.product_name LIKE CONCAT('%', ?, '%')")
		args = append(args, f.ProductName)
	}
	if f.Status != 0 {
		conds = append(conds, "o.status = ?")
		args = append(args, f.Status)
	}

	if len(conds) == 0 {
		return "1 = 1", args
	}
	return strings.Join(conds, " AND "), args
}

// ListPoints 按筛选条件分页查询积分流水（order_no 精确匹配快照列，id 倒序）
func (r *Repository) ListPoints(ctx context.Context, f model.PointsListFilter) ([]model.PointsRecord, int, error) {
	where, args := "1 = 1", []any{}
	if f.OrderNo != "" {
		where, args = "p.order_no = ?", []any{f.OrderNo}
	}

	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM points_record p WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count points_record: %w", err)
	}

	const query = `SELECT p.id, p.user_id, p.order_id, p.order_no, p.points, p.remark, p.created_at
	               FROM points_record p
	               WHERE %s
	               ORDER BY p.id DESC
	               LIMIT ? OFFSET ?`
	list := make([]model.PointsRecord, 0, f.PageSize)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(query, where), append(args, f.PageSize, (f.PageIndex-1)*f.PageSize)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query points_record list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var it model.PointsRecord
		if err := rows.Scan(&it.ID, &it.UserID, &it.OrderID, &it.OrderNo, &it.Points, &it.Remark, &it.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan points_record row: %w", err)
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate points_record rows: %w", err)
	}
	return list, count, nil
}

// ListNotifications 按筛选条件分页查询通知记录（title 模糊，id 倒序）
func (r *Repository) ListNotifications(ctx context.Context, f model.NotificationListFilter) ([]model.Notification, int, error) {
	where, args := "1 = 1", []any{}
	if f.Title != "" {
		where, args = "n.title LIKE CONCAT('%', ?, '%')", []any{f.Title}
	}

	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification n WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count notification: %w", err)
	}

	const query = `SELECT n.id, n.user_id, n.order_id, n.title, n.content, n.is_read, n.created_at
	               FROM notification n
	               WHERE %s
	               ORDER BY n.id DESC
	               LIMIT ? OFFSET ?`
	list := make([]model.Notification, 0, f.PageSize)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(query, where), append(args, f.PageSize, (f.PageIndex-1)*f.PageSize)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query notification list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var it model.Notification
		if err := rows.Scan(&it.ID, &it.UserID, &it.OrderID, &it.Title, &it.Content, &it.IsRead, &it.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification row: %w", err)
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate notification rows: %w", err)
	}
	return list, count, nil
}
