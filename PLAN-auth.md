# PLAN-auth：JWT + Redis 鉴权

> 2026-08-19 立项。chatSys/诊股接口此前全量裸奔（无用户表、无登录接口、无中间件），本计划为全部 `/api/v1/**` 业务接口补齐 token 鉴权（无 session/cookie，`Authorization: Bearer {token}`）。

## Context

项目现状**零鉴权**：无用户表、无登录接口、无中间件，全部业务路由裸奔（多处 service 注释「无登录态固定 admin」）。前端 gyz-admin（Vue2）已有成熟习惯：`Authorization: Bearer {token}`、token 存 Cookie、`code=401` 触发重登弹窗。

已确认决策：

- **方案 B：JWT + Redis**——JWT 签发/本地验签 + Redis 白名单（`auth:token:{uid} → jti`），实现登出即时失效、单设备互踢、主动踢人（DEL key）
- **本服务自建登录**：新建 `admin_user` 表 + bcrypt 密码 + 种子账号 `admin/admin123`
- **全量路由保护**：login/logout/getinfo 三个接口 + 全部 `/api/v1/dxsf/**` 业务路由挂鉴权；swagger 保持公开

### 两个前端契约发现（决定接口形状，不可破坏）

1. **login 失败必须 HTTP 200 + 业务码 400**（不能 HTTP 4xx）：gyz-admin 登录页 `handleError` 对 reject 值直接调 `error.includes('密码')`，HTTP 错误时 reject 的是 Error 对象 → TypeError → 登录按钮永久转圈。`ErrInvalidCredentials` 文案含「密码」关键词，前端据此定位密码输入框。
2. **login 响应 token 在 body 根**（不在 data 里）：gyz-admin store 从响应根解构 `{token, passwd_expired, expire}`。

已知前端行为（本期不改前端）：业务接口 HTTP 401 走 axios error 分支只弹 toast，不触发「重新登录」弹窗（body code=401 才触发）。保留 HTTP 401 语义是项目约定，不迁就。

## 设计决策摘要

| 决策点 | 结论 |
| --- | --- |
| 依赖 | `golang-jwt/jwt/v5`、`redis/go-redis/v9`、`golang.org/x/crypto/bcrypt`（已间接依赖转直接）；测试 `alicebob/miniredis/v2`（仅 test） |
| JWT | HS256，claims = `user_id`/`username` + `RegisteredClaims`（`ID`=jti，crypto/rand 16B hex；`ExpiresAt`/`IssuedAt`） |
| Redis key | `auth:token:{user_id}` → value = jti，`SET ... EX ttl`（TTL=JWT 有效期）。**单设备登录**：重新登录覆盖即互踢 |
| 种子方式 | Go 代码 `bcrypt.GenerateFromPassword` 动态生成（SQL 固定哈希=明文泄露，且每次哈希不同） |
| 中间件位置 | `internal/router/auth.go`（与 cors.go 同目录，不新建 middleware 目录；薄壳调 `svc.VerifyAccessToken`） |
| context key | `internal/model/auth.go` 定义 `CtxKeyUserID`/`CtxKeyUsername` 常量（model 零依赖，router/handler 双向可用） |
| 401 响应 | `response.Fail(c, 401, 401, "登录已过期，请重新登录")`；缺头/坏 token/验签失败/白名单不命中统一文案（防探测），日志区分原因 |
| Redis 故障 | 启动 Ping 失败退出（对齐 MySQL）；校验期 Redis 错误 → 500 fail-closed；登录期 SET 失败 → 登录失败 |
| swagger | `@BasePath` `/api/v1/dxsf` → `/api/v1`，12 处 `@Router` 加 `/dxsf` 前缀 |

## 实施步骤

### Step 1：依赖与配置

- `go get github.com/golang-jwt/jwt/v5@latest && go get github.com/redis/go-redis/v9@latest && go mod tidy`
- `internal/config/config.go`：Config 增 `JWTSecret`、`JWTTTLHours`（默认 24）、`RedisAddr`（默认 127.0.0.1:6379）、`RedisPassword`、`RedisDB`；新增 `getEnvInt` 辅助函数
- `.env.example`：新增 5 个配置项，`JWT_SECRET` 给 32+ 位示例串并注明「必填，空值启动失败」
- `cmd/server/main.go`：`JWTSecret == ""` 时启动直接退出（不给默认弱密钥）

### Step 2：数据库层

- `internal/database/schema.sql` 末尾追加 `admin_user` 表（id/username/password CHAR(60)/nickname/role 默认 'admin'/avatar/status/last_login_at/last_login_ip/created_at/updated_at，`uk_username` 唯一索引，utf8mb4_0900_ai_ci）。CREATE TABLE IF NOT EXISTS 天然幂等
- 新建 `internal/database/redis.go`：`ConnectRedis(cfg) (*redis.Client, error)`——`redis.NewClient` + 5 秒超时 Ping，失败 Close 返回错误（对齐 `Connect` 风格）
- `internal/database/database.go` 的 `Seed` 末尾追加 `seedAdminUser(db)`：不走 `seedIfEmpty`（哈希需动态生成），模式为 `COUNT(*) FROM admin_user`=0 时 `bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)` + INSERT（username='admin', nickname='系统管理员', role='admin'）

### Step 3：model 层

新建 `internal/model/auth.go`：

- `CtxKeyUserID = "userID"` / `CtxKeyUsername = "username"` 常量
- `AdminUser`（db tag 对齐表列，Password `json:"-"`）
- `LoginReq{username, password}` 均 `binding:"required"`（前端多传的 phone_code/uuid/rememberMe 由 gin 静默忽略）
- `LoginResp{code, msg, token, expire, passwd_expired, data}`——**token 在根**，运行时直接 `c.JSON` 的结构体（非 swagger.go 文档专用类型）
- `UserInfo{roles, name, avatar, introduction, permissions}` + `GetInfoResp`（文档展开）

### Step 4：repository 层

新建 `internal/repository/auth.go`（挂在现有 `Repository` 结构体）：

- `GetAdminUserByUsername(ctx, username) (*model.AdminUser, error)`：`ErrNoRows` 返回 `(nil, nil)`（对齐 `GetTeacherDetailByID` 先例，service 判 nil → 凭据错误）
- `TouchAdminUserLogin(ctx, id, ip)`：UPDATE `last_login_at=NOW(), last_login_ip=?`

### Step 5：service 层（核心）

- `internal/service/service.go`：`Service` 增 `rdb *redis.Client`、`jwtSecret []byte`、`jwtTTL time.Duration`；`New(repo, rdb, jwtSecret string, jwtTTL)`（显式参数，service 不依赖 config 包）
- 新建 `internal/service/auth.go`，哨兵错误（中文即文案）：

```go
var (
    ErrInvalidCredentials = errors.New("用户名或密码错误")   // 含「密码」关键词，前端登录页定位密码框
    ErrUserDisabled       = errors.New("账号已停用，请联系管理员")
    ErrUnauthorized       = errors.New("登录已过期，请重新登录") // 验签失败/过期/白名单不命中统一文案
)
```

方法：

- `Login(ctx, username, password, ip)`：查用户（nil 或 bcrypt 不匹配 → 同返 `ErrInvalidCredentials`，防用户枚举）→ `status==0` → `ErrUserDisabled` → 生成 jti → 签 HS256 token → `rdb.Set(auth:token:{uid}, jti, ttl)`（失败整体登录失败）→ `TouchAdminUserLogin`
- `VerifyAccessToken(ctx, raw) (*AccessClaims, error)`：`jwt.ParseWithClaims` + `WithValidMethods([]string{"HS256"})` → 白名单 GET 比对 jti（不符/过期 → `ErrUnauthorized`；Redis 基础设施错误原样返回，中间件映 500）
- `Logout(ctx, claims)`：`DEL auth:token:{uid}`（幂等）
- `GetUserInfo(ctx, userID)`：按 id 回查（查无 → `ErrUnauthorized`）→ `{roles: [role], name: nickname, avatar: "", introduction: "", permissions: ["*:*:*"]}`（roles 必须非空数组，前端校验）

- `internal/service/teacher.go:74`、`resign.go:40`、`diagnose.go:40`：注释补 `TODO: 接入登录态后改取 c.GetString(model.CtxKeyUsername)`，业务代码不动

### Step 6：中间件与 handler

新建 `internal/router/auth.go`：`Auth(svc) gin.HandlerFunc`——解析 `Authorization: Bearer xxx`（缺头/前缀错 → 401 + Abort）；`VerifyAccessToken` → `ErrUnauthorized` 401、其他错误 500（fail-closed）；通过后 `c.Set(CtxKeyUserID/CtxKeyUsername)`。

新建 `internal/handler/auth.go`：`AuthHandler{svc}`，三方法带 swag 注释：

- `Login` POST：绑定失败/凭据错误/停用 **一律 `c.JSON(200, {code:400, msg:err.Error()})`**（该接口特例，原因见 Context）；成功 `c.JSON(200, LoginResp{code:200, msg:"登录成功", token, expire, passwd_expired:false})`；传 `c.ClientIP()`
- `Logout` POST：context 取 claims → `response.OKMsg(c, "退出成功", nil)`
- `GetInfo` GET：`response.OKMsg(c, "success", info)`；`ErrUnauthorized` → 401

### Step 7：组装

- `internal/router/router.go`：签名 `New(db *sql.DB, rdb *redis.Client, cfg *config.Config)`；路由：

```go
r.POST("/api/v1/login", ah.Login)                    // 公开
authed := r.Group("/api/v1", Auth(svc))
authed.POST("/logout", ah.Logout)
authed.GET("/getinfo", ah.GetInfo)
dxsf := r.Group("/api/v1/dxsf", Auth(svc))           // 业务组挂中间件
diag := r.Group("/api/v1/dxsf/teacher/diagnose", Auth(svc))
```

- `cmd/server/main.go`：MySQL 之后 `ConnectRedis(cfg)`（失败 exit(1)，defer Close）→ `router.New(db, rdb, cfg)`；`@BasePath` 改 `/api/v1`

### Step 8：Swagger

- `internal/handler/` 12 处 `@Router` 注释加 `/dxsf` 前缀（teacher 7、resign 2、diagnose 3）
- `main.go` 加 `@SecurityDefinitions.apikey ApiKeyAuth`（header / Authorization），业务接口注 `@Security ApiKeyAuth`
- `swag init -g cmd/server/main.go -o docs`，生成物随代码提交

### Step 9：测试

- 新建 `internal/service/auth_test.go`：纯函数用例（`service.New(nil, nil, "test-secret...", ttl)`）——签发→解析 claims 正确；篡改 payload 验签失败；过期 token 报错
- 新建 `internal/router/auth_test.go`（httptest + miniredis）：无头 401；坏 token 401；伪造签名 401；签发但未写白名单 401；白名单命中放行；过期后白名单仍在也 401

### Step 10：文档

- `README.md`：快速开始补 Redis（`brew install redis && brew services start redis`）；鉴权章节（三接口 + 401 约定 + admin/admin123 + curl 示例）；现有业务 curl 示例补 Authorization 头
- `CLAUDE.md`：常用命令补 Redis；架构节补鉴权约定（新接口默认进鉴权组）

## 验证清单

```bash
# 前置：Redis 起来、.env 补 JWT_SECRET
brew services start redis && redis-cli ping
go build ./... && go vet ./... && go test ./...
go run ./cmd/server   # 首启自动建 admin_user 表 + 种子 admin/admin123

# 1. 无 token 访问业务 → HTTP 401 + {"code":401,"msg":"登录已过期，请重新登录"}
curl -si 'http://localhost:8080/api/v1/dxsf/teacher/options'

# 2. 登录成功（token 在 body 根）
curl -s -X POST localhost:8080/api/v1/login -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}'

# 3. 错误密码 → HTTP 200 + {"code":400,"msg":"用户名或密码错误"}（前端表单定位路径）
# 4. getinfo 带 token → data.roles 非空
# 5. 业务接口带 token → 200
# 6. 再登录一次拿新 token → 旧 token 访问 401（单设备互踢）
# 7. logout 后再访问 → 401
# 8. redis-cli TTL auth:token:1（TTL 与 JWT 对齐）；DEL 即踢人
# 9. swagger 公开：curl localhost:8080/swagger/index.html
# 10. swag init 后 go build ./...（docs 编译通过）
# 11. 前端 gyz-admin：VUE_APP_BASE_API 指 localhost:8080，登录页 admin/admin123+任意验证码走通登录→列表
```

## 实施结果（2026-08-19 落地）

- 新增文件：`internal/service/auth.go`、`internal/service/password.go`、`internal/router/auth.go`、`internal/handler/auth.go`、`internal/model/auth.go`、`internal/repository/auth.go`、`internal/database/redis.go`、测试 `internal/service/auth_test.go` + `internal/router/auth_test.go`（miniredis）
- 修改文件：`internal/router/router.go`（`New(db, rdb, cfg)` + 鉴权路由组）、`cmd/server/main.go`（Redis 注入 + `JWT_SECRET` 空值退出 + `@BasePath /api/v1` + securityDefinitions）、`internal/config/config.go`、`.env.example`、`internal/database/{database.go,schema.sql}`、三处业务 service 补 TODO 注释、README/CLAUDE.md 同步
- 测试：service 6 项（签发/篡改/过期/错签/key 格式/bcrypt）+ router 9 项（无头/坏头/乱串/伪造签名/未白名单/旧设备/放行注入/过期白名单仍在/logout 闭环）全绿
- 实施期修正两处：中间件 401 分支不再读 nil claims（验签失败时 `VerifyAccessToken` 返回 nil claims，原实现 panic）；`getinfo` handler `c.GetInt64`（`c.GetInt` 截断 int64）
- 已知前端行为：业务接口 HTTP 401 只弹 toast 不触发重登弹窗（gyz-admin 拦截器 401 弹窗分支仅在 HTTP 200 + body 401 命中）；如需弹窗需前端改拦截器或后端切恒 200（均本期不动）
- 端到端验证（2026-08-19，redis@6.2.18 本机实测）：无 token 401 统一文案 ✅；登录 token 在根/HTTP 恒 200/错误文案含「密码」✅；getinfo roles 非空 ✅；业务接口带 token 200 ✅；单设备互踢旧 token 401 ✅；logout 即时失效且幂等 ✅；TTL=86400 与 JWT 24h 对齐 ✅；swagger 公开 ✅；Redis 停机时校验 500 fail-closed、登录失败、恢复后自愈 ✅
- 踩坑记录：schema.sql 注释 `--（` 中文括号紧跟 `--` 不带空格，MySQL 1064 报错（`-- ` 后必须有空格），已修；本机 redis 为版本化 formula `redis@6.2`（keg-only，`/usr/local/opt/redis@6.2/bin/`，不进 PATH）
