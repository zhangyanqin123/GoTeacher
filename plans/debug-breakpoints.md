# 断点调试指南：为什么断点会导致 `context canceled`，以及正确的调试姿势

## 背景

调试 `POST /api/v1/dxsf/chatSys/resign/add` 时，在 `GetTeachersByIDs` 打断点，恢复执行后请求走进了错误分支：

```go
// internal/repository/resign.go:111
if err != nil {
    return nil, fmt.Errorf("query teacher briefs: %w", err)  // err = context canceled
}
```

**这不是 SQL 或表结构问题**（同款 SQL 手动执行毫秒级返回），是请求上下文已被取消。断点本身无害，有害的是「冻结期间客户端断开」。

## 机制链条

1. dlv 命中断点后**冻结整个进程**——所有 goroutine 停摆，包括 http server 的连接监视协程
2. 冻结期间，发请求的客户端等不及了：前端 axios 自带超时（通常 10~30s）、浏览器刷新、或 curl 被 Ctrl+C——TCP 连接断开
3. 恢复运行后，`net/http` 的连接监视协程读到断开，**cancel 掉 `c.Request.Context()`**
4. 之后所有携带该 ctx 的 DB 调用（`QueryContext(ctx, ...)`）立刻返回 `context.Canceled`，走进 `internal/repository/resign.go:111` 的错误分支

关键结论：**SQL 与表结构无问题，是 ctx 已失效。** 原生 curl 默认无超时——只要不 Ctrl+C，断点暂停多久都行，恢复后请求照常走完（已实测验证）。

## 正确的断点姿势

### 1. 用终端原生 curl 触发请求

**不加 `-m` 超时、不要 Ctrl+C**。断点暂停期间 curl 会一直等，恢复后请求照常完成并拿到响应。

```bash
curl -s -X POST 'http://127.0.0.1:8080/api/v1/dxsf/chatSys/resign/add' \
  -H 'Content-Type: application/json' \
  --data-raw '{"originalTeacherId":1,"replaceTeacherId":2,"transferContent":["group"],"remark":"debug-test"}'
```

注意：
- 不加 `-m 10` 之类的超时参数
- 不 Ctrl+C / 不关终端
- 挂着等，断点释放后自然拿到 `{"code":200,"msg":"转移成功","data":null}`

### 2. 前端联调深调试时不要走浏览器 UI

浏览器里的前端页面（axios 默认超时，通常 10~30s）会在断点冻结期间超时断开。深调试时改用上面的无超时 curl 重放同一请求（可从浏览器 DevTools → Network → Copy as cURL 拿到完整命令，删掉其中的 `-m`/`--max-time` 参数）。

### 3. 判定 ctx 是否已断开

断点下探到 repository 层后，在调试控制台（DEBUG CONSOLE）执行：

```
ctx.Err()
```

- 返回 `context.Canceled` → 客户端已断开，本次请求注定失败。**重发请求即可，无需重启服务**（服务本身完好，只是这一条请求的 ctx 失效）
- 返回 `<nil>` → ctx 正常，继续单步

## 不冻结进程的替代手段

断点冻结整个进程，对 HTTP 服务是「核武器」。多数调试场景有更温和的选择：

| 手段 | 适用场景 | 用法 |
|---|---|---|
| **Logpoint（记录点）** | 观察中间值，不需要暂停 | VSCode 行号右侧 → 「添加记录点」，填 `{err}` 等表达式，进程不停只打日志 |
| **条件断点** | 断点在循环/高频调用里，只关心特定条件 | 右键断点 → 编辑条件，如 `err != nil`，缩小冻结范围到真正关心的那次调用 |
| **命中次数** | 只关心第 N 次命中 | 右键断点 → Hit Count |
| **Debug Test** | 调试纯逻辑（不涉及 HTTP 层） | 测试函数上方 `run test` 旁的 `debug test`，断点跑单测，完全不经过 HTTP 客户端 |

现有可 Debug Test 的样例：`internal/sanitize/sanitize_test.go`。若未来给 service 层加集成测试（连本地 MySQL），深调试应优先走这条路，彻底摆脱 HTTP 客户端超时问题。

## 写操作的数据安全

以 `TransferResign`（`internal/repository/resign.go:169`）为例：

- 单事务原子：删重叠绑定 + 移剩余绑定 + 落 teacher_resign 快照，三者同一事务
- 断点停在事务内、恢复后即使 ctx 断开，commit 前失败即**整体回滚**，不会写半截状态
- 唯一要留意的是**成功提交后**的测试数据：调试请求会真实落库（如 teacher_resign 记录 + 绑定真实转移）。清理示例：

```sql
-- 删除调试产生的转移记录
DELETE FROM teacher_resign WHERE remark = 'debug-test';
-- 如需还原绑定归属（视实际转移内容）
-- UPDATE teacher_sales SET teacher_id = <原老师ID> WHERE teacher_id = <接替老师ID>;
```

## 故障速查表

| 现象 | 结论 | 处理 |
|---|---|---|
| curl 卡住不动 | 多半进程被断点冻结，不是服务挂死 | 检查 VSCode 是否停在断点上；确认要继续就按 F5 放行 |
| 日志/错误分支见 `context canceled` | 客户端先断开了（断点冻结超时 / axios 超时 / Ctrl+C） | 检查是否有未释放的断点；用无超时 curl 重发请求 |
| 断点不命中 | 请求路径没走到，或进程非 debug 模式启动 | 确认用 F5（Debug Server）启动而非 `go run`；检查断点是否在真实执行路径上 |
| 8080 无进程监听 | 服务没起或已退出 | 查看 VSCode 调试控制台报错；注意 `go run` 产生的临时进程名不含 "server" |

## 现有调试入口

`.vscode/launch.json` → `Debug Server (bindSales)` 配置：

- `program: cmd/server`，`envFile: .env`，F5 启动
- `dlvFlags: ["--check-go-version=false"]`：本机 dlv 1.27.1 要求 Go >= 1.25，项目为 1.24.2，跳过版本校验（已实测可正常断点）
