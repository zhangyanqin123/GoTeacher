# PLAN-ab-module: AB 版模块配置管理（后端域 + 管理台页面 + H5 聚合接口）

> 需求：C 端 H5 gyz-h5-spacestation 的 AB 版（mass 大众版 / data 数据版）模块显隐，从前端本地静态配置（src/config/abModules.ts 单层 map）升级为两级结构由后端统一管理：① 模块（spacestation/f10）+ 配置项（topBanner 等）增删改查接口；② 建表与表结构设计；③ GoProject-web 新增「AB 模块配置」菜单页。2026-09-01 落地。
> 用户确认决策：管理台 CRUD 挂 Auth（防公网篡改 C 端配置），**仅 H5 聚合查询免鉴权**；前端 Tabs 双页签。

## 接口契约（10 个）

管理台（POST 动作型风格，挂 `Auth`，前缀 `/api/v1/ab`）：

| 接口 | 请求体 | 成功响应 | 失败 |
|---|---|---|---|
| `POST /ab/modules/list` | `{module_key?, module_name?, page_index, page_size}`（均模糊） | `success` + `data.list []AbModule / data.count`，含 `item_count` 子查询；sort_no,id 升序 | 400 请求体非法 |
| `GET /ab/modules/options` | — | 全量 `data [{id, module_key, module_name}]`（配置项页父模块下拉） | — |
| `POST /ab/modules/add` | `{module_key≤50 必填, module_name≤100 必填, sort_no}` | `新增成功` | 400 标识格式非法/已存在 |
| `POST /ab/modules/edit` | `{id>0, module_name, sort_no}`（**无 module_key**） | `编辑成功` | 404 模块不存在 |
| `POST /ab/modules/delete` | `{id>0}` | `删除成功` | 400 有子项；404 不存在 |
| `POST /ab/items/list` | `{module_id?, item_key?, page_index, page_size}`（module_id 精确，0/不传=不过滤） | `data.list []AbModuleItem`（含 JOIN 带出 `module_key`，`versions` 输出数组） | 400 请求体非法 |
| `POST /ab/items/add` | `{module_id>0, item_key≤50, item_name≤100, versions≥1, sort_no}` | `新增成功` | 400 key 格式/版本值域/同模块重名；404 模块不存在 |
| `POST /ab/items/edit` | `{id>0, module_id, item_name, versions, sort_no}`（**无 item_key**；module_id 可改=挪模块） | `编辑成功` | 400 版本值域/重名；404 配置项/模块不存在 |
| `POST /ab/items/delete` | `{id>0}` | `删除成功` | 404 配置项不存在 |

H5 聚合（**免鉴权**，直挂引擎同 login 先例）：

```
GET /api/v1/ab/config →
{ "code":200, "msg":"success",
  "data": { "spacestation": { "topBanner":["mass","data"], "carouselAd":["data"], ... },
            "f10": { ...同构... } } }
```

- data 直接是两级 map，无 list/count 包装（非分页结构先例：GET /orders/products data 为数组）；空模块输出 `"key": {}` 不省略，H5 端 `cfg[moduleKey]?.[itemKey]` 无需判空
- 路径不用 `/ab/modules`（那是鉴权组分页列表语义）——config 是全量快照，无参幂等可被网关/CDN 缓存

## 表结构（schema.sql 追加，Migrate 幂等）

- `ab_module`（id, module_key UNIQUE, module_name, sort_no, created_at, updated_at）
- `ab_module_item`（id, module_id 逻辑外键, item_key, item_name, versions VARCHAR(64), sort_no, 时间；`uk_module_item(module_id, item_key)` 复合唯一键一键两用：同模块内唯一 + 最左前缀服务按模块查/数）

## 设计决策

1. **versions 存逗号分隔 VARCHAR(64)**（'mass,data'），不选 JSON TEXT：项目无 JSON 列先例（teacher_resign.salesman_name 逗号串先例）、值域固定枚举永不含逗号、聚合接口 `strings.Split` 直出 `[]string`。service `normalizeVersions` 去重→值域校验→按 `abVersionOrder=[mass,data]` 定序 join，**库内形态恒定**（'mass,data'/'data'），未来扩展版本枚举只改常量无 schema 变更
2. **item_key 的值是 camelCase 原文**（topBanner）——H5 TS 常量名原样存储透传，属业务数据不受 JSON 键名 snake_case 约束（固定字段名 module_key/item_key/versions 等全蛇形）。handler 注释/表注释/本 PLAN 三处显式说明，防后人当「不一致」修正掉
3. **业务键创建后不可改**：EditReq 不含 module_key/item_key（后端契约层杜绝），前端编辑态 disabled + tooltip——H5 按 key 硬引用，改=断链；换 key 走删旧建新。item 的 module_id 可改（挪模块合法，service 校验目标模块存在性 + 目标模块内重名排除自身）
4. **删父模块拦截不级联**：`ErrAbModuleHasItems` 400 + 列表 item_count 列 + Popconfirm 提示三层防误删；级联会让 H5 整模块 UI 瞬间消失（map 丢 key → 全部 v-if 失败），误删影响面过大
5. **module_id 筛选不用指针**：值域从 1 起、0 无业务含义，约定 0/不传=不过滤（与「传 0 是有效过滤值」约定的差异：那条针对值域含 0 的 status 类字段）
6. **versions 非空**：binding `min=1` + 前端 required 双保险——「全隐藏」语义由删除配置项承担，避免误操作静默藏掉整块 UI
7. **key 格式正则**：module_key `^[a-z][a-z0-9_]{0,49}$`（对齐 H5 页面域小写命名）、item_key `^[A-Za-z][A-Za-z0-9]{0,49}$`（camelCase 常量名）；binding 只做长度基础校验，格式/值域在 service 出中文文案
8. **低频配置域不做并发守卫**：service 先查重名覆盖绝大多数场景，DB 唯一键（uk_module_key/uk_module_item）兜底，极端并发撞唯一键 500 可接受
9. **seed 单函数管两表**（seedAbModule 判 ab_module COUNT=0，一个事务插模块显式 id 1/2 + 16 条配置项 module_id 固定引用）：避免两表独立判空的「模块表有数据、item 表空」中间态错插风险（用户自建模块占位 seed 显式 id）。重灌必须两表一起 `TRUNCATE ab_module_item; TRUNCATE ab_module;` 后重启
10. **前端**：`pages/abmodule/` 五文件（index Tabs 双页签照 order 页 + ModuleTab/ItemTab 照 ProductTab + 两个合一 EditModal）；versions 编辑用 Checkbox.Group（大众版/数据版，required），列表 Tag 渲染（mass=蓝/data=橙）；两 tab 独立查询无 refreshKey 联动，**编辑弹窗 open 时重拉模块下拉**保证模块 tab 刚新增的模块可选
11. **免鉴权暴露面**：/ab/config 公网可 GET，仅模块显隐配置无用户数据；将来加敏感字段须重评鉴权

## 验证记录（2026-09-01）

- **curl 全套**：无 token 401 ✓；重复 module_key 400「模块标识已存在」✓；中文 key 400 格式文案 ✓；versions 乱序+重复 `["data","mass","data"]` 归一化存输出 `["mass","data"]` ✓；同模块重名 item_key 400 ✓；非法版本值 `["gray"]` 400 ✓；item_key 含下划线 400 ✓；挪模块编辑 + versions 收敛 ✓；删有子项模块 400 拦截 ✓；删空模块成功 ✓；删/编不存在 404 ✓
- **聚合接口**：无 token 200；两级 map camelCase key 原文输出；carouselAd 仅 `["data"]`；空模块输出 `{}` ✓
- **seed**：全新空表首启自动建表+首灌 2 模块 16 项（item_count=8×2）✓；重启不重复（幂等）✓；TRUNCATE 重灌走同一条 seedIfEmpty 判空路径
- **浏览器全流程**（Playwright 驱动 localhost:5173/abmodule）：左侧菜单「AB 模块配置」出现 ✓；模块表 item_count 列 ✓；配置项表 16 条 versions 彩色 Tag（carouselAd 仅数据版）✓；编辑弹窗 topBanner 置灰 + 模块 Select 回显「空间站（spacestation）」+ Checkbox 双勾选回显 ✓；**编辑闭环**：取消大众版保存 → 列表 Tag 变「数据版」+ updated_at 刷新 → /ab/config 同步 `["data"]` → 勾回恢复 ✓；父模块下拉筛选 8 条 ✓；删 spacestation 弹「模块下存在配置项，请先删除全部配置项」且行数不变 ✓
- **构建**：go build/vet/test 全过；前端 oxlint 仅 1 个 set-state-in-effect warning（与既有 ReportModal/AuditModal 同款模式）、tsc 通过；`swag init` 生成 10 路径 + 15 个 Ab definitions（AbConfigResp.data 为 object/map 型）
- 踩坑备注：keep-alive 多 tab 下 DOM querySelector 会命中 display:none 的旧 tab 表格，脚本验证需按 `offsetParent !== null` 限定可见面板

## 改动文件清单

**GoProject**：`internal/database/schema.sql`（两表 DDL）、`internal/database/ab_module_seed.sql`（新）、`internal/database/database.go`（embed + seedAbModule）、`internal/model/ab_module.go`（新，行模型 3 + Req/Filter 10）、`internal/model/swagger.go`（4 个文档类型）、`internal/repository/ab_module.go`（新，17 方法）、`internal/service/ab_module.go`（新，哨兵 8 + normalizeVersions + 11 方法）、`internal/handler/ab_module.go`（新，10 方法）、`internal/router/router.go`（ab 鉴权组 9 条 + config 免鉴权 1 条）、`docs/`（swag 生成物）

**GoProject-web**：`src/api/abmodule.ts`（新，类型 + 9 API）、`src/constants/abmodule.ts`（新，AB_VERSIONS 字典）、`src/pages/abmodule/`（新，index/ModuleTab/ModuleEditModal/ItemTab/ItemEditModal）、`src/router/pages.tsx`（菜单注册一行）
