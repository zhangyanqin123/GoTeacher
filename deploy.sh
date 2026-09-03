#!/usr/bin/env bash
# gyz-server（Go 后端）版本化发版 / 回滚 —— 移植自前端 GoProject-web/deploy.sh（决策 11）
# 镜像按语义版本 tag 命名（1.2.0 风格），旧版本保留可回滚；
# latest 只是「当前运行版本」的指针别名，容器始终运行具体版本 tag。
# 与前端的核心差异（后端本就 compose 管理）：容器切换走 compose
#   APP_TAG=x.y.z docker compose up -d --no-build --no-deps server consumer
# 保留 restart/env 插值/优雅停机既有语义，避免脚本里复制 run 参数造成双事实来源；
# server/consumer 单镜像双二进制，一条 up 同步双服务、门禁同管、回滚整体回；
# 回滚锚点是 tag 而非镜像 ID（compose image: 只认 tag）；门禁预算 120s（首启含 Migrate+Seed）。
# 兼容 macOS 自带 bash 3.2：不用 mapfile / declare -A，去重计数交 awk。
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

REPO=gyz-server
CONTAINER_SERVER=gyz-server     # compose container_name（固定，inspect/logs 直接用）
CONTAINER_CONSUMER=gyz-consumer
KEEP="${KEEP:-5}"                    # prune 保留的版本数
HEALTH_BUDGET=120                    # 双服务健康门禁预算（秒）：server 最坏判定 30+5*6=60s + 首启 Migrate+Seed
CONSUMER_STABLE_SECS=10              # consumer 就绪稳定窗：持续 running 该秒数且零重启才认可
CONSUMER_READY_PATTERN='order consumers started'  # cmd/consumer/main.go 三队列订阅成功后的就绪日志

info() { printf '\033[32m%s\033[0m\n' "$*"; }
warn() { printf '\033[33m%s\033[0m\n' "$*" >&2; }   # 走 stderr：避免被 $() 命令替换误捕获
err()  { printf '\033[31m%s\033[0m\n' "$*" >&2; }

# 从 .env 提取变量值（compose 插值读 .env，本脚本的 docker build 不读——shell 环境优先，回落 .env）
env_val() { sed -n "s/^$1=//p" .env 2>/dev/null | tail -1; }

# server 容器当前运行的镜像引用（如 gyz-server:1.2.0），未运行则为空
current_image() { docker inspect -f '{{.Config.Image}}' "$CONTAINER_SERVER" 2>/dev/null || true; }

# 当前运行版本 tag：优先 Config.Image 引用；引用是 latest 时（手动 compose up 的场景）
# 改用镜像 ID 与各版本 tag 比对（--no-trunc 才是完整 ID，与 inspect .Image 同格式），保证 list 仍能标注当前运行
current_tag() {
  local cur id
  cur=$(current_image)
  if [ -n "$cur" ] && [ "$cur" != "$REPO:latest" ]; then
    echo "${cur#$REPO:}"
    return 0
  fi
  id=$(docker inspect -f '{{.Image}}' "$CONTAINER_SERVER" 2>/dev/null || true)
  [ -z "$id" ] && return 0
  docker images "$REPO" --no-trunc --format '{{.Tag}}\t{{.ID}}' \
    | awk -F'\t' -v id="$id" '$1!="latest" && $1!="<none>" && $2==id {print $1; exit}'
}

# 镜像存在性预检：--no-build 切换时 compose 缺镜像报 pull access denied（文案误导），这里快速失败给干净信息
require_image() { # $1=tag
  docker image inspect "$REPO:$1" >/dev/null 2>&1 \
    || { err "镜像 $REPO:$1 不存在（./deploy.sh list 查看可用版本）"; return 1; }
}

# 构建：版本 tag + latest 双打标；GIT_REV 记录构建时代码提交（语义 tag 不含 hash，靠它溯源）；
# compose 日常 up -d --build 路径不传版本 build-arg（落 dev/unknown），版本 ENV 只由此处注入；
# 弱网换源变量非空才透传（防空串覆盖 Dockerfile ARG 默认值），shell 环境优先、回落 .env
build_img() { # $1=tag
  local go_img rt_img goproxy
  go_img="${GO_IMAGE:-$(env_val GO_IMAGE)}"
  rt_img="${RUNTIME_IMAGE:-$(env_val RUNTIME_IMAGE)}"
  goproxy="${GOPROXY:-$(env_val GOPROXY)}"
  docker build \
    ${go_img:+--build-arg GO_IMAGE=$go_img} \
    ${rt_img:+--build-arg RUNTIME_IMAGE=$rt_img} \
    ${goproxy:+--build-arg GOPROXY=$goproxy} \
    --build-arg IMAGE_TAG="$1" \
    --build-arg GIT_REV="$(git rev-parse --short HEAD)" \
    -t "$REPO:$1" -t "$REPO:latest" .
}

# 切换 server/consumer 到指定版本 tag：--no-build 只用已构建镜像，--no-deps 硬保证不动中间件
# （中间件前置条件由 preflight 承担）；一条 up 同步双服务，单镜像双二进制中间态无版本偏斜
switch_tag() { # $1=tag
  APP_TAG="$1" docker compose up -d --no-build --no-deps server consumer >/dev/null
}

# 用镜像 ID 恢复（无版本 tag 历史的首发场景）：ID 重指 latest 后按 latest 切换
recover_by_id() { # $1=镜像 ID
  docker tag "$1" "$REPO:latest" >/dev/null || return 1
  switch_tag latest
}

# 门禁失败出口：dump 两容器日志后返回 1（调用方进入回滚/恢复）
gate_fail() { # $1=原因
  err "健康门禁失败: $1"
  echo "---- $CONTAINER_SERVER 日志 ----"
  docker logs --tail 20 "$CONTAINER_SERVER" 2>/dev/null || true
  echo "---- $CONTAINER_CONSUMER 日志 ----"
  docker logs --tail 20 "$CONTAINER_CONSUMER" 2>/dev/null || true
  return 1
}

# 双服务健康门禁：2s 轮询，预算内 server healthy 且 consumer 就绪则通过；
# 任一容器 unhealthy / exited / restarting / 被重启 / 消失即刻失败并 dump 日志。
# server 走 compose healthcheck（/health 探针）；consumer 无 healthcheck 无端口
# （唯一进程即 PID 1，进程退出容器即退出），用 running + RestartCount==0 + 稳定窗 + 就绪日志兜底
health_gate() {
  local deadline=$(( SECONDS + HEALTH_BUDGET )) st c_since=0 c_ok=0 s_ok=0 s_since=0
  while [ "$SECONDS" -lt "$deadline" ]; do
    # --- server：compose Health.Status 优先；Docker Desktop 偶发 health 状态机卡 starting
    #（实测 /health 已连续 200 却 120s 不转 healthy，疑 VM 资源压力下探测调度延迟，与
    #  PLAN-docker 已知的 VM 内存挤压同族）——starting 超 20s 后容器内直探兜底，
    #  与 compose healthcheck 同一条 wget，绕开状态机直接验证服务响应 ---
    if [ "$s_ok" -eq 0 ]; then
      st=$(docker inspect -f '{{.State.Status}}/{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$CONTAINER_SERVER" 2>/dev/null || echo gone)
      case "$st" in
        running/healthy) s_ok=1 ;;
        running/starting|running/)
          [ "$s_since" -eq 0 ] && s_since=$SECONDS
          if [ $(( SECONDS - s_since )) -ge 20 ] \
             && docker exec "$CONTAINER_SERVER" wget -q --spider http://127.0.0.1:8080/health 2>/dev/null; then
            warn "server healthcheck 状态机未转 healthy，容器内直探 /health 通过（视为就绪）"
            s_ok=1
          fi ;;
        *) gate_fail "server: $st"; return 1 ;;
      esac
    fi
    # --- consumer：状态/重启数合一探测（restarting 与 exited 同为失败；非 0 重启即 crashloop）---
    # 注意 RestartCount 是 inspect 顶层字段（不在 State 里，写 .State.RestartCount 会模板报错误判 gone）
    if [ "$c_ok" -eq 0 ]; then
      st=$(docker inspect -f '{{.State.Status}}/{{.RestartCount}}' "$CONTAINER_CONSUMER" 2>/dev/null || echo gone)
      case "$st" in
        running/0)
          [ "$c_since" -eq 0 ] && c_since=$SECONDS
          if [ $(( SECONDS - c_since )) -ge "$CONSUMER_STABLE_SECS" ] \
             && docker logs "$CONTAINER_CONSUMER" 2>&1 | grep -q "$CONSUMER_READY_PATTERN"; then
            c_ok=1
          fi ;;
        *) gate_fail "consumer: $st"; return 1 ;;
      esac
    fi
    [ "$s_ok" -eq 1 ] && [ "$c_ok" -eq 1 ] && return 0
    sleep 2
  done
  gate_fail "门禁超时（预算 ${HEALTH_BUDGET}s）"
  return 1
}

# 发版前置检查：docker 可用 / compose 可解析 / 中间件三件套 healthy。
# switch 用 --no-deps 跳过依赖等待，中间件门由这里显式承担：任一不健康则中止，零容器被动过
preflight() {
  docker info >/dev/null 2>&1 || { err "docker daemon 不可用"; exit 1; }
  docker compose config --services >/dev/null 2>&1 \
    || { err "docker-compose.yml 解析失败（请在项目根目录运行）"; exit 1; }
  local svc cid st
  for svc in mysql redis rabbitmq; do
    cid=$(docker compose ps -q "$svc")
    if [ -z "$cid" ]; then
      err "中间件 $svc 未运行，请先 docker compose up -d"; exit 1
    fi
    st=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid")
    if [ "$st" != "healthy" ]; then
      err "中间件 $svc 状态 ${st}（非 healthy），请先 docker compose up -d 等其就绪"; exit 1
    fi
  done
}

# 版本 tag 列表，版本感知降序（sort -t. 分段数字排：1.10.0 > 1.9.0，字典序会错）。
# 同样不能信 docker images 的输出顺序——BuildKit 层缓存命中时连续构建的镜像 CreatedAt 相同
version_tags() {
  docker images "$REPO" --format '{{.Tag}}' | grep -Ev '^(latest|<none>)$' \
    | sort -t. -k1,1nr -k2,2nr -k3,3nr || true
}

# 下一个语义版本：取现有最大 x.y.z 按指定段位 bump（patch/minor/major）；
# 无语义版本历史时以 1.0.0 为首版（不 bump）；撞名则 patch 递增直到未占用
next_version() { # $1=patch|minor|major
  local base ma mi pa new
  base=$(docker images "$REPO" --format '{{.Tag}}' \
    | awk -F. 'NF==3 && $1 ~ /^[0-9]+$/ && $2 ~ /^[0-9]+$/ && $3 ~ /^[0-9]+$/ {print}' \
    | sort -t. -k1,1nr -k2,2nr -k3,3nr | head -1)
  if [ -z "$base" ]; then echo "1.0.0"; return 0; fi
  IFS=. read -r ma mi pa <<< "$base"
  case "$1" in
    major) ma=$((ma + 1)); mi=0; pa=0 ;;
    minor) mi=$((mi + 1)); pa=0 ;;
    *)     pa=$((pa + 1)) ;;
  esac
  new="$ma.$mi.$pa"
  while docker image inspect "$REPO:$new" >/dev/null 2>&1; do
    pa=$((pa + 1)); new="$ma.$mi.$pa"
  done
  echo "$new"
}

cmd_list() {
  local cur tags
  cur=$(current_tag)
  echo "== $REPO 镜像版本（新→旧）=="
  tags=$(version_tags)
  if [ -z "$tags" ]; then
    echo "  （无镜像版本，首次 deploy 将发 1.0.0）"
  else
    echo "$tags" | awk -v cur="$cur" '{ printf "  %-36s %s\n", $1, ($1==cur ? "<- 当前运行" : "") }'
    # 列表非空但全是 dev 等手工非语义 tag 时，仍提示首发版本号
    if ! echo "$tags" | awk -F. 'NF==3 && $1 ~ /^[0-9]+$/ && $2 ~ /^[0-9]+$/ && $3 ~ /^[0-9]+$/ {f=1} END{exit !f}'; then
      echo "  （暂无语义版本 x.y.z，首次 deploy 将发 1.0.0；其余为手工保留的非语义 tag）"
    fi
  fi
  echo "  （latest 是当前运行版本的指针别名，不算独立版本）"
}

cmd_deploy() { # $1=可省: patch(缺省)|minor|major|显式 x.y.z
  preflight
  if [ "${CHECK:-}" = "1" ]; then
    info "==> CHECK=1: 构建前先跑 go vet"
    go vet ./...
  fi
  local tag arg="${1:-}"
  if [ -z "$arg" ] || [ "$arg" = "patch" ] || [ "$arg" = "minor" ] || [ "$arg" = "major" ]; then
    tag=$(next_version "${arg:-patch}")
  elif [[ "$arg" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    tag="$arg"
  else
    err "无效版本: ${arg}（应为 patch | minor | major 或 x.y.z 形如 1.2.0）"
    exit 1
  fi
  docker image inspect "$REPO:$tag" >/dev/null 2>&1 \
    && { err "镜像 $REPO:$tag 已存在，换一个版本号或用 bump 关键字"; exit 1; }
  if git status --porcelain | grep -q .; then
    warn "工作区不干净，建议先提交再发版（本镜像 GIT_REV 记录的是当前 HEAD）"
  fi
  local prev prev_id
  prev=$(current_tag)   # 版本 tag；可能为空 = 从未用版本 tag 部署过（如手动 up 的 latest）
  # 发版前镜像 ID：无版本 tag 时按它恢复（build 双打标会覆盖 latest，旧内容只剩 ID 可锚定）
  prev_id=$(docker inspect -f '{{.Image}}' "$CONTAINER_SERVER" 2>/dev/null || true)

  info "==> 构建 $REPO:$tag"
  build_img "$tag"   # 失败 set -e 直接退出，运行中容器毫发无损

  info "==> 切换 server/consumer → $tag"
  switch_tag "$tag"

  info "==> 双服务健康门禁（预算 ${HEALTH_BUDGET}s）"
  if health_gate; then
    docker tag "$REPO:$tag" "$REPO:latest" >/dev/null
    info "✔ 上线成功: ${tag}（latest 已重指）"
    warn "server 容器已重建，前端 nginx 若短暂 502 可 docker restart goproject-web 立即恢复（不重启也会 --restart 自愈）"
    if [ -n "$prev" ] && [ "$prev" != "$tag" ]; then
      echo "  如需回退: ./deploy.sh rollback prev    # 即 $prev"
    fi
    cmd_list
    return 0
  fi

  err "✘ 新版本 ${tag} 未通过健康门禁"
  if [ -n "$prev" ]; then
    info "==> 自动回滚 → $prev"
    if switch_tag "$prev" && health_gate; then
      docker tag "$REPO:$prev" "$REPO:latest" >/dev/null
      warn "✔ 已回滚到 ${prev}（latest 已重指）。失败镜像 $REPO:$tag 保留未删，确认无用可 docker rmi 清理"
      exit 1
    fi
  elif [ -n "$prev_id" ]; then
    # 首次版本化部署：无版本 tag 可回，但发版前镜像 ID 还在（latest 刚被新构建覆盖，旧内容只剩 ID）
    info "==> 无版本 tag 历史，按发版前镜像 ID 恢复（${prev_id} → latest）"
    if recover_by_id "$prev_id" && health_gate; then
      warn "✔ 已恢复发版前镜像（latest 重指）。失败镜像 $REPO:$tag 保留未删，确认无用可 docker rmi 清理"
      exit 1
    fi
  else
    err "无历史版本可回滚（首次部署且无运行中容器）"
    exit 1
  fi
  err "FATAL: 回滚/恢复也失败！手工恢复（镜像 ID ${prev_id:-未知}）："
  echo "  docker tag <镜像ID> $REPO:latest && docker compose up -d --no-build --no-deps server consumer"
  exit 1
}

cmd_rollback() { # $1=tag | prev | 空(列表)
  local cur cur_id target
  cur=$(current_tag)
  cur_id=$(docker inspect -f '{{.Image}}' "$CONTAINER_SERVER" 2>/dev/null || true)
  docker inspect "$CONTAINER_SERVER" >/dev/null 2>&1 \
    || { err "容器 $CONTAINER_SERVER 未在运行（先 docker compose up -d）"; exit 1; }

  if [ -z "${1:-}" ]; then
    cmd_list
    echo
    echo "用法: ./deploy.sh rollback prev    # 回滚到上一版本"
    echo "      ./deploy.sh rollback <tag>   # 回滚到指定历史版本"
    return 0
  fi

  target="$1"
  if [ "$target" = "prev" ]; then
    # 版本降序去掉当前运行项，取最新的一个（cur 为空时不匹配任何项，即取最新版本）
    target=$(version_tags | awk -v cur="$cur" '$1!=cur {print; exit}')
    [ -n "$target" ] || { err "找不到可回滚的历史版本"; exit 1; }
  fi
  require_image "$target" || exit 1

  preflight
  info "==> 回滚 ${cur:-latest} → $target"
  if switch_tag "$target" && health_gate; then
    docker tag "$REPO:$target" "$REPO:latest" >/dev/null
    info "✔ 回滚成功: ${target}（latest 已重指）"
    warn "server 容器已重建，前端 nginx 若短暂 502 可 docker restart goproject-web 立即恢复（不重启也会 --restart 自愈）"
    cmd_list
    return 0
  fi

  err "✘ 回滚后健康门禁失败，尝试恢复原版本 ${cur:-<发版前镜像>}"
  if [ -n "$cur" ] && switch_tag "$cur" && health_gate; then
    warn "已恢复原版本 $cur"
  elif [ -n "$cur_id" ] && recover_by_id "$cur_id" && health_gate; then
    warn "已按镜像 ID 恢复原版本（${cur_id} → latest）"
  else
    err "FATAL: 恢复失败！手工恢复（镜像 ID ${cur_id}）："
    echo "  docker tag ${cur_id} $REPO:latest && docker compose up -d --no-build --no-deps server consumer"
  fi
  exit 1
}

cmd_prune() { # $1=-n 干跑
  local dry=n victims protected
  if [ "${1:-}" = "-n" ]; then dry=y; fi
  # 保护集：compose 项目全部容器（含已退出）引用的镜像 ID——删了会让 compose 状态引用悬空；
  # 运行中的 server/consumer 天然在列（当前运行版本永不删），--no-trunc 才与 docker images 完整 ID 同格式
  protected=$(docker compose ps -aq 2>/dev/null | while read -r c; do
    docker inspect -f '{{.Image}}' "$c" 2>/dev/null || true
  done | tr '\n' ' ')
  # 版本 tag 降序（即新→旧），按镜像 ID 去重、保留最近 KEEP 个，其余列出；
  # 当前运行版本计入保留名额（它就是「最近 KEEP 个」之一）但无论何时都不删
  victims=$(docker images "$REPO" --no-trunc --format '{{.Tag}}\t{{.ID}}' \
    | awk -F'\t' '$1!="latest" && $1!="<none>"' | sort -t. -k1,1nr -k2,2nr -k3,3nr \
    | awk -F'\t' -v keep="$KEEP" -v prot="$protected" \
        '!seen[$2]++ { n++; if (n>keep && index(" " prot " ", " " $2 " ")==0) print $1 }')
  if [ -z "$victims" ]; then info "无需清理（保留最近 $KEEP 个版本）"; return 0; fi
  echo "将删除以下旧版本（保留最近 $KEEP 个）:"
  echo "$victims" | sed 's/^/  /'
  if [ "$dry" = "y" ]; then echo "（干跑模式，未实际删除）"; return 0; fi
  local ans
  read -r -p "确认删除? [y/N] " ans || true
  case "$ans" in
    y|Y) echo "$victims" | while read -r t; do docker rmi "$REPO:$t" >/dev/null && info "已删除 $t"; done ;;
    *)   info "已取消" ;;
  esac
  info "提示: 可按需 docker builder prune -f 清构建缓存（防 VM 内存挤压触发 rabbitmq memory alarm），代价是下次构建失去层缓存"
}

usage() {
  cat <<'EOF'
gyz-server（Go 后端）版本化发版 / 回滚
server/consumer 单镜像双二进制：一条 compose up 同步切换，健康门禁覆盖双服务。

用法: ./deploy.sh <命令>

  deploy [bump]  发版：构建语义版本镜像 → compose 切换 server/consumer → 双服务健康门禁（120s）→ 失败自动回滚
                 bump 缺省 patch（1.2.3→1.2.4）；minor（→1.3.0）/ major（→2.0.0）升对应段位；
                 或显式版本号 ./deploy.sh deploy 1.2.0；无历史版本时首版 1.0.0
  rollback       列出可回滚版本（标注当前运行）
  rollback prev  回滚到上一版本
  rollback <tag> 回滚到指定历史版本
  list           列出全部镜像版本
  prune [-n]     清理旧版本（保留最近 KEEP 个，默认 5）；-n 只看清单不删

环境变量（均缺省不传，非空才生效）:
  GO_IMAGE / RUNTIME_IMAGE / GOPROXY   弱网换源，shell 环境优先、回落 .env 同名配置
  KEEP           prune 保留版本数（默认 5）
  CHECK=1        构建前先跑 go vet ./...（弱网下 docker build 前快速失败）

注意: 发版必须走本脚本；手动 docker compose up -d --build 仅用于日常调试（会覆盖 latest 指针且无版本历史）。
      APP_TAG 是脚本内部注入 compose 的变量，手动 compose 命令勿带（会打出无 GIT_REV 的版本 tag 污染历史）。
EOF
}

case "${1:-}" in
  deploy)   shift; cmd_deploy "$@" ;;
  rollback) shift; cmd_rollback "$@" ;;
  list)     cmd_list ;;
  prune)    shift; cmd_prune "$@" ;;
  *)        usage ;;
esac
