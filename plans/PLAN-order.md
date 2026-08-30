# 订单系统 Demo（Gin → MySQL → RabbitMQ 异步链路）

## 背景与目标

学习型 Demo：在 handicap-service 内新增订单业务域，打通「HTTP 创建订单 → MySQL 落库 → RabbitMQ fanout 广播 → 三个独立消费者（库存/积分/通知）异步处理」的完整链路。前端 gyz-admin 提供 orderDemo 四页面（创建订单 / 订单列表 / 积分列表 / 通知列表）直观观察异步状态翻转。

```
POST /api/v1/orders            ┌── order.stock   → 扣库存（条件 UPDATE，不足则订单置取消）
        │                       │
Gin handler → service          fanout exchange "order.created"
        │                       │
MySQL 落库 orders（事务提交）──发布──┼── order.points → 加积分（INSERT IGNORE 幂等）
        │                       │
                                └── order.notify  → 发通知（INSERT IGNORE 幂等）

cmd/consumer（独立进程）：连 MySQL + RabbitMQ，三队列各一 goroutine，prefetch=1 手动 ack
```

## 关键决策

### 1. Consumer 独立进程 cmd/consumer

HTTP 服务（cmd/server）与消费者（cmd/consumer）完全解耦：可单独重启观察消息积压与重投，贴合目标拓扑。消费者不鉴权不发消息（`service.New(repo, nil, "", 0, "", nil)`），只复用 repository/service 的业务方法。每队列独立 channel——AMQP channel 非线程安全，多 goroutine 共用会串方法帧。

### 2. MQ 拓扑：fanout exchange `order.created` + 三条 durable 队列

一个事件广播给三个消费者正是 fanout 语义（无路由键）。拓扑声明幂等（`internal/mq.Channel` 内 declare），server 与 consumer 谁先启动都能建齐。消息 persistent（broker 重启不丢）。

### 3. 幂等设计（fanout 重投/消费者重复消费不产生副作用）

| 消费者 | 幂等守卫 |
| --- | --- |
| 库存 | 消费前查订单 `stock_status != 0` 即跳过；扣减本身是条件 UPDATE `WHERE stock >= qty` |
| 积分 | `points_record.uk_order` 唯一键 + `INSERT IGNORE`（重投不重复加分） |
| 通知 | `notification.uk_order` 唯一键 + `INSERT IGNORE` |

### 4. 订单状态机（三步骤列并发回写，无竞态）

- `orders.status`：1 处理中 → 2 已完成 / 3 已取消（库存不足）
- `stock_status`/`points_status`/`notify_status`：0 待处理 / 1 成功 / 2 失败，各自由对应消费者回写
- 完成判定：每次回写执行 `UPDATE orders SET {col}=1, status = CASE WHEN status=1 AND 三列全 1 THEN 2 ELSE status END`。
  MySQL 单表 UPDATE 的 SET 按书写顺序求值、后读可见前写：col 恒在 CASE 之前，
  CASE 读 col 得新值 1（本步骤刚完成）、读其余两列得旧值——谁最后补齐谁翻状态，多消费者并发无竞态
- **取消路径（补偿模式）**：fanout 三消费者并行消费**无全序**，积分/通知可能先于取消落库
  （实测踩坑：service 预检 status==3 拦不住这个窗口）。三层守卫达成最终一致：
  1. `INSERT ... SELECT ... WHERE status = 1`：积分/通知落库在库端原子判取消（拦「取消已生效后」的处理）
  2. 取消事务 `MarkOrderStockFailed`：**先** `UPDATE status=3`（封住新插入）**再** `DELETE` 该订单的积分/通知流水（回滚已落库的）
  3. `CASE WHEN status = 1 AND ...`：已取消订单即便三列并发写成全 1 也不翻完成态（防御）
- 创建订单时仍有库存预检（400 提前失败），仅为友好拦截；并发正确性全靠消费侧条件 UPDATE

### 5. 可靠性取舍（Demo 级，明确不做的事）

- **发布非原子**：订单落库（单条 INSERT 自动提交）后再 publish，发布失败仅记日志不回滚——
  事件丢失窗口存在。彻底解法是 outbox 本地消息表（事务内写事件表 + 定时补发），列为后续项
- **消费失败即丢**：handler 出错 `nack(requeue=false)` 丢弃 + slog.Error，无重试无死信。
  真实系统应配 DLX（死信交换机）供人工重放，列为后续项
- 创建订单时有库存预检（400 提前失败），仅为友好拦截；并发正确性全靠消费侧条件 UPDATE

### 6. 接口与表

接口（全部挂 Auth，snake_case，分页 `page_index`/`page_size`）：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/v1/orders` | 创建订单（user_id 取登录态；金额 = price×quantity 后端计算） |
| POST | `/api/v1/orders/list` | 订单列表（order_no 精确 / product_name 模糊 / status 精确 1-3） |
| GET | `/api/v1/orders/products` | 商品下拉（data 直接为数组不分页） |
| POST | `/api/v1/points/list` | 积分列表（order_no 精确） |
| POST | `/api/v1/notifications/list` | 通知列表（title 模糊） |

表：`product`（含库存）、`orders`（order 是保留字故复数；三步骤列 + 状态）、
`points_record` / `notification`（均带 `uk_order` 幂等键）。建表在 schema.sql（go:embed 幂等），
种子 order_seed.sql（4 商品，其一库存仅 3 便于演示取消路径）。

### 7. 积分与通知内容

- 积分：1 元 1 分按订单金额向下取整（`CalcPoints`，纯函数有单测）
- 通知：Demo 渠道为站内记录表（不接短信/推送），内容含订单号/商品/数量/金额快照

## 启动与验证

```bash
# 依赖：RabbitMQ（Docker，管理台 :15672 guest/guest）
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management
# 弱网拉取：docker pull docker.m.daocloud.io/library/rabbitmq:3-management && docker tag ... rabbitmq:3-management
#      MySQL/Redis 见 README 快速开始

go run ./cmd/server     # :8080 HTTP（发布端）
go run ./cmd/consumer   # 三消费者（独立进程）

# 1. 登录拿 token（见 README 鉴权节）
# 2. 创建订单 → consumer 日志逐条输出 stock deducted / points added / notification sent
# 3. 订单列表观察 status 1→2 与三步骤列 0→1 翻转
# 4. 幂等验证：管理台向 order.created 重发同一事件 JSON → 积分/通知不重复落库、库存不二次扣减
# 5. 取消路径：并发超卖（或 SQL 手改 stock 后抢间隙下单）→ 订单 status=3、积分/通知补偿回滚
```

## 实测记录（2026-08-28，Docker RabbitMQ 3-management + MySQL 8.0.11，全部通过）

| 验证项 | 结果 |
| --- | --- |
| 创建订单（智能手表 Pro x2） | status 1 → 2，stock/points/notify 三列 0 → 1，消费者日志三条齐全 |
| 库存扣减 | 50 → 48（条件 UPDATE） |
| 积分 | +3998（1 元 1 分向下取整） |
| 通知 | 「下单成功」含订单号/商品/数量/金额快照 |
| 幂等重发（管理台 API 重发同一事件） | 积分 count 不变、库存不二次扣减 |
| 库存不足预检（库存 3 买 5） | 400「库存不足」 |
| 消费侧取消（伪造超库存事件） | status=3、stock_status=2、已落库积分/通知被补偿 DELETE（最终一致） |
| 非法消息（payload 非法 JSON） | nack 丢弃 + slog.Error 日志 |

## 后续项

- outbox 本地消息表：落库与发消息原子化，消除事件丢失窗口
- 死信队列 DLX：消费失败的消息进死信供人工重放，替代「失败即丢」
- 订单号发号器：当前时间戳+4 位随机，并发碰撞由 uk_order_no 兜底（1062 → 500 重试）
- 通知真实渠道（短信/站内推送）与 is_read 已读流
