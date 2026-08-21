# 诊股记录接口实施记录（PLAN-diagnose）

> 前端 `gyz-admin/src/api/dxData/diagnoseSys/diagnose.js` 的 mock 落地为 Go 后端真实接口。
> 接口路径/参数/响应结构与前端 mock 严格一致，前端仅删 mock 段启用 `request` 调用
> （baseURL 走 `VUE_APP_LOCAL_API`，对齐 teacher.js / resign.js 模式），调用处 `diagnoseQuery.vue` 零改动。

## 接口清单

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | POST | `/api/v1/dxsf/teacher/diagnose/list` | 诊股记录列表（分页 + 12 条件筛选，条件走 JSON body，2026-08-21 由 GET 改 POST 对齐 resign/list） |
| 2 | GET | `/api/v1/dxsf/teacher/diagnose/detail` | 诊股详情（主表全字段 + 审核流程记录） |
| 3 | POST | `/api/v1/dxsf/teacher/diagnose/submitReport` | 提交诊股报告（首次编写 / 重新提审 → 状态 2） |
| 4 | POST | `/api/v1/dxsf/teacher/diagnose/audit` | 审核诊股报告（通过 / 驳回） |

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

### 第一期（接口落地）

| 文件 | 类型 | 说明 |
| --- | --- | --- |
| `internal/database/schema.sql` | 修改 | 追加 diagnose / diagnose_audit_log 两表 DDL |
| `internal/database/diagnose_seed.sql` | 新增 | 6 条主表 + 17 条日志种子（照抄 mock） |
| `internal/database/database.go` | 修改 | embed diagnose_seed.sql + seedDiagnose() |
| `internal/model/diagnose.go` | 新增 | Diagnose / Filter / 请求体 / AuditLog / Detail |
| `internal/repository/diagnose.go` | 新增 | 列表/详情/条件 UPDATE + 事务写日志 |
| `internal/service/diagnose.go` | 新增 | 状态机 / 白名单 / 哨兵错误 / 富文本判空 |
| `internal/handler/diagnose.go` | 新增 | 4 个入口，list 走 JSON body，errors.Is 映射 |
| `internal/model/flexint.go` | 修改 | 曾加 FlexFloat64 供 buy_price 宽容解析，2026-08-21 契约收紧为只收 JSON 数值后删除（唯一使用者消失） |
| `internal/router/router.go` | 修改 | 注册 `/api/v1/dxsf/teacher/diagnose` 路由组 |
| gyz-admin `src/api/dxData/diagnoseSys/diagnose.js` | 修改 | 删 mock，启用 LOCAL baseURL 真实调用 |

### 第二期（XSS 加固 + remark 富文本化）

| 文件 | 类型 | 说明 |
| --- | --- | --- |
| `internal/sanitize/sanitize.go` | 新增 | bluemonday 白名单策略，入口 `RichText()` |
| `internal/sanitize/sanitize_test.go` | 新增 | 注入用例 golden 断言 + 幂等性 |
| `internal/service/diagnose.go` | 修改 | 写路径 2 处 + 读路径 2 处净化接入 |
| `internal/database/schema.sql` | 修改 | diagnose.remark 改 TEXT |
| `internal/database/database.go` | 修改 | migrateDiagnoseRemark 幂等 ALTER |
| `go.mod` / `go.sum` | 修改 | + bluemonday v1.0.27 |
| gyz-admin `package.json` / lockfile | 修改 | + dompurify ^2.4.7 |
| gyz-admin `src/utils/sanitize.js` | 新增 | DOMPurify 封装 sanitizeHtml/htmlToText |
| gyz-admin `src/views/dxData/diagnoseSys/diagnoseQuery.vue` | 修改 | 7 处模板 + methods + 样式 |

## 富文本 XSS 加固 + remark 富文本化（第二期）

> 背景：C 端提交的 remark 为富文本（`<p>test</p>`），原 VARCHAR(200) + 前端 `{{ }}` 插值导致
> 显示原始标签；同时 reportContent/rejectReason 前端 v-html 直渲、后端零净化，构成存储型 XSS 链路。
> 方案：**后端 bluemonday 白名单为主 + 前端 DOMPurify 兜底**（纵深防御），范围仅诊股模块。

### 净化矩阵

| 字段 | 写入方 | 后端处理 | 前端处理 |
| --- | --- | --- | --- |
| reportContent | 本服务 submitReport | **写路径**净化（先于判空） | v-html 前包 sanitize() |
| rejectReason | 本服务 audit reject | **写路径**净化（先于判空） | E 表格 slot + line-clamp + el-tooltip(纯文本) |
| diagnose.remark | C 端（外部，本服务无写入口） | **读路径**净化（list/detail 返回前） | `{{ }}` → v-html + sanitize |
| audit_log.remark | 本服务/种子/C端 | detail 读路径统一净化（幂等） | 同 rejectReason |
| 列表 reportContent | 本服务 | 不二次净化（写路径已净化，避免每页大 HTML 逐行扫描） | 弹窗 B 渲染前 sanitize 兜底 |

### 关键决策

1. **bluemonday v1.0.27**：UGCPolicy 基线 + 补 div/span/table 系/hr/sub/sup/del/ins +
   td/th 的 colspan/rowspan(Integer)。style **受限保留**（`AllowStyles(...).OnElements(...)`，
   v1.0.27 API 是全局属性集×元素列表）：只放行 color/background-color/font-size/font-weight/
   font-style/text-align/text-decoration × p/span/div/li/td/th/h1-h6——TinyMCE 颜色/字号工具栏依赖；
   CSS 解析器剥 url()/expression()/函数值，无脚本执行面。不放行 script/iframe/object/embed/form/
   style 标签、link/meta/base、所有 on* 事件属性。
2. **净化先于判空**：纯恶意标签内容（如全 `<script>`）净化后为空 → 400 `reportContent must not be empty`
   （有意收紧，非落库空串——联调期勿误判为回归）。
3. **独立包 internal/sanitize**：不挂 service 下，为将来 C 端写路径复用同一白名单预留。
   入口唯一 `RichText(s)`，sync.Once 懒加载，对自身输出幂等（单测固化；读路径二次净化无畸变）。
4. **remark 列迁移**：schema.sql 改 `TEXT NOT NULL` + database.go `migrateDiagnoseRemark` 幂等
   ALTER（INFORMATION_SCHEMA 查 DATA_TYPE，已是 text 跳过；CREATE TABLE IF NOT EXISTS 不改存量表，
   必须靠此函数升级）。8.0.11 不支持 TEXT 表达式默认值 → 不带 DEFAULT，对齐 report_content 惯例。
   VARCHAR→TEXT 放宽转换保留数据。
5. **前端 dompurify ^2.4.7**（v3 不兼容 node 14）：FORBID_TAGS 黑名单与后端白名单互补，
   style 不禁（后端已限属性集）。封装 `src/utils/sanitize.js` 导出 `sanitizeHtml`/`htmlToText`。
6. **E 表格 tooltip**：show-overflow-tooltip 对自定义 slot 富文本不生效 → el-tooltip
   （content 传 `htmlToText` 纯文本，天然无 XSS 面，open-delay 300）+ `.log-remark-cell` 两行截断。

### 验证记录（第二期）

- 单测：`go test ./internal/sanitize/` 全绿（script 剥离 / onerror 剥离 / javascript: href 连 <a> 一并剥 /
  style 只留安全属性 / iframe 剥离 / 幂等性；注意实际输出格式为 `color: red` 无尾分号，
  `<a href="javascript:...">` 整个标签剥掉只剩文本——比预期更严格）
- 写路径 curl：submitReport 注入 `<script>+onerror` → 落库/返回均为 `<p>ok<img src="x"></p>`；
  audit reject 注入 iframe/javascript: → 日志 remark 为 `<p>依据不足</p>link`（查库断言）；
  全 `<script>` 内容 → 400
- 读路径 curl：库中手工注入脏 remark（script + background:url(javascript:)）→ list/detail 返回
  `<p>t<span style="color: red">ext</span></p>`
- 列迁移：重启后 `SHOW COLUMNS` remark 为 text、数据不变；二次重启不重复 ALTER（幂等）
- 前端静态：无残留 `{{ ...remark }}` 插值、9 处 sanitize 调用、dompurify 2.4.7 安装成功（node 14）；
  浏览器渲染由用户手动验证（项目禁跑 dev/lint）
- dev 库注入的测试数据已还原为种子状态

### 范围外（后续任务）

- 全项目其余约 30 处 v-html 直渲数据库内容（protocol、info/examine 等）未处理
- C 端写 remark 的接口（不在本仓库）建议复用 internal/sanitize 白名单入库净化



> **2026-08-18 字段命名整体迁移 snake_case**：本文中的驼峰字段名（userNickName/reportContent/auditLogs 等）已全部改为蛇形（user_nick_name/report_content/audit_logs），以 [PLAN-api-snake-case.md](PLAN-api-snake-case.md) 为准。
