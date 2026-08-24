# PLAN-live：直播小鹅通登录链接透传（/guyuzhoudb）

> 立项：2026-08-24。mofang 前端直播间中转页（`/live-room/:id`）需要小鹅通登录鉴权链接做账号打通；
> 本服务本地复刻远程网关的 `GET /guyuzhoudb/live/get_login_url`，纯透传小鹅通开放平台接口，
> 供 mofang 前端在本地联调（前端开关 `LOCAL_GO_BASE`，见 mofang `src/services/live/`）。

## Context

mofang 中转页流程：拼 PC 直播间页 `redirect_uri` → 调本接口取 `login_url` → 客户端新开 webview 打开
`login_url`（在小鹅通域名注入登录态）→ 小鹅通自动 302 到 `redirect_uri` 进入直播间。此前前端纯 mock
（`USE_MOCK=true`），本接口为本地 Go 复刻版：**不持有小鹅通 secret，不代取 access_token**，
凭证由前端从远程网关 `get_access_token` 接口取得后透传（`ensureXeAccessToken()`）。

## 上游契约（小鹅通开放平台 xe.login.url/1.0.0）

- `POST https://api.xiaoe-tech.com/xe.login.url/1.0.0`，频率限制 10s/10000 次（远超本服务量级，不做本地限流）
- 请求体：`{"access_token":"...","user_id":"...","data":{"login_type":1,"redirect_uri":"https://..."}}`
  - `login_type`：1=PC 2=H5 3=App；`redirect_uri` 可选（不传由小鹅通默认跳转）
- 响应：`{"code":0,"msg":"success","data":{"login_url":"https://...","permission_denied_url":""}}`
  - `code=0` 成功；**`login_url` 有效期仅 1 分钟**（前端即取即跳，后端禁止任何缓存）
  - `permission_denied_url`：店铺无 SDK 权益包时的跳转链接（透传给前端备用）

## 决策

| # | 决策 | 理由 |
| --- | --- | --- |
| 1 | **公开不挂 Auth**（全服务第三个例外：login/swagger 之后） | ① mofang 是 C 端另一 token 体系（App 桥 `jsGetToken` 注入 dxzg JWT，本服务无密钥验不了，挂了必 401）；② `/guyuzhoudb` 前缀独立于 `/api/v1` 鉴权域（对齐远程网关路径形态）；③ 接口凭证即入参 `access_token`，有效性由小鹅通侧校验。CLAUDE.md「新增业务接口默认进鉴权组」约束的是 gyz-admin 管理端接口，C 端透传接口不适用 |
| 2 | 标准库 `net/http` 包级单例 client，`Timeout: 10s` | 本项目首个出站 HTTP 调用，不引第三方库；10s 覆盖建连+读写，login_url 1 分钟时效下超时再长无意义；单例复用连接池 |
| 3 | `service.New` 增第 5 参 `xiaoeAPIBase` | 延续「显式收参、service 不依赖 config 包」惯例（同 jwtSecret）；全工程调用点仅 router.go 与 auth_test.go 两处 |
| 4 | 上游域名走配置 `XIAOE_API_BASE`（默认 `https://api.xiaoe-tech.com`） | 环境可替换（如后续测试代理），默认值保证零配置可用 |
| 5 | 哨兵错误 → 502：`ErrXeUpstream`（code!=0）/ `ErrXeEmptyLoginURL`（code=0 但空）；网络/解码/非 200 → 非哨兵 err，handler default 分支 502 通用文案 | 上游业务失败最常见是 access_token 无效/过期，上游 code/msg 文案不可控且防探测，只进 slog.Warn 不透出（联调排障看日志）；错误语义是「上游失败」非本服务错误，故 502 而非 500 |
| 6 | handler 只做形状校验：access_token/user_id 非空+长度上限（512/64）、login_type ∈{1,2,3}、redirect_uri 非空须 http(s):// 且 ≤2048 | 凭证语义校验交给上游（本服务无判定能力）；长度上限挡滥用；透传字符串不做 HTML 净化——json.Marshal 天然转义、无存储回显面，与 diagnose 富文本 XSS 场景不同 |
| 7 | access_token 不落日志（只落 user_id） | 凭证不进日志文件/控制台 |
| 8 | wire 类型（xeLoginReq/xeLoginResp）定义在 service/live.go；model 只放 Swagger 文档类型 XeLoginURLResp | 请求/响应结构仅本域使用不跨层，对齐「model/swagger.go 运行时不使用」惯例 |

## 实施记录

- `internal/config/config.go` + `.env.example`：`XIAOE_API_BASE`
- `internal/service/service.go`：Service 加 `xiaoeAPIBase` 字段，New 加参（auth_test.go 同步补 `""`）
- `internal/service/live.go`：`GetXeLoginURL`（POST JSON 转发、io.LimitReader 1MB 防超大响应、错误分流见上表）
- `internal/handler/live.go`：`GetXeLoginURL`（query 蛇形取参、400/502 映射、Swagger 注释）
- `internal/model/swagger.go`：`XeLoginURLResp` 文档类型
- `internal/router/router.go`：`r.GET("/guyuzhoudb/live/get_login_url", lh.GetXeLoginURL)`（login 之后，公开区）
- `internal/service/live_test.go`：httptest 模拟上游，6 个 case（成功四字段全透传/业务错/空 login_url/500/坏 JSON/网络错）
- README：鉴权例外句 + 直播接口章节；docs 由 `swag init` 重新生成

## 已知瑕疵

- **Swagger 路径显示错误**：全局 `@BasePath /api/v1`（main.go），本接口 `@Router /guyuzhoudb/live/get_login_url`
  在 Swagger UI 显示为 `/api/v1/guyuzhoudb/...` 且 Try it out 会 404。不改全局 BasePath（会破坏既有全部注解
  的显示），实际路径以 README/curl 为准
- mofang 对 HTTP 4xx/5xx 只弹通用「未知错误」不读响应 body（fetch 封装的既有行为），本接口的中文失败
  文案需看 Network 面板/curl

## 后续（本期不做）

- user_id 真实来源：mofang 前端暂调试写死（`DEBUG_XE_USER_ID`，TODO 标注），待用户体系明确后接入
- `get_access_token`（含小鹅通 HS256 签名、secret 托管）是否迁入本服务：另立项
