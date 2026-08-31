# PLAN-product-crud:商品管理 CRUD(订单管理模块新增「商品列表」tab)

> 需求:① 前端订单管理模块新增「商品列表」tab,支持商品信息增删改查;② 对应后端接口开发。2026-08-31 落地。

## 接口契约(POST /api/v1/products/*,挂 Auth,对齐 admin/user 域动作型风格)

| 接口 | 请求体 | 成功响应 | 失败 |
|---|---|---|---|
| `/products/list` | `{product_name?, page_index, page_size}`(名称模糊) | `success` + `data.list []Product / data.count`,id 倒序 | 400 请求体非法 |
| `/products/add` | `{product_name≤100 必填, price>0 必填, stock≥0}` | `新增成功`,data null | 400 请求体非法 |
| `/products/edit` | `{id>0, product_name, price, stock}` | `编辑成功` | 404 商品不存在 |
| `/products/delete` | `{id>0}` | `删除成功` | 404 商品不存在 |

- GET `/orders/products`(创建订单下拉)**保留不动**,与管理列表接口并存
- snake_case 全链路;`ProductManageListResp` 为 Swagger 分页响应类型( `ProductListResp` 已被下拉占用)

## 设计决策

1. **落点**:商品属订单域,方法全部加进现有三层 `order.go` 文件(`handler/service/repository` 各 4 方法)+ `model/order.go`(Req 类型),不新建域文件不迁移代码
2. **删除 = 物理 DELETE**:`orders.product_name` 冗余快照、无外键(schema 设计本就考虑商品可删),历史订单展示不受影响;删除后下单 `GetProduct → nil → 404`;**在途 MQ 订单会被取消**(DeductProductStock 按 product id UPDATE 影响 0 行 → MarkOrderStockFailed 回滚积分/通知,表现同库存不足,与抢票超卖防护同一路径)。既有行为注意:商品全删光后重启 server,`seedProduct` 空表会重播 4 个种子商品
3. **不做商品名唯一校验**:product 表无 uk,「有校验无约束」不一致,不引入
4. **编辑无并发守卫**:管理员改信息非状态机,对齐 `UpdateAdminUser` 查后改模式;`UpdateProduct` 直接 SET 与消费者异步扣库存(stock = stock - ?)存在覆盖竞态,demo 可接受(repo 注释已注明)
5. **stock=0 合法**(售罄)故 binding 用 `gte=0` 不加 required;price=0 被 `required` 拒(demo 无 0 元商品)
6. **前端拆子组件**:`pages/order/ProductTab.tsx`(搜索+表格+操作列)+ `ProductEditModal.tsx`(新增/编辑合一弹窗),挂 `index.tsx` 第 2 个 tab;路由菜单零改动
7. **tab 间联动**:`OrderPage` 维护 `productsKey`,ProductTab 增删改成功回调 `onMutated` 使其 +1,`OrderCreate` 的商品下拉 useEffect 依赖它重新拉取——否则下拉只在首次挂载拉一次,商品变更后不同步(实测踩到后补)

## 验证记录(2026-08-31)

- **curl 七步**:add 200「新增成功」→ 模糊查命中 → edit 200 → delete 200 → 再删 404「商品不存在」→ 空名 400「请求体非法」→ 未登录 401,全符合契约
- **浏览器全流程**(Playwright 驱动 http://localhost:5173/order):新增「磁吸无线充 ￥149.90/666」→ 编辑改名「Pro」+库存 888(回填正确)→ 模糊查「磁吸」1 条 → Popconfirm 删除后消失 → 创建订单下拉随增删同步出现/消失
- `go build/vet/test` 与前端 `npm run build`(tsc)全通过;`swag init` 生成物已更新(docs/ +862 行)
- 已知非本次问题:antd 静态 `message` 无法消费动态主题的 console warning(request.ts 拦截器全局既有模式)

## 改动文件清单

**GoProject**:`model/order.go`(Req 5 个)、`model/swagger.go`(ProductManageListResp)、`repository/order.go`(PageListProducts/Create/Update/Delete)、`service/order.go`(ListManagedProducts/Create/Update/Delete,复用哨兵 ErrProductNotFound)、`handler/order.go`(ProductList/Add/Edit/Delete)、`router/router.go`(4 路由)、`docs/`(swag 生成物)

**GoProject-web**:`api/order.ts`(ProductRow/ProductListQuery/ProductSaveReq + 4 API)、`pages/order/ProductTab.tsx`(新)、`pages/order/ProductEditModal.tsx`(新)、`pages/order/index.tsx`(五 tab + productsKey 联动)
