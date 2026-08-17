# handicap-service

Go 学习项目：涨跌家数分布统计接口 + 老师管理（chatSys）接口 + 老师离职转移接口 + 诊股记录接口。数据从 MySQL 查询返回（非硬编码），首次启动自动建表并写入种子数据。

## 技术栈

| 组件 | 选型 | 说明 |
| --- | --- | --- |
| 语言 | Go 1.24+ | |
| Web 框架 | [Gin](https://github.com/gin-gonic/gin) | 路由、中间件、JSON 渲染 |
| 数据访问 | database/sql + [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) | Go 官方标准库，无 ORM |
| 数据库 | MySQL 8 | Homebrew 安装 |
| 环境变量 | [godotenv](https://github.com/joho/godotenv) | 支持 `.env` 文件 |

## 接口

```
GET /handicap/v1/index-points/houses_up_or_down?secuMarket=000001&range=today
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| secuMarket | 是 | 市场代码，如 `000001` |
| range | 否 | 统计区间，默认 `today`；可选 `today` / `week` / `month` |

返回（data 为 null 表示该市场该区间暂无统计数据）：

```json
{
  "code": 200,
  "msg": "ok",
  "data": {
    "above7": 53,
    "between5_7": 37,
    "between3_5": 111,
    "between0_3": 1352,
    "equal0": 87,
    "betweenN3_0": 635,
    "betweenN5_N3": 30,
    "betweenN7_N5": 3,
    "belowN7": 1,
    "total": 2312,
    "upCount": 1553,
    "downCount": 669,
    "flatCount": 87
  }
}
```

## 老师管理接口（chatSys）

对应前端 `gyz-admin/src/api/dxData/chatSys/teacher.js`（接口文档即该文件，原本全为前端 mock），路径/参数/响应结构与 mock 严格一致，前端删掉 mock 段启用注释的 `request` 调用即可直接联调。

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | GET | `/api/v1/dxsf/chatSys/teacher/list` | 老师列表（分页 + 多条件筛选） |
| 2 | GET | `/api/v1/dxsf/chatSys/teacher/options` | 老师下拉选项（含停用，带部门名） |
| 3 | PUT | `/api/v1/dxsf/chatSys/teacher/update` | 编辑老师（title / rating / avatar / signature） |
| 4 | GET | `/api/v1/dxsf/chatSys/teacher/bindSales/list` | 老师绑定业务员列表（详情弹窗） |
| 5 | POST | `/api/v1/dxsf/chatSys/teacher/bindSales` | 绑定业务员（全量替换，空数组 = 解绑全部） |
| 6 | GET | `/api/v1/dxsf/chatSys/teacher/bindSales/boundUserIds` | 全量已绑定业务员关系对（绑定弹窗人员树过滤用） |
| 7 | GET | `/api/v1/dxsf/chatSys/resign/list` | 离职转移记录列表（分页 + 多条件筛选） |
| 8 | POST | `/api/v1/dxsf/chatSys/resign/add` | 新增离职转移 |

### 列表筛选参数

| 参数 | 匹配 | 说明 |
| --- | --- | --- |
| deptId / id / qualification / bindSalesCount / status | 精确 | status 传 `"1"`/`"0"`；qualification 传中文 |
| account / nickname / name / title / updateBy | 模糊 | LIKE %val% |
| updateBeginTime + updateEndTime | 范围 | yyyy-MM-dd，按 updatedAt 闭区间，成对生效 |
| pageIndex / pageSize | 分页 | 默认 1 / 10（绑定列表默认 pageSize=5），pageSize 上限 100 |

### 响应约定（与前端 mock 对齐的兼容点）

- 成功 msg：查询类 `success`，写操作 `编辑成功` / `绑定成功`（非 `ok`）
- `status` 输出字符串 `"1"`/`"0"`（前端 el-switch 直接比较）
- `qualification` 存/传中文 `已认证`/`未认证`
- 时间字段输出 `YYYY-MM-DD HH:mm:ss`（`model.DateTimeString` 在扫描点格式化，避免 RFC3339 带 T）
- `rating` 原样存取：种子为 1-5 星，编辑后为 0 无 / 1 初级 / 2 高级
- 分页返回 `data.list` / `data.count`；老师不存在时绑定列表返回空 list + count 0（不报错）

### curl 示例

```bash
# 列表（模糊 + 精确 + 分页）
curl -s 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/list?status=1&name=%E5%BC%A0&pageIndex=1&pageSize=10'

# 下拉选项
curl -s 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/options'

# 编辑老师（rating: 0 无 / 1 初级 / 2 高级）
curl -s -X PUT 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/update' \
  -H 'Content-Type: application/json' \
  -d '{"id":1,"title":"首席投顾","rating":2,"avatar":"","signature":"签名"}'

# 老师绑定业务员列表
curl -s 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/bindSales/list?teacherId=1&pageIndex=1&pageSize=5'

# 绑定业务员（全量替换；userIds 为业务员表 id）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/bindSales' \
  -H 'Content-Type: application/json' \
  -d '{"teacherId":1,"userIds":[1,2,3]}'

# 全量已绑定业务员关系对（人员树过滤 + 提交合并，不分页）
curl -s 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/bindSales/boundUserIds'
# → {"code":200,"msg":"success","data":[{"teacherId":1,"userId":2},...]}
```

### 表设计

| 表 | 说明 |
| --- | --- |
| `teacher` | 老师；`qualification` 存中文、`status` TINYINT（接口输出字符串）、`rating` 存原值 |
| `sales_user` | 业务员桩表（真实系统为 admin 的 sys_user），种子 = mock 的 salesPool 25 人 |
| `teacher_sales` | 绑定关系；`uk_teacher_user` 唯一约束；`bind_sales_count` 不落库，由子查询统计（单一事实来源） |

种子数据照抄 mock（12 老师 / 25 业务员 / 143 条绑定），仅在表为空时写入，重灌方式：`TRUNCATE TABLE teacher; TRUNCATE TABLE sales_user; TRUNCATE TABLE teacher_sales;` 后重启。

设计决策与实施记录见 [PLAN-teacher.md](PLAN-teacher.md)。

## 老师离职转移接口（chatSys）

对应前端 `gyz-admin/src/api/dxData/chatSys/resign.js`（同上，原为前端 mock）。姓名/部门为冗余快照，后端从 `teacher` 表回查（前端传的冗余字段忽略）；业务员快照取原老师全部绑定业务员（多个逗号分隔）。

### 列表筛选参数

| 参数 | 匹配 | 说明 |
| --- | --- | --- |
| deptId / originalTeacherId / replaceTeacherId | 精确 | deptId 匹配原老师部门 |
| salesmanName | 模糊 | LIKE %val% |
| transferBeginTime + transferEndTime | 范围 | yyyy-MM-dd，按 transferTime 闭区间，成对生效 |
| pageIndex / pageSize | 分页 | 默认 1 / 10，pageSize 上限 100，id 倒序（最新在前） |

### 新增请求体

```json
{ "originalTeacherId": 4, "replaceTeacherId": 1, "transferContent": ["group", "friend"], "remark": "离职交接" }
```

- `transferContent` 白名单 `group`（转移客户群）/ `friend`（转移好友），非空、非法值 400
- 原/接替老师不能相同（400），老师不存在 404
- `groupCount` / `friendCount` 可选携带（缺省 0，负数 400）：系统暂无群/好友业务表，前端不传即存 0
- `operator` 无登录态固定 `admin`；`operateIp` 取 `c.ClientIP()`；`transferTime` 库端 NOW()

### 响应约定

- 列表 `data.list` / `data.count`；`transferContent` 输出数组（库存逗号串）；时间字段 `YYYY-MM-DD HH:mm:ss`
- 新增成功 `{code:200, msg:"转移成功"}`

### curl 示例

```bash
# 列表（部门 + 时间范围）
curl -s 'http://localhost:8080/api/v1/dxsf/chatSys/resign/list?deptId=3&transferBeginTime=2025-08-01&transferEndTime=2025-08-31&pageIndex=1&pageSize=10'

# 新增离职转移
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/chatSys/resign/add' \
  -H 'Content-Type: application/json' \
  -d '{"originalTeacherId":4,"replaceTeacherId":1,"transferContent":["group","friend"],"remark":"离职交接"}'
```

### 表设计

| 表 | 说明 |
| --- | --- |
| `teacher_resign` | 离职转移记录；`transfer_content` 存逗号串（接口输出数组，`model.StringSlice`）；`salesman_name/dept` 存原老师全部绑定业务员逗号串；`original_teacher_dept_id` 供 deptId 筛选 |

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
| id / buyPrice / buyNum / status | 精确 | 数值格式错误 400；status 白名单 1-6；传 `0` 亦为有效过滤值（指针区分未传） |
| userNickName / userName / stockCode / stockName / teacherName | 模糊 | LIKE %val% |
| submitBeginTime + submitEndTime | 范围 | yyyy-MM-dd，按 submitTime 闭区间，成对生效 |
| reportBeginTime + reportEndTime | 范围 | yyyy-MM-dd，按 reportSubmitTime 闭区间（未提审的天然排除） |
| pageIndex / pageSize | 分页 | 默认 1 / 10，pageSize 上限 100，id 倒序（最新在前） |

### 请求体

```json
// POST /diagnose/submitReport：reportContent 为富文本 HTML，去标签全空白视为未填写（400）
{ "id": 1, "reportContent": "<p>诊股结论：持股待涨</p>" }

// POST /diagnose/audit：auditType 白名单 professional / compliance，result 白名单 pass / reject；
// reject 时 rejectReason 必填（富文本，同样判空）
{ "id": 1, "auditType": "professional", "result": "reject", "rejectReason": "<p>结论依据不足</p>" }
```

### 响应约定

- 成功 msg：`success` / `提交成功` / `审核通过` / `已驳回`（mock 约定）
- `buyPrice` DECIMAL 扫 float64 输出 `1680.5`；时间字段 `YYYY-MM-DD HH:mm:ss`，`reportSubmitTime` 未提审输出空串
- detail = 主表全字段 + `auditLogs` 数组（按 id 正序 = 时间序）；记录不存在 404（较 mock 的 `data:null` 收紧）
- 并发守卫：写前查询区分 404/400，事务内条件 UPDATE（`WHERE status IN (1,3,5)` / `WHERE status = ?`），`RowsAffected == 0` 回滚返回 400（纯 SELECT-then-UPDATE 有 TOCTOU）
- 审核 operator 无登录态固定 `专业审核员` / `合规审核员`；`log_time` 库端 NOW()

### curl 示例

```bash
# 列表（状态 + 股票名模糊）
curl -s 'http://localhost:8080/api/v1/dxsf/diagnose/list?status=2&stockName=%E4%BA%94%E7%B2%AE&pageIndex=1&pageSize=10'

# 详情（含审核流程日志，按时间正序）
curl -s 'http://localhost:8080/api/v1/dxsf/diagnose/detail?id=5'

# 提交诊股报告（状态 1/3/5 → 2）
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/diagnose/submitReport' \
  -H 'Content-Type: application/json' \
  -d '{"id":1,"reportContent":"<p>诊股结论：持股待涨</p>"}'

# 专业审核通过（2 → 4）；驳回把 result 换成 reject 并带 rejectReason
curl -s -X POST 'http://localhost:8080/api/v1/dxsf/diagnose/audit' \
  -H 'Content-Type: application/json' \
  -d '{"id":1,"auditType":"professional","result":"pass"}'
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
    ├── database/             # 连接池 + 迁移（schema.sql）+ 种子（seed.sql / teacher_seed.sql / resign_seed.sql / diagnose_seed.sql）
    ├── model/                # Entity/DTO：HouseUpDown、Teacher、Resign、Diagnose、DateTimeString、StringSlice
    ├── repository/           # DAO/Mapper：按 secuMarket + range 查询 / teacher・resign・diagnose CRUD
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

### 2. 初始化数据库

```bash
mysql -u root -e "CREATE DATABASE IF NOT EXISTS handicap_db CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;"
```

### 3. 配置环境变量

```bash
cp .env.example .env   # 按需修改 DB_PASSWORD（Homebrew 初始 root 无密码）
```

### 4. 启动

```bash
go mod tidy
go run ./cmd/server
```

首次启动自动执行：建表（幂等）→ 表为空则插入种子数据。之后启动直接可用。

### 5. 验证

```bash
# 快乐路径：返回上方样例数据
curl -s 'http://localhost:8080/handicap/v1/index-points/houses_up_or_down?secuMarket=000001&range=today'

# 无数据路径：HTTP 200 + data:null
curl -i 'http://localhost:8080/handicap/v1/index-points/houses_up_or_down?secuMarket=000001&range=week'

# 参数错误：HTTP 400
curl -i 'http://localhost:8080/handicap/v1/index-points/houses_up_or_down?secuMarket=&range=today'
```

## 数据库设计

单表 `house_up_down_stats`（DDL 见 `internal/database/schema.sql`）：

- 联合唯一索引 `(secu_market, stat_range, stat_date)`：同一市场、区间、日期只能有一条统计，天然去重
- 查询按 `secu_market + stat_range` 过滤、`stat_date DESC` 取最新，命中最左前缀
- `range` 是 MySQL 保留字，列名用 `stat_range`，Go 侧字段仍叫 `Range`（db tag 映射）

### 种子数据说明

- 仅在表为空时插入（重启不重复），重灌方式：`TRUNCATE TABLE house_up_down_stats` 后重启
- 只种 1 行 `000001/today`，`stat_date = CURDATE()` 保证"今天"始终命中
- 样例 `total=2312` 与 9 档之和 2309 不一致（差 3），照抄原样——真实接口返回如此，可留意

## 常见问题

| 现象 | 原因与解决 |
| --- | --- |
| 连接报 `error 1045` | 密码错误，检查 `.env` 的 `DB_PASSWORD` |
| 连接报 `error 2002` | MySQL 未启动：`brew services start mysql` |
| 认证失败（caching_sha2_password） | 兜底改回旧插件：`ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY '<密码>';` |
| 跨天重启后返回值变化 | 按约定返回 `stat_date` 最近一天的统计，属正常行为 |

## 进阶方向（本期未实现）

- `INSERT ... ON DUPLICATE KEY UPDATE` 增量更新统计
- 补充 week/month 种子数据
- 接入 sqlx 自动映射结构体
- 单元测试（handler / service / repository）
- 优雅停机（signal + http.Server.Shutdown）