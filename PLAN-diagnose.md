# 诊股记录接口实施记录（PLAN-diagnose）

> 前端 `gyz-admin/src/api/dxData/diagnoseSys/diagnose.js` 的 mock 落地为 Go 后端真实接口。
> 接口路径/参数/响应结构与前端 mock 严格一致，前端仅删 mock 段启用 `request` 调用
> （baseURL 走 `VUE_APP_LOCAL_API`，对齐 teacher.js / resign.js 模式），调用处 `diagnoseQuery.vue` 零改动。

## 接口清单

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | GET | `/api/v1/dxsf/diagnose/list` | 诊股记录列表（分页 + 12 条件筛选） |
| 2 | GET | `/api/v1/dxsf/diagnose/detail` | 诊股详情（主表全字段 + 审核流程记录） |
| 3 | POST | `/api/v1/dxsf/diagnose/submitReport` | 提交诊股报告（首次编写 / 重新提审 → 状态 2） |
| 4 | POST | `/api/v1/dxsf/diagnose/audit` | 审核诊股报告（通过 / 驳回） |

## 状态机

```
[1 待诊股] ──submitReport──► [2 待专业审核] ──professional pass──► [4 待合规审核] ──compliance pass──► [6 终态]
                                │ professional reject                          │ compliance reject
                                ▼                                             ▼
                             [3 专业审核不通过] ──submitReport──► 2         [5 合规审核不通过] ──submitReport──► 2
```

- submitReport 仅允许状态 1/3/5（2/4/6 严格拒绝：在途重提使审核对象错位、审计链断裂；6 为终态）
- professional 审核仅允许状态 2；compliance 审核仅允许状态 4（顺序错位拒绝）

## 设计决策

### 1. 审核流程日志落库，不做读取时推导

mock 的 `buildAuditLogs` 是按当前状态实时推导的。真实系统若照搬，**驳回后重新提审会丢掉首次驳回
记录**（推导视图只剩最后一次审核），`rejectReason` 也无处保存。因此建 `diagnose_audit_log` 表，
submitReport / audit 在同一事务内写日志，detail 读路径纯 SELECT（`ORDER BY id` 正序 = 时间序）。
'用户提交' 日志只由种子（及将来 C 端创建链路）产生，本任务 4 个接口无创建入口。

### 2. 状态机用"写前查询 + 事务内条件 UPDATE"双保险

- 写前 `GetDiagnose`：区分 404（记录不存在）与 400（状态不允许），并取 `teacher_name` 供日志 operator
- 事务内条件 UPDATE（`WHERE status IN (1,3,5)` / `WHERE status = ?`）作并发守卫：
  `RowsAffected == 0` 回滚返回 false → service 转 `ErrInvalidStatusTransition`。
  纯 SELECT-then-UPDATE 有 TOCTOU（并发双通过）；纯 RowsAffected 分不清 404/400

### 3. log_type / result 存中文展示串

值域就是前端硬编码的展示文案（用户提交/诊股报告/专业审核/合规审核；已提交/通过/不通过），
永不下发回后端。对齐 `teacher.qualification` 存中文先例，读取零转换；写入侧是固定字面量无脏值风险。

### 4. 列型选择

- `buy_price DECIMAL(10,2)`：金额不用浮点列；驱动以 []byte 返回 DECIMAL，database/sql 直接
  convertAssign 到 float64，JSON 输出 `1680.5` 与 mock 同构
- `report_content` / 日志 `remark` 用 `TEXT NOT NULL`：富文本 HTML；避免 NULL 扫普通 string 报错
- `report_submit_time DATETIME NULL`：NULL=未提交，复用 `DateTimeString`（Scan nil 分支 → ""），
  前端空串即"未提审"，与 mock 同构
- 模糊查询列（昵称/姓名/代码/名称/老师）不建索引：前导通配 LIKE 打不进 B-tree；
  只建 status / submit_time / report_submit_time

### 5. 可选数值筛选用指针

`DiagnoseListFilter` 的 ID/BuyPrice/BuyNum/Status 用 `*int64/*float64/*int`：前端 `isEmpty('')`
语义下传 "0" 是有效过滤值，值类型分不清"未传"与 0（对齐 teacher.go `BindSalesCount *int` 先例）。
string→数值转换放 handler（失败即 400），service 只做白名单/语义校验。

### 6. 富文本判空

`stripHTML` 去标签 + 替换 `&nbsp;` 后 TrimSpace 判空，与前端 `isRichEmpty` 同构；
reportContent 与 rejectReason 共用（`<p>&nbsp;</p>` 视为未填写）。

### 7. 复用既有代码

- `normalizePage` / `defaultListPageSize` / `PageResult` / `DateTimeString` / `queryPage` 全部复用
- `response.OKMsg` 的 msg 按前端 mock 约定："success" / "提交成功" / "审核通过" / "已驳回"
- 审核通过文案 `proPassRemark`/`compPassRemark` 与种子 SQL 硬编码同值（'报告专业、结论合理' / '内容合规'），
  改动需两处同步

### 8. detail 不存在收紧为 404

mock 原为 200 + data:null；收紧为 404 对齐 teacher/resign 先例。前端只用行 id 调用（列表行必存在），
无副作用。

## 踩坑记录

1. **验证时旧进程占用验证端口**：阶段 1 验证用的 8081 服务昨晚未退出，次日新代码起服务 bind 失败、
   curl 打到旧进程返回 404 page not found。排查：`lsof -nP -iTCP:8081 -sTCP:LISTEN` 对比
   `ps` 启动时间，清掉旧 `go run` 及其子进程再起。8080 上还有用户自己的旧服务（非本会话启动），
   本任务一律用 `SERVER_PORT=8081` 验证，不动 8080。
2. **多 seed 文件每表独立 COUNT 判空**（沿用 PLAN-resign 踩坑 1）：seedDiagnose 只查 diagnose 表行数。
3. **TEXT NOT NULL 无默认值**（MySQL 8 限制）：INSERT 必须显式传值，Go 零值 `''` 天然满足；
   种子 SQL 同理全部显式写 `''`/实值。

## 验证记录

- 建表/种子：启动后 MySQL 核对 diagnose 6 行（状态 1-6 各一）、diagnose_audit_log 17 行；
  再次重启无重复插入（幂等）
- 列表 curl 矩阵：默认 count=6 且 id DESC / pageSize=2&pageIndex=2 → [4,3] / stockName=五粮 → [2] /
  status=6 → [6] / buyPrice=1680.5&buyNum=100 精确 → [1] / submit 08-01~08-10 闭合区间 → [1,2,3,4] /
  report 07-25~07-31 → [5]（未提审的 1 天然排除）/ teacherName=哥 → [1,4] / id=3 → [3] /
  userNickName=投资 → [2] —— 全部通过
- 非法参数：buyPrice=abc / id=abc → 400；status=9 → 400；detail 缺 id → 400；detail?id=999 → 404
- detail?id=5：全字段 + 4 条正序日志（用户提交→诊股报告→专业审核通过→合规审核不通过）；
  detail?id=1：reportSubmitTime=""、reportContent=""（NULL→空串）
- 正向链 id=1：提交→2→专业通过→4→合规通过→6，日志 1→4 条
- 驳回链 id=2：专业驳回（rejectReason 落库）→3→重新提审→2，**首次驳回记录保留**（日志 4 条，
  落库方案核心收益验证点）
- 拒绝矩阵：状态 2/4/6 提交 → 400；状态 3/5/6 审核 → 400；状态 2 做 compliance / 状态 4 做
  professional → 400；auditType/result 白名单外 → 400；reject 缺 reason / reason 全空白 → 400；
  空 reportContent / 全空白标签 → 400；id=999 提交/审核 → 404；body 格式错误 → 400 —— 全部通过
- 错误用例不入库：日志无孤儿行（error_orphan_logs=0），主表行数不变
- 前端：diagnose.js 无 mock 残留、4 个导出函数名/签名不变、diagnoseQuery.vue 零 diff；
  浏览器联调由用户手动执行（dev server 需 node 14）

## 文件清单

| 文件 | 类型 | 说明 |
| --- | --- | --- |
| `internal/database/schema.sql` | 修改 | 追加 diagnose / diagnose_audit_log 两表 DDL |
| `internal/database/diagnose_seed.sql` | 新增 | 6 条主表 + 17 条日志种子（照抄 mock） |
| `internal/database/database.go` | 修改 | embed diagnose_seed.sql + seedDiagnose() |
| `internal/model/diagnose.go` | 新增 | Diagnose / Filter / 请求体 / AuditLog / Detail |
| `internal/repository/diagnose.go` | 新增 | 列表/详情/条件 UPDATE + 事务写日志 |
| `internal/service/diagnose.go` | 新增 | 状态机 / 白名单 / 哨兵错误 / 富文本判空 |
| `internal/handler/diagnose.go` | 新增 | 4 个入口，query 解析 + errors.Is 映射 |
| `internal/router/router.go` | 修改 | 注册 `/api/v1/dxsf/diagnose` 路由组 |
| gyz-admin `src/api/dxData/diagnoseSys/diagnose.js` | 修改 | 删 mock，启用 LOCAL baseURL 真实调用 |
