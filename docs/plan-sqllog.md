# 方案：给所有 SQL 执行处增加日志输出

> 背景与设计决策存档。实施完成后保留供后续维护参考；改动接口前先读对应 PLAN-*.md。

## 背景与目标

项目用裸 `database/sql`（无 ORM），repository 层约 46 处 `QueryRowContext/QueryContext/ExecContext` 调用（含 3 处事务、8 处事务内 `tx.ExecContext`），此前完全没有 SQL 日志，排查问题时看不到实际执行的 SQL 与参数。

目标：一键打开调试开关即可在服务日志里看到全部 SQL（含参数、耗时、影响行数），默认关闭不影响正常运行。

## 选型结论

- **零新依赖**：手写 `database/sql/driver` 层包装器（Connector/Conn/Stmt），在 `database.Connect` 里用 `sql.OpenDB(包装 connector)` 替代 `sql.Open("mysql", dsn)`。repository / handler / service 层零改动，事务内 SQL、启动时 Migrate/Seed 全自动覆盖。
- **开关方式**：SQL 日志打 `slog.Debug`，`.env` 新增 `LOG_LEVEL`（默认 info），调试时设 `debug` 打开。与现有 slog 体系融合。

## 关键设计事实（决定日志点位置）

- 项目 DSN 未设 `interpolateParams`（internal/config/config.go 的 mysql.Config），go-sql-driver v1.10 在带参时 Conn 层返回 `driver.ErrSkip`，`database/sql` 随即走 **Prepare → Stmt.ExecContext/QueryContext** 路径。即：**带参 SQL（绝大多数）的日志点在 Stmt 层，无参 DDL/COUNT 在 Conn 层**，两层都要打。
- Conn 层透传 `ErrSkip` 时**绝不能打日志**（是路径探测，不是执行），否则同一条 SQL 打两条。
- 事务内 `tx.ExecContext` 复用同一个 driver.Conn，自然被覆盖，无需改动；但 `BEGIN/COMMIT/ROLLBACK` 不走 Exec/Query，需在包装的 `Tx.Commit/Rollback`、`Conn.Begin/BeginTx` 处单独打。
- mysql 驱动实现了 `SessionResetter`（丢失 → 连接池每次放回都关闭重连）、`Validator`、`Pinger`、`NamedValueChecker`（丢失 → `model.StringSlice` 等 Valuer 转换退化）——**必须全部委托**，靠编译期 `var _` 断言防手滑。

## 文件改动

| 文件 | 动作 | 要点 |
|---|---|---|
| `internal/database/sqllog.go` | **新增**（约 280 行） | 包装器全部类型 + 编译期接口断言 + formatArgs/logSQL |
| `internal/database/sqllog_test.go` | **新增** | 零依赖 fake 桩 + 表驱动断言 |
| `internal/database/database.go` | 修改 | Connect：`sql.Open` → `OpenConnector + sql.OpenDB`；blank import 改具名 `mysql` |
| `internal/config/config.go` | 修改 | Config 加 `LogLevel string`；Load 加 `getEnv("LOG_LEVEL", "info")` |
| `cmd/server/main.go` | 修改 | `config.Load()` 提到 `slog.SetDefault` 之前；handler 加 `Level: parseLevel(cfg.LogLevel)`；新增私有 `parseLevel`（`slog.Level.UnmarshalText`，非法值回落 info） |
| `.env.example` | 修改 | 加 `LOG_LEVEL=info` 及注释 |

## sqllog.go 结构

```
Connector{inner driver.Connector}
  NewConnector(inner)                     // Connect 成功后打 op=connect 日志（排查连接泄漏）
                                          // Driver 纯委托

Conn{inner driver.Conn}
  Prepare / Close                         // Prepare 不打日志，query 存入返回的 Stmt
  Begin / BeginTx                         // 打 BEGIN
  PrepareContext                          // 同 Prepare
  ExecContext / QueryContext              // 计时→调内层→ErrSkip 原样透传不打日志；否则打日志
  PingContext / ResetSession / IsValid / CheckNamedValue   // 纯委托（丢了连接池退化）

Stmt{inner driver.Stmt; query string}    ← 带参 SQL 主日志点
  ExecContext / QueryContext              // 主日志点：query 来自 Prepare 时的原文 + args + cost (+rows)
  Close / NumInput / Exec / Query / CheckNamedValue / ColumnConverter

Rows{inner; query; count; start; once sync.Once}
  Close                                   // 打 op=rows：行数+全程耗时（QueryContext 的 cost 只到首行，此处补全程）
  Next(count++) / Columns
  委托防将来退化：HasNextResultSet/NextResultSet/ColumnType* 系列

Result{inner} / Tx{inner}
  Tx.Commit/Rollback                      // 打 COMMIT / ROLLBACK

logSQL(op, query, args, cost, rows, err)  // 先 logger.Enabled(Debug) 再 formatArgs（级别关闭时零格式化成本）
formatArgs / formatValue / truncate       // nil→NULL、[]byte→string、time→"2006-01-02 15:04:05"、512 字节截断
```

日志格式（slog text）：

```
time=... level=DEBUG msg="[SQL]" op=exec query="UPDATE teacher SET ..." args="[...] " rows=1 cost=2.1ms
```

### database.go Connect 改造

```go
d := mysql.MySQLDriver{}                 // v1.10 实现了 driver.DriverContext
c, err := d.OpenConnector(cfg.DSN)       // eager 解析 DSN，错误处理结构不变
if err != nil { return nil, fmt.Errorf("parse mysql dsn: %w", err) }
db := sql.OpenDB(sqllog.NewConnector(c))
```

连接池参数（SetMaxOpenConns 等）与 PingContext 探活逻辑不变。

## 测试（sqllog_test.go）

桩用 `driver.Conn` 空值内嵌 + 只覆写被调方法（fakeConn/fakeStmt/fakeRows/fakeConnector），logger 用 `slog.NewTextHandler(&bytes.Buffer, &slog.HandlerOptions{Level: LevelDebug})` + `t.Cleanup` 恢复：

1. `TestExecContextLogsAndDelegates`：日志含 query/args/rows/cost，fake 收到相同参数
2. `TestQueryContextLogsAndDelegates`
3. `TestErrSkipNotLogged`：ErrSkip 原样透传且 buf 为空（**防双日志，最关键回归**）
4. `TestStmtExecLogs`：日志中 query 是 Prepare 时的 SQL 原文（主路径）
5. `TestTxBoundariesLogged`：BEGIN/COMMIT/ROLLBACK
6. `TestFormatArgs`（表驱动）：nil/[]byte/time/多参/截断
7. `TestOptionalInterfacesDelegated`：ResetSession/IsValid/PingContext/CheckNamedValue 逐个委托计数（防连接池退化）

## 风险点

1. ErrSkip 误处理 → 双日志或带参 SQL 直接报错（测试 3 覆盖）
2. 日志点只放 Conn 层会漏全部带参 SQL（测试 4 覆盖）
3. 级别关闭时仍 formatArgs → 每条 SQL 白拼字符串（先 Enabled 检查）
4. 可选接口丢失是静默退化（编译期断言 + 测试 7 双保险）
5. QueryContext 的 cost 只到首行（MySQL 流式），行数与全程耗时在 Rows.Close 补充

## 如何查看 Go 服务日志

当前日志全部打到 **stdout**（cmd/server/main.go 的 slog TextHandler → os.Stdout；gin.Default() 自带请求访问日志也在 stdout）：

- 前台运行直接看终端：`go run ./cmd/server`
- 落文件 + 实时跟踪：`go run ./cmd/server 2>&1 | tee logs/app.log`，另开窗口 `tail -f logs/app.log`
- 临时开 SQL 日志（命令行环境变量优先于 .env）：`LOG_LEVEL=debug go run ./cmd/server`

## 验证方式

1. `go build ./... && go vet ./... && go test ./...`
2. `LOG_LEVEL=debug go run ./cmd/server`，另开终端 curl：
   - `GET /api/v1/dxsf/chatSys/teacher/options` → 无参 SELECT，Conn.QueryContext 一条
   - `GET /api/v1/dxsf/chatSys/teacher/list?page=1&pageSize=10` → 带参，Stmt 层日志含 query 原文与 args
   - `POST /api/v1/dxsf/resign/add` → 事务，BEGIN + 多条 + COMMIT
   - 启动阶段 Migrate/Seed 的 SQL 也应出现在终端
3. 不设 LOG_LEVEL（默认 info）重复 curl：无 `[SQL]` 输出、接口行为不变
4. 连续多次请求后 `op=connect` 条数远小于 SQL 条数（SessionResetter 委托生效、连接池未退化）
