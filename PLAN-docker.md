# PLAN-docker：中间件容器化（MySQL / Redis / RabbitMQ 统一编排）

日期：2026-08-30

## 背景

此前三个依赖形态混杂：MySQL 是本机 dmg 安装（launchd 自启、占 3306）、Redis 是 brew `redis@6.2` 手动 daemonize（占 6379）、RabbitMQ 已是裸 `docker run` 容器。本次新增 `docker-compose.yml` 把三者统一编排，Go 服务（server / consumer）仍跑宿主机 `go run`，不容器化。

## 拓扑

```
宿主机                          Docker Compose
┌─────────────┐   3307→3306    ┌────────────────┐
│ go run      │ ─────────────► │ handicap-mysql │  mysql:8.0（volume mysql_data）
│ ./cmd/server│   6380→6379    ├────────────────┤
│ ./cmd/consumer  ───────────► │ handicap-redis │  redis:6.2 AOF（volume redis_data）
│             │   5672→5672    ├────────────────┤
└─────────────┘ ─────────────► │ rabbitmq       │  rabbitmq:3-management（volume rabbitmq_data）
                                └────────────────┘
本机 dmg MySQL 仍占 3306、brew redis@6.2 仍占 6379 —— 并存互不影响
```

## 决策

1. **端口避让并存（3307/6380），不接管 3306/6379**：本机 MySQL 是 dmg 安装的 launchd 服务，停它要 sudo 且重启机器自启回来；容器换端口 + `.env` 改 `DB_PORT=3307`、`REDIS_ADDR=127.0.0.1:6380` 即零冲突切换。RabbitMQ 本机无装，维持 5672/15672 原映射，`RABBITMQ_URL` 不动。
2. **数据不迁移，靠种子重建**：go 服务对空库自动 `Migrate + Seed`（schema/seed 均 go:embed；admin_user 由 Go 代码 bcrypt 动态生成 admin/admin123），切换零数据操作。切换前已 mysqldump 留底：`/tmp/handicap_db_backup_20260830.sql`（本机 3306 的旧库仍在，双保险）。Redis 只存鉴权白名单 token，丢了重新登录即可，但 AOF + volume 让容器重启 token 不丢。
3. **RabbitMQ 从裸容器收编 compose**：`docker rm -f` 旧容器后由 compose 管理（container_name 沿用 `rabbitmq`，联调习惯不变）。队列/消息丢弃无影响：拓扑由 `internal/mq` 幂等声明，消费无积压/持久化需求（Demo 取舍见 PLAN-order.md）。
4. **compose 变量插值与 godotenv 同一 `.env` 双用**：`MYSQL_USER/MYSQL_PASSWORD` 直接 `${DB_USER}/${DB_PASSWORD}` 插值，go 服务连接凭证一处配置两边生效，不会漂移。root 密码复用 `DB_PASSWORD`（学习项目取舍，不为此引入新键；要独立时加 `MYSQL_ROOT_PASSWORD` 键即可）。**密码用 `:?` 语法强制必填**：缺失/为空时 `docker compose` 直接报错退出，无内置默认值——首版 fallback 曾直接写与 `DB_PASSWORD` 同值的真实密码，会随 yml 进 git，当日即改（校验：`docker compose config --quiet` 通过、`--env-file /dev/null` 模拟缺失时报错）。
5. **镜像版本**：`mysql:8.0` 对齐本机 MySQL 8（utf8mb4_0900_ai_ci 显式 command 声明防漂移，TZ=Asia/Shanghai 对齐 Go 侧 `Loc: time.Local`）；`redis:6.2` 对齐本机 brew 版本减少变量；`rabbitmq:3-management` 本地已有。
6. **healthcheck**：mysql `mysqladmin ping`（无凭证在服务器就绪后退出码 0，实测有效）、redis `redis-cli ping`、rabbitmq `rabbitmq-diagnostics -q ping`。均 named volume 持久化，`restart: unless-stopped`。

## 实测记录（2026-08-30）

- **MySQL 首启初始化约 2 分钟**：entrypoint 临时服务器阶段只走 socket 不监听 TCP（日志 `port: 0`），期间 `mysqladmin ping -h 127.0.0.1` 连不上 → healthcheck 报 unhealthy 属预期误判，等 `ready for connections ... port: 3306` 后转 healthy。
- **RabbitMQ 冷启动 >2 分钟**（节点 + management 插件），`retries: 12`（120s）不够会误判 unhealthy（服务实际正常，手动 diagnostics ping 通过）→ 调到 `retries: 30`。
- Docker Hub 直连超时（弱网）：经 `docker.m.daocloud.io/library/<image>` 拉取后 `docker tag` 成目标名，compose 正常识别。
- 切换后全链路验证通过：三容器 healthy / 本机 3306 与 6379 仍存活（并存确认）/ 登录+商品列表（MySQL 3307 + 种子 + Redis 6380 白名单三合一）/ 下单 `status 1→2` 三步骤列全 1（RabbitMQ 收编后消费链路无回归）。

## 常用命令

```bash
docker compose up -d            # 起齐三依赖（首次 mysql 初始化约 2 分钟）
docker compose ps               # 看健康状态（三容器均 healthy 才算就绪）
docker compose logs -f mysql    # 看日志（mysql 首启看 ready for connections ... port: 3306）
docker compose down             # 停（volume 保留）
docker compose down -v          # 停并清 volume（= 清空数据库重灌种子，慎重）
```

## 后续项（本期不做）

- Go 服务本身容器化（多阶段 Dockerfile + 全栈 compose profile）
- `.env` 拆分 `MYSQL_ROOT_PASSWORD` 独立键（当前复用 DB_PASSWORD）
- 弱网镜像源的 fallback 写进 compose（当前手工 pull + tag）
