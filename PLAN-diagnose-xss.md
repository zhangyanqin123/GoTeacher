# 诊股模块富文本 XSS 加固 + remark 富文本化（第二期）

## Context（背景）

第一期诊股模块已上线（commit `f639d68` / `644d12f`）。用户报告两个问题：
1. **remark 显示异常**：C 端用户提交的 remark 是富文本（`<p>test</p>`），但列是 VARCHAR(200)、前端 `{{ }}` 插值 → 显示原始标签。产品定位已确认：**remark = 富文本**。
2. **存储型 XSS 链路**：reportContent/rejectReason 是 TinyMCE 富文本，前端 B/D/E 弹窗 `v-html` 直接渲染、后端零净化 → `<script>`/`onerror` 可完整落库并执行。

用户决策：防线**后端 bluemonday 白名单为主 + 前端 DOMPurify 兜底**（纵深防御）；范围**仅诊股模块**（全项目另有约 30 处 v-html 属存量问题，列入后续任务）。

**边界确认（已核验）**：本仓库 4 个接口无 diagnose.remark 写入口（remark 由 C 端写入，不在本仓库）→ remark 只能靠**读路径净化 + 前端兜底**；本服务写入的 reportContent/rejectReason 走**写路径净化**。

## 净化矩阵

| 字段 | 写入方 | 前端渲染 | 后端处理 | 前端处理 |
|---|---|---|---|---|
| reportContent | 本服务 submitReport | B/D/E v-html | 写路径净化（净化先于判空） | v-html 前包 sanitize() |
| rejectReason | 本服务 audit reject | E 表格列 | 写路径净化 | 表格列 slot + clamp + tooltip |
| diagnose.remark | C 端（外部） | A/B/D/E `{{ }}` | **读路径**净化（list/detail 返回前） | `{{ }}`→v-html + sanitize |
| audit_log.remark | 本服务/种子/C端 | E 表格列 | detail 读路径统一净化（幂等） | 同 rejectReason |

## 关键决策

| 决策点 | 结论 |
|---|---|
| bluemonday 版本 | v1.0.27（≥v1.0.26 含 mXSS 修复与 AllowStylePropertiesOnElements；Go 1.24 兼容） |
| 白名单 | UGCPolicy 基线 + 补 div/span/table 系/hr/sub/sup/del/ins + td/th 的 colspan/rowspan(Integer) |
| style 策略 | **受限保留**：AllowStylePropertiesOnElements 只放行 color/background-color/font-size/font-weight/font-style/text-align/text-decoration × p,span,div,li,td,th,h1-h6（TinyMCE 颜色/字号工具栏依赖；bluemonday CSS 解析器剥 url()/expression()/函数值，无执行面） |
| 净化先于判空 | 纯恶意标签内容 → 400（而非落库空串），有意收紧，PLAN 记录 |
| remark 列迁移 | schema.sql 改 TEXT + database.go 加幂等 ALTER（INFORMATION_SCHEMA 查 DATA_TYPE，已是 text 跳过；ErrNoRows 跳过）。本库 MySQL 8.0.11 **不支持 TEXT DEFAULT ('')**（需 ≥8.0.13）→ TEXT 不加 DEFAULT，对齐 report_content 既有惯例 |
| dompurify 版本 | **^2.4.7**（engines node >=8.9 <16；v3 需现代工具链不兼容） |
| 前端策略 | FORBID_TAGS 黑名单（script/iframe/object/embed/form/input/button/select/textarea/link/meta/base/style）+ ALLOW_DATA_ATTR:false，与后端白名单互补；**保留 style**（后端已限属性集） |
| 列表 reportContent | 读路径不净化（本服务已净化、避免每页 10 行大 HTML 逐行扫描）；弹窗 B 用行数据前包 sanitize 兜底 |
| E 表格 tooltip | show-overflow-tooltip 对富文本 slot 不兼容 → el-tooltip + `-webkit-line-clamp:2` + content 传 htmlToText 纯文本（tooltip 天然无 XSS 面） |
| 种子数据 | 不动（纯文本在 v-html 下按文本渲染正确） |

## 复用/参考

- 后端净化入口统一 `internal/sanitize.RichText(s)`（独立包，为将来 C 端写路径复用同白名单预留）
- `service.stripHTML` 判空逻辑不变（作用在净化后的值上）
- 前端封装 `src/utils/sanitize.js` 导出 `sanitizeHtml`；组件内 methods `sanitize`/`htmlToText`

---

## 阶段 1：后端净化（bluemonday + 写/读路径）

**新** `internal/sanitize/sanitize.go`：
```go
var (once sync.Once; policy *bluemonday.Policy)
func RichText(s string) string  // sync.Once 懒加载；空串直返 ""；对自身输出幂等
// buildPolicy: UGCPolicy() 基线
//   + AllowElements("div","span","table","thead","tbody","tfoot","tr","td","th","hr","sub","sup","del","ins")
//   + AllowAttrs("colspan","rowspan").Matching(bluemonday.Integer).OnElements("td","th")
//   + AllowStylePropertiesOnElements(安全属性集 × 元素集)  // 见决策表
//   不放行：script/iframe/object/embed/form/style 标签、link/meta/base、所有 on* 事件属性
```

**新** `internal/sanitize/sanitize_test.go`（golden 用例，也是阶段 4 curl 断言来源）：

| 输入 | 期望 |
|---|---|
| `<p>ok<script>alert(1)</script></p>` | `<p>ok</p>` |
| `<img src=x onerror=alert(1)>` | `<img src="x">` |
| `<a href="javascript:alert(1)">c</a>` | href 剥离（保留 rel=nofollow） |
| `<span style="color:red;background:url(javascript:1)">t</span>` | `<span style="color:red;">t</span>` |
| TinyMCE 正常样本 | 原样保留 |
| `RichText(RichText(x)) == RichText(x)` | 幂等通过 |

**改** `internal/service/diagnose.go` 4 处：
- 写路径：`SubmitDiagnoseReport` 判空前 `req.ReportContent = sanitize.RichText(...)`；`AuditDiagnose` reject 分支判空前净化 `req.RejectReason`
- 读路径：`ListDiagnoses` 循环净化 `list[i].Remark`；`GetDiagnoseDetail` 净化 `it.Remark` + 循环净化 `logs[i].Remark`

验证：`go build/vet/test ./...` 全绿；起服务（SERVER_PORT=8081，8080 是用户进程）后 curl：
- submitReport 注入 `<p>ok<script>alert(1)</script></p>` → 200，detail 返回 `<p>ok</p>`
- audit reject 注入 `<img src=x onerror=alert(1)>` → 落库 `<img src="x">`（查库断言）
- 全 `<script>` 内容提交 → 400（净化收紧判空，预期变化）
- dev 库手工 `UPDATE diagnose SET remark='<p>t<script>x</script></p>' WHERE id=6` → list/detail 返回已净化

## 阶段 2：remark 列迁移 TEXT

**改** `internal/database/schema.sql` L108：`remark TEXT NOT NULL COMMENT '用户备注（富文本 HTML）'`（新库定义，与 ALTER 逐字一致）
**改** `internal/database/database.go`：`Migrate()` 追加 `migrateDiagnoseRemark(db)`：
```go
// SELECT DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS
//   WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='diagnose' AND COLUMN_NAME='remark'
// ErrNoRows→nil（表未建，交给 schema）；已是 text→nil（幂等出口）；
// 否则 ALTER TABLE diagnose MODIFY COLUMN remark TEXT NOT NULL COMMENT '用户备注（富文本 HTML）'
```
VARCHAR→TEXT 放宽转换保留数据。

验证：重启服务 → `SHOW COLUMNS FROM diagnose LIKE 'remark'` 为 text；再重启不重复 ALTER（幂等）；升级前后 `SELECT remark` 数据不变。

## 阶段 3：前端 DOMPurify + 渲染改造

**改** `gyz-admin/package.json`：dependencies 加 `"dompurify": "^2.4.7"`（node 14 下 `npm install`，更新 lockfile）
**新** `src/utils/sanitize.js`：
```js
import DOMPurify from 'dompurify'
const DEFAULT_CONFIG = { FORBID_TAGS: ['script','iframe','object','embed','form','input','button','select','textarea','link','meta','base','style'], ALLOW_DATA_ATTR: false }
export function sanitizeHtml(html, config) { return !html ? '' : DOMPurify.sanitize(String(html), { ...DEFAULT_CONFIG, ...config }) }
```
**改** `src/views/dxData/diagnoseSys/diagnoseQuery.vue`：
- import sanitizeHtml + methods `sanitize(html)` / `htmlToText(html)`（去标签纯文本，tooltip 用）
- 模板 7 处：
  - L88/L110/L143/L163 备注 `{{ }}` → `<div class="rich-content" v-html="sanitize(xxxDialog.data.remark)" />`（A/B/D/E 四弹窗）
  - L112/L146/L166 诊股内容裸 v-html → 包 `sanitize(...)`
  - L179 表格列：去 `:show-overflow-tooltip`，改 slot + el-tooltip（content 传 htmlToText 纯文本，open-delay 300，空值 disabled）+ `.log-remark-cell`（line-clamp:2 + /deep/ p margin:0 + img max-width）
- 可选：Tinymce/index.vue init 加 `invalid_elements: 'script,iframe,object,embed,form,input'`（编辑器体验项，非防线）

验证：静态（项目禁跑 dev/lint，由用户手动）：无残留 `{{ ...remark }}` 插值、import 正确；浏览器手测：四弹窗 remark 富文本渲染（加粗/颜色生效）、E 表格两行截断 + hover 纯文本全文、reportContent 样式不回归；兜底演练：库里手工注入 `<script>` → 页面不执行。

## 阶段 4：验证收尾

- 复跑阶段 1 全部 curl（双防线确认）；`go build/vet/test`
- **改** `PLAN-diagnose.md`：追加「富文本 XSS 加固」章节（净化矩阵、policy 说明、迁移说明、C 端复用 internal/sanitize 建议、其余 30 处 v-html 范围外记录）
- git status 核对：后端 6 文件、前端 3 文件 + lockfile；提交（前端 `--no-verify`）
- dev 库阶段 1 手工 UPDATE 的脏 remark 还原为种子值

## 风险

1. dompurify 必须 ^2.4.7（v3 不兼容 node 14）
2. ALTER 窗口：本表量级小风险低；C 端生产库大则低峰手动 ALTER（幂等检查兼容）
3. 净化收紧：纯恶意内容 400，联调期可能误判为回归，PLAN 已记录
4. 双净化幂等：bluemonday 对自身输出幂等（单测固化）
