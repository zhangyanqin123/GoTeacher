# 诊股记录模块表结构

> 对应前端 `diagnoseSys/diagnose.js` / `diagnoseQuery.vue`
> DDL 源：`internal/database/schema.sql`

## 兼容约定（与 mock 同构，勿改）

- 昵称/姓名/股票名/老师为**冗余快照**（用户/老师可能被改/删，记录保留当时值）
- `buy_price DECIMAL(10,2)`，接口输出浮点（如 `1680.5`）
- `report_content` 富文本 HTML，未编写为空串（`TEXT NOT NULL` 避免 NULL 扫 string）
- `report_submit_time` 可空，NULL = 未提交（接口输出空串）
- 模糊查询列为前导通配 LIKE，不建索引（打不进 B-tree）

---

## 1. diagnose — 诊股记录

| 字段 | 类型 | 允许 NULL | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | BIGINT UNSIGNED | 否 | AUTO_INCREMENT | 主键 |
| user_nick_name | VARCHAR(50) | 否 | '' | 用户昵称（冗余快照） |
| user_name | VARCHAR(50) | 否 | '' | 用户姓名（冗余快照） |
| stock_code | VARCHAR(20) | 否 | '' | 股票代码，如 SH600519 |
| stock_name | VARCHAR(50) | 否 | '' | 股票名称（冗余快照） |
| buy_price | DECIMAL(10,2) | 否 | 0.00 | 买入价格（元，两位小数） |
| buy_num | INT UNSIGNED | 否 | 0 | 买入股数 |
| teacher_name | VARCHAR(50) | 否 | '' | 诊股老师（冗余快照） |
| submit_time | DATETIME | 否 | CURRENT_TIMESTAMP | 用户提交时间 |
| report_content | TEXT | 否 | — | 诊股报告（富文本 HTML，未编写为空串） |
| report_submit_time | DATETIME | 是 | NULL | 诊股提审时间（NULL = 未提交） |
| status | TINYINT | 否 | 1 | 状态：1 待诊股 / 2 待专业审核 / 3 专业审核不通过 / 4 待合规审核 / 5 合规审核不通过 / 6 合规审核通过 |
| remark | TEXT | 否 | — | 用户备注（富文本 HTML，净化后存储） |
| created_at | DATETIME | 否 | CURRENT_TIMESTAMP | 创建时间 |
| updated_at | DATETIME | 否 | CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP | 更新时间 |

### 索引

| 索引名 | 类型 | 字段 |
| --- | --- | --- |
| PRIMARY | 主键 | id |
| idx_status | 普通 | status |
| idx_submit_time | 普通 | submit_time |
| idx_report_submit_time | 普通 | report_submit_time |

### 状态流转（status）

```
1 待诊股 → (老师提交报告) → 2 待专业审核
2 待专业审核 → 通过 → 4 待合规审核
2 待专业审核 → 不通过 → 3 专业审核不通过
4 待合规审核 → 通过 → 6 合规审核通过
4 待合规审核 → 不通过 → 5 合规审核不通过
```

---

## 2. diagnose_audit_log — 诊股审核流程日志

> 落库为准，保留驳回历史与驳回原因；详情弹窗按 id 正序展示。

| 字段 | 类型 | 允许 NULL | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| id | BIGINT UNSIGNED | 否 | AUTO_INCREMENT | 主键 |
| diagnose_id | BIGINT UNSIGNED | 否 | — | 诊股记录ID（diagnose.id） |
| log_type | VARCHAR(10) | 否 | — | 类型：用户提交 / 诊股报告 / 专业审核 / 合规审核 |
| operator | VARCHAR(50) | 否 | '' | 操作人：用户姓名 / 诊股老师 / 专业审核员 / 合规审核员 |
| result | VARCHAR(8) | 否 | — | 结果：已提交 / 通过 / 不通过 |
| remark | TEXT | 否 | — | 备注：用户 remark / 固定文案 / 驳回原因（HTML） |
| log_time | DATETIME | 否 | CURRENT_TIMESTAMP | 记录时间 |
| created_at | DATETIME | 否 | CURRENT_TIMESTAMP | 创建时间 |

### 索引

| 索引名 | 类型 | 字段 |
| --- | --- | --- |
| PRIMARY | 主键 | id |
| idx_diagnose_id | 普通 | diagnose_id |

### log_type × result 组合示例

| log_type | result | 触发时机 |
| --- | --- | --- |
| 用户提交 | 已提交 | 用户提交诊股申请（status → 1） |
| 诊股报告 | 已提交 | 老师提交诊股报告（status 1 → 2） |
| 专业审核 | 通过 | 专业审核通过（status 2 → 4） |
| 专业审核 | 不通过 | 专业审核驳回（status 2 → 3） |
| 合规审核 | 通过 | 合规审核通过（status 4 → 6） |
| 合规审核 | 不通过 | 合规审核驳回（status 4 → 5） |

---

## 两表关系

```
diagnose (1) ──── (N) diagnose_audit_log
    id  ◄──────────  diagnose_id
```

- 一条诊股记录对应多条审核日志，日志只增不改（append-only）
- 日志表无外键约束，靠应用层保证一致性（`idx_diagnose_id` 支撑按记录查询）
