#!/usr/bin/env bash
# ============================================================
# sync_sales_user.sh — 从 admin 系统同步业务员到本地 sales_user 桩表
#
# 背景：绑定弹窗人员树走 admin 真实接口（/api/v1/dept + /api/v1/sys-user），
# userId 是 admin 库 sys_user 的真实 id；本地 sales_user 种子仅为前端 mock
# 同构（id 1-25），ID 空间不一致时绑定列表 INNER JOIN 匹配不上 → 列表为空。
#
# 同步逻辑与前端 teacherQuery.vue 的 buildSalesTree 同构：
#   定位「市场部」→ 只取其直接下级部门的人员（更深层部门不展示也不同步）
#
# 用法：
#   TOKEN=<admin登录token> ./scripts/sync_sales_user.sh
# 可选环境变量：
#   ADMIN_BASE   默认 https://test-dxzg-api.dexunzhenggu.cn/admin
#   DB_*         默认读项目根 .env（DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME）
#
# token 获取：登录 gyz-admin 测试服，DevTools → Network 任一 /admin 请求的
# Authorization: Bearer <token> 里的值
# ============================================================
set -euo pipefail

cd "$(dirname "$0")/.."

ADMIN_BASE="${ADMIN_BASE:-https://test-dxzg-api.dexunzhenggu.cn/admin}"
TOKEN="${TOKEN:-}"

if [[ -z "$TOKEN" ]]; then
  echo "错误：缺少 TOKEN（admin 登录 token）" >&2
  echo "用法：TOKEN=<token> ./scripts/sync_sales_user.sh" >&2
  exit 1
fi

# 复用项目 .env 的数据库配置
env_val() { grep -m1 "^$1=" .env | cut -d= -f2; }
DB_HOST="${DB_HOST:-$(env_val DB_HOST)}"
DB_PORT="${DB_PORT:-$(env_val DB_PORT)}"
DB_USER="${DB_USER:-$(env_val DB_USER)}"
DB_PASSWORD="${DB_PASSWORD:-$(env_val DB_PASSWORD)}"
DB_NAME="${DB_NAME:-$(env_val DB_NAME)}"

mysql_exec() { mysql -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" -p"$DB_PASSWORD" "$DB_NAME" "$@" 2>/dev/null; }

auth=(-H "Authorization: Bearer $TOKEN")

# 1. 拉部门树，定位「市场部」（名称全等，与前端 findDeptNode 同逻辑）
dept_json=$(curl -sf -m 15 "${auth[@]}" "$ADMIN_BASE/api/v1/dept") || {
  echo "错误：拉取部门树失败（检查 TOKEN 是否过期、网络是否可达）" >&2; exit 1; }

# jq 公共片段：响应结构兼容 {data: [...]} 或直接数组；递归找「市场部」节点
find_market='
  def find(nodes): nodes[]? as $n |
    (if ($n.deptName // $n.label) == "市场部" then $n else empty end),
    find($n.children // []);
  first(find(.data // .))'

market_node=$(echo "$dept_json" | jq -c "$find_market")
[[ -z "$market_node" || "$market_node" == "null" ]] && {
  echo "错误：部门树中未找到「市场部」" >&2; exit 1; }

# 2. 取市场部直接下级部门（id + 名称，层级≥2 不同步，与前端展示范围一致）
child_depts=$(jq -c '(.children // []) | map({id: (.deptId // .id), name: (.deptName // .label // "")}) | map(select(.id != null))' <<<"$market_node")
dept_count=$(jq 'length' <<<"$child_depts")
[[ "$dept_count" -eq 0 ]] && { echo "错误：「市场部」下无直接子部门" >&2; exit 1; }
# 注：变量统一 ${var} 写法 —— macOS bash 3.2 下 $var 后紧跟多字节字符（中文/全角冒号）
# 会把变量名解析进下一字节，set -u 直接报 unbound variable
echo "市场部直接下级部门 ${dept_count} 个：$(jq -r 'map(.name) | join("、")' <<<"$child_depts")"

# 3. 逐部门拉人员（pageSize=1000 避免分页漏人，与前端一致），部门名随行带上
tmp_users=$(mktemp)
echo '[]' >"$tmp_users"

dept_ids=$(jq -r '.[].id' <<<"$child_depts")
for dept_id in $dept_ids; do
  dept_name=$(jq -r --argjson id "$dept_id" '.[] | select(.id == $id) | .name' <<<"$child_depts")
  users_json=$(curl -sf -m 15 "${auth[@]}" \
    "$ADMIN_BASE/api/v1/sys-user?deptId=$dept_id&pageIndex=1&pageSize=1000") || {
    echo "警告：部门 ${dept_id}（${dept_name}）人员拉取失败，跳过" >&2; continue; }

  # 字段兜底：userId 必有；昵称 nickName || username；电话 phonenumber || phone || mobile
  dept_users=$(echo "$users_json" | jq -c --arg dn "$dept_name" '
    (.data.list // .data // []) | map(select(.userId != null) |
      { id: .userId,
        phone: (.phonenumber // .phone // .mobile // "" | tostring),
        nickname: (.nickName // .username // ""),
        dept_name: $dn })')
  jq -c --argjson a "$(<"$tmp_users")" --argjson b "$dept_users" '$a + $b' <<<"$dept_users" >"$tmp_users.u" && mv "$tmp_users.u" "$tmp_users"
  echo "  部门 ${dept_name}：$(jq 'length' <<<"$dept_users") 人"
done

# 4. 同一人可能在多个部门节点下出现，按 userId 去重（与前端同构）
merged=$(jq -c 'group_by(.id) | map(.[0]) | sort_by(.id)' <<<"$(cat "$tmp_users")")
rm -f "$tmp_users"

total=$(jq 'length' <<<"$merged")
[[ "$total" -eq 0 ]] && { echo "错误：未拉到任何人员" >&2; exit 1; }
echo "去重后待同步：${total} 人"

# 5. 批量 UPSERT（VALUES() 写法：本机 MySQL 8.0.11，AS 别名语法需 8.0.19+；
#    单引号转义防注入，空值落空串对齐 NOT NULL 列）
values=$(jq -r 'map("(\(.id), '\''\(.phone | gsub("'"'"'"; "’"))'\'', '\''\(.nickname | gsub("'"'"'"; "’"))'\'', '\''\(.dept_name | gsub("'"'"'"; "’"))'\'')") | join(", ")' <<<"$merged")

sql="INSERT INTO sales_user (id, phone, nickname, dept_name) VALUES $values
     ON DUPLICATE KEY UPDATE phone = VALUES(phone), nickname = VALUES(nickname), dept_name = VALUES(dept_name)"
echo "$sql" | mysql_exec

# 6. 验证：孤儿绑定应归零
synced=$(mysql_exec -N -e "SELECT COUNT(*) FROM sales_user")
orphan=$(mysql_exec -N -e "SELECT COUNT(*) FROM teacher_sales ts LEFT JOIN sales_user su ON su.id = ts.user_id WHERE su.id IS NULL")
echo "完成：sales_user 现有 ${synced} 行；剩余孤儿绑定 ${orphan} 条"
[[ "$orphan" -eq 0 ]] || echo "提示：仍有 ${orphan} 条绑定指向不在市场部树中的人员（详情列表会缺这些行）" >&2
