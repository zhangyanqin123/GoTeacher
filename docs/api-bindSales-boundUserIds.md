# 接口说明：查询已绑定老师的业务员

## 1. 基本信息

| 项目 | 说明 |
| --- | --- |
| 路径 | `/api/v1/dxsf/chatSys/teacher/bindSales/boundUserIds` |
| 方法 | `GET` |
| 用途 | 返回 `teacher_sales` 表的**全量绑定关系对**，供前端「绑定业务员」弹窗的人员树过滤已绑定人员、以及全量替换语义下的提交合并 |
| 鉴权 | 无（当前项目仅全局 CORS 中间件，无鉴权） |
| 分页 | 不分页（数据量与业务员数同量级，很小） |

## 2. 请求参数

无任何请求参数（Query / Body 均不需要）。

## 3. 响应

### 3.1 字段说明

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | `number` | `200` 成功；失败时与 HTTP 状态码一致 |
| `msg` | `string` | 成功固定 `success` |
| `data` | `array` | 绑定关系对列表，按 `teacher_sales.id` 升序；空表返回 `[]`（非 `null`） |
| `data[].teacherId` | `number` | 老师 ID（`teacher.id`） |
| `data[].userId` | `number` | 业务员 ID（`sales_user.id`，即前端人员树 staff 节点的 `userId`） |

> 注意：同一 `userId` 可能出现多次（被多个老师绑定，`teacher_sales` 唯一键是 `teacher_id + user_id` 组合）。

### 3.2 成功示例

```json
{
  "code": 200,
  "msg": "success",
  "data": [
    { "teacherId": 5, "userId": 1 },
    { "teacherId": 5, "userId": 2 },
    { "teacherId": 3, "userId": 178 }
  ]
}
```

### 3.3 空数据示例

```json
{ "code": 200, "msg": "success", "data": [] }
```

### 3.4 错误响应

| HTTP 状态码 | `code` | `msg` | 场景 |
| --- | --- | --- | --- |
| 500 | 500 | `internal server error` | 数据库查询失败（真实原因只进服务端日志） |

```json
{ "code": 500, "msg": "internal server error", "data": null }
```

## 4. curl 示例

```bash
curl -s 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/bindSales/boundUserIds'
```

## 5. 前端使用场景

前端封装：`gyz-admin/src/api/dxData/chatSys/teacher.js` 的 `listBoundSalesUserIds()`，使用页面：`teacherQuery.vue`。

### 5.1 人员树过滤（树中隐藏所有已绑定业务员）

```js
// computed：全量已绑定 userId 集合
boundUserIdSet() {
  return new Set(this.boundSalesList.map(r => r.userId))
}
// 组装树时剔除 boundUserIdSet 中存在的 staff 节点
```

### 5.2 提交合并（全量替换语义防清空）

`POST /teacher/bindSales` 为**全量替换**（事务内先删后插，提交的 `userIds` 替换该老师全部绑定），因此提交前必须把「当前老师原有绑定」与「新勾选」合并：

```js
// 提取当前老师原有绑定
const currentIds = boundSalesList.filter(r => r.teacherId === currentRow.id).map(r => r.userId)
// 新勾选兜底剔除已绑定，再合并去重
const userIds = Array.from(new Set([...currentIds, ...newCheckedIds]))
```

### 5.3 拉取时机与降级

- **每次打开绑定弹窗都重新拉取**（不能缓存：提交后绑定关系已变化）
- 拉取失败时 `console.warn` 并置空列表 → 降级为树不过滤；提交侧仍有兜底剔除防御

## 6. 设计说明

| 决策 | 原因 |
| --- | --- |
| 返回 `teacherId + userId` 对，而非仅 userId 平铺 | 一个接口同时满足两个需求：全量 userId 集合过滤树 + 按 `teacherId` 提取当前老师原有绑定做合并 |
| 不分页 | 全量过滤场景必须拿齐数据；`teacher_sales` 数据量与业务员数同量级（种子 143 条），一次返回开销可忽略 |
| 不 JOIN `sales_user` | 前端只需 ID 对做集合运算，昵称等部门信息人员树已有 |
| `data` 为数组而非 `{list, count}` | 与现有 `GET /teacher/options` 下拉类接口风格一致 |

## 7. 关联接口

| 接口 | 关系 |
| --- | --- |
| `GET /api/v1/dxsf/chatSys/teacher/bindSales/list` | 单个老师的绑定业务员分页列表（详情弹窗用），不含 `userId`，无法用于树过滤 |
| `POST /api/v1/dxsf/chatSys/teacher/bindSales` | 绑定提交（全量替换语义），本接口的数据用于其提交前的合并 |
