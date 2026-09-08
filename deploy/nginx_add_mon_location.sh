#!/bin/bash
# 在 test-dxzg-api.dexunzhenggu.cn.conf 的两个 server 块（443 主入口 / 9091 备用口）各插入
# location /api/v1/mon/event → 127.0.0.1:8090（gyz_mon 服务）
#
# 原则：不删除/覆盖任何既有文件——先 cp 备份原 conf 为新文件 .bak.<日期>_gyzmon；
#       幂等：已存在该 location 则跳过；nginx -t 失败自动用备份回滚。
set -e

CONF=/usr/local/nginx/conf/vhost/dxzg/test-dxzg-api.dexunzhenggu.cn.conf
BAK="${CONF}.bak.$(date +%Y%m%d)_gyzmon"

if grep -q "location /api/v1/mon/event" "$CONF"; then
    echo "==> 已存在 mon/event location，跳过插入"
    exit 0
fi

sudo cp "$CONF" "$BAK"
echo "==> 已备份: $BAK"

# 锚点 location /caizhushou/api 在 443 与 9091 两个 server 块各出现一次，sed 全局在锚点前插入
sudo sed -i '/location \/caizhushou\/api {/i\    # gyz_mon 前端监控上报（H5 探针免鉴权双通道，2026-09-08 新增，服务目录 ../src/gyz_mon）\n    location /api/v1/mon/event {\n        proxy_pass   http://127.0.0.1:8090;\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_set_header X-Forwarded-Proto $scheme;\n    }\n' "$CONF"

echo "==> nginx 配置校验:"
if ! sudo /usr/local/nginx/sbin/nginx -t; then
    echo "==> nginx -t 失败，回滚配置"
    sudo cp "$BAK" "$CONF"
    exit 1
fi

echo "==> reload nginx:"
sudo /usr/local/nginx/sbin/nginx -s reload
echo "==> 完成，插入结果:"
grep -n "gyz_mon\|mon/event" "$CONF"
