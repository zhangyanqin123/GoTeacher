# handicap-service

Go 学习项目：老师管理（chatSys）接口 + 老师离职转移接口 + 诊股记录接口。数据从 MySQL 查询返回（非硬编码），首次启动自动建表并写入种子数据。业务接口统一 Bearer token 鉴权（见下文「鉴权」）。

配套管理台前端在与本仓库同级的 `GoProject-web/`（React + Vite + TypeScript + antd 5，承载本服务全部接口，见 [plans/PLAN-web.md](plans/PLAN-web.md) 与下文「前端管理台」）。

## 技术栈

| 组件 | 选型 | 说明 |
| --- | --- | --- |
| 语言 | Go 1.24+ | |
| Web 框架 | [Gin](https://github.com/gin-gonic/gin) | 路由、中间件、JSON 渲染 |
| 数据访问 | database/sql + [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) | Go 官方标准库，无 ORM |
| 数据库 | MySQL 8 | Docker 容器（docker-compose 统一编排 MySQL/Redis/RabbitMQ，见 plans/PLAN-docker.md） |
| 缓存 | [go-redis/v9](https://github.com/redis/go-redis) | 鉴权白名单（token 主动失效） |
| 消息队列 | [RabbitMQ](https://www.rabbitmq.com/) + [amqp091-go](https://github.com/rabbitmq/amqp091-go) | 订单事件 order.created fanout 广播（订单 Demo，见 plans/PLAN-order.md） |
| 鉴权 | [golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt) + bcrypt | JWT HS256 签发 + Redis 白名单 |
| 环境变量 | [godotenv](https://github.com/joho/godotenv) | 支持 `.env` 文件 |

## 鉴权（JWT + Redis 白名单）

设计决策详见 `plans/PLAN-auth.md`。除 `POST /api/v1/login`、`/swagger/**` 与 `/guyuzhoudb/**`（直播小鹅通透传，凭证由上游校验，见下文「直播接口」与 [plans/PLAN-live.md](plans/PLAN-live.md)）外，全部接口需带 `Authorization: Bearer {token}`；未授权统一 HTTP 401 + `{"code":401,"msg":"登录已过期，请重新登录"}`。

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

设计决策与实施记录见 [plans/PLAN-teacher.md](plans/PLAN-teacher.md)。

## 老师离职转移接口（chatSys）

对应前端 `gyz-admin/src/api/dxData/chatSys/resign.js`（同上，原为前端 mock）。姓名/部门为冗余快照，后端从 `teacher` 表回查（前端传的冗余字段忽略）；业务员快照取原老师全部绑定业务员（多个逗号分隔）。

### 列表筛选参数

| 参数 | 匹配 | 说明 |
| --- | --- | --- |
| dept_id | 精确 | 匹配原老师部门快照列 original_teacher_dept_id，传空串/null 表示未填 |
| original_teacher / replace_teacher / salesman | 模糊 | 姓名类，分别匹配 original_teacher_name / replace_teacher_name / salesman_name 快照列，LIKE %val% |
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
- `operator` 无登录态固定 `admin`；`operate_ip` 取 `c.ClientIP()` 仅落库审计（列表接口不返回，2026-08-20 移除展示，DB 列与数据保留）；`transfer_time` 库端 NOW()

### 响应约定

- 列表 `data.list` / `data.count`；时间字段 `YYYY-MM-DD HH:mm:ss`
- 新增成功 `{code:200, msg:"转移成功"}`

### curl 示例

```bash
# 列表（部门 + 姓名模糊 + 时间范围；筛选条件走 JSON body，空串/null 表示未填）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/resign/list' \
  -H 'Content-Type: application/json' \
  -d '{"dept_id":3,"original_teacher":"张三","transfer_begin_time":"2025-08-01","transfer_end_time":"2025-08-31","page_index":1,"page_size":10}'

# 新增离职转移
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/resign/add' \
  -H 'Content-Type: application/json' \
  -d '{"original_teacher_id":4,"replace_teacher_id":1,"transfer_content":"首席投顾"}'
```

### 表设计

| 表 | 说明 |
| --- | --- |
| `teacher_resign` | 离职转移记录；`salesman_name/dept` 存原老师全部绑定业务员逗号串；`original_teacher_dept_id` 供列表 dept_id 筛选；`group_count` = 原老师绑定业务员数（后端计算入库）；`transfer_content` 为转移内容自由文本（原 `remark` 改名，存量库由启动迁移 CHANGE 幂等改名）；`friend_count` 列已移除（存量库由启动迁移幂等 DROP） |

种子数据照抄 mock 的 6 条（id 101-106），仅在表为空时写入，重灌方式：`TRUNCATE TABLE teacher_resign;` 后重启。

设计决策与实施记录见 [plans/PLAN-resign.md](plans/PLAN-resign.md)。

## 诊股记录接口（diagnoseSys）

对应前端 `gyz-admin/src/api/dxData/diagnoseSys/diagnose.js`（同上，原为前端 mock），调用处 `diagnoseQuery.vue` 零改动。昵称/姓名/股票名/老师为冗余快照；审核流程日志落库（`diagnose_audit_log`），驳回后重新提审保留完整审核历史（mock 为读取时推导，会丢首次驳回记录）。

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | POST | `/api/v1/dxsf/teacher/diagnose/list` | 诊股记录列表（分页 + 12 条件筛选，条件走 JSON body） |
| 2 | GET | `/api/v1/dxsf/teacher/diagnose/detail` | 诊股详情（主表全字段 + 审核流程记录） |
| 3 | POST | `/api/v1/dxsf/teacher/diagnose/submit/report` | 提交诊股报告（首次编写 / 重新提审 → 状态 2） |
| 4 | POST | `/api/v1/dxsf/teacher/diagnose/audit` | 审核诊股报告（status 直传目标状态 3/4/5/6，前端换算） |

### 状态机

```
[1 待诊股] ──submitReport──► [2 待专业审核] ──professional pass──► [4 待合规审核] ──compliance pass──► [6 终态]
                                │ professional reject                          │ compliance reject
                                ▼                                             ▼
                             [3 专业审核不通过] ──submitReport──► 2         [5 合规审核不通过] ──submitReport──► 2
```

- submitReport 仅允许状态 1/3/5（在途重提使审核对象错位、审计链断裂，2/4/6 严格拒绝；6 为终态）
- professional 审核仅允许状态 2；compliance 审核仅允许状态 4（顺序错位拒绝）

### 列表筛选参数（POST JSON body）

| 参数 | 匹配 | 说明 |
| --- | --- | --- |
| id / buy_price / buy_num / status | 精确 | 只接受 JSON 数值，传数值字符串/空串一律 400（前端负责把 el-input 产出转数字再发）；status 白名单 1-6；传 `0` 亦为有效过滤值（null/缺省区分未传） |
| user_nick_name / user_name / stock_code / stock_name / teacher_name | 模糊 | LIKE %val% |
| submit_begin_time + submit_end_time | 范围 | yyyy-MM-dd，按 submit_time 闭区间，成对生效 |
| report_begin_time + report_end_time | 范围 | yyyy-MM-dd，按 report_submit_time 闭区间（未提审的天然排除） |
| page_index / page_size | 分页 | 默认 1 / 10，page_size 上限 100，id 倒序（最新在前） |

### 请求体

```json
// POST /diagnose/submit/report：report_content 为富文本 HTML，去标签全空白视为未填写（400）
{ "id": 1, "report_content": "<p>诊股结论：持股待涨</p>" }

// POST /diagnose/audit：status 为前端按状态机换算的目标状态，后端白名单校验后直接落库
// （2026-08-21 由 audit_type+result 改直传）：2→3 专业驳回 / 2→4 专业通过 / 4→5 合规驳回 / 4→6 合规通过；
// status 为 3/5（驳回）时 reject_reason 必填（富文本，同样判空）
{ "id": 1, "status": 3, "reject_reason": "<p>结论依据不足</p>" }
```

### 响应约定

- 成功 msg：`success` / `提交成功` / `审核通过` / `已驳回`（mock 约定）
- `buy_price` DECIMAL 扫 float64 输出 `1680.5`；时间字段 `YYYY-MM-DD HH:mm:ss`，`report_submit_time` 未提审输出空串
- detail = 主表全字段 + `audit_logs` 数组（按 id 正序 = 时间序）；记录不存在 404（较 mock 的 `data:null` 收紧）
- 并发守卫：写前查询区分 404/400，事务内条件 UPDATE（`WHERE status IN (1,3,5)` / `WHERE status = ?`），`RowsAffected == 0` 回滚返回 400（纯 SELECT-then-UPDATE 有 TOCTOU）
- 审核 operator 无登录态固定 `专业审核员` / `合规审核员`；`log_time` 库端 NOW()

### curl 示例

```bash
# 列表（状态 + 股票名模糊，筛选条件走 JSON body）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/diagnose/list' \
  -H 'Content-Type: application/json' \
  -d '{"status":2,"stock_name":"五粮","page_index":1,"page_size":10}'

# 详情（含审核流程日志，按时间正序）
curl -s 'http://localhost:8080/api/v1/dxsf/teacher/diagnose/detail?id=5'

# 提交诊股报告（状态 1/3/5 → 2）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/diagnose/submit/report' \
  -H 'Content-Type: application/json' \
  -d '{"id":1,"report_content":"<p>诊股结论：持股待涨</p>"}'

# 专业审核通过（2 → 4，status 由前端换算直传）；驳回传 status:3 并带 reject_reason
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/diagnose/audit' \
  -H 'Content-Type: application/json' \
  -d '{"id":1,"status":4}'
```

### 表设计

| 表 | 说明 |
| --- | --- |
| `diagnose` | 诊股记录主表；`buy_price DECIMAL(10,2)`、`report_content` TEXT NOT NULL（避免 NULL 扫 string）、`report_submit_time` NULL=未提交；只建 status / submit_time / report_submit_time 索引（模糊列前导通配 LIKE 打不进 B-tree） |
| `diagnose_audit_log` | 审核流程日志（落库为准）；`log_type` / `result` 存中文展示串（对齐 `teacher.qualification` 先例）；`remark` 存驳回原因（HTML）；`diagnose_id` 索引 |

种子数据照抄 mock 的 6 条主表（状态 1-6 各一）+ 17 条日志，仅在表为空时写入，重灌方式：`TRUNCATE TABLE diagnose; TRUNCATE TABLE diagnose_audit_log;` 后重启。

设计决策与实施记录见 [plans/PLAN-diagnose.md](plans/PLAN-diagnose.md)。

## 用户管理接口（登录账号）

管理 `admin_user` 表登录账号（admin/admin123 种子之外的手工开户入口），用户信息仅用户名+密码。前端 gyz-admin 右上角头像下拉菜单「用户管理」进入（页面 `views/dxData/chatSys/userManage.vue`，静态 hidden 路由 `/userManage/index`）。`admin_user` 是系统账号域，路由前缀 `/api/v1/admin` 不挂 `/dxsf`。

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | POST | `/api/v1/admin/user/list` | 用户列表（分页 + username 模糊；密码永不返回） |
| 2 | POST | `/api/v1/admin/user/add` | 新增用户（username 6-64 位 password） |
| 3 | POST | `/api/v1/admin/user/edit` | 编辑用户（password 留空=不修改） |
| 4 | POST | `/api/v1/admin/user/delete` | 删除用户（立即踢下线；不能删自己） |

### 响应约定

- 成功 msg：`success` / `新增成功` / `编辑成功` / `删除成功`
- 密码永不输出：列表 SELECT 不查 password 列 + model `json:"-"` 双保险
- 编辑时 `password` 传空串表示不修改密码；改密码且目标非操作者本人时，目标账号被立即踢下线（DEL Redis 白名单）
- 删除必踢下线（否则已删账号 JWT 在 TTL 内仍有效）；不能删除当前登录账号（400）
- 用户名唯一：重名 400「用户名已存在」（库内 `uk_username` 兜底并发）；用户不存在 404
- 新增落库默认值：nickname 取 username 兜底、role 固定 `admin`、status=1 启用；last_login_* 保持 NULL（列表显示空）

### curl 示例

```bash
# 列表（username 模糊，空串表示未填）
curl -s -X POST 'http://localhost:8080/api/v1/admin/user/list' \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"username":"","page_index":1,"page_size":10}'

# 新增用户
curl -s -X POST 'http://localhost:8080/api/v1/admin/user/add' \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"username":"tester1","password":"123456"}'

# 编辑用户（password 留空=不修改密码；改用户名不踢人，token 以 userID 为准）
curl -s -X POST 'http://localhost:8080/api/v1/admin/user/edit' \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"id":2,"username":"tester1b","password":""}'

# 删除用户（该账号当前 token 立即失效；删自己 400）
curl -s -X POST 'http://localhost:8080/api/v1/admin/user/delete' \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"id":2}'
```

设计决策与实施记录见 [plans/PLAN-admin-user.md](plans/PLAN-admin-user.md)。

## 直播接口（小鹅通透传，mofang C 端）

对应 mofang 前端 `src/services/live/index.ts`（直播间中转页账号打通）。真实环境该路径走远程网关 guyuzhoudb 域，本服务为本地复刻：纯透传小鹅通开放平台 `xe.login.url/1.0.0`，**不持有小鹅通 secret**（access_token 由前端从远程网关 `get_access_token` 取得后透传）。公开无鉴权——mofang 是 C 端另一 token 体系本服务验不了，凭证有效性由小鹅通侧校验，决策见 [plans/PLAN-live.md](plans/PLAN-live.md)。

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | GET | `/guyuzhoudb/live/get_login_url` | 透传小鹅通取登录链接（webview 打开后在小鹅通域名注入登录态再 302 到 redirect_uri） |
| 2 | GET | `/guyuzhoudb/live/register_user` | 透传 xe.user.register：按手机号幂等注册换小鹅通 user_id（get_login_url 的前置，user_id 须在该店铺存在，否则登录页报「获取用户信息失败」） |

- get_login_url query：`access_token`（必填）、`user_id`（必填，**须为该店铺已存在的小鹅通用户 id**，用 register_user 换取）、`login_type`（必填：1=PC 2=H5 3=App）、`redirect_uri`（可选，http(s):// 开头，登录成功后跳转的小鹅通页面链接）
- register_user query：`access_token`（必填）、`phone`（必填，11 位数字手机号）；返回 `data.user_id` + `data.user_exists`（0=新建 1=已存在，幂等可重复调）
- 返回 get_login_url 的 `data.login_url` / `data.permission_denied_url`；**login_url 有效期仅 1 分钟，即取即跳、禁止缓存**
- 失败映射：参数形状非法 400（中文文案）；上游业务错（access_token 无效等）/网络错 502（上游 code/msg 只进服务端日志；phone 不落日志）
- 上游域名可配：`XIAOE_API_BASE`（默认 `https://api.xiaoe-tech.com`）

```bash
# 注册换 user_id（幂等：已存在用户也返回 user_id）
curl -i 'http://localhost:8080/guyuzhoudb/live/register_user?access_token=<token>&phone=18820205724'
# → {"code":200,"msg":"success","data":{"user_id":"u_api_xxx","user_exists":1}}

# 登录链接（user_id 用上一步返回值；redirect_uri 需 URL 编码）
curl -i 'http://localhost:8080/guyuzhoudb/live/get_login_url?access_token=<token>&user_id=u_api_xxx&login_type=1&redirect_uri=https%3A%2F%2Fapp1.pc.xiaoe-tech.com%2Flive_pc%2Fl_1'

# 缺参 400 示例
curl -s 'http://localhost:8080/guyuzhoudb/live/get_login_url?user_id=u_1&login_type=1'
# → {"code":400,"msg":"参数 access_token 不能为空"}
```

## 订单系统 Demo 接口（MQ 异步链路，设计决策见 plans/PLAN-order.md）

创建订单：Gin → MySQL 落库 → 发布 `order.created`（RabbitMQ fanout）→ 库存/积分/通知三消费者（独立进程 `cmd/consumer`）异步处理。

订单状态：`1` 处理中 → `2` 已完成（stock/points/notify 三步骤列全 1）/ `3` 已取消（库存不足，补偿回滚积分与通知）；
步骤列：`0` 待处理 / `1` 成功 / `2` 失败。幂等：积分/通知表 `order_id` 唯一键 + INSERT IGNORE（库端 `WHERE status=1` 原子判取消），扣库存条件 UPDATE。

### 启动（两个进程）

```bash
# 依赖已并入 docker-compose（MySQL/Redis/RabbitMQ 一条命令起齐，见「快速开始」；RabbitMQ 管理台 :15672 guest/guest）
# 弱网可经镜像加速拉取：docker pull docker.m.daocloud.io/library/<image>:<tag> 后 docker tag 成 compose 里的目标名
go run ./cmd/server     # HTTP 发布端（:8080）
go run ./cmd/consumer   # 三个消费者（独立进程，可单独重启观察积压/重投）
```

### curl 示例

```bash
TOKEN=<登录返回的 token>

# 创建订单（user_id 取登录态；金额后端按 price×quantity 计算；商品见 GET /orders/products）
curl -s -X POST 'http://localhost:8080/api/v1/orders' \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"product_id":1,"quantity":2}'
# → {"code":200,"msg":"下单成功","data":{"id":1,"order_no":"202608281830121234","product_name":"智能手表 Pro","quantity":2,"amount":3998,"status":"1","stock_status":"0","points_status":"0","notify_status":"0",...}}

# 商品下拉（含价格/库存）
curl -s 'http://localhost:8080/api/v1/orders/products' -H "Authorization: Bearer $TOKEN"

# 订单列表（观察 status 翻转与三步骤列点亮；status 精确 1/2/3，传 null 不过滤）
curl -s -X POST 'http://localhost:8080/api/v1/orders/list' \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"page_index":1,"page_size":10,"order_no":"","product_name":"手表","status":null}'

# 积分列表 / 通知列表（消费者异步写入）
curl -s -X POST 'http://localhost:8080/api/v1/points/list' \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"page_index":1,"page_size":10}'
curl -s -X POST 'http://localhost:8080/api/v1/notifications/list' \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"page_index":1,"page_size":10}'
```

### 表设计

| 表 | 说明 |
| --- | --- |
| `product` | 商品（含库存，种子 4 个，其一库存仅 3 演示取消路径） |
| `orders` | 订单（order 为保留字故复数；status + stock/points/notify 三步骤列，冗余商品名快照） |
| `points_record` | 积分流水（`uk_order` 唯一键 = 消息幂等；1 元 1 分向下取整） |
| `notification` | 通知记录（`uk_order` 唯一键 = 消息幂等；Demo 为站内表不接真实推送） |

## 目录结构（对应 Spring Boot 分层）

```
.
├── cmd/server/main.go        # HTTP 入口：加载配置 → 连库/MQ → 建表/种子 → 路由 → 启动
├── cmd/consumer/main.go      # 消费者入口：库存/积分/通知三队列（订单 Demo，见 plans/PLAN-order.md）
├── docs/                     # swag 生成物（docs.go/json/yaml），swag init 输出目录
├── plans/                    # 设计决策与实施记录（PLAN-*.md + 早期 API/调试文档）
└── internal/
    ├── config/               # 配置：环境变量 + 默认值，组装 MySQL DSN
    ├── database/             # 连接池 + 迁移（schema.sql）+ 种子（teacher_seed.sql / resign_seed.sql / diagnose_seed.sql）
    ├── model/                # Entity/DTO：Teacher、Resign、Diagnose、DateTimeString、StringSlice
    ├── mq/                   # RabbitMQ：拓扑声明 + 发布 + 消费骨架（订单 Demo）
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
- Docker（MySQL/Redis/RabbitMQ 统一由 `docker-compose.yml` 编排：`docker compose up -d`，健康状态 `docker compose ps`）
- 端口避让：本机已装 MySQL（3306）/Redis（6379）时容器映射 **3307/6380**，`.env` 填容器端口，与本机服务并存互不影响（决策见 plans/PLAN-docker.md）

### 2. 初始化数据库

无需手工建库：容器 `MYSQL_DATABASE=handicap_db` 自动建库（utf8mb4_0900_ai_ci），首次 `docker compose up -d` 即完成；空库启动 go 服务自动建表 + 种子。

### 3. 配置环境变量

```bash
cp .env.example .env   # 按需修改 DB_USER/DB_PASSWORD（对齐 compose 的 MYSQL_USER）与端口（容器 3307/6380）；JWT_SECRET 必填（openssl rand -hex 32 生成）
```

### 4. 启动

```bash
go mod tidy
go run ./cmd/server
```

首次启动自动执行：建表（幂等，含 admin_user）→ 表为空则插入种子数据（admin_user 种子：admin/admin123）。之后启动直接可用。

### 容器化运行（全栈，见 plans/PLAN-docker.md）

`docker compose up -d` 直接起全栈（中间件 + server + consumer）；server 容器不映射宿主机端口，与宿主机 `go run ./cmd/server`（:8080）并存零冲突：

```bash
docker compose up -d --build   # 全栈：三件套中间件 + server + consumer 容器
docker compose ps              # 看健康状态（server 有 healthcheck，consumer 无端口仅看 running）
docker compose logs -f server consumer
docker compose down            # 全停（volume 保留）
```

- **端口零冲突**：server 容器不映射宿主机 8080，宿主机 `go run` 开发流不受影响（两者可同时跑）
- 镜像：多阶段构建（`golang:1.24-alpine` → `alpine:3.21`，约 55MB），单镜像双二进制 `/app/server` 与 `/app/consumer`（consumer 服务 `command` 覆盖）；tzdata + `TZ=Asia/Shanghai` 对齐 DSN 时区、ca-certificates 支撑小鹅通 HTTPS
- 配置：容器内直连服务名（`DB_HOST=mysql` 等），凭证/`JWT_SECRET` 从 `.env` 插值注入（`:?` 强制必填），**不进镜像层**
- 弱网：`.env` 末段取消注释 `GO_IMAGE`/`RUNTIME_IMAGE`/`GOPROXY` 换 daocloud/goproxy 源
- 前端联动：GoProject-web 容器经 `goproject_default` 网络直连 `handicap-server:8080`（容器名），Swagger 也走前端 `/swagger` 反代；宿主机 8080 空闲留给 go run
- 构建后建议 `docker builder prune -f`：buildkit 缓存可达数 GB，挤爆 Docker Desktop VM 内存会触发 RabbitMQ 内存告警（AMQP 拒连，详见 PLAN-docker.md 实测记录）

#### 版本化发版（deploy.sh，对齐前端模式，见 plans/PLAN-docker.md 决策 12）

```bash
./deploy.sh deploy            # 发版：构建语义版本镜像（缺省 patch+1；minor/major 升段位；或显式 1.2.0）
                              #   → compose 切换 server/consumer → 双服务健康门禁（120s）→ 失败自动回滚
./deploy.sh rollback prev     # 回滚上一版（rollback <tag> 回任意历史版；不带参数列版本）
./deploy.sh list              # 版本列表（标注当前运行）
./deploy.sh prune [-n]        # 清理旧版本（保留 KEEP=5；-n 干跑）
CHECK=1 ./deploy.sh deploy    # 构建前先跑 go vet（弱网下 docker build 前快速失败）
```

- 镜像按语义三段式 tag（`handicap-server:1.0.0` 风格）保留历史版本可回滚；`latest` 只是「当前运行版本」的指针别名；`IMAGE_TAG`/`GIT_REV` 双 ENV 打进镜像溯源（compose 日常 `up -d --build` 落 `dev/unknown`，可区分手动/发版构建）
- 发版必须走 deploy.sh；手动 `docker compose up -d --build` 仅日常调试（会覆盖 latest 指针且无版本历史），**勿带 APP_TAG**（会打出无 GIT_REV 的版本 tag 污染历史）
- 停机窗口：容器重建期间（server 优雅停 15s + 启动 + 门禁判定，约 20~60s）前端 API 短暂 502，可 `docker restart goproject-web` 立即恢复（不重启也会 --restart 自愈）
- 发版耗时：代码已变时 `COPY . .` 层缓存失效，VM 内 go build 全量重编译（15 分钟起，`go mod download` 层仍命中）；deploy.sh 自身已排除出构建上下文（改脚本不触发重编译）

### 5. 验证

```bash
# 老师列表（POST body，分页返回 data.list / data.count）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/teacher/list' \
  -H 'Content-Type: application/json' \
  -d '{"page_index":1,"page_size":10}'

# 下拉选项
curl -s 'http://localhost:8080/api/v1/dxsf/teacher/options'
```

## 前端管理台（同级目录 GoProject-web/）

配套管理台为独立前端目录 `../GoProject-web/`（原在本仓库 `web/`，2026-08-30 迁出；设计决策与实测记录见 [plans/PLAN-web.md](plans/PLAN-web.md)）：React 18 + Vite + TypeScript(strict) + antd 5，承载本服务全部接口——登录鉴权、用户管理、老师管理（编辑/详情/绑定业务员）、离职转移、诊股记录（状态机 + 富文本）、订单四页（创建/订单/积分/通知）、直播工具调试页。替代旧 Vue2 前端 gyz-admin 的本地联调流（旧项目仍可独立运行，无耦合）。

### 启动

```bash
cd ../GoProject-web
npm install
npm run dev        # :5173，dev proxy /api 与 /guyuzhoudb → http://localhost:8080
```

- 前置：`go run ./cmd/server` 已在 :8080 运行（MySQL/Redis 依赖见「快速开始」）；订单异步链路另需 RabbitMQ + `go run ./cmd/consumer`
- 登录账号：种子 admin/admin123
- `npm run typecheck` 类型检查；`npm run build` 生产构建（产物在 `GoProject-web/dist/`）

### 契约要点（与后端严格对齐，改动接口时勿破坏）

- 登录失败也 HTTP 200 + `code:400`，`token` 在 body 根；错误文案含「密码」时前端定位密码输入框
- 分页入参 `page_index`/`page_size`，列表返回 `data.list`/`data.count`；绑定业务员列表回显驼峰 `pageIndex/pageSize` 为唯一特例
- 诊股/订单列表数值筛选必须是 JSON number（字符串一律 400）；`status` 类字段输出字符串 `"1"/"0"`
- 审核目标状态由前端换算直传（专业通过 4 / 驳回 3，合规通过 6 / 驳回 5），集中在 `GoProject-web/src/constants/diagnose.ts`

## 常见问题

| 现象 | 原因与解决 |
| --- | --- |
| 连接报 `error 1045` | 密码错误，检查 `.env` 的 `DB_PASSWORD` |
| 连接报 `error 2002` | MySQL 容器未就绪：`docker compose up -d mysql` 后 `docker compose ps` 看 healthy（首次初始化约 1 分钟） |
| 认证失败（caching_sha2_password） | go-sql-driver 原生支持，容器场景基本不会遇到；本机直连老客户端时兜底 `ALTER USER ... IDENTIFIED WITH mysql_native_password ...` |
| server 启动报 `dial rabbitmq` | RabbitMQ 容器未起：`docker compose up -d rabbitmq`；连接串在 `.env` 的 `RABBITMQ_URL`（订单 Demo 对 MQ fail-fast） |
| 下了单但订单一直「处理中」 | 消费者进程没起：另开终端 `go run ./cmd/consumer`（消费与 HTTP 是两个独立进程） |
| 全栈下单报 `channel/connection is not open`（504） | server 的 MQ channel 被 rabbitmq 重启/内存告警关掉后不自动重建（amqp091 无内置重连）：`docker compose --profile app restart server` 即恢复（根因与后续加固见 plans/PLAN-docker.md 实测记录） |
| rabbitmq 容器 unhealthy 且 server/consumer 连不上 | 多为 Docker Desktop VM 内存紧张触发 rabbitmq memory alarm（alarm 期间 AMQP 拒连）：`docker builder prune -f` 清构建缓存，alarm 自动 clear 后重启 server/consumer |
| 前端容器调不通后端（:80 反代 502） | 确认 server 容器已起且映射 `8080:8080`（`docker compose --profile app ps`），且宿主机 8080 无残留 `go run` 进程占位 |

## 进阶方向（本期未实现）

- 接入 sqlx 自动映射结构体
- 单元测试（handler / service / repository）
- 优雅停机（signal + http.Server.Shutdown）