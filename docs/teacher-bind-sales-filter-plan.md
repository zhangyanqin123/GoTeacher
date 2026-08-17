# 绑定业务员人员树过滤已绑定人员 — 实施计划

> 背景：`teacherQuery.vue` 的「绑定业务员」弹窗中，人员树（`el-tree`）目前展示市场部下的全部人员，已被任何老师绑定的业务员仍可被选中，导致重复绑定。

## 目标

1. 后端新增「查询已绑定老师的业务员」接口，返回全部绑定关系；
2. 前端在人员树中**过滤掉所有已绑定业务员**（含当前老师自己已绑定的）；
3. 由于 `POST /teacher/bindSales` 是**全量替换语义**（`ReplaceTeacherSales` 事务内先删后插），前端提交时必须把**当前老师原有绑定与新勾选合并**，否则会静默清空原绑定。

**已确认的决策**：树中过滤所有已绑定人员（含当前老师的）；提交时合并当前老师原有绑定 + 新勾选。代价是该弹窗无法解绑（可接受，后续可在详情弹窗单独做解绑）。

## 关键现状

- 后端（gin + `database/sql` 手写 SQL，无 ORM）：绑定关系在 `teacher_sales`（`teacher_id`+`user_id`，组合唯一键，同一业务员可被多个老师绑定）。现有 `bindSales/list` 接口不返回 userId，无法满足前端取「已绑定 userId 集合」的需求。
- 前端（Vue 2.6 + element-ui 2.13，纯 JS）：树数据一次性加载后由 `salesTreeLoaded` 缓存；人员节点形如 `{ nodeKey, type: 'staff', userId, deptId, label }`。

---

## 阶段一：后端新增接口

**路由**：`GET /api/v1/dxsf/chatSys/teacher/bindSales/boundUserIds`（无参数，不分页——`teacher_sales` 数据量与业务员数同量级，很小）

**响应**：`{ code: 200, msg: "success", data: [{ userId: 1, teacherId: 2 }, ...] }`

- 返回 `teacherId + userId` 对（而非仅 userId 平铺列表）：前端既要用全量 userId 集合过滤树，又要按 `teacherId === 当前老师id` 提取「当前老师原有绑定」做提交合并，一个接口同时满足两个需求。
- data 直接为数组，与现有 `Options` 接口风格一致（`response.OKMsg(c, "success", list)`）。

按现有分层模式修改 5 个文件：

1. `internal/model/teacher.go` — 新增 `TeacherSalesBoundItem` 结构体（`teacherId` + `userId`）
2. `internal/repository/teacher.go` — 新增 `ListAllTeacherSales`：`SELECT ts.teacher_id, ts.user_id FROM teacher_sales ts ORDER BY ts.id`，无需 JOIN `sales_user`；空表返回空切片
3. `internal/service/teacher.go` — 新增透传方法 `ListAllTeacherSales`（同 `ListTeacherOptions` 模式）
4. `internal/handler/teacher.go` — 新增 `BoundUserIds` 方法，错误处理照抄 `Options`
5. `internal/router/router.go` — 追加 `chat.GET("/teacher/bindSales/boundUserIds", th.BoundUserIds)`
6. `README.md` — 补充新接口章节

## 阶段二：前端改造

1. `src/api/dxData/chatSys/teacher.js` — 新增 `listBoundSalesUserIds()`（沿用 `LOCAL` baseURL 模式）
2. `src/views/dxData/chatSys/teacherQuery.vue`：
   - data 新增 `boundSalesList: []`、`boundListLoading: false`
   - `onBindDialogOpen`：树保持首次懒加载；**每次打开都重新拉取**绑定列表（提交后绑定关系已变化，不能缓存），失败 `console.warn` 降级为不过滤
   - computed 新增：
     - `boundUserIdSet`：全量已绑定 userId 集合（过滤树用）
     - `currentTeacherBoundIds`：当前老师原有绑定（提交合并用）
     - `filteredSalesTreeData`：对缓存树剪枝——移除已绑定 staff 节点；子节点清空的 dept 节点一并移除（不修改缓存原数据）
   - 模板：`el-tree :data` 改绑 `filteredSalesTreeData`；树容器加 `v-loading="boundListLoading"`
   - `confirmBindSales`：提交前合并——新勾选兜底剔除已绑定，与当前老师原有绑定去重合并后提交

## 阶段三：验证

**后端**：

1. `go build ./...` && `go vet ./...`
2. 启动服务后 `curl http://localhost:8080/api/v1/dxsf/chatSys/teacher/bindSales/boundUserIds`，与 DB `SELECT teacher_id, user_id FROM teacher_sales ORDER BY id` 比对一致；空表时返回 `data: []`

**前端**（`npm run dev` 启动 gyz-admin）：

1. 老师管理页 → 「绑定业务员」：已绑定业务员（任意老师的）不再出现在树中；无绑定数据时树完整
2. 勾选新业务员确认 → DB 核对该老师记录 = 原有绑定 + 新勾选（未被清空）
3. 重新打开弹窗：刚绑定的业务员已被过滤；搜索框过滤功能不受影响

## 风险与边界

- **合并提交依赖弹窗打开时的绑定快照**：并发场景下（他人同时绑定）以最后一次提交为准（全量替换语义固有），可接受
- **绑定列表接口失败**：前端降级为不过滤 + 兜底防御过滤，不会阻断弹窗使用
- 该弹窗从此无法解绑（已确认接受）；如需解绑可后续在详情弹窗单独做
