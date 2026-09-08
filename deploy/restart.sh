#!/bin/bash
# 重启 gyz_mon 服务（Linux 测试服务器部署用，与 go build 产物、.env 一起 scp 上传）
#
# 流程：pkill 杀旧进程 -> chmod +x 二进制 -> nohup 后台启动 -> 日志写 server_out.log
# 本项目无 -c/子命令：config.Load 用 godotenv 读工作目录 .env，
# 故必须 cd 到部署目录再启动（模式对齐同目录 dxzg_api/restart.sh）。
#
# 用法:
#   ./restart.sh                默认二进制 gyz_mon
#   ./restart.sh my_binary      自定义二进制名

set -e

BINARY="${1:-gyz_mon}"
LOG_FILE="${LOG_FILE:-server_out.log}"

# 脚本所在目录（无论从哪调用都能定位到二进制和配置）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [ ! -f ".env" ]; then
    echo "==> 错误：当前目录下找不到 .env"
    exit 1
fi
if [ ! -f "$BINARY" ]; then
    echo "==> 错误：当前目录下找不到二进制 $BINARY"
    exit 1
fi

echo "==> 工作目录: $SCRIPT_DIR"
echo "==> 二进制: $BINARY  日志: $LOG_FILE"

# ---------- 1. 杀掉旧进程 ----------
# 按进程名精确匹配（不带 -f，避免误杀命令行里含 gyz_mon 的 ssh/bash 会话）
# 没有匹配进程时 pkill 返回非零，属正常
if pkill -9 -e "$BINARY" 2>/dev/null; then
    echo "==> 已 kill 旧进程"
else
    echo "==> 当前无运行中的 $BINARY 进程，跳过 kill"
fi
sleep 1

# ---------- 2. 给二进制加可执行权限 ----------
chmod +x "$BINARY"
echo "==> 已 chmod +x $BINARY"

# ---------- 3. 后台启动，日志覆盖写入 server_out.log ----------
# nohup + & 脱离终端，关闭 SSH 也不退出
# GIN_MODE 用 env 前缀注入而非 .env：gin 包 init 时读环境变量，早于 godotenv.Load() 注入
nohup env GIN_MODE=release "$SCRIPT_DIR/$BINARY" > "$LOG_FILE" 2>&1 &
APP_PID=$!
echo "$APP_PID" > app.pid

# 启动要连远端中间件（MySQL/Redis/MQ 全在 192.168.61.1）+ Migrate，多留几秒再探活
sleep 3
if kill -0 "$APP_PID" 2>/dev/null; then
    echo "==> 启动成功，PID=$APP_PID"
    echo "==> 日志实时查看: tail -f $SCRIPT_DIR/$LOG_FILE"
    ps axu | grep "$BINARY" | grep -v grep || true
else
    echo "==> 警告：进程未正常运行，请查看日志: cat $SCRIPT_DIR/$LOG_FILE"
    exit 1
fi
