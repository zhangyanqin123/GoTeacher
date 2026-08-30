# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目定位

Go 学习项目 `handicap-service`：chatSys（老师管理/离职转移）+ 诊股记录接口。**核心约束：chatSys 与诊股接口的 JSON 键名全链路 snake_case（`page_index`/`dept_id`/`original_teacher_id`），与前端 gyz-admin 对接页面的键名严格一致**（2026-08-18 由 camelCase 整体迁移，决策见 PLAN-api-snake-case.md）。新增字段一律蛇形；URL 路径段（如 `bindSales`）不在此约束内。新增/修改接口前先读对应 `PLAN-*.md` 了解设计决策。

## 常用命令

```bash
go run ./cmd/server          # 启动（首次自动建表 + 种子；默认 :8080）
go build ./...
go test ./...                # 跑全部测试
go test ./internal/sanitize -run TestRichText -v   # 单个测试
go vet ./...
swag init -g cmd/server/main.go -o docs   # 接口注释改动后重新生成 Swagger 文档
```

- 依赖 MySQL/Redis/RabbitMQ 统一由 Docker Compose 起：`docker compose up -d`（健康状态 `docker compose ps`，决策见 PLAN-docker.md）。容器映射 **3307/6380** 避让本机 dmg MySQL（3306）与 brew redis@6.2（6379），与本机服务并存互不影响
- 库无需手工建：compose 的 `MYSQL_DATABASE=handicap_db` 自动建库，`MYSQL_USER/MYSQL_PASSWORD` 插值自 `.env` 的 DB_USER/DB_PASSWORD（app_user）；空库首启 go 服务自动建表+种子；`JWT_SECRET` 必填（空值启动退出），见 PLAN-auth.md
- 配置走 `.env`（模板 `.env.example`；本仓库实际 `.env` 端口已指容器 3307/6380）
- 重灌种子：`TRUNCATE TABLE <表>;` 后重启（种子仅在表空时写入，schema/seed SQL 均 go:embed 随二进制发布；admin_user 种子为 Go 代码 bcrypt 动态生成 admin/admin123，不走 seed SQL）
- 无 lint 工具链，无 Makefile
- Swagger：handler 注释即文档源（@Summary/@Param/@Success 等），启动后 `/swagger/index.html` 可视化；`docs/` 生成物需随代码提交（`cmd/server/main.go` blank import 依赖）；文档专用响应类型（弥补 `PageResult.List` 为 any 无法展开 schema）集中在 `internal/model/swagger.go`，运行时不使用

## 架构

标准分层（对标 Spring Boot），依赖只允许自上而下：

```
handler → service → repository → model
```

- `internal/router/router.go` 是唯一组装点：`repository.New(db) → service.New(repo, rdb, secret, ttl) → 各 handler`。repository 与 service 均为**单一结构体**，按业务域拆文件，不按实体建多 struct
- `cmd/server/main.go`：加载配置 → 连 MySQL/Redis → `Migrate`（幂等建表 + 存量列型升级）→ `Seed` → 路由启动
- 数据访问是裸 `database/sql`（无 ORM）：动态 WHERE 靠拼 SQL 片段 + args；模糊查询用 `LIKE CONCAT('%',?,'%')`；LIMIT/OFFSET 只能拼常量，参数放最后

### 鉴权约定（JWT + Redis 白名单，设计决策见 PLAN-auth.md）

- 除 `POST /api/v1/login` 与 `/swagger/**` 外全部挂 `Auth` 中间件（`internal/router/auth.go`）：解析 `Authorization: Bearer` → `service.VerifyAccessToken` 验签 + 白名单比对 → `c.Set` 用户信息（key 常量在 `internal/model/auth.go`）。**新增业务接口默认进鉴权组**（挂 `Auth(svc)` 或放 `authed` 组内）
- 单设备模式：Redis `auth:token:{user_id}` 存当前有效 jti，TTL=JWT 有效期；重新登录覆盖即互踢，`DEL` 即踢人；登出/logout 幂等
- 401 统一文案 `登录已过期，请重新登录`（哨兵错误 `service.ErrUnauthorized`，不区分失败原因防探测）；Redis 故障映 500（fail-closed）
- **login 接口是响应约定特例**：失败也返回 HTTP 200 + `code:400`（前端登录页对 reject 值调 `error.includes('密码')`）；`token`/`expire`/`passwd_expired` 在 body 根而非 data 内；`ErrInvalidCredentials` 文案必须含「密码」关键词（前端据此定位密码输入框），勿改措辞

### 错误处理约定

service 层定义哨兵错误（`ErrTeacherNotFound` 等），handler 用 `errors.Is` 映射 HTTP 状态码（404/400）。响应统一走 `internal/response`：`{code, msg, data}`，写操作 msg 用约定中文（`编辑成功`/`转移成功` 等，非 `ok`），查询类为 `success`。失败 msg 为中文可展示文案（前端拦截器直接弹 `error.response.data.msg`）：哨兵错误文本即文案，handler 透传 `err.Error()`；HTTP 状态码保留 4xx/5xx 语义（不切恒 200，保留监控/网关 5xx 告警）。

### 响应格式约定（对接前端 gyz-admin，改动接口时勿破坏）

- JSON 键名全链路 snake_case：model `json:"..."` tag、handler `c.Query` 参数名、Swagger `@Param`、请求/响应体均蛇形（`db` tag 本就蛇形，两者天然同名）；分页参数 `page_index`/`page_size`
- 时间输出 `YYYY-MM-DD HH:mm:ss`：DATETIME 列用 `model.DateTimeString`（sql.Scanner 在扫描点格式化，避免 RFC3339 带 T）；NULL 扫为空串
- `status` 等输出字符串 `"1"`/`"0"`（库存 TINYINT）；`qualification`/审核日志的 `log_type`、`result` 存中文展示串
- 分页统一 `data.list` / `data.count`；默认 page_size=10（绑定列表 5），上限 100
- 数值筛选传 `0` 是有效过滤值：用指针区分「未传」与「传 0」

### 并发守卫（写操作标准模式）

纯 SELECT-then-UPDATE 有 TOCTOU：写前查一次区分 404/400，事务内条件 UPDATE（如 `WHERE status IN (1,3,5)`），`RowsAffected == 0` 回滚返回 400。诊股状态机（submitReport/audit）是典型实现。

### 富文本 XSS

`internal/sanitize.RichText`（bluemonday 白名单）是存储型 XSS 主防线，富文本入库前必过；独立成包供 C 端复用，策略对输出幂等。历史存量库列型升级（如 VARCHAR→TEXT）写在 `database.Migrate` 内的幂等函数里，CREATE TABLE IF NOT EXISTS 不会改已建表。

## 表设计原则

- 冗余快照（姓名/部门/股票名等）由后端从主表回查，忽略前端传的同名字段
- 可推导的计数不落库，用子查询统计（如 `bind_sales_count`），保持单一事实来源
- 模糊列前导通配 LIKE 打不进 B-tree，只给精确/范围条件建索引
