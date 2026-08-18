# 接口说明：查询已绑定老师的业务员

## 1. 基本信息

| 项目 | 说明 |
| --- | --- |
| 路径 | `/api/v1/dxsf/chatSys/teacher/bindSales/boundUserIds` |
| 方法 | `GET` |
| 用途 | 返回 `teacher_sales` 表的**全量已绑定业务员 userId**（去重平铺数组），供前端「绑定业务员」弹窗的人员树过滤已绑定人员 |
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
| `data` | `array<number>` | 已绑定业务员 userId 去重平铺数组，按 `user_id` 升序；空表返回 `[]`（非 `null`） |

> 同一 `userId` 可能被多个老师绑定（`teacher_sales` 唯一键是 `teacher_id + user_id` 组合），数组已 `DISTINCT` 去重。

### 3.2 成功示例

```json
{
  "code": 200,
  "msg": "success",
  "data": [1, 2, 178]
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
// data：已绑定 userId 平铺数组 + computed：全量已绑定 userId 集合
this.boundUserIdList = res.data || []
boundUserIdSet() {
  return new Set(this.boundUserIdList)
}
// 组装树时剔除 boundUserIdSet 中存在的 staff 节点
```

### 5.2 提交（追加语义）

`POST /teacher/bindSales` 为**追加语义**（`INSERT IGNORE` 命中唯一键即跳过，幂等），前端提交时只需传**新勾选**的 userId，无需合并原绑定：

```js
// 兜底剔除已绑定（防接口失败降级时误选），直接提交新勾选
const userIds = uniqueStaff.map(n => n.userId).filter(id => !this.boundUserIdSet.has(id))
bindTeacherSales({ teacherId, userIds })
```

### 5.3 拉取时机与降级

- **每次打开绑定弹窗都重新拉取**（不能缓存：提交后绑定关系已变化）
- 拉取失败时 `console.warn` 并置空数组 → 降级为树不过滤；提交侧仍有兜底剔除防御

## 6. 设计说明

| 决策 | 原因 |
| --- | --- |
| 返回去重 userId 平铺数组，而非 `{teacherId, userId}` 对 | 绑定提交已改为追加语义，前端不再需要按 `teacherId` 提取当前老师原绑定做合并，此接口只服务树过滤一个需求 |
| 不分页 | 全量过滤场景必须拿齐数据；`teacher_sales` 数据量与业务员数同量级（种子 143 条），一次返回开销可忽略 |
| 不 JOIN `sales_user` | 前端只需 ID 做集合运算，昵称等部门信息人员树已有 |
| `data` 为数组而非 `{list, count}` | 与现有 `GET /teacher/options` 下拉类接口风格一致 |

## 7. 关联接口

| 接口 | 关系 |
| --- | --- |
| `GET /api/v1/dxsf/chatSys/teacher/bindSales/list` | 单个老师的绑定业务员分页列表（详情弹窗用），不含 `userId`，无法用于树过滤 |
| `POST /api/v1/dxsf/chatSys/teacher/bindSales` | 绑定提交（**追加语义**，重复绑定幂等）；本接口数据由其产生 |
