# 老师离职转移接口实施记录（PLAN-resign）

> 前端 `gyz-admin/src/api/dxData/chatSys/resign.js` 的 mock 落地为 Go 后端真实接口。
> 接口路径/参数/响应结构与前端 mock 严格一致，前端仅删 mock 段启用 `request` 调用，调用处零改动。

## 接口清单

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | POST | `/api/v1/dxsf/teacher/resign/list` | 离职转移记录列表（分页 + 多条件筛选，条件走 JSON body） |
| 2 | POST | `/api/v1/dxsf/teacher/resign/add` | 新增离职转移 |

## 设计决策

### 1. transfer_content 存逗号分隔 VARCHAR

库存 `'group'`，接口输出 `["group"]`。用 `model.StringSlice`（sql.Scanner + driver.Valuer）
在扫描点/写库点转换，风格对齐已有的 `model.DateTimeString`。不引入 JSON 列：与项目"原生 SQL、无 ORM"的
简单风格一致，且白名单只有单个值，无需 JSON 的表达能力。

### 2. 姓名/部门为冗余快照，后端回查

离职后老师记录可能被改/删，转移记录需保留"当时"的值，因此表里冗余存储。add 时后端从 `teacher` 表回查
姓名/部门（`GetTeachersByIDs` 一次 IN 查询拿两个老师），**前端传的 4 个冗余字段一律忽略**——单一事实来源，
避免前端伪造。`original_teacher_dept_id` 一并落库，供 deptId 筛选（与 mock 按 originalTeacherDeptId 过滤同构）。

### 3. salesman 取原老师全部绑定业务员

mock 里 salesman 是随机池；真实语义定为：`teacher_sales JOIN sales_user` 取原老师全部绑定业务员，
姓名/部门各自逗号拼接（一一对应），未绑定存空串。列宽 VARCHAR(500) 兜底（25 业务员全绑也放得下），
salesmanName 模糊查询 LIKE 天然兼容逗号串。

### 4. groupCount 后端计算（原「可选携带、缺省 0」方案已废弃）

2026-08 变更：移除好友概念，业务确认**一个绑定的业务员对应一个客户群（诊股群）**，因此 groupCount 的真实
语义 = 原老师绑定的业务员数量。AddResign 内 `len(salesmen)` 直接得出（业务员快照本来就要查，零额外查询），
不接收前端传值。friendCount 字段与 `friend_count` 列整体删除，存量库由 `migrateTeacherResign` 幂等
DROP + 注释同步 + transfer_content 残留清洗；transferContent 白名单同步收敛为 `["group"]`（传 friend 400）。

### 5. ORDER BY id DESC

mock 的 add 是 `unshift` 到队首（最新在上），DESC 与之同构；审计日志类列表惯例也是最新在前。

### 6. 确认转移真实移动绑定（去重合并）

add 从「只写快照」升级为事务内真实转移 `teacher_sales`（老师列表 `bind_sales_count` 是子查询统计，
移动数据后计数自动生效：原老师归 0、接替老师增加，列表接口零改动）。三步同一事务，原子：

1. **删重叠**：`DELETE ts FROM teacher_sales ts JOIN teacher_sales ts2 ON ts2.teacher_id = ? AND ts2.user_id = ts.user_id WHERE ts.teacher_id = ?`——
   uk_teacher_user 允许不同老师绑同一业务员，去重合并 = 重叠者保留接替老师现有行（bind_time 不变）。
   MySQL 多表 DELETE 自连接合法（1093 只限单表 UPDATE/DELETE 的同表子查询）。
2. **移剩余**：`UPDATE teacher_sales SET teacher_id = ? WHERE teacher_id = ?`——整批迁移，
   行 id 与原 bind_time 保留（非重叠绑定的"原绑定时间"需求由 UPDATE 天然满足，展示顺序稳定）。
3. **落快照**：原 INSERT 搬入事务。

事务内不设 `RowsAffected==0` 守卫：0 行是幂等正常结果（service 已在事务前拦截空绑定，
事务内空集只可能是拦截后到执行前的并发解绑窗口，按无操作处理优于报错）；并发给接替老师绑定重叠业务员由
uk_teacher_user 1062 兜底（整体回滚 500，重试即成功），不加 `SELECT ... FOR UPDATE`（项目无先例）。
前置依赖：service 的 ErrSameTeacher 校验承重（同 ID 时语句 1 会清空全部绑定）。
groupCount/salesman 快照取自转移前的查询，快照语义不变（可能大于实际移动数）。

### 7. 复用既有代码

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
  同老师 400 / 空内容 400 / 白名单外 400 / remark>200 字 400 / 老师 999 → 404 /
  ~~负数计数 400 / 可选携带 friendCount=66 入库~~（2026-08 起计数用例已废弃，见变更记录）—— 全部通过；
  错误用例不入库（行数核对 8 = 6 种子 + 2 成功）
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
| `internal/repository/resign.go` | 新 | ListResigns / resignWhere / GetTeachersByIDs / ListTeacherSalesmen / TransferResign（三步事务：删重叠/移剩余/落快照 + 2 个 tx helper） |
| `internal/service/resign.go` | 新 | ListResigns / AddResign / normalizeTransferContent + 4 个业务错误 |
| `internal/handler/resign.go` | 新 | List / Add（错误映射 400/404/500） |
| `internal/router/router.go` | 改 | chat 组注册 2 行 |
| `gyz-admin/src/api/dxData/chatSys/resign.js` | 改 | 删 mock，启用 request 调用（baseURL=VUE_APP_LOCAL_API） |

## 变更记录

- **2026-08 移除好友概念**：业务确认好友维度取消，groupCount 语义 = 原老师绑定业务员数（一业务员一群），
  由后端计算入库。变更：`transferContent` 白名单收敛为 `["group"]`（传 friend 400）；`friend_count` 列
  删除（存量库 `database.migrateTeacherResign` 幂等 DROP + group_count 注释同步 + transfer_content
  残留清洗）；种子 group_count 改为与实际绑定数一致（0/0/0/6/14/10）；前端删「转移好友」勾选项与
  「好友数」列。涉及：model/service/repository/handler 的 resign 文件、schema.sql、database.go、
  resign_seed.sql、gyz-admin teacherQuery.vue 与 resign.js。
- **2026-08 确认转移真实移动绑定**：add 原只写 teacher_resign 快照、不碰 teacher_sales，
  导致确认转移后老师列表关联业务人员数不变。`InsertResign` 改名 `TransferResign` 扩为三步事务
  （删重叠/移剩余/落快照，见设计决策 6），重叠绑定去重合并（保留接替老师现有 bind_time）；
  service 调用点同步换名。无 schema 变更、无新增错误类型（1062 冲突走既有 500 分支）。
- **2026-08-19 拦截原老师空绑定**：联调发现原老师无绑定业务员时会落一条 salesman_name/dept 与
  group_count 全空的转移记录（运营无价值，且易被误认为转移 bug——本次排查即如此）。AddResign 在
  业务员快照查询后增加空切片判断，返回新增哨兵错误 `ErrOriginalTeacherNoSalesman`，handler 映射
  400（零额外查询：salesmen 本就要查来做快照）。这反转了设计决策 6「空绑定为合法转移」的旧表述：
  事务内仍不设 RowsAffected 守卫（拦截后到执行前的并发解绑窗口，空集按无操作处理）。
  种子 101/102/103（group_count=0）为历史记录，不追改。涉及：service/handler 的 resign 文件、
  README、Swagger 注释。
- **2026-08-19 错误 msg 中文化**：前端拦截器 error 分支取 `error.response.data.msg` 直接弹给用户，
  英文 msg 对运营不友好（且与成功侧中文「转移成功」割裂）。方案：仅中文化 msg，HTTP 状态码保留
  4xx/5xx 语义（不切恒 200——保留监控/网关 5xx 告警；讨论中确认 axios 非 2xx 会 reject 并造英文
  `Request failed with status code 404`，但响应体挂在 `error.response.data` 上可正常取用）。
  落地：15 个 service 哨兵错误文本改中文（**文本即 API 契约**，改动需与前端确认），handler 哨兵分支
  统一透传 `err.Error()`（消灭 teacher/resign 写死文案与哨兵文本的重复，对齐 diagnose 既有模式）；
  硬编码英文改中文（`服务器内部错误`/`参数 xx 必须是整数`等），body 绑定失败改固定 `请求体非法`
  （gin 英文详情进 slog 不外露）。涉及全部 service/handler 文件、README、CLAUDE.md。

> **2026-08-18 字段命名整体迁移 snake_case**：本文中的驼峰字段名（originalTeacherId/transferContent 等）已全部改为蛇形（original_teacher_id/transfer_content），以 [PLAN-api-snake-case.md](PLAN-api-snake-case.md) 为准。

> **2026-08-19 接口路径与请求方式变更**：两接口迁至 `/teacher/` 前缀下对齐老师管理风格——`GET /resign/list` → `POST /teacher/resign/list`（查询条件从 query string 改 JSON body，数值字段用 FlexInt64 宽容解析前端空串，同 TeacherListReq 先例），`POST /resign/add` → `POST /teacher/resign/add`（仅改路径）。service/repository 零改动。
