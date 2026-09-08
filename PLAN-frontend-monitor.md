# PLAN: 前端监控（H5 探针 ingest + 查询/告警）

> 数据源：gyz-h5-personalcenter 内联探针 `window.__GYZMON__`（gif GET `?d=<urlencoded JSON>`）。
> 判型语义：**capbad 含 syntax = 机型语法不兼容（P0）**；src 404 = 部署问题；错误集中单版本 = 业务 bug。
> 完整排查手册见 H5 仓库 `GYZMON监控日志查询手册.md`（nginx access_log 手查，本服务上线前的过渡通道）。

## 表（schema.sql 末尾，Migrate 幂等）

- `mon_event`：探针 18 字段 + `cap_syntax` 派生列（写入时 `strings.Contains(capbad, "syntax")`——
  无序逗号串 LIKE 会漏判，精确匹配走此列）；索引 `(env,event_type,created_at)` / `created_at` /
  `session_id` / `capbad`；不建唯一键（gif 弱网重发容忍 at-least-once）
- `mon_alert`：`uk_dedup_pending(dedup_key, status)` 保证同规则+环境仅一条 pending——持续触发滚动
  UPDATE，ack 释放唯一键可再开新单；不自动关单，人工 ack 收口

## 接口

| 路径 | 鉴权 | 说明 |
|---|---|---|
| `GET /api/v1/mon/event?d=` | 公开 | 探针 gif 通道（返回 1x1 gif，任何异常静默丢弃仅记日志）；d>8KB 拒收 |
| `POST /api/v1/mon/event` | 公开 | 批量通道：raw body 手动 Unmarshal（sendBeacon 发 text/plain 不校验 CT），数组≤50 条 |
| `POST /api/v1/mon/event/list` | Auth | 多口径分页：时间强制（缺省近 24h、≤31 天）/event_types IN/env/ver 前缀/cv/sid 精确/mdl 模糊/**capbads FIND_IN_SET**/cap_syntax/rt/msg/src 模糊；list 不含 stack/cap/ua 全文（msg 100 字预览） |
| `GET /api/v1/mon/event/detail?id=` | Auth | 单行全字段（详情抽屉） |
| `POST /api/v1/mon/overview` | Auth | 指标卡（total/errors/sessions/devices/syntax/by_type）+ by_ver/by_mdl(含 syntax 数)/by_cv(升序)/by_route/by_src(resource_error) Top10；≤7 天 |
| `POST /api/v1/mon/alert/list` | Auth | 告警多条件分页 |
| `POST /api/v1/mon/alert/ack` | Auth | 确认（ack_user 取鉴权上下文；并发重复确认 404 语义） |

清洗链（service）：t 白名单（未知类型丢弃记日志）→ 全字段 rune 截断（与列宽一致）→ cap_syntax 派生。
**msg/stack 不过 sanitize.RichText**——bluemonday 会剥掉堆栈中的 `<anonymous>` 破坏诊断信息；
XSS 防线由前端 React 默认转义承担。

## 告警任务（service/mon_alert.go StartMonAlertJob）

- 运行：main `go monSvc.StartMonAlertJob(ctx)`（signal ctx + ticker，单 run panic recover）；单实例部署，
  扩容前需加分布式锁
- 规则（内置常量 + .env 阈值，不做规则 CRUD）：
  - `PROBE_FAIL`（P0）：窗口内 probe_fail > `MON_ALERT_PROBE_FAIL_THRESHOLD`（默认 0）→ detail 带 Top3 机型/cv/wv + 样例 sid
  - `CHUNK_LOAD_SURGE`（P0）：chunk_load_error > 10（默认）→ Top3 src 判 404 + Top ver 判发版
  - `ERROR_SURGE`（P1）：win+vue+unrejected 合计 > 50（默认）→ Top msg/路由
- 只做绝对阈值不做环比（当前错误从 0→N 跳变，环比分母为 0 抖动大）
- 保留清理：每日分批 `DELETE ... LIMIT 5000`（`MON_RETENTION_DAYS=90`）
- webhook：`MON_ALERT_WEBHOOK_URL` 非空时推送钉钉/企微 text 格式

## 前端（GoProject-web，React18+antd5）

`/monitor` 单页三 Tab（APP_PAGES 一行注册）：
- 概览：Statistic 指标卡 + 5 聚合表（零图表依赖）；机型行点击 → 下钻事件日志（机型+仅语法不兼容）
- 事件日志：12 字段多口径搜索（照 diagnose 模板）+ 详情抽屉（stack/cap JSON 格式化）
- 告警：pending 优先列表 + 确认弹窗（ack_note 必填）+ detail 摘要 Tooltip

## H5 切换（阶段四，待公网域名确认）

探针 endpoint 从随包 `./static/mon.gif` 切 `https://<域名>/api/v1/mon/event`（GET d= 协议不变，
仅改 gyz-h5-personalcenter 的 vite.config endpoints）；gif 保留作回滚通道。

## 风险与演进

- 公开接口防刷：截断+8KB 上限+IP 记录；量大后加 per-IP 限流（`MON_INGEST_RATE_PER_MIN` 预留）
- 写入量：boot≈每次 PV；PV >100 万/天需按天分区（重建表，第一天就决策）或 mon_stat_hourly 预聚合
- 探针新增事件类型：t 白名单会静默丢弃（服务端日志可见），探针侧同步通知加白名单
