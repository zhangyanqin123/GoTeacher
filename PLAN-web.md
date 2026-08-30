# PLAN-web — React 管理台（web/）

> 配套前端项目 `web/`：React + Vite + TypeScript(strict) + antd 5，承载 handicap-service 后端全部功能，替代旧 Vue2 前端 gyz-admin 的联调角色（旧项目仅参考交互形态，不复用代码与工具链）。
> 已确认决策：直播透传接口做成「直播工具」调试页；富文本用 @wangeditor + dompurify；绑定业务员手输 user_ids（后端无业务员候选接口）；目录名 `web/`。

## 1. 背景与目标

后端 `handicap-service`（Gin :8080 + MySQL 8 + Redis + RabbitMQ）已实现：JWT 鉴权（Redis 白名单单设备互踢）、老师管理 chatSys（含绑定业务员）、离职转移、诊股记录（报告状态机 + 专业/合规双重审核）、登录账号管理、订单 Demo（MQ 异步链路）、小鹅通直播透传。

旧前端 gyz-admin（Vue2 + element-ui + Node 14）通过 localAuth 联调流对接部分接口，工具链老旧。本计划新建独立 React 管理台，按后端契约全量接入。

## 2. 技术选型

- **React 18（显式钉版，重要）**：Vite 脚手架默认 React 19，antd 5 官方只承诺到 React 18，`@wangeditor/editor-for-react` peer 只声明 17/18——`npm i react@18 react-dom@18` + `@types/react@18 @types/react-dom@18`
- Vite（本机 Node v22.12.0，无版本限制）、TypeScript strict（禁 any，`tsc --noEmit` + npm script `typecheck` 兜底，不引入 lint 体系）
- antd 5（zhCN locale + dayjs）、@ant-design/icons、react-router-dom（createBrowserRouter）、axios
- 富文本：`@wangeditor/editor` + `@wangeditor/editor-for-react`（受控封装，卸载 `editor.destroy()`）+ `dompurify`（v3 自带 TS 类型）
- 无全局状态库：鉴权用 Context + hooks（不 over-engineer）

## 3. 目录结构

```
web/
├── index.html  package.json  vite.config.ts  tsconfig.json
└── src/
    ├── main.tsx                 # ConfigProvider(zhCN) + RouterProvider
    ├── api/
    │   ├── request.ts           # axios 实例 + 拦截器（契约核心，见 §5）
    │   ├── types.ts             # ApiResponse / PageReq / PageResp / ApiError
    │   ├── auth.ts  teacher.ts  resign.ts  diagnose.ts  adminUser.ts  order.ts  live.ts
    ├── constants/               # 字典：diagnose.ts order.ts teacher.ts user.ts
    ├── hooks/
    │   ├── useAuth.tsx          # Context + Provider + useAuth（token + userInfo）
    │   ├── usePagedList.ts      # 搜索表格页统一分页 hook（requestId 竞态丢弃）
    │   └── useTeacherOptions.ts # /options 拉取 + 模块级缓存
    ├── components/
    │   ├── StatusTag.tsx        # 传 dict + value 渲染彩色 tag
    │   ├── RichTextEditor/      # wangeditor 受控封装
    │   ├── RichTextView.tsx     # dompurify 净化（dangerouslySetInnerHTML 唯一收口）
    │   └── SearchCard.tsx       # Card > Form(inline) + 查询/重置 + Table
    ├── layouts/AdminLayout.tsx  # Sider 菜单 + Header 用户下拉（退出）
    ├── router/index.tsx         # createBrowserRouter + RequireAuth
    ├── utils/
    │   ├── token.ts             # localStorage：hs_token / hs_username
    │   └── format.ts            # cleanQuery（去空值）、金额/空时间渲染
    └── pages/
        ├── login/
        ├── teacher/             # TeacherList + EditModal/DetailDrawer/BindSalesmanModal
        ├── resign/              # ResignList + AddResignModal
        ├── diagnose/            # DiagnoseList + ReportModal/AuditModal/DetailDrawer
        ├── user/                # UserList + UserEditModal
        ├── order/               # OrderPage（Tabs 四页：创建/订单/积分/通知）
        └── live/                # 直播调试页
```

分层规则：`api/` 只做 HTTP 与类型（无 UI），页面不直接碰 axios；行/请求类型内联在对应 api 文件，不另设 types/ 目录（避免同名字段两处维护）。

## 4. 后端接口契约摘要（对接约定，依据 router.go + handler + model 实测）

- 统一响应 `{code, msg, data}`；`code===200` 成功；HTTP 状态保留 4xx/5xx；失败 msg 为可展示中文
- JSON 键名全 snake_case；分页入参 `page_index`/`page_size`（默认 10，上限 100）；列表返回 `data.list`/`data.count`
- 401 统一 `{"code":401,"msg":"登录已过期，请重新登录"}`（单设备互踢）
- **login 特例**：失败也 HTTP 200 + `code:400`；`token`/`expire`/`passwd_expired` 在 **body 根**；`ErrInvalidCredentials` 文案含「密码」
- **status 类字段输出字符串** `"1"/"0"`：teacher.status、order.status、order.stock/points/notify_status、notification.is_read；admin_user.status 是 number（与 teacher 相反，字典分开建）
- **数值字段双契约**：诊股 `id/buy_price/buy_num/status`、订单 `status` 必须 JSON number（字符串/空串一律 400）；teacher/resign 的 `dept_id/id/bind_sales_count` 走 FlexInt64 宽容
- **分页回显特例**：`GET /teacher/bind/salesman/list` 返回驼峰 `pageIndex/pageSize`（老师域唯一特例，独立类型勿复用 PageResp）
- 时间后端已格式化 `"YYYY-MM-DD HH:mm:ss"`；可空字段 NULL→`""`，渲染 `-`；日期范围筛选传 `YYYY-MM-DD` 且 begin/end 成对
- 后端 CORS 反射任意 Origin，前端仍走 Vite proxy（`/api`、`/guyuzhoudb` → `http://localhost:8080`）

### 路由清单

| 模块 | 接口 |
|---|---|
| 鉴权 | `POST /login`、`POST /logout`、`GET /getinfo`（baseURL /api/v1） |
| 老师管理 | `POST /dxsf/teacher/list`、`GET /options`、`GET /detail?id=`、`POST /edit`、`GET /bind/salesman/list`、`GET /bind/salesman/users`、`POST /bind/salesman` |
| 离职转移 | `POST /dxsf/teacher/resign/list`、`POST /add` |
| 诊股 | `POST /dxsf/teacher/diagnose/list`、`GET /detail?id=`、`POST /submit/report`、`POST /audit` |
| 用户管理 | `POST /admin/user/list|add|edit|delete` |
| 订单 | `POST /orders`、`POST /orders/list`、`GET /orders/products`、`POST /points/list`、`POST /notifications/list` |
| 直播（公开） | `GET /guyuzhoudb/live/get_login_url?access_token=&user_id=&login_type=(1PC/2H5/3App)&redirect_uri=` → `{login_url, permission_denied_url}`；`GET /guyuzhoudb/live/register_user?access_token=&phone=(11位)` → `{user_id, user_exists}`；400 形状错 / 502 上游错 |

### 关键业务规则

- 老师编辑仅 4 字段：`{id, title, level(0无/3初级/5高级), avatar, sign}`（detail 返回 level/sign，列名 rating/signature 映射）
- 离职转移 `{original_teacher_id, replace_teacher_id, transfer_content}`，快照后端回查；下拉用 `/options`（含停用）
- 诊股状态机：`1 待诊股 →(提交报告)→ 2 待专业审核 →3 专业驳回 / 4 待合规审核 →5 合规驳回 / 6 合规通过(终态)`；3/5 重新提审回落 2
  - **审核换算在前端**（集中在 constants）：专业通过=4 / 驳回=3；合规通过=6 / 驳回=5；驳回时 `reject_reason` 富文本必填（strip 标签后判空）
  - 操作列按 status 渲染：1→编写报告、2→专业审核、3/5→重新提审、4→合规审核、6→查看报告、恒有→详情
- 用户管理：新增 `{username, password(6-64)}`；编辑 password 空=不改；不能删当前登录账号（**getinfo 不返回 id，前端按 `hs_username` 比对禁用**）；删除即踢下线
- 订单：创建 `{product_id, quantity(1-999)}`（金额/快照后端算，user_id 取登录态）；商品下拉 label `名称 ￥价格（库存 N）`、stock≤0 disabled；数量上限=库存；可得积分=`floor(amount)`；下单成功切列表 Tab 观察三步骤异步翻转（需 `go run ./cmd/consumer` + RabbitMQ）
- 直播调试页：`register_user` 按 phone 幂等换 user_id → `get_login_url`（顺序依赖）；表单 access_token（≤512）/phone（11 位数字）/login_type 下拉/redirect_uri（可选 http(s):// 前缀）；login_url 有效期 1 分钟，展示 + 一键复制

## 5. axios 封装设计（src/api/request.ts）

模块扩充（避免 any）：

```ts
declare module 'axios' {
  export interface AxiosRequestConfig {
    silent?: boolean   // 失败不弹全局 message（登录页/静默 getinfo 用）
    rawBody?: boolean  // 返回完整 body 不剥 data（登录特例：token 在 body 根）
  }
}
const instance = axios.create({ baseURL: '/api/v1', timeout: 15000 })
```

1. **请求拦截**：`hs_token` 存在则注入 `Authorization: Bearer <token>`
2. **HTTP 2xx**：`code===200` → 返回 `rawBody ? body : body.data`（泛型由包装函数 `get<T>/post<T>` 声明）；`code!==200`（login 失败即此形态）→ `!silent && message.error`，reject `ApiError`
3. **HTTP 非 2xx**：401 → 清 token/username + 透传 msg + 跳 `/login?redirect=`（模块级 flag 去重——互踢时并发请求同时 401）；其他 → `!silent && message.error(body.msg)`；网络错误 → `网络异常，请稍后重试`
4. 登录特例：`login()` 以 `{rawBody, silent}` 调用，登录页 catch `ApiError` 按 `err.message.includes('密码')` 定位密码框（勿改写 msg）

## 6. 鉴权流

- `utils/token.ts` 管 localStorage 两键：`hs_token`、`hs_username`
- `useAuth`：挂载时无 token 即 ready；有 token 则 silent getinfo → 成功且 roles 非空 setUser；失败清存储。`logout()`：POST /logout（catch 静默，幂等）→ 清存储 → replace('/login')
- 路由：`/login` 独立；`/` 挂 RequireAuth（无 token Navigate login 带 redirect；未 ready Spin）+ AdminLayout；子路由 `/teacher`（index 默认）、`/resign`、`/diagnose`、`/users`、`/order`、`/live`、`* → 404`。不做动态菜单（roles 恒 admin，权限路由属过度设计）
- 登录页：antd Form；msg 含「密码」→ setFields 定位密码框 + focus；否则 username/表单级提示

## 7. 字典与通用件

- `DictItem<V> = { value, label, color }` + `toOptions` 派生下拉 options
- diagnose.ts：状态 1-6（1 default/2 processing/3 error/4 processing/5 error/6 success）+ `CAN_SUBMIT=[1,3,5]` + 换算表 `{专业:{通过:4,驳回:3}, 合规:{通过:6,驳回:5}}`
- order.ts：订单 1/2/3、步骤 0/1/2（三列共用）；teacher.ts：level 0/3/5、status `'1'/'0'` 字符串；user.ts：status number
- qualification：库里自由中文串、精确匹配——用 `AutoComplete`（可输可选，placeholder 注明精确匹配），不做固定 Select
- `StatusTag`：`<StatusTag dict={DIAGNOSE_STATUS} value={row.status} />`
- `usePagedList<T,Q>`：`{list, count, loading, page, pageSize, search(getQuery), reset(emptyQuery), onPaginationChange}`；useRef 自增 requestId 丢弃过期响应；`cleanQuery` 发请求前剔除 `''/null/undefined` 键（诊股不 400 的关键）
- 分页 pageSizeOptions ≤100；绑定业务员小表格默认 5，弹窗内自管局部分页 state（驼峰响应独立类型）

## 8. 富文本方案

- `RichTextEditor`：受控 `value(HTML)/onChange`，ref 持实例，**卸载 editor.destroy()**（防重复挂载/泄漏）；工具栏裁剪标题/加粗/列表/链接/图片，图片 base64 入库（Demo 可接受）
- `RichTextView`：`DOMPurify.sanitize` + dangerouslySetInnerHTML（全项目唯一出现点，净化收口单点）；后端 bluemonday 是主防线，前端是展示兜底
- 使用点：诊股报告、驳回原因、详情只读展示

## 9. 分阶段实施（每阶段独立验证，git 小步提交）

### 阶段 0：脚手架 + axios + 登录鉴权 + 布局
内容：§2/§3 脚手架（**钉 React 18**）+ proxy + request.ts + useAuth/RequireAuth + AdminLayout（菜单全量，子页占位）+ 登录页。
验证：错密码 → 密码框红字（HTTP 200 + code 400）；admin/admin123 → 跳 /teacher；F5 保持登录（getinfo 重放）；改坏 token → 401 自动回登录带 redirect；退出后旧 token curl 任意接口 401。

### 阶段 1：用户管理
内容：adminUser 4 接口 + UserList + 新增/编辑弹窗（密码留空不改 placeholder、6-64 校验）+ Popconfirm 删除（当前账号按 hs_username 禁用）。
验证：新增/编辑（密码留空仍用旧密码登录）/删除他人（其 token 被踢）/模糊搜索。

### 阶段 2：老师管理 + 离职转移（顺带落 usePagedList/StatusTag/字典/useTeacherOptions）
内容：teacher 7 接口 + resign 2 接口；老师列表筛选（id/dept_id/bind_sales_count InputNumber、资质 AutoComplete、状态 -1全部/1/0、更新时间范围、更新人）；编辑弹窗（detail 回显 4 字段）；详情 Drawer；BindSalesmanModal（上半已绑定小表格分页 5，下半 tags 手输数字 user_ids + users 接口回显已绑定参考）；离职列表 + 新增弹窗（options 双下拉 showSearch + transfer_content textarea）。
验证：筛选逐一命中；level 保存后列表 rating 变化；绑定后小表格新增、重复绑定幂等；转移后快照行正确。

### 阶段 3：诊股（富文本 + 状态机）
内容：RichTextEditor/RichView + diagnose 4 接口 + 列表（数值 InputNumber、双时间区间拆四键）+ 操作列按 status + ReportModal + AuditModal（通过 4/6；驳回嵌套填 reject_reason 3/5）+ DetailDrawer（audit_logs 表）。
验证：1→提交→2→专业通过→4→合规通过→6 全链路，audit_logs 累积；专业驳回→3→重新提审回落 2；非法流转 400；`<script>alert(1)</script>` 注入后详情无执行。

### 阶段 4：订单四页 + 直播调试页 + 收尾
内容：order 5 接口 + Tabs 四页（创建页实时金额/积分、下单成功切列表刷新；列表三步骤 Tag；积分/通知列表）+ live 2 接口调试页（register_user 换 user_id → get_login_url 复制）+ README「前端」章节 + `npm run build` 零错误。
验证：**额外终端 `go run ./cmd/consumer`**（否则状态永不翻转）；下单观察 status 1→2、三步骤 0→1；积分=floor(金额)、通知出现；买库存 3 商品超量 → 订单 3 已取消；直播页无凭证验证 502 文案展示。

## 10. 风险与注意点（实现逐条对照）

1. **React 19 陷阱**：脚手架默认 19，antd5/wangeditor peer 不符，必须钉 18
2. **数值字段双契约**：诊股/订单严格 JSON number（空串 `buy_price:""` 直接 400）——InputNumber + cleanQuery 剔空
3. **status 类型不统一**：teacher 字符串 / admin_user number / diagnose、order number——字典分开建，Select value 类型跟着走
4. **审核换算集中 constants**，白名单 3/4/5/6 直传；驳回富文本 strip 标签判空
5. **登录响应特例**：不能只按 HTTP 状态判断；勿吞/改写含「密码」的 msg
6. **绑定业务员无数据源**：后端只有 user_ids 入参与已绑定查询；手输 ID 方案；勿假设 admin/user 就是业务员域（如需真选择器须后端补接口）
7. **401 互踢去重**：并发请求同时 401 防多次 toast/跳转；本地残留（含 hs_username）清干净
8. **wangeditor 生命周期**：unmount 必须 destroy；dangerouslySetInnerHTML 仅 RichTextView 一处
9. 分页边界：默认 10（绑定 5），上限 100，pageSizeOptions 不放 200
10. `.gitignore` 补 node_modules/dist

## 11. 端到端验证

```bash
# 后端
brew services start mysql
/usr/local/opt/redis@6.2/bin/redis-server --daemonize yes
go run ./cmd/server          # :8080，种子账号 admin/admin123
# 订单联调：
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management
go run ./cmd/consumer

# 前端
cd web && npm install && npm run dev   # :5173，proxy → :8080
```

## 实测记录

### 2026-08-30 全五阶段实施完成（Playwright 无头 Chrome 驱动真实浏览器 + curl 双通道验证）

**阶段 0（脚手架 + 登录鉴权）**
- `npm create vite@latest web -- --template react-ts`；**antd 默认装到 6.x，显式退回 antd@5（5.29.3）+ @ant-design/icons@5**；React 钉 18.3.1
- TS 6 模板坑：`baseUrl` 已弃用（paths 用相对 `./src/*`）；`erasableSyntaxOnly` 禁构造器参数属性（ApiError 改显式字段）；`verbatimModuleSyntax` 要求 `import type`
- 拦截器定型：剥壳结果放回 `response.data` 保持 AxiosResponse 类型诚实，`get/post` 统一 unwrap；`postRawBody` 承接登录特例；AuthProvider 包在 RouterProvider 外不能用 useNavigate，logout 改 `window.location.replace` 整页跳转
- 验证 ✓：无 token 访问 /teacher 踢回登录；错误密码文案「用户名或密码错误」定位密码框；admin/admin123 进布局六菜单；F5 保持登录（getinfo 重放显示「系统管理员」）；退出回登录页；零 console 错误

**阶段 1（用户管理）**
- 验证 ✓：列表/新增 smoketest1/编辑回显+密码留空提交「编辑成功」/当前账号删除禁用（`cursor:not-allowed` 灰 span）/删除他人/username 模糊筛选
- 脚本坑：antd 两字按钮自动插空格（「重 置」），`:has-text("重置")` 子串匹配不到，用 `/重\s*置/`

**阶段 2（老师 + 离职转移）**
- 验证 ✓：资质 AutoComplete 筛选命中 9 行全「已认证」；重置回 10 行；编辑弹窗 detail 回显（title「首席投顾test」/level）；保存「编辑成功」；详情 Drawer 全字段；绑定弹窗已绑定小表格（张明 0 行）→ tags 输 9999 →「绑定成功」+ 小表格/参考数组更新；离职转移列表 + 新增（李娜→张明）「转移成功」，快照正确（业务员 5 人逗号串、group_count=5、操作人 admin）
- 脚本坑：Select 双下拉的旧 dropdown 隐藏 DOM 残留，`.first()` 匹配到不可见项——限定 `:not(.ant-select-dropdown-hidden)`

**阶段 3（诊股状态机 + 富文本）**
- curl 全链路 ✓（ID=1 九步）：1→提交→2→专业驳回(3)→重新提审→2→专业通过(4)→合规驳回(5)→重新提审→2→专业通过(4)→合规通过(6)；终态再审核 400「当前状态不允许此操作」；audit_logs 累积 10 条（用户提交/诊股报告/专业审核/合规审核全轨迹）；`<script>alert(1)</script>` 落库即被后端 bluemonday 净化
- 浏览器 ✓（ID=2）：驳回原因留空被前端拦「不能为空」；填原因驳回「已驳回」回 3；重新提审编辑器回显旧报告；提交「提交成功」回落 2；详情 Drawer audit_logs 表含「专业审核员」；`<img onerror=alert(2)>` 载荷不执行、零 page error
- 数据注意：验证脚本会流转种子数据（ID=1 终态 6、ID=2 在 2/3 间往返），重跑需自备 status=1 记录或重灌种子

**阶段 4（订单 + 直播 + 收尾）**
- 环境：RabbitMQ（docker，Up 39h）+ `go run ./cmd/consumer` 已在跑
- 验证 ✓：商品下拉 label「智能手表 Pro（￥1999，库存 38）」；金额/积分实时计算；下单成功自动切列表 Tab；新订单「处理中+三步骤待处理」→ 5 秒刷新「已完成+三步骤成功」（MQ 异步翻转）；积分列表 1999 分+备注快照；通知列表「下单成功」未读；直播页假 token 错误文案「获取小鹅通用户失败」正确透传（502 上游错）
- `npm run build` ✓（3.12s，gzip 704KB）；README 增「前端管理台」章节

### 遗留与后续项

- 构建产物单一 chunk 2.1MB（antd+wangeditor 全量），可 `manualChunks` 拆分或路由级动态 import（Demo 暂不处理）
- 绑定业务员为手输 user_ids（后端无候选列表接口）；如需真选择器须后端补业务员查询接口
- `passwd_expired` 恒 false，登录页未做改密引导（后端先行项）
- ESLint 用模板自带 oxlint（`npm run lint`）；未配 prettier（遵循后端仓库无 lint 工具链的定位）
