# 接口说明：老师绑定业务员列表

## 1. 基本信息

| 项目 | 说明 |
| --- | --- |
| 路径 | `/api/v1/dxsf/chatSys/teacher/bindSales/list` |
| 方法 | `GET` |
| 用途 | 分页查询某老师已绑定的业务员列表（老师管理页「绑定业务员」详情弹窗用） |
| 鉴权 | 无（当前项目仅全局 CORS 中间件，无鉴权） |
| 数据来源 | `teacher_sales` JOIN `sales_user`，按 `teacher_sales.id` 升序（即绑定先后顺序） |
| 相关代码 | `internal/handler/teacher.go`（SalesList）→ `internal/service/teacher.go`（ListTeacherSales）→ `internal/repository/teacher.go`（ListTeacherSalesByTeacher） |

## 2. 请求参数

### 2.1 Query 参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `teacherId` | `number` | 是 | 老师 ID，必须为正整数，否则返回 400 |
| `pageIndex` | `number` | 否 | 页码，默认 `1`（传 0 / 负数 / 非数字均回退到 1） |
| `pageSize` | `number` | 否 | 每页条数，**默认 `5`**（与老师列表接口的默认 10 不同，对齐前端 mock 的 `query.pageSize \|\| 5`）；上限 `100`，超过被钳制 |

### 2.2 请求示例

```
GET /api/v1/dxsf/chatSys/teacher/bindSales/list?teacherId=1&pageIndex=1&pageSize=5
```

## 3. 响应

统一结构 `{code, msg, data}`，查询类 `msg` 固定为 `success`，分页统一 `data.list` / `data.count`。

### 3.1 `data.list` 行字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `phone` | `string` | 业务员手机号（来自 `sales_user.phone`） |
| `nickname` | `string` | 业务员昵称（来自 `sales_user.nickname`） |
| `deptName` | `string` | 部门名（来自 `sales_user.dept_name` 冗余快照） |
| `bindTime` | `string` | 绑定时间，格式 `YYYY-MM-DD HH:mm:ss`（无 T、无时区，前端直接展示） |

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `data.count` | `number` | 该老师绑定业务员总数（用于前端分页组件） |

### 3.2 成功示例

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "list": [
      { "phone": "13800138001", "nickname": "陈晓", "deptName": "市场一部", "bindTime": "2025-08-01 10:30:00" },
      { "phone": "13800138002", "nickname": "李强", "deptName": "市场二部", "bindTime": "2025-08-03 14:20:00" }
    ],
    "count": 12
  }
}
```

### 3.3 空数据示例（老师不存在 / 未绑定任何人）

老师不存在时**不报错**，返回空结果（对齐前端 mock 的 `find 不到 → count 0 → list []` 语义，不视为错误）：

```json
{ "code": 200, "msg": "success", "data": { "list": [], "count": 0 } }
```

### 3.4 错误响应

| HTTP 状态码 | `code` | `msg` | 场景 |
| --- | --- | --- | --- |
| 400 | 400 | `teacherId is required` | 未传 `teacherId`、非整数或 ≤ 0 |
| 500 | 500 | `internal server error` | 数据库等内部错误（真实原因只进服务端日志） |

```json
{ "code": 400, "msg": "teacherId is required", "data": null }
```

## 4. curl 示例

```bash
# 默认分页（pageSize=5）
curl -s 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/bindSales/list?teacherId=1'

# 显式指定分页
curl -s 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/bindSales/list?teacherId=1&pageIndex=2&pageSize=10'

# 缺少 teacherId → 400
curl -s 'http://localhost:8080/api/v1/dxsf/chatSys/teacher/bindSales/list'
```

## 5. 兼容约定（与前端 mock 严格对齐，改动勿破坏）

1. **默认 pageSize=5**：与老师列表接口的默认 10 不同，勿「统一」成同一个常量
2. **`bindTime` 字符串格式**：`model.DateTimeString` 在 SQL 扫描点格式化为 `2006-01-02 15:04:05`，避免 `time.Time` 序列化成 RFC3339（带 T）破坏前端直接展示
3. **`list` 无数据时输出 `[]` 而非 `null`**：repository 用 `make([]..., 0, limit)` 初始化
4. **不返回 `userId`**：弹窗仅展示快照字段；人员树过滤请用配套接口 `GET /teacher/bindSales/boundUserIds`（返回去重 userId 平铺数组）

## 6. 设计说明

| 决策 | 原因 |
| --- | --- |
| 老师不存在返回空而非 404 | mock 同构（前端 `find` 不到就给空数组），弹窗场景下老师由列表接口带出，存在性已被上游保证 |
| 按 `teacher_sales.id` 排序 | 种子数据 `bind_time` 与绑定顺序一致，稳定分页（无跳行/重复） |
| 分页默认值在 service 层兜底 | handler 只透传，`normalizePage` 统一处理「未传 / 越界 / 超上限」三种情况 |
| 快照字段由后端从 `sales_user` 回查 | 冗余快照单一事实来源，忽略前端传的同名字段（表设计原则） |

## 7. 关联接口

| 接口 | 关系 |
| --- | --- |
| `GET /api/v1/dxsf/chatSys/teacher/bindSales/boundUserIds` | 全量已绑定业务员 userId（绑定弹窗人员树过滤用），详见 [api-bindSales-boundUserIds.md](./api-bindSales-boundUserIds.md) |
| `POST /api/v1/dxsf/chatSys/teacher/bindSales` | 绑定提交（追加语义，重复绑定幂等）；本接口数据变化即由其产生 |
| `GET /api/v1/dxsf/chatSys/teacher/list` | 老师列表，`bindSalesCount` 子查询统计的即本接口的 `count` |
