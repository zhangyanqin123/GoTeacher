# Go 学习项目计划：涨跌家数分布统计接口（handicap-service）

## Context（背景）

用户有 Node.js 和 Spring Boot 基础，正在学习 Go。要在空目录 `/Users/a1/Documents/GoProject` 从零搭建一个 Go Web 项目，要求服务能启动、连通 MySQL，实现唯一接口 `GET /handicap/v1/index-points/houses_up_or_down?secuMarket=000001&range=today`，数据从 MySQL 查询返回（非硬编码）。

环境事实：Go 1.24.2 已装；MySQL 未装（计划 brew 安装）；Docker 未装；目录当前为空。

**已确认的技术选型**：Gin（Web 框架）+ database/sql + go-sql-driver/mysql（数据访问）+ brew 安装 MySQL 8。

## 接口定义

```
GET /handicap/v1/index-points/houses_up_or_down?secuMarket=000001&range=today
```

返回示例：

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

## 分层设计（映射 Spring Boot，方便学习迁移）

```
handler(Controller) → service(Service) → repository(DAO) → model(Entity)
```

## 目录结构

```
GoProject/
├── go.mod / go.sum            # module: handicap-service
├── README.md                  # 启动步骤、目录说明、常见问题
├── PLAN.md                    # 本计划文档
├── .gitignore                 # 忽略 .env、二进制
├── .env.example               # 环境变量模板（复制为 .env）
├── cmd/server/main.go         # 入口：加载配置→连库→迁移/种子→路由→启动
└── internal/
    ├── config/config.go       # 环境变量+默认值，用 mysql.Config 拼 DSN
    ├── database/
    │   ├── database.go        # sql.Open + 连接池 + Ping + Migrate + Seed
    │   ├── schema.sql         # 建表 DDL（go:embed 内嵌，幂等）
    │   └── seed.sql           # 种子数据（go:embed 内嵌）
    ├── model/house_up_down.go # 结构体 HouseUpDown（json tag = 接口字段名）
    ├── repository/house_up_down.go  # 按 secuMarket+range 查询，无数据返回 (nil,nil)
    ├── service/house_up_down.go     # 校验：secuMarket 必填、range 缺省 today、白名单 today/week/month
    ├── handler/house_up_down.go     # Gin handler：绑定 query、错误→HTTP 状态码映射
    ├── response/response.go         # 统一响应 {code,msg,data} + OK/Fail 快捷函数
    └── router/router.go             # 组装 repo→service→handler，注册路由 group /handicap/v1
```

刻意不做（避免过度设计）：不引入 ORM、migration 工具、viper、redis、swagger、测试框架。

## MySQL 准备

```bash
brew install mysql
brew services start mysql        # 注册 launchd 自启
mysql -u root -e "SELECT VERSION();"   # Homebrew 初始 root 无密码
```

root 密码策略（执行时确认）：默认方案 root 无密码 + `.env` 留空；备选设置 `ALTER USER 'root'@'localhost' IDENTIFIED BY 'root123';` 并写入 `.env`。

建库（第一次启动前执行一次）：

```sql
CREATE DATABASE IF NOT EXISTS handicap_db CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
```

## 表结构 DDL（internal/database/schema.sql）

注意：`range` 是 MySQL 保留字，列名用 `stat_range`，Go 字段仍叫 `Range`（靠 db tag 映射）。列名 snake_case，Go camelCase。

```sql
CREATE TABLE IF NOT EXISTS house_up_down_stats (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  secu_market   VARCHAR(10) NOT NULL COMMENT '市场代码，如 000001',
  stat_range    VARCHAR(10) NOT NULL COMMENT '统计区间：today/week/month',
  above7        INT NOT NULL DEFAULT 0,
  between5_7    INT NOT NULL DEFAULT 0,
  between3_5    INT NOT NULL DEFAULT 0,
  between0_3    INT NOT NULL DEFAULT 0,
  equal0        INT NOT NULL DEFAULT 0,
  between_n3_0  INT NOT NULL DEFAULT 0,
  between_n5_n3 INT NOT NULL DEFAULT 0,
  between_n7_n5 INT NOT NULL DEFAULT 0,
  below_n7      INT NOT NULL DEFAULT 0,
  total         INT NOT NULL DEFAULT 0,
  up_count      INT NOT NULL DEFAULT 0,
  down_count    INT NOT NULL DEFAULT 0,
  flat_count    INT NOT NULL DEFAULT 0,
  stat_date     DATE NOT NULL,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_market_range_date (secu_market, stat_range, stat_date),
  KEY idx_stat_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='涨跌家数分布统计';
```

## 种子数据策略

启动时 `Migrate`（执行 schema.sql，幂等）→ `Seed`（`COUNT(*)` 为 0 才插入，事务包裹）。只种 1 行样例数据（000001/today，`stat_date=CURDATE()`），这样 `range=week` 或别的市场天然构成"无数据"验证路径。样例 `total=2312` 与 9 档之和 2309 不一致，照抄原样并在 README 注明（教学点）。

```sql
INSERT INTO house_up_down_stats
  (secu_market, stat_range, above7, between5_7, between3_5, between0_3, equal0,
   between_n3_0, between_n5_n3, between_n7_n5, below_n7,
   total, up_count, down_count, flat_count, stat_date)
VALUES
  ('000001', 'today', 53, 37, 111, 1352, 87, 635, 30, 3, 1, 2312, 1553, 669, 87, CURDATE());
```

## 关键实现要点

- **config**：`os.Getenv` + 默认值（DBHost=127.0.0.1, DBPort=3306, DBUser=root, DBPassword 空, DBName=handicap_db, ServerPort=8080）；`godotenv.Load()` 支持 `.env`（可去掉，不影响逻辑）；DSN 用 `mysql.Config{... ParseTime: true, Loc: time.Local}` 的 `FormatDSN()` 生成——`ParseTime: true` 必须，否则 DATETIME 扫描报错。
- **database**：连接池 `SetMaxOpenConns(20)/SetMaxIdleConns(5)/SetConnMaxLifetime(time.Hour)`；`PingContext` 5s 探活。
- **model**：`json` tag 与接口字段名完全一致（`above7`、`between5_7`、`betweenN3_0`…），`json:"-"` 隐藏 id/时间戳，`StatDate` 类型 `string`（DATE 可扫成 string）。
- **repository**：`SELECT ... WHERE secu_market=? AND stat_range=? ORDER BY stat_date DESC LIMIT 1`；`errors.Is(err, sql.ErrNoRows)` → 返回 `(nil, nil)`；其他错误 `fmt.Errorf("%w")` 包装。
- **service**：`secuMarket==""` → `ErrMissingMarket`；`range` 缺省 `"today"`；`slices.Contains`（Go 1.21+ 标准库）白名单校验。
- **handler** 错误映射：缺参/非法 range → 400；DB 错误 → `slog.Error` 记日志 + 500 `"internal server error"`（不泄露细节）；无数据 → 200 + `data:null`（正常业务结果，非 404）。
- **main.go**：连库失败 `slog.Error` + `os.Exit(1)`（给出清晰提示）；`r.Run(":" + cfg.ServerPort)`。

## 实施步骤

1. `go mod init handicap-service`；`go get github.com/gin-gonic/gin`、`github.com/go-sql-driver/mysql`、`github.com/joho/godotenv`；`go mod tidy`
2. brew 安装并启动 MySQL，建库（步骤见上，需用户确认执行）
3. 自底向上写代码：model → config → database（连接/迁移/种子）→ repository → service → response → handler → router → main（每层可独立编译）
4. 写 README、.gitignore、.env.example
5. 启动 + 验证

## 验证方案

```bash
go run ./cmd/server

# 1. 快乐路径：与样例逐字段一致（total=2312, above7=53, upCount=1553...）
curl -s 'http://localhost:8080/handicap/v1/index-points/houses_up_or_down?secuMarket=000001&range=today'

# 2. 无数据路径：HTTP 200 + data:null
curl -i 'http://localhost:8080/handicap/v1/index-points/houses_up_or_down?secuMarket=000001&range=week'

# 3. 参数错误：400
curl -i 'http://localhost:8080/handicap/v1/index-points/houses_up_or_down?secuMarket=&range=today'
curl -i 'http://localhost:8080/handicap/v1/index-points/houses_up_or_down?secuMarket=000001&range=year'

# 4. 数据库核对：种子只插入 1 行
mysql -u root -e "SELECT secu_market, stat_range, total, stat_date FROM handicap_db.house_up_down_stats;"

# 5. 重启幂等：Ctrl+C 后再次启动，接口结果不变（种子跳过）
```

## 风险与说明

- 执行阶段需安装 MySQL（系统级操作），安装耗时较长，逐个步骤确认
- 若 `caching_sha2_password` 认证失败，兜底 `ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY '...'`
- README 记录：错误 1045（密码错）/ 2002（MySQL 未启动）；跨天重启返回最近一天数据的行为约定