# PLAN-docker：容器化编排（中间件 + Go 服务，MySQL / Redis / RabbitMQ / server / consumer）

日期：2026-08-30（中间件三件套）；2026-08-31 追加 Go 服务容器化

## 背景

此前三个依赖形态混杂：MySQL 是本机 dmg 安装（launchd 自启、占 3306）、Redis 是 brew `redis@6.2` 手动 daemonize（占 6379）、RabbitMQ 已是裸 `docker run` 容器。首期（08-30）新增 `docker-compose.yml` 把三者统一编排。次日（08-31）补齐最后一块：server/consumer 以 `profiles: ["app"]` 容器化（多阶段 Dockerfile、单镜像双二进制），`docker compose up -d` 默认仍只起中间件（宿主机 `go run` 开发流不变），`--profile app up -d` 一键全栈。配套前端 GoProject-web 容器（独立仓库，`:80` 入口）的 nginx 反代 `host.docker.internal:8080` 零改动衔接——server 容器仍映射宿主机 `8080:8080`，端口背后从 go run 进程换成容器。同日再演进（二改）：前端容器改走 compose 网络直连 `handicap-server:8080`，server 去掉宿主机 `8080` 映射并撤掉 app profile——`up -d` 即全栈且与宿主机 go run 零端口冲突（决策 10）。

## 拓扑

```
宿主机                          Docker Compose（默认网络，服务名即 DNS）
┌─────────────┐   3307→3306    ┌────────────────┐
│ go run      │ ─────────────► │ handicap-mysql │  mysql:8.0（volume mysql_data）
│ ./cmd/server│   6380→6379    ├────────────────┤
│ ./cmd/consumer  ───────────► │ handicap-redis │  redis:6.2 AOF（volume redis_data）
│             │   5672→5672    ├────────────────┤
└─────────────┘ ─────────────► │ rabbitmq       │  rabbitmq:3-management（volume rabbitmq_data）
                                ├────────────────┤
                                │ handicap-server │ ┐
                                ├────────────────┤ │ 同镜像 handicap-server:latest
                                │ handicap-      │ │ 单镜像双二进制 /app/server、
                                │ consumer       │ ┘ /app/consumer（command 覆盖）
                                └────────────────┘
浏览器 → GoProject-web 容器 :80（nginx）→（goproject_default 网络）→ handicap-server 容器 :8080
server 不映射宿主机端口：8080 空闲留给 go run（与容器化全栈并存）；Swagger 走前端 /swagger 反代
本机 dmg MySQL 仍占 3306、brew redis@6.2 仍占 6379 —— 并存互不影响
```

容器网络内 app 直连服务名（`DB_HOST=mysql`、`REDIS_ADDR=redis:6379`、`RABBITMQ_URL=amqp://...@rabbitmq:5672/`，端口用容器内端口非宿主机映射端口）；凭证/密钥从 `.env` 插值经 compose `environment` 注入，不进镜像层。

## 决策

1. **端口避让并存（3307/6380），不接管 3306/6379**：本机 MySQL 是 dmg 安装的 launchd 服务，停它要 sudo 且重启机器自启回来；容器换端口 + `.env` 改 `DB_PORT=3307`、`REDIS_ADDR=127.0.0.1:6380` 即零冲突切换。RabbitMQ 本机无装，维持 5672/15672 原映射，`RABBITMQ_URL` 不动。
2. **数据不迁移，靠种子重建**：go 服务对空库自动 `Migrate + Seed`（schema/seed 均 go:embed；admin_user 由 Go 代码 bcrypt 动态生成 admin/admin123），切换零数据操作。切换前已 mysqldump 留底：`/tmp/handicap_db_backup_20260830.sql`（本机 3306 的旧库仍在，双保险）。Redis 只存鉴权白名单 token，丢了重新登录即可，但 AOF + volume 让容器重启 token 不丢。
3. **RabbitMQ 从裸容器收编 compose**：`docker rm -f` 旧容器后由 compose 管理（container_name 沿用 `rabbitmq`，联调习惯不变）。队列/消息丢弃无影响：拓扑由 `internal/mq` 幂等声明，消费无积压/持久化需求（Demo 取舍见 PLAN-order.md）。
4. **compose 变量插值与 godotenv 同一 `.env` 双用**：`MYSQL_USER/MYSQL_PASSWORD` 直接 `${DB_USER}/${DB_PASSWORD}` 插值，go 服务连接凭证一处配置两边生效，不会漂移。root 密码复用 `DB_PASSWORD`（学习项目取舍，不为此引入新键；要独立时加 `MYSQL_ROOT_PASSWORD` 键即可）。**密码用 `:?` 语法强制必填**：缺失/为空时 `docker compose` 直接报错退出，无内置默认值——首版 fallback 曾直接写与 `DB_PASSWORD` 同值的真实密码，会随 yml 进 git，当日即改（校验：`docker compose config --quiet` 通过、`--env-file /dev/null` 模拟缺失时报错）。08-31 起 `JWT_SECRET` 同样 `:?` 必填注入 server。
5. **镜像版本**：`mysql:8.0` 对齐本机 MySQL 8（utf8mb4_0900_ai_ci 显式 command 声明防漂移，TZ=Asia/Shanghai 对齐 Go 侧 `Loc: time.Local`）；`redis:6.2` 对齐本机 brew 版本减少变量；`rabbitmq:3-management` 本地已有；Go 镜像 `golang:1.24-alpine`（≥go.mod 的 1.24.2）构建 + `alpine:3.21` 运行，两基座与 `GOPROXY` 均 ARG 参数化（弱网 `.env` 覆盖换 daocloud 源，键见 `.env.example` 末段）。
6. **healthcheck**：mysql `mysqladmin ping`（无凭证在服务器就绪后退出码 0，实测有效）、redis `redis-cli ping`、rabbitmq `rabbitmq-diagnostics -q ping`（timeout 08-31 调 15s：VM 资源紧张时该 erlang 脚本冷执行 >5s，5s 曾致 depends_on 误判 unhealthy）、server `wget --spider /health`（新增的公开免鉴权探针，见决策 9）。均 named volume 持久化（app 无写盘不需卷），`restart: unless-stopped`。
7. **Go 镜像构建（08-31）**：多阶段单镜像双二进制——一个 builder `CGO_ENABLED=0 -trimpath -ldflags="-s -w"` 产出 `/app/server` 与 `/app/consumer`，compose 里 consumer 服务用 `command: ["/app/consumer"]` 覆盖（不拆两个 build target：同 module 同依赖，双镜像徒增 tag 管理与版本漂移风险）。运行时 `apk add tzdata ca-certificates` + `TZ=Asia/Shanghai`：tzdata 保 DSN `Loc: time.Local` 为 +8（否则 DATETIME 偏 8h，且容器内 `date`/日志时间戳也是 UTC 难调试）；ca-certificates 是小鹅通透传 `https://api.xiaoe-tech.com` 必需（裸 alpine x509 报错）。**HEALTHCHECK 只写 compose 不写 Dockerfile**：镜像被 server/consumer 共用，镜像级探活会在 consumer 容器（无端口）上永远 unhealthy。CMD 而非 ENTRYPOINT，给 command 覆盖留口。`.dockerignore` 红线：`docs/` 不能整目录排除（`cmd/server/main.go` blank import docs.go），只排 swagger.json/yaml 副本（docs.go 内联 docTemplate 自包含，容器内 /swagger 200 实测验证过）；`.env` 绝不进构建上下文。
8. **app profile 隔离（08-31，同日被决策 10 取代）**：server/consumer 挂 `profiles: ["app"]`，`docker compose up -d` 默认只起三件套——宿主机 `go run` 开发流完全不变；`docker compose --profile app up -d --build` 全栈。两者都占宿主机 8080，全栈前先停宿主机 go run（stop/down 统一带 `--profile app`，不带 profile 的 down 停不掉 app 容器）。
9. **/health 探针与优雅退出（08-31，代码改动）**：`router.go` 新增公开免鉴权 `GET /health`（`response.OK` → 200，纯进程探活**不 ping 依赖**——中间件挂了重启 server 容器无意义且中断在途请求，依赖可达性由启动 fail-fast 保证；日后要 readiness 另加 /ready 不污染 liveness）；`cmd/server/main.go` 由 `r.Run` 改 `http.Server` + `signal.NotifyContext(SIGINT,SIGTERM)` + `Shutdown(10s)`（风格对齐 cmd/consumer；`errors.Is(err, http.ErrServerClosed)` 过滤正常退出，10s < compose `stop_grace_period: 15s`）。consumer 无端口不设 healthcheck：唯一进程即 PID 1，进程退出容器即退出，restart 负责拉起。
10. **前端容器网络直连 + 撤 profile + 去端口映射（08-31 二改）**：GoProject-web 容器加入 `goproject_default` 网络（`docker run --network`，前端独立仓库、不纳入本 compose，避免跨仓编排），nginx `API_UPSTREAM` 直指 `http://handicap-server:8080`（容器名 + 容器内端口），去掉 `host.docker.internal` 绕宿主机一圈的依赖；server 随之去掉 `ports: "8080:8080"`——宿主机 8080 永远留给 go run，容器化全栈与宿主机开发流并存零冲突。app profile 的初衷就是避 8080 冲突（决策 8），端口不冲突后失去存在意义，server/consumer 撤掉 `profiles: ["app"]`，`up -d` 即全栈。代价与对策：宿主机浏览器失去 :8080 直连入口，Swagger 改走前端 nginx `/swagger` 反代（前端 template 反代正则加 `swagger`）；nginx 静态 `proxy_pass` 在启动时解析容器名，须先起后端 compose 再起前端容器，顺序颠倒靠前端 `--restart unless-stopped` 反复拉起自愈。

## 实测记录（2026-08-30）

- **MySQL 首启初始化约 2 分钟**：entrypoint 临时服务器阶段只走 socket 不监听 TCP（日志 `port: 0`），期间 `mysqladmin ping -h 127.0.0.1` 连不上 → healthcheck 报 unhealthy 属预期误判，等 `ready for connections ... port: 3306` 后转 healthy。
- **RabbitMQ 冷启动 >2 分钟**（节点 + management 插件），`retries: 12`（120s）不够会误判 unhealthy（服务实际正常，手动 diagnostics ping 通过）→ 调到 `retries: 30`。
- Docker Hub 直连超时（弱网）：经 `docker.m.daocloud.io/library/<image>` 拉取后 `docker tag` 成目标名，compose 正常识别。
- 切换后全链路验证通过：三容器 healthy / 本机 3306 与 6379 仍存活（并存确认）/ 登录+商品列表（MySQL 3307 + 种子 + Redis 6380 白名单三合一）/ 下单 `status 1→2` 三步骤列全 1（RabbitMQ 收编后消费链路无回归）。

## 实测记录（2026-08-31，Go 服务容器化）

- **镜像 55.4MB**（server 二进制 39MB + consumer 7MB + alpine 基座）；`docker run --rm` 无 JWT_SECRET 冒烟打出 fail-fast 错误退出；容器内 `date` 显示 CST +08（tzdata 生效）、busybox wget 在位。
- **首次构建 go build 花了 2406s（40 分钟）**：`go mod download` 仅 30s（goproxy.cn），慢在 Docker Desktop VM（4C/7.65GB）内编译——正常，但代价高。层缓存锚点 `COPY go.mod go.sum` 先行，改业务代码不重下依赖；**构建后记得 `docker builder prune`**：本次 buildkit 缓存积到 4.77GB 挤爆 VM 内存（见下条）。
- **`# syntax=docker/dockerfile:1` 在弱网下是坑**：该指令要联网拉 BuildKit frontend（auth.docker.io 超时构建直接失败），而本 Dockerfile 只用基础指令，删掉即好——已留注释在 Dockerfile 头部。
- **RabbitMQ memory alarm 连环坑（VM 内存挤压）**：go build 的 buildkit 缓存把 VM（7.65GB）内存挤到高水位 → rabbitmq `alarm_handler: set system_memory_high_watermark` → **alarm 期间 AMQP 直接 connection refused**，server/consumer 双双 fail-fast 重启循环，且 `rabbitmq-diagnostics` 探针也超时（healthcheck unhealthy）。处置：`docker builder prune` 清缓存 → alarm 自动 clear（日志 `clear,system_memory_high_watermark`）。同场踩坑：healthcheck `timeout: 5s` 不够 diagnostics 冷执行，已调 15s。
- **server 的 MQ channel 不自动重建（既有弱点，非容器化引入）**：alarm 期间被 rabbitmq 关闭的 channel 在 alarm 恢复后仍是死的（amqp091 无内置重连），下单报 `publish order.created failed ... channel/connection is not open`（504），订单落库 status 1 但事件丢失（PLAN-order.md 的 Demo 取舍：order persisted, event lost）。`docker compose restart server` 即恢复。/health 探不出去世 channel（纯进程探活的边界）。consumer 无此问题（channel 致命错误 os.Exit 交给 restart）。**排错口诀：rabbitmq 重启/告警后，重启 server 容器**。
- VM 内存紧张时接口显著变慢：login（bcrypt）实测 5-8s，curl `--max-time 5` 会截断成空响应误导排查——排查时放宽超时再看。
- 全栈验证通过：`--profile app up -d --build`（二次起 build 层全命中缓存）/ 五容器齐（server healthy、consumer running）/ `/health` 200 / 登录+商品+下单 `status 1→2` stock/points/notify 全 1（consumer 容器消费）/ `docker compose stop server` 走优雅退出日志（`shutdown signal received` → `server stopped`，未被 SIGKILL）/ 前端 GoProject-web 容器 :80 → nginx 反代 → `host.docker.internal:8080` → server 容器登录 200（前端零改动）/ `--profile app down` 全停后普通 `up -d` 只起三件套（profile 隔离语义实测）。
- 弱网基础镜像：`golang:1.24-alpine`、`alpine:3.21` 均经 `docker.m.daocloud.io/library/` 拉取 + tag。

## 实测记录（2026-08-31 二改，前端容器网络直连 + 撤 profile）

- 切换流程：`docker compose down`（旧 app profile 容器随项目标签一并移除，无残留）→ 改 compose 撤 profile/去 ports → `docker compose up -d --build`（Go 代码未动，层缓存全命中）→ 前端 `docker build` 重建（template 加 swagger 反代）→ `docker rm -f goproject-web` 后按新参数 `docker run` 重挂。
- **rabbitmq down/up 重启的 listener 滞后坑**：healthcheck（`rabbitmq-diagnostics -q ping`）先转 healthy，AMQP 5672 listener 滞后数十秒才监听——期间 server 连接 refused 走 restart 循环，listener 就绪后自愈拉起（无需人工干预，又一条「rabbitmq 重启后看 server 自动恢复」的证据；排错时别急着手动重启）。
- 弱网 build：本地缺 `node:22-alpine`（此前被清理），auth.docker.io 拒连 → `docker.m.daocloud.io/library/node:22-alpine` 拉取 + tag 后 build 通过；`nginx:stable-alpine` 本地已有直接复用（buildkit 对本地已有 tag 仍尝试 resolve metadata，node 缺失才卡住）。
- 全链路验证：前端容器内 `wget http://handicap-server:8080/health` 200（容器名 DNS + 网络连通）/ 经前端 `:80` nginx 登录 200「登录成功」/ `/swagger/index.html` 经前端反代 200（去端口映射后的新文档入口）/ `lsof :8080` 空闲。
- **并存零冲突验证（本次改动核心卖点）**：宿主机起 go 二进制占 8080（`/health` 200）的同时，前端 `:80` 登录依旧 200（走容器 server）——容器化全栈与宿主机 `go run` 开发流真正并存、互不影响，验证后进程已清理。

## 常用命令

```bash
docker compose up -d --build   # 全栈：中间件 + server/consumer（改代码后 --build；首次 mysql 初始化约 2 分钟）
docker compose ps              # 看健康状态（server healthy、consumer running 才算就绪）
docker compose logs -f server  # 看业务日志（mysql 首启看 ready for connections ... port: 3306）
docker compose stop server     # 优雅停 server（SIGTERM，15s 宽限）
docker compose down            # 全停（volume 保留）
docker compose down -v         # 停并清 volume（= 清空数据库重灌种子，慎重）
docker builder prune -f        # 清构建缓存（build 后 VM 内存紧张/告警时必做，见实测记录）
# 前端容器（GoProject-web 仓库）：先起本 compose 再起前端（nginx 启动时解析容器名）
docker run -d -p 80:80 --name goproject-web --network goproject_default --restart unless-stopped goproject-web
```

## 后续项（本期不做）

- ~~Go 服务本身容器化（多阶段 Dockerfile + 全栈 compose profile）~~ ✅ 2026-08-31 完成
- **server 的 MQ publisher 连接自愈**：`NotifyClose` 监听 channel 关闭 → `os.Exit(1)` 交 restart 拉起（对齐 consumer 哲学），替代当前「rabbitmq 重启后手动重启 server」的排错口诀
- ~~**GoProject-web 容器纳入同一 compose 网络**：去掉 `host.docker.internal` 依赖，`API_UPSTREAM` 直指 `http://handicap-server:8080`~~ ✅ 2026-08-31 完成（决策 10：`docker run --network goproject_default` 共享网络，前端独立仓库不纳入本 compose）
- `/ready` 就绪探针（ping 依赖，供滚动发布判断；与 /health 的 liveness 分离）
- `.env` 拆分 `MYSQL_ROOT_PASSWORD` 独立键（当前复用 DB_PASSWORD）
- 镜像加 `USER nobody` 运行时加固（app 零写盘可行，当前保留 root 便于 exec sh 调试）
- CI 构建镜像（当前仅本机构建）
