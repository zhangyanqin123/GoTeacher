# 老师离职转移接口实施记录（PLAN-resign）

> 前端 `gyz-admin/src/api/dxData/chatSys/resign.js` 的 mock 落地为 Go 后端真实接口。
> 接口路径/参数/响应结构与前端 mock 严格一致，前端仅删 mock 段启用 `request` 调用，调用处零改动。

## 接口清单

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | GET | `/api/v1/dxsf/chatSys/resign/list` | 离职转移记录列表（分页 + 多条件筛选） |
| 2 | POST | `/api/v1/dxsf/chatSys/resign/add` | 新增离职转移 |

## 设计决策

### 1. transfer_content 存逗号分隔 VARCHAR

库存 `'group,friend'`，接口输出 `["group","friend"]`。用 `model.StringSlice`（sql.Scanner + driver.Valuer）
在扫描点/写库点转换，风格对齐已有的 `model.DateTimeString`。不引入 JSON 列：与项目"原生 SQL、无 ORM"的
简单风格一致，且白名单只有两个值，无需 JSON 的表达能力。

### 2. 姓名/部门为冗余快照，后端回查

离职后老师记录可能被改/删，转移记录需保留"当时"的值，因此表里冗余存储。add 时后端从 `teacher` 表回查
姓名/部门（`GetTeachersByIDs` 一次 IN 查询拿两个老师），**前端传的 4 个冗余字段一律忽略**——单一事实来源，
避免前端伪造。`original_teacher_dept_id` 一并落库，供 deptId 筛选（与 mock 按 originalTeacherDeptId 过滤同构）。

### 3. salesman 取原老师全部绑定业务员

mock 里 salesman 是随机池；真实语义定为：`teacher_sales JOIN sales_user` 取原老师全部绑定业务员，
姓名/部门各自逗号拼接（一一对应），未绑定存空串。列宽 VARCHAR(500) 兜底（25 业务员全绑也放得下），
salesmanName 模糊查询 LIKE 天然兼容逗号串。

### 4. groupCount/friendCount 可选携带、缺省 0

系统暂无客户群/好友业务表，后端无法算出真实数量，不造假随机数（mock 的做法不进真实库）。接口接受非负
可选值（负数 400），前端当前不传即存 0；将来 IM 侧回调出真实数量时表结构不用动。

### 5. ORDER BY id DESC

mock 的 add 是 `unshift` 到队首（最新在上），DESC 与之同构；审计日志类列表惯例也是最新在前。

### 6. 复用既有代码

- `ErrTeacherNotFound` 复用 service/teacher.go（同包共享，勿重复声明）
- `normalizePage` / `defaultListPageSize` / `PageResult` / `DateTimeString` / `queryPage` 全部复用
- operator 无登录态固定 `admin`，对齐 UpdateTeacher 的 update_by 先例

## 踩坑记录

1. **多 seed 文件不要共用判空**：每张表独立 COUNT 判空、独立事务（沿用 PLAN-teacher 踩坑 5 的教训），
   老库升级无需动 teacher 表数据。
2. **repository/resign.go 初版多 import 了 `database/sql`**（以为要用 sql.ErrNoRows，实际 QueryContext
   无行返回空循环），go vet 报 imported and not used，删除即可。
3. **本地 8080 端口被调试二进制占用**（`cmd/server/__debug_bin*`，VS Code debug 残留），导致新代码起的
   服务 bind 失败、请求打到旧进程返回 404。排查：`lsof -nP -iTCP:8080 -sTCP:LISTEN`，kill 后重启。
   （该调试产物已加入 .gitignore 关注列表，未提交。）
4. **前端 dev server 需要 node 14**：项目 engines `>=8.9 <16`，node 22 下 `error:0308010C`（openssl legacy）
   + node-sass 4 不兼容。用 `~/.nvm/versions/node/v14.21.3/bin` 启动正常。

## 验证记录

- curl 矩阵：默认分页 6 条 DESC / pageSize=2&pageIndex=4 空页 / deptId=3 → 3 条 / originalTeacherId=4 → 1 条 /
  replaceTeacherId=1 → 1 条 / salesmanName 模糊 → 1 条 / 时间 07-31~08-01（DATE_ADD 闭区间边界）→ 仅 101 /
  deptId=abc → HTTP 400 —— 全部通过
- add 分支：成功（业务员全量逗号串、忽略前端冗余值、operateIp=127.0.0.1、transferTime=NOW）/
  同老师 400 / 空内容 400 / 白名单外 400 / remark>200 字 400 / 老师 999 → 404 / 负数计数 400 /
  可选携带 friendCount=66 入库 —— 全部通过；错误用例不入库（行数核对 8 = 6 种子 + 2 成功）
- 浏览器端到端：列表加载/筛选（原老师=赵丽 → 1 条）/ 新增弹窗提交 → msgSuccess + 弹窗关闭 + 列表刷新，
  新记录 id=109 落库正确（陈芳 → 王强，业务员 6 人逗号串，operate_ip=::1），Console 0 errors

## 文件清单

| 文件 | 类型 | 说明 |
| --- | --- | --- |
| `internal/database/schema.sql` | 改 | 追加 `teacher_resign` DDL |
| `internal/database/resign_seed.sql` | 新 | 种子 6 条（照抄 mock id 101-106） |
| `internal/database/database.go` | 改 | embed resign_seed.sql + seedResign |
| `internal/model/resign.go` | 新 | Resign / ResignInsert / ResignAddReq / ResignListFilter / TeacherBrief / TeacherSalesmanBrief |
| `internal/model/stringslice.go` | 新 | StringSlice（Scanner + Valuer） |
| `internal/repository/resign.go` | 新 | ListResigns / resignWhere / GetTeachersByIDs / ListTeacherSalesmen / InsertResign |
| `internal/service/resign.go` | 新 | ListResigns / AddResign / normalizeTransferContent + 4 个业务错误 |
| `internal/handler/resign.go` | 新 | List / Add（错误映射 400/404/500） |
| `internal/router/router.go` | 改 | chat 组注册 2 行 |
| `gyz-admin/src/api/dxData/chatSys/resign.js` | 改 | 删 mock，启用 request 调用（baseURL=VUE_APP_LOCAL_API） |
