# PLAN：mon/event 上报双通道部署到测试服务器

> 分支 `dev_xzp_deploy_mon_ingest`（2026-09-08 实施完成）。本文是部署拓扑与决策记录，
> 重发/排障/回滚均以下面「运维命令」为准。

## 部署拓扑

```
H5 探针（公网）
  → https://test-dxzg-api.dexunzhenggu.cn/api/v1/mon/event
  → nginx（服务器 192.168.1.129，443/9091 两 server 块 location /api/v1/mon/event）
  → 127.0.0.1:8090 gyz_mon（宿主机 nohup 进程，/data/dxzg/web-server/server/src/gyz_mon/）
  → MySQL 192.168.61.1:3307 / Redis 192.168.61.1:6380 / RabbitMQ 192.168.61.1:5672（本地 Mac docker 映射）
```

- 源码不上传服务器，只传产物三件套：`gyz_mon`（linux/amd64 静态二进制）+ `.env` + `restart.sh`
- 服务器：CentOS 7 x86_64；SSH `dev@192.168.1.129`（sudo 免密；**22 端口间歇瞬抖，超时重试即可**，443 服务不受影响）

## 代码改动（本分支）

- `internal/router/router.go`：仅保留 `GET/POST /api/v1/mon/event`，其余路由整体注释
  （含 swagger/health/login/业务/管理台）；未使用的 handler 局部变量与 swagger/response
  import 同步注释（unused 编译不过）；CORS 保留（H5 跨域上报前提）。依赖链 repo→service→handler
  组装不动，main 的 fail-fast/Migrate 全保留。**合回 main 前还原全部注释**
- **勿跑 `go mod tidy`**：swaggo 依赖因 import 注释暂显 unused，tidy 会从 go.mod 删掉它们

## 关键决策

| 决策 | 理由 |
|---|---|
| 端口 8090 | 侦察时空闲；nginx 直连 127.0.0.1:8090（仿 conf 内 `/rpc/hq/v1 → 127.0.0.1:8800` 先例，不建 upstream） |
| `.env` 独立文件不上 git（`deploy/.env.server` 被 gitignore） | 含 DB 密码/JWT_SECRET；本地 `.env` 不动（本地开发流继续 127.0.0.1） |
| `MON_ALERT_ENABLED=false` | 服务器实例与本地实例共用同一 MySQL，双跑告警 ticker 会重复扫描/推送（代码注释明确单实例部署） |
| `GIN_MODE` 用 `nohup env GIN_MODE=release` 前缀注入 | gin 包 init 读环境变量**早于** godotenv.Load()，写 .env 不生效（容器化没暴露此问题因 compose 直接注入进程 env） |
| nginx 用幂等脚本改（`deploy/nginx_add_mon_location.sh`） | 先 `cp` 备份成新文件（不删任何既有文件）→ sed 在两个 server 块的 `location /caizhushou/api` 锚点前插入 → `nginx -t` 失败自动回滚。`/api/v1/**` 前缀此前无人认领，零冲突 |
| restart.sh 仿 dxzg_api/restart.sh | cd 部署目录（godotenv 读 cwd 的 .env）→ pkill -9 → chmod +x → nohup → server_out.log + app.pid → 3s 后探活（连远端中间件比本机部署慢） |

## 实测记录（2026-09-08）

| 验证项 | 命令要点 | 结果 |
|---|---|---|
| 服务器→中间件连通 | `/dev/tcp/192.168.61.1/{3307,6380,5672}` | 三端口全通 |
| 服务器本机 GET | `curl 127.0.0.1:8090/.../mon/event?d=...` | 200 + image/gif 43B |
| 服务器本机 POST | `{"t":"boot",...}` | `{"code":200,"data":{"accepted":1}}` |
| 域名 GET | `--get --data-urlencode 'd={"t":"device",...}'` | 200 + 43B（~68ms） |
| 域名 POST | `{"t":"win_error",...}` | `accepted:1`（~44ms） |
| 端到端落库 | 本地 `mon_event` 表 | 域名请求 2 条入库，ip=192.168.61.1（nginx X-Real-IP 链路生效） |
| 白名单校验 | `t=deploy_test` | rejected:1（未知类型拒收，符合设计） |

## 运维命令

```bash
# 改代码后重发（本地 Mac）：
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/gyz_mon ./cmd/server   # 首次全量 ~2min，增量秒级
scp /tmp/gyz_mon dev@192.168.1.129:/data/dxzg/web-server/server/src/gyz_mon/
ssh dev@192.168.1.129 '/data/dxzg/web-server/server/src/gyz_mon/restart.sh'

# 看日志/状态：
ssh dev@192.168.1.129 'tail -50 /data/dxzg/web-server/server/src/gyz_mon/server_out.log; netstat -tln | grep 8090'

# 回滚 nginx（还原 conf 内容，不删备份文件）：
ssh dev@192.168.1.129 'sudo cp /usr/local/nginx/conf/vhost/dxzg/test-dxzg-api.dexunzhenggu.cn.conf.bak.20260908_gyzmon /usr/local/nginx/conf/vhost/dxzg/test-dxzg-api.dexunzhenggu.cn.conf && sudo /usr/local/nginx/sbin/nginx -t && sudo /usr/local/nginx/sbin/nginx -s reload'
```

## 风险与注意

- 依赖本地 Mac 在线且 docker 三件套在跑（服务器进程 fail-fast，中间件不可达会退出重启才能恢复）
- 上报事件 `t` 必须命中白名单（probe_fail/chunk_load_error/win_error/resource_error/unhandled_rejection/vue_error/boot/device），否则静默拒收
- 本地 8080 `go run` 与服务器 8090 互不影响；但两者写同一 mon_event 表，管理台查询看到的是全量混合数据
