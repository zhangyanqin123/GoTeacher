# JSON 字段命名蛇形迁移记录（PLAN-api-snake-case）

> 2026-08-18：chatSys（老师管理/离职转移）+ 诊股三模块的接口 JSON 键名由 camelCase 整体迁移为 snake_case，
> 前端 gyz-admin 同步修改。本文记录决策、字段清单与过渡设计，三模块的 PLAN-*.md 接口清单中的驼峰字段名以本文为准。

## 背景

历史字段命名跟随前端 mock 为驼峰（`originalTeacherId`）。mock 已删、前端直连真实接口后，统一改为与
DB 列名（`db` tag）一致的蛇形，消除「同一个字段两套名字」的心智负担：json tag 与 db tag 天然同名，
新增字段无需再做驼峰转换。

## 三个决策

1. **全链路统一蛇形**：query 参数 + JSON 请求体 + 响应体全部改（`pageIndex→page_index`、`deptId→dept_id`、
   `originalTeacherId→original_teacher_id`）。URL 路径段（`/bindSales` 等）保持原样，不在改名范围。
2. **前端 JS 键也用蛇形**：queryParams 对象键、el-table-column prop、v-model、校验 rules 直接用蛇形，
   无转换层（对齐项目既有 `page_index` 先例，如 terms/index.vue）。
3. **按模块分三阶段实施**：一 teacher、二 resign、三 diagnose+清理；每阶段后端 tag/query/注释 + 前端对应
   键 + swag init + curl 冒烟原子落地，阶段结束系统可正常联调。

## 迁移字段清单（后端 47 个 json tag + 27 个 query 键）

- **teacher**：`dept_id/dept_name/work_no/bind_sales_count/created_at/updated_at/update_by`（Teacher），
  `dept_name`（TeacherOption/TeacherSalesRow）、`bind_time`、`teacher_id/user_ids`（TeacherBindReq）
- **resign**：`original_teacher_id/name/dept_id/dept`、`replace_teacher_id/name/dept`、`salesman_name/salesman_dept`、
  `transfer_content/group_count/operate_ip/transfer_time`、`created_at/updated_at`（Resign 15 个）；
  `original_teacher_id/replace_teacher_id/transfer_content`（ResignAddReq）；`dept_id/dept_name`（TeacherBrief/TeacherSalesmanBrief）
- **diagnose**：`user_nick_name/user_name/stock_code/stock_name/buy_price/buy_num/teacher_name/submit_time/
  report_content/report_submit_time`（Diagnose）、`report_content`（SubmitReportReq）、
  `audit_type/reject_reason`（AuditReq）、`audit_logs`（DiagnoseDetail）
- query 键随 handler `c.Query` 一并改；`id/account/name/status/operator/remark` 等单词键不动

## 过渡设计：queryPage 双读兼容（已清理）

`queryPage`（handler/teacher.go）被 3 模块 4 处调用。若阶段一直接改只读蛇形，未迁移的 resign/diagnose 前端
分页会**静默失效**（`Atoi("")`=0 → service 兜底 1/10，无报错难排查）。故阶段一改为蛇形优先、camel 兜底
（`page_index` 为 0 且 `pageIndex` 非空才回读），阶段三末三模块迁移完成后删除兜底分支。
其余筛选参数**不做双读**——每模块与其前端页面同阶段原子切换，无跨模块消费者。

## 跨模块依赖（teacherQuery.vue 双模块属性所致）

- `submitTransfer` 读 `original.dept_name`/`replace.dept_name` 消费的是 teacher 模块 options 响应，
  随阶段一改（此时 transferForm 其余键仍驼峰）
- `handleNodeClick` 同方法写 teacherQuery + resignQuery 两套键，阶段一改 teacher 两行、阶段二补 resign 两行

## 前端不可混改的边界

- `listUser`/`getDeptList` 等远程 go-admin 接口仍为驼峰（另一套后端），保持不动
- 本地 UI 键不上线不改名：`dateRange`、`submitTimeRange`/`reportTimeRange`（发送前 delete）
- Pagination 组件 props 为通用 `page`/`limit`，与后端参数名零耦合，只改父组件绑定目标

## 验证结论

- 每阶段：`go build/vet/test` + `swag init` 重生成（docs/ 三文件随代码提交）+ curl 冒烟
  （蛇形参数/响应键、驼峰 body 应 400、过渡期 camel 分页兜底、清理后 camel 分页被忽略回落默认页）
- 冒烟副作用（新增的 resign 记录、诊股状态流转与日志）已回滚至种子状态
- 前端禁 lint/dev（历史项目约定），验证靠键名残留 grep（清零）+ 用户手动 npm run dev 走查

## 相关文档

- 字段清单与设计决策：PLAN-teacher.md / PLAN-resign.md / PLAN-diagnose.md（其中驼峰字段名以本文为准）
- 前端侧记录：gyz-admin 仓库 PLAN-snake-case.md
