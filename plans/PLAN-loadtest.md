# PLAN-loadtest:抢票场景压测方案(分阶段)——RabbitMQ 消费观察 + 库存不击穿验证

> 目的:① 直观查看 RabbitMQ 数据消费情况;② product 表所有商品库存改为 100000;③ 模拟抢票——瞬间大量并发下单、每次限购 1 件,验证库存不被击穿(某商品成功下单件数 ≤ 库存数)。要求先做「功能基本可用」的 MVP,后续再优化,按阶段实施。

## 背景与关键现状

防超卖守卫**已经存在**,本方案无需补实现:

- 下单链路:`POST /api/v1/orders`(JWT)→ `service.CreateOrder` 仅做库存**预检**(友好拦截)→ INSERT orders(status=1)→ 发布 `order.created`(fanout → order.stock/order.points/order.notify 三队列)
- 真正守卫在消费者:`internal/repository/order.go` 的 `DeductProductStock`——`UPDATE product SET stock = stock - ? WHERE id = ? AND stock >= ?`(条件 UPDATE),RowsAffected=0 → 订单置 status=3 已取消并事务内回滚积分/通知(`MarkOrderStockFailed`)
- 压测的本质是**验证**该守卫在瞬时高并发下守恒,同时在 RabbitMQ management UI(已部署,:15672)上直观看到消息涌入 → 堆积 → 匀速消费的全过程

**方案选型**:新增独立压测工具 `cmd/loadtest/main.go`(零外部依赖,不引入 hey/wrk;与 cmd/server、cmd/consumer 并列符合项目布局),复用 `internal/config` + `internal/database` 直连 MySQL 做守恒校验。**MVP 阶段不改任何现有后端代码**(每次限购 1 件由压测工具固定 quantity=1 实现,不改下单接口)。

## 阶段 0:环境准备(手动命令)

```bash
# 1. 所有商品库存置 100000(密码读 .env 的 DB_PASSWORD)
docker exec handicap-mysql mysql -uapp_user -p"$(grep '^DB_PASSWORD' .env | cut -d= -f2)" \
  --default-character-set=utf8mb4 handicap_db -e "UPDATE product SET stock = 100000;"

# 2. 清空历史订单数据(无外键约束;不清 product——种子只在 product 空表时写,不会重复插)
docker exec handicap-mysql mysql -uapp_user -p"$(grep '^DB_PASSWORD' .env | cut -d= -f2)" handicap_db \
  -e "TRUNCATE orders; TRUNCATE points_record; TRUNCATE notification;"

# 3. 双进程启动(测订单链路两个都要起)
go run ./cmd/server
go run ./cmd/consumer

# 4. 浏览器开 http://localhost:15672 —— Queues 页盯 order.stock / order.points / order.notify
```

三容器(handicap-mysql 3307 / handicap-redis 6380 / rabbitmq 5672+15672)由 docker compose 起,无需改动。

## 阶段 1:MVP —— cmd/loadtest 压测工具(本次实施)

新增 `cmd/loadtest/main.go`(单文件,约 250 行):

**参数(flag,均有默认值)**:`-base-url`(http://localhost:8080)、`-username/-password`(admin/admin123)、`-product`(商品ID)、`-total`(总请求数,默认 3000)、`-concurrency`(并发数,默认 50)。**quantity 固定 1,不暴露为参数**(抢票约束)。HTTP client:`Timeout 10s`、`MaxIdleConnsPerHost = concurrency`。

**流程**:

1. `POST /api/v1/login` 拿 token(响应根字段 `token`,注意 login 是 200+code 的特例契约)
2. 直连 MySQL(复用 `config.Load()` 读 .env + `database.Connect`),记初始库存 `SELECT stock FROM product WHERE id = ?`
3. worker pool:`concurrency` 个 goroutine 从 job channel 取号并发 `POST /orders {"product_id":N,"quantity":1}`;按响应分类计数:HTTP 成功(code=200)/ 库存不足(code=400)/ 其他错误;累计耗时;每 500 单打印进度
4. 发完后**轮询等待三队列消费完**(直观体现异步):`SELECT COUNT(*) FROM orders WHERE stock_status = 0 OR points_status = 0 OR notify_status = 0` 清零,每 2s 一次,`-wait-timeout` 默认 300s
5. **守恒校验(不击穿判定)**:
   - 成功占用库存件数 `sold = SELECT IFNULL(SUM(quantity),0) FROM orders WHERE product_id = ? AND status IN (1, 2)`(处理中也占库存;status=3 已取消不占——库存消费者扣不动时已回滚)
   - `final = SELECT stock FROM product WHERE id = ?`
   - 判定:`final == initial - sold` 且 `sold <= initial` → 输出 `✓ 未击穿`;否则 `✗ 击穿` 并 diff 明细,退出码非 0
6. 汇总报告:请求总数/成功/库存不足/其他错误、下单受理数、最终 status 分布(1/2/3 计数)、初始库存/剩余库存/理论剩余(initial−sold)、耗时与实际 TPS

**统计口径**(写进代码注释):HTTP 200 = 下单受理(订单落库),不等于最终成功——异步消费可能转 status=3;「成功下单件数」以 DB 订单状态为准,不信压测侧计数。

## 阶段 2+:优化路线(本次只列不做)

1. **压测工具增强**:RPS 限速、阶梯加压、p50/p95/p99 延迟分位、报告落盘
2. **订单号防碰撞**:`genOrderNo` 改雪花 ID 或纳秒级后缀(实测 ~140 TPS 下 0.7% 请求因秒级+4 位随机碰撞被 uk_order_no 拒绝,见实测记录)
3. **服务端挡流量**:Redis 原子预扣库存(DECR/Lua)、令牌桶限流中间件、商品预检读缓存
4. **消费端吞吐**:prefetch 调大 / 批量 ack / 多消费者并发(条件 UPDATE 天然并发安全);取消路径(事务三语句,实测 ~5 单/s)是吞吐短板
5. **可靠性**:outbox 表(落库与发消息原子)、DLX 死信重放(现状 nack 丢弃)

## 风险与边界

- MySQL 连接池 MaxOpenConns=20(`internal/database/database.go`):并发 50 时请求排队属预期,瓶颈在 DB 写,不影响正确性验证
- `total ≤ 100000` 时不会触达库存下限;若 total 超库存,多出部分会走「库存不足→status=3 取消」路径——本身是可演示的防护行为,守恒校验依然成立
- 压测产生几千条订单:orders/list 前端每页上限 100,仅翻页变多,无功能影响;复盘后重跑阶段 0 即复位
- JWT TTL 24h,压测期间 token 不会过期;同 token 多请求不触发互踢(互踢只在重新登录时)
- config.Load 会校验 JWT_SECRET 非空——在项目根跑 loadtest 读同一 .env,天然满足

## 验证(端到端)

1. 阶段 0 命令逐一执行:`product` 表 stock 全 100000、双进程日志正常、15672 可访问
2. 小规模试跑:`go run ./cmd/loadtest -total 20 -concurrency 5` —— 输出与 DB 一致,队列瞬间清空
3. 正式压测:`go run ./cmd/loadtest -product 1 -total 3000 -concurrency 50` —— 15672 三队列 ready 堆积后回落(消费曲线直观可见);工具输出 `✓ 未击穿`
4. 边界验证(超卖防护直接证据):把商品 3 库存改回 100 再以 `-total 2000` 打它 → 观察 100 单成功、其余转 status=3 已取消、final=0、守恒成立
5. `go build ./...` + `go vet ./...` 通过(loadtest 为纯新增,不触碰现有代码;不写单测——工具类 main,端到端验证即测试)

## 实测记录(2026-08-30,阶段 0+1 落地)

**MQ 消费直观观察**(rabbitmqctl 快照,与 15672 UI 一致):3000 单瞬时涌入,三队列 ready 同步堆到 ~2900,消费者以 ~19 msg/s 匀速回落,约 140s 清空;**unacked 恒为 1**——prefetch=1 + 手动 ack 的直接体现。

| 轮次 | 场景 | 下单 | 结果 | 守恒 |
|---|---|---|---|---|
| 试跑 | 商品1,5 并发×20 单 | 20 受理 | 20 完成 | 100000−20=99980 ✓ |
| 正式 | 商品1,50 并发×3000 单 | TPS 137,2979 受理 | 2979 完成 | 99980−2979=97001 ✓ |
| 边界 | 商品3 库存 100,50 并发×2000 单 | TPS 156,1986 受理 | **100 完成 + 1886 取消** | 100−100=0 ✓ **未击穿** |

边界轮即超卖防护的直接证据:2000 单抢 100 件,恰好 100 件成交、库存归零,一件不多。

**实测发现(观察点,均不影响守恒正确性)**:

1. **订单号碰撞 500**:`genOrderNo` 是秒级时间戳 + 4 位随机(空间 1 万),~140 TPS 下同秒约 140 单,期望碰撞约 1 次/秒——两轮共 35 次 INSERT 被 uk_order_no 拒绝返回 500(占比 ~0.7%)。属现有代码已知特性(demo 注释已言明 uk 兜底),修复列入阶段 2+(雪花 ID 或纳秒后缀)。
2. **取消路径消费慢**:库存不足取消是事务三语句(UPDATE+DELETE+DELETE),实测 ~5 单/s;边界轮 1886 个取消需 ~6 分钟,**`-wait-timeout` 须放宽至 600s**(正常轮 300s 足够)。
3. **工具轮询口径修正**:等待消费完成最初用「三步骤列全非 0」,但取消单(status=3)的 points/notify 步骤按设计跳过不回写(列恒 0),边界场景大量取消单导致条件永不清零、300s 误报超时。已改为:stock_status 每单必回写(1 成功/2 取消)作主锚点 + 未取消单(status 1/2)三列齐全。

**环境终态**:四商品库存已复位 100000;orders 保留边界轮 1986 条(100 成功/1886 取消)供前端订单页直观查看抢票结果,清空随时 `TRUNCATE orders; TRUNCATE points_record; TRUNCATE notification;`。
