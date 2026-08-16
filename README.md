# handicap-service

Go 学习项目：涨跌家数分布统计接口 + 老师管理（chatSys）接口。数据从 MySQL 查询返回（非硬编码），首次启动自动建表并写入种子数据。

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
```

### 表设计

| 表 | 说明 |
| --- | --- |
| `teacher` | 老师；`qualification` 存中文、`status` TINYINT（接口输出字符串）、`rating` 存原值 |
| `sales_user` | 业务员桩表（真实系统为 admin 的 sys_user），种子 = mock 的 salesPool 25 人 |
| `teacher_sales` | 绑定关系；`uk_teacher_user` 唯一约束；`bind_sales_count` 不落库，由子查询统计（单一事实来源） |

种子数据照抄 mock（12 老师 / 25 业务员 / 143 条绑定），仅在表为空时写入，重灌方式：`TRUNCATE TABLE teacher; TRUNCATE TABLE sales_user; TRUNCATE TABLE teacher_sales;` 后重启。

设计决策与实施记录见 [PLAN-teacher.md](PLAN-teacher.md)。

## 目录结构（对应 Spring Boot 分层）

```
.
├── cmd/server/main.go        # 入口：加载配置 → 连库 → 建表/种子 → 路由 → 启动
└── internal/
    ├── config/               # 配置：环境变量 + 默认值，组装 MySQL DSN
    ├── database/             # 连接池 + 迁移（schema.sql）+ 种子（seed.sql / teacher_seed.sql）
    ├── model/                # Entity/DTO：HouseUpDown、Teacher、DateTimeString
    ├── repository/           # DAO/Mapper：按 secuMarket + range 查询 / teacher CRUD
    ├── service/              # Service：参数校验、默认值、白名单
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