# PLAN：用户管理（admin_user CRUD）

2026-08-20 实施。需求：新增用户管理页面（新增/编辑/删除登录账号），入口在 gyz-admin 右上角头像下拉菜单；用户信息仅用户名+密码。前端 `userManage.vue` + `adminUser.js`，后端四接口 `POST /api/v1/admin/user/{list,add,edit,delete}`。

## 决策记录

1. **路由前缀 `/api/v1/admin` 独立建组，不挂 `/dxsf`**：admin_user 是系统账号域非业务域，未来管理类接口（操作日志等）可继续挂此组。四接口全 POST + JSON body（同 resign 先例，模糊搜索含中文免 URL 编码）。
2. **编辑密码可选**：`password` 留空=不修改密码，repository 两条 UPDATE 二选一（单条原子）；前端编辑弹窗密码非必填、placeholder「留空则不修改密码」。
3. **踢下线规则**（tokenKey = `auth:token:{id}`）：
   - 删除账号：必 DEL——否则已删账号 JWT 在 TTL 内仍过鉴权（白名单 key 未失效）
   - 改密码且目标非操作者本人：DEL（强制重登）；本人改密不踢（单设备模式踢了自己体验差且无安全收益）
   - 仅改用户名不踢：token/Redis key 均以 userID 为准，getinfo 实时查库，无陈旧问题
4. **删除守卫**：`id == operatorID` 返 400「不能删除当前登录账号」（service 层比对，operatorID 由 handler 从 `c.GetInt64(model.CtxKeyUserID)` 取）。
5. **用户名唯一**：service 先 `ExistsAdminUserByUsername(username, excludeID)`（编辑排除自身）返 400「用户名已存在」；并发窗口由库内 `uk_username` 唯一索引兜底（1062 走 500）。
6. **404 判定走 SELECT 不走 RowsAffected**：UPDATE/DELETE 前先 `GetAdminUserByID` 判 nil（MySQL 值未变时 affected rows 也是 0，同 ExistsTeacher 先例注释）。
7. **INSERT 显式写表单外默认值**（不依赖 schema DEFAULT，代码内可审计）：nickname=username 兜底、role='admin'、avatar=''、status=1；last_login_* 不写保持 NULL。
8. **密码永不输出**：列表 SELECT 不查 password 列 + `AdminUser.Password json:"-"` 双保险；前端编辑回显 password 恒置空串。
9. **前端路由静态 hidden**（同 `/profile` 先例，constantRoutes 注册），不进后端动态菜单；入口在 Navbar.vue 头像下拉菜单「个人中心」与「退出登录」之间。
10. **无需迁移**：admin_user 表已存在（PLAN-auth.md 建表），`uk_username` 已就绪。

## 验证结论

- `go build ./... && go vet ./... && go test ./...` 全绿；Swagger 四接口收录（`/admin/user/*`）
- curl 冒烟：401 鉴权 → list 无 password 字段 → add 重名 400 → edit 留空密码不改 → 改密后旧密码失效 → 删除后目标 token 401 → 删自己 400 → 不存在 404（详见 README「用户管理接口」）
