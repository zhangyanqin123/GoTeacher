# handicap-service

Go 学习项目：老师管理（chatSys）接口 + 老师离职转移接口 + 诊股记录接口。数据从 MySQL 查询返回（非硬编码），首次启动自动建表并写入种子数据。业务接口统一 Bearer token 鉴权（见下文「鉴权」）。

## 技术栈

| 组件 | 选型 | 说明 |
| --- | --- | --- |
| 语言 | Go 1.24+ | |
| Web 框架 | [Gin](https://github.com/gin-gonic/gin) | 路由、中间件、JSON 渲染 |
| 数据访问 | database/sql + [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) | Go 官方标准库，无 ORM |
| 数据库 | MySQL 8 | Homebrew 安装 |
| 缓存 | [go-redis/v9](https://github.com/redis/go-redis) | 鉴权白名单（token 主动失效） |
| 鉴权 | [golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt) + bcrypt | JWT HS256 签发 + Redis 白名单 |
| 环境变量 | [godotenv](https://github.com/joho/godotenv) | 支持 `.env` 文件 |

## 鉴权（JWT + Redis 白名单）

设计决策详见 `PLAN-auth.md`。除 `POST /api/v1/login` 与 `/swagger/**` 外，全部接口需带 `Authorization: Bearer {token}`；未授权统一 HTTP 401 + `{"code":401,"msg":"登录已过期，请重新登录"}`。

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | POST | `/api/v1/login` | 登录签发 token（初始账号 admin/admin123） |
| 2 | POST | `/api/v1/logout` | 退出登录（token 立即失效，幂等） |
| 3 | GET | `/api/v1/getinfo` | 当前用户信息（roles/name/permissions） |

- **login 响应是全局约定的特例**：失败也返回 HTTP 200 + `code:400`（gyz-admin 登录页对 reject 值调 `error.includes('密码')`，HTTP 4xx 会使其抛 TypeError）；`token`/`expire`/`passwd_expired` 在 body 根而非 `data` 内（前端 store 从响应根解构）。前端多传的 `phone_code`/`uuid` 静默忽略
- **单设备登录**：Redis key `auth:token:{user_id}` 存当前有效 token 的 jti（TTL=JWT 有效期），重新登录覆盖即互踢旧设备；`DEL` 该 key 即主动踢人
- token 有效期 `JWT_TTL_HOURS`（默认 24h）；`JWT_SECRET` 必填，空值启动退出

```bash
# 1. 登录拿 token
TOKEN=$(curl -s -X POST 'http://localhost:8080/api/v1/login' \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')

# 2. 业务接口带 token
curl -s 'http://localhost:8080/api/v1/dxsf/teacher/options' -H "Authorization: Bearer $TOKEN"

# 3. 退出（token 立即失效）
curl -s -X POST 'http://localhost:8080/api/v1/logout' -H "Authorization: Bearer $TOKEN"
```

## 老师管理接口（chatSys）

对应前端 `gyz-admin/src/api/dxData/chatSys/teacher.js`（接口文档即该文件，原本全为前端 mock），路径/参数/响应结构与 mock 严格一致，前端删掉 mock 段启用注释的 `request` 调用即可直接联调。

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | POST | `/api/v1/dxsf/teacher/list` | 老师列表（分页 + 多条件筛选） |
| 2 | GET | `/api/v1/dxsf/teacher/options` | 老师下拉选项（含停用，带部门名） |
| 3 | POST | `/api/v1/dxsf/teacher/edit` | 编辑老师（title / level / avatar / sign） |
| 4 | GET | `/api/v1/dxsf/teacher/bind/salesman/list` | 老师绑定业务员列表（详情弹窗） |
| 5 | POST | `/api/v1/dxsf/teacher/bind/salesman` | 绑定业务员（追加语义，重复绑定幂等） |
| 6 | GET | `/api/v1/dxsf/teacher/bind/salesman/users` | 全量已绑定业务员 userId（绑定弹窗人员树过滤用） |
| 7 | POST | `/api/v1/dxsf/teacher/resign/list` | 离职转移记录列表（分页 + 多条件筛选，条件走 body） |
| 8 | POST | `/api/v1/dxsf/teacher/resign/add` | 新增离职转移 |

### 列表筛选参数

| 参数 | 匹配 | 说明 |
| --- | --- | --- |
| dept_id / id / qualification / bind_sales_count / status | 精确 | 数值字段传 null 表示未填；status 数值 -1 全部 / 1 启用 / 0 停用；qualification 传中文 |
| account / nickname / name / title / update_by | 模糊 | LIKE %val% |
| update_begin_time + update_end_time | 范围 | yyyy-MM-dd，按 updated_at 闭区间，成对生效 |
| page_index / page_size | 分页 | 默认 1 / 10（绑定列表默认 page_size=5），page_size 上限 100 |

### 响应约定（与前端 mock 对齐的兼容点）

- 成功 msg：查询类 `success`，写操作 `编辑成功` / `绑定成功`（非 `ok`）
- 失败 msg 一律中文可展示文案（前端拦截器 error 分支直接弹 `error.response.data.msg`），HTTP 状态码保留 4xx/5xx 语义；哨兵错误文本即文案，handler 透传 `err.Error()`
- `status` 输出字符串 `"1"`/`"0"`（前端 el-switch 直接比较）
- `qualification` 存/传中文 `已认证`/`未认证`
- 时间字段输出 `YYYY-MM-DD HH:mm:ss`（`model.DateTimeString` 在扫描点格式化，避免 RFC3339 带 T）
- `level` 原样存取（列名 rating）：种子为 1-5 星，编辑后为 0 无 / 3 初级 / 5 高级
- 分页返回 `data.list` / `data.count`；老师不存在时绑定列表返回空 list + count 0（不报错）；绑定业务员列表额外回显 `pageIndex`/`pageSize`（驼峰，该接口前端返回结构约定，snake_case 约定的例外）
- 绑定业务员列表行：`username` 取 `sales_user.nickname`（桩表无独立账号列，与离职转移快照同源），同表 `nickname` 一并输出

### curl 示例

```bash
# 列表（POST body：模糊 + 精确 + 分页，数值字段传 null 表示未填）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/list' \
  -H 'Content-Type: application/json' \
  -d '{"status":1,"name":"张","dept_id":null,"page_index":1,"page_size":10}'

# 下拉选项
curl -s 'http://localhost:8080/api/v1/dxsf/teacher/options'

# 编辑老师（level: 0 无 / 3 初级 / 5 高级）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/edit' \
  -H 'Content-Type: application/json' \
  -d '{"id":1,"title":"首席投顾","level":5,"avatar":"","sign":"签名"}'

# 老师绑定业务员列表（data 回显 pageIndex/pageSize，为该接口前端约定的驼峰例外）
curl -s 'http://localhost:8080/api/v1/dxsf/teacher/bind/salesman/list?id=1&page_index=1&page_size=5'
# → {"code":200,"msg":"success","data":{"list":[{"id":179,"username":"zzy5","nickname":"zzy5","dept_name":"市场部一部","bind_time":"2026-08-19 14:37:03"}],"count":1,"pageIndex":1,"pageSize":5}}

# 绑定业务员（追加语义；user_ids 为业务员表 id）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/bind/salesman' \
  -H 'Content-Type: application/json' \
  -d '{"id":1,"user_ids":[1,2,3]}'

# 全量已绑定业务员 userId（人员树过滤，不分页）
curl -s 'http://localhost:8080/api/v1/dxsf/teacher/bind/salesman/users'
# → {"code":200,"msg":"success","data":[1,2,3,...]}
```

### 表设计

| 表 | 说明 |
| --- | --- |
| `teacher` | 老师；`qualification` 存中文、`status` TINYINT（接口输出字符串）、`rating` 存原值 |
| `sales_user` | 业务员桩表（真实系统为 admin 的 sys_user），种子 = mock 的 salesPool 25 人 |
| `teacher_sales` | 绑定关系；`uk_teacher_user` 唯一约束；`bind_sales_count` 不落库，由子查询统计（单一事实来源） |

> 绑定弹窗人员树走 admin 真实接口（`/api/v1/dept` + `/api/v1/sys-user`），userId 为 admin 系 sys_user 的真实 id，
> 与种子的 mock id（1-25）不是一套 ID 空间。绑定后详情列表 INNER JOIN `sales_user`，ID 对不上时列表为空（count 正常）。
> 需先执行 `TOKEN=<admin登录token> ./scripts/sync_sales_user.sh` 从 admin 测试服同步市场部人员（与前端人员树同范围），脚本会 UPSERT 并校验孤儿绑定数。

种子数据照抄 mock（12 老师 / 25 业务员 / 143 条绑定），仅在表为空时写入，重灌方式：`TRUNCATE TABLE teacher; TRUNCATE TABLE sales_user; TRUNCATE TABLE teacher_sales;` 后重启。

设计决策与实施记录见 [PLAN-teacher.md](PLAN-teacher.md)。

## 老师离职转移接口（chatSys）

对应前端 `gyz-admin/src/api/dxData/chatSys/resign.js`（同上，原为前端 mock）。姓名/部门为冗余快照，后端从 `teacher` 表回查（前端传的冗余字段忽略）；业务员快照取原老师全部绑定业务员（多个逗号分隔）。

### 列表筛选参数

| 参数 | 匹配 | 说明 |
| --- | --- | --- |
| dept_id / original_teacher_id / replace_teacher_id | 精确 | dept_id 匹配原老师部门 |
| salesman_name | 模糊 | LIKE %val% |
| transfer_begin_time + transfer_end_time | 范围 | yyyy-MM-dd，按 transfer_time 闭区间，成对生效 |
| page_index / page_size | 分页 | 默认 1 / 10，page_size 上限 100，id 倒序（最新在前） |

### 新增请求体

```json
{ "original_teacher_id": 4, "replace_teacher_id": 1, "transfer_content": "首席投顾" }
```

- 原/接替老师不能相同（400），老师不存在 404
- 原老师无绑定业务员时 400（无可转移内容，不再落 group_count=0 的空记录）
- `transfer_content` 为转移内容自由文本（2026-08-19 由原 `remark` 改名），≤200 字符
- `group_count` 由后端计算（=原老师绑定业务员数，一个绑定业务员对应一个客户群），请求体不接收该字段；旧客户端多传的 `group_count`/`friend_count`/`remark` 会被 gin 静默忽略
- `operator` 无登录态固定 `admin`；`operate_ip` 取 `c.ClientIP()`；`transfer_time` 库端 NOW()

### 响应约定

- 列表 `data.list` / `data.count`；时间字段 `YYYY-MM-DD HH:mm:ss`
- 新增成功 `{code:200, msg:"转移成功"}`

### curl 示例

```bash
# 列表（部门 + 时间范围；筛选条件走 JSON body，数值字段传空串/null 表示未填）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/resign/list' \
  -H 'Content-Type: application/json' \
  -d '{"dept_id":3,"transfer_begin_time":"2025-08-01","transfer_end_time":"2025-08-31","page_index":1,"page_size":10}'

# 新增离职转移
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/resign/add' \
  -H 'Content-Type: application/json' \
  -d '{"original_teacher_id":4,"replace_teacher_id":1,"transfer_content":"首席投顾"}'
```

### 表设计

| 表 | 说明 |
| --- | --- |
| `teacher_resign` | 离职转移记录；`salesman_name/dept` 存原老师全部绑定业务员逗号串；`original_teacher_dept_id` 供 deptId 筛选；`group_count` = 原老师绑定业务员数（后端计算入库）；`transfer_content` 为转移内容自由文本（原 `remark` 改名，存量库由启动迁移 CHANGE 幂等改名）；`friend_count` 列已移除（存量库由启动迁移幂等 DROP） |

种子数据照抄 mock 的 6 条（id 101-106），仅在表为空时写入，重灌方式：`TRUNCATE TABLE teacher_resign;` 后重启。

设计决策与实施记录见 [PLAN-resign.md](PLAN-resign.md)。

## 诊股记录接口（diagnoseSys）

对应前端 `gyz-admin/src/api/dxData/diagnoseSys/diagnose.js`（同上，原为前端 mock），调用处 `diagnoseQuery.vue` 零改动。昵称/姓名/股票名/老师为冗余快照；审核流程日志落库（`diagnose_audit_log`），驳回后重新提审保留完整审核历史（mock 为读取时推导，会丢首次驳回记录）。

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | GET | `/api/v1/dxsf/diagnose/list` | 诊股记录列表（分页 + 12 条件筛选） |
| 2 | GET | `/api/v1/dxsf/diagnose/detail` | 诊股详情（主表全字段 + 审核流程记录） |
| 3 | POST | `/api/v1/dxsf/diagnose/submitReport` | 提交诊股报告（首次编写 / 重新提审 → 状态 2） |
| 4 | POST | `/api/v1/dxsf/diagnose/audit` | 审核诊股报告（通过 / 驳回） |

### 状态机

```
[1 待诊股] ──submitReport──► [2 待专业审核] ──professional pass──► [4 待合规审核] ──compliance pass──► [6 终态]
                                │ professional reject                          │ compliance reject
                                ▼                                             ▼
                             [3 专业审核不通过] ──submitReport──► 2         [5 合规审核不通过] ──submitReport──► 2
```

- submitReport 仅允许状态 1/3/5（在途重提使审核对象错位、审计链断裂，2/4/6 严格拒绝；6 为终态）
- professional 审核仅允许状态 2；compliance 审核仅允许状态 4（顺序错位拒绝）

### 列表筛选参数

| 参数 | 匹配 | 说明 |
| --- | --- | --- |
| id / buy_price / buy_num / status | 精确 | 数值格式错误 400；status 白名单 1-6；传 `0` 亦为有效过滤值（指针区分未传） |
| user_nick_name / user_name / stock_code / stock_name / teacher_name | 模糊 | LIKE %val% |
| submit_begin_time + submit_end_time | 范围 | yyyy-MM-dd，按 submit_time 闭区间，成对生效 |
| report_begin_time + report_end_time | 范围 | yyyy-MM-dd，按 report_submit_time 闭区间（未提审的天然排除） |
| page_index / page_size | 分页 | 默认 1 / 10，page_size 上限 100，id 倒序（最新在前） |

### 请求体

```json
// POST /diagnose/submitReport：report_content 为富文本 HTML，去标签全空白视为未填写（400）
{ "id": 1, "report_content": "<p>诊股结论：持股待涨</p>" }

// POST /diagnose/audit：audit_type 白名单 professional / compliance，result 白名单 pass / reject；
// reject 时 reject_reason 必填（富文本，同样判空）
{ "id": 1, "audit_type": "professional", "result": "reject", "reject_reason": "<p>结论依据不足</p>" }
```

### 响应约定

- 成功 msg：`success` / `提交成功` / `审核通过` / `已驳回`（mock 约定）
- `buy_price` DECIMAL 扫 float64 输出 `1680.5`；时间字段 `YYYY-MM-DD HH:mm:ss`，`report_submit_time` 未提审输出空串
- detail = 主表全字段 + `audit_logs` 数组（按 id 正序 = 时间序）；记录不存在 404（较 mock 的 `data:null` 收紧）
- 并发守卫：写前查询区分 404/400，事务内条件 UPDATE（`WHERE status IN (1,3,5)` / `WHERE status = ?`），`RowsAffected == 0` 回滚返回 400（纯 SELECT-then-UPDATE 有 TOCTOU）
- 审核 operator 无登录态固定 `专业审核员` / `合规审核员`；`log_time` 库端 NOW()

### curl 示例

```bash
# 列表（状态 + 股票名模糊）
curl -s 'http://localhost:8080/api/v1/dxsf/diagnose/list?status=2&stockName=%E4%BA%94%E7%B2%AE&page_index=1&page_size=10'

# 详情（含审核流程日志，按时间正序）
curl -s 'http://localhost:8080/api/v1/dxsf/diagnose/detail?id=5'

# 提交诊股报告（状态 1/3/5 → 2）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/diagnose/submitReport' \
  -H 'Content-Type: application/json' \
  -d '{"id":1,"report_content":"<p>诊股结论：持股待涨</p>"}'

# 专业审核通过（2 → 4）；驳回把 result 换成 reject 并带 reject_reason
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/diagnose/audit' \
  -H 'Content-Type: application/json' \
  -d '{"id":1,"audit_type":"professional","result":"pass"}'
```

### 表设计

| 表 | 说明 |
| --- | --- |
| `diagnose` | 诊股记录主表；`buy_price DECIMAL(10,2)`、`report_content` TEXT NOT NULL（避免 NULL 扫 string）、`report_submit_time` NULL=未提交；只建 status / submit_time / report_submit_time 索引（模糊列前导通配 LIKE 打不进 B-tree） |
| `diagnose_audit_log` | 审核流程日志（落库为准）；`log_type` / `result` 存中文展示串（对齐 `teacher.qualification` 先例）；`remark` 存驳回原因（HTML）；`diagnose_id` 索引 |

种子数据照抄 mock 的 6 条主表（状态 1-6 各一）+ 17 条日志，仅在表为空时写入，重灌方式：`TRUNCATE TABLE diagnose; TRUNCATE TABLE diagnose_audit_log;` 后重启。

设计决策与实施记录见 [PLAN-diagnose.md](PLAN-diagnose.md)。

## 目录结构（对应 Spring Boot 分层）

```
.
├── cmd/server/main.go        # 入口：加载配置 → 连库 → 建表/种子 → 路由 → 启动
└── internal/
    ├── config/               # 配置：环境变量 + 默认值，组装 MySQL DSN
    ├── database/             # 连接池 + 迁移（schema.sql）+ 种子（teacher_seed.sql / resign_seed.sql / diagnose_seed.sql）
    ├── model/                # Entity/DTO：Teacher、Resign、Diagnose、DateTimeString、StringSlice
    ├── repository/           # DAO/Mapper：teacher・resign・diagnose CRUD
    ├── service/              # Service：参数校验、默认值、白名单、诊股状态机
    ├── handler/              # Controller：参数绑定、错误 → HTTP 状态码
    ├── response/             # 统一响应 {code, msg, data}
    └── router/               # 路由与依赖组装
```

依赖方向（只允许自上而下）：`handler → service → repository → model`

## 快速开始

### 1. 环境要求

- Go 1.24+
- MySQL 8（未安装先执行：`brew install mysql && brew services start mysql`）
- Redis（鉴权白名单依赖。本机用版本化 formula：`brew install redis@6.2`，keg-only 不进 PATH，启动 `/usr/local/opt/redis@6.2/bin/redis-server --daemonize yes`）

### 2. 初始化数据库

```bash
mysql -u root -e "CREATE DATABASE IF NOT EXISTS handicap_db CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;"
```

### 3. 配置环境变量

```bash
cp .env.example .env   # 按需修改 DB_PASSWORD；JWT_SECRET 必填（openssl rand -hex 32 生成）
```

### 4. 启动

```bash
go mod tidy
go run ./cmd/server
```

首次启动自动执行：建表（幂等，含 admin_user）→ 表为空则插入种子数据（admin_user 种子：admin/admin123）。之后启动直接可用。

### 5. 验证

```bash
# 老师列表（POST body，分页返回 data.list / data.count）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/list' \
  -H 'Content-Type: application/json' \
  -d '{"page_index":1,"page_size":10}'

# 下拉选项
curl -s 'http://localhost:8080/api/v1/dxsf/teacher/options'
```

## 常见问题

| 现象 | 原因与解决 |
| --- | --- |
| 连接报 `error 1045` | 密码错误，检查 `.env` 的 `DB_PASSWORD` |
| 连接报 `error 2002` | MySQL 未启动：`brew services start mysql` |
| 认证失败（caching_sha2_password） | 兜底改回旧插件：`ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY '<密码>';` |

## 进阶方向（本期未实现）

- 接入 sqlx 自动映射结构体
- 单元测试（handler / service / repository）
- 优雅停机（signal + http.Server.Shutdown）