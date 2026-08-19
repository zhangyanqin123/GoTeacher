# 老师管理（chatSys）模块设计与实施记录

## 背景

前端 `gyz-admin`（Vue2 + Element UI）的老师管理页 `src/views/dxData/chatSys/teacherQuery.vue` 通过 `src/api/dxData/chatSys/teacher.js` 调接口，该文件是接口文档（原全为 Promise mock，注释写明后端就绪后删 mock、启用注释的 `request` 调用即可）。本模块在后端按文档实现 5 个接口，路径/参数/响应与 mock 严格一致。

## 接口清单

| # | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 1 | POST | `/api/v1/dxsf/teacher/list` | 分页 + 多条件筛选；返回 `data.list / data.count` |
| 2 | GET | `/api/v1/dxsf/teacher/options` | 全量下拉（含停用）：`[{id, name, deptName}]` |
| 3 | POST | `/api/v1/dxsf/teacher/edit` | 编辑 title / level(0无3初级5高级) / avatar / sign |
| 4 | GET | `/api/v1/dxsf/teacher/bind/salesman/list` | 老师绑定业务员分页（默认 pageSize=5） |
| 5 | POST | `/api/v1/dxsf/teacher/bind/salesman` | ~~全量替换绑定；`userIds: []` = 解绑全部~~ → 2026-08-18 起改为**追加语义**（INSERT IGNORE 幂等，仅新增绑定；空数组 no-op，无解统能力） |

## 与前端 mock 对齐的兼容约定（勿改）

| 约定 | 原因 |
| --- | --- |
| `status` 输出字符串 `"1"`/`"0"` | el-switch `active-value="1"` 直接比较 |
| `qualification` 存中文 `已认证`/`未认证` | el-tag 直接展示，下拉 value 即中文 |
| 时间输出 `YYYY-MM-DD HH:mm:ss` | 前端无 formatter 直接展示；RFC3339 带 T 会破格式 |
| 查询 msg=`success`、写 msg=`编辑成功`/`绑定成功` | mock 响应原样 |
| `level` 原样存取 | 列表展示 1-5 星（种子），编辑弹窗提交 0/3/5（0 无 / 3 初级 / 5 高级） |
| 绑定列表：老师不存在 → 空 list + count 0 | mock 同构，不视为错误 |

## 表设计（internal/database/schema.sql）

- `teacher`：老师表；`dept_name` 冗余存储（与 mock 同构，避免 join 部门表）；`avatar` TEXT（base64 data URL）
- `sales_user`：业务员桩表（真实系统为 admin 的 sys_user）；种子 = mock salesPool 25 人，id 1-25
- `teacher_sales`：绑定关系，`uk_teacher_user (teacher_id, user_id)` 唯一约束；`bind_time` 默认 NOW()
- `bind_sales_count` 不落库，列表用相关子查询 `(SELECT COUNT(*) FROM teacher_sales WHERE teacher_id=t.id)` 带出——单一事实来源，避免绑定事务双写漂移

种子（teacher_seed.sql）：照抄 mock——12 老师（id/日期原样）、25 业务员、每老师绑前 N 个业务员（bind_time 用 salesPool 对应值），共 143 条绑定。

## 分层实现（沿用 house_up_down 模式）

- `model/teacher.go`：Teacher / TeacherOption / TeacherSalesRow / 请求 DTO / TeacherListFilter / PageResult
- `model/datetime.go`：`DateTimeString` 实现 `sql.Scanner`，把 DATETIME 在扫描点格式化为 `2006-01-02 15:04:05`
- `repository/teacher.go`：动态 WHERE（`strings.Builder` 拼 SQL + args）；模糊 `LIKE CONCAT('%',?,'%')`；时间闭区间 `updated_at >= ? AND < DATE_ADD(?, INTERVAL 1 DAY)`（避免函数包列失索引）；`ReplaceTeacherSales` 事务内先删后插
- `service/teacher.go`：哨兵错误 + 白名单校验（level 0/3/5、签名 ≤200 字符、userIds 存在性）；分页默认列表 10 / 绑定 5、上限 100
- `handler/teacher.go`：query/body 绑定、`errors.Is` 错误映射（400 参数错 / 404 老师不存在 / 500 内部错误只进日志）
- `response.OKMsg`：新增（mock 约定 msg 不是 `ok`），不动原 `OK`

## 踩坑记录

1. **多语句 Exec 报 1064**：go-sql-driver 默认一次 Exec 只允许一条语句，schema.sql 从 1 条 CREATE 变 4 条后报语法错。选择应用层 `splitStatements` 按分号切分逐条执行（处理单引号字符串内的分号与 `\'` 转义），而非 DSN 开 `multiStatements=true`（作用于整个连接池，风险面大）。
2. **DATETIME 扫进 string 变 RFC3339**：`ParseTime: true` 下驱动返回 `time.Time`，`database/sql` 转字符串走 `time.Time.String()` 得到 `2025-01-15T09:30:00+08:00`。用自定义 `DateTimeString` 类型在 `Scan` 里 `Format("2006-01-02 15:04:05")` 解决。
3. **MySQL affected rows 语义**：值未变时 `UPDATE` 的 RowsAffected 也是 0，不能用它判断"老师不存在"；改用 `ExistsTeacher`（`SELECT 1 ...` + `sql.ErrNoRows`）前置检查。
4. **`bindSalesCount=0` 筛选**：用 `*int` 指针区分"未传"与"传 0"，零值结构体字段会丢掉这个条件。
5. **seed 幂等粒度**：原 `Seed` 只看 house 表空不空，老库永远轮不到 teacher 种子；拆成 `seedHouseUpDown` + `seedTeacher` 各自判空、各自事务。

## 验证结论（2026-08-16，全通过）

- `gofmt` / `go vet` / `go build` 通过
- 列表：默认分页 count=12；status=0（3 条）、name=张（1 条）、qualification=已认证（9 条）、bindSalesCount=0（3 条）、deptId、时间范围、组合条件、id 精确全部与 mock 一致
- options：12 条含停用；绑定列表：默认 pageSize=5、显式 pageSize 生效、不存在的老师返回空
- 编辑：字段落库、level 白名单 400、不存在 404、updatedAt/updateBy 自动更新
- 绑定：全量替换、空数组解绑、重复 id 去重、不存在 userId 400、不存在 teacherId 404
- 重启幂等：种子不重复、编辑/绑定结果保留；原 house 接口无回归（total=2312）

> **2026-08-18 字段命名整体迁移 snake_case**：本文接口清单与验证记录中的驼峰字段名（bindSalesCount/deptId 等）已全部改为蛇形（bind_sales_count/dept_id），以 [PLAN-api-snake-case.md](PLAN-api-snake-case.md) 为准。
