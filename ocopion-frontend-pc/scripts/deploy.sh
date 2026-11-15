#!/bin/bash

set -euo pipefail

# GitHub Actions 通过环境变量指定服务器；本地继续兼容 ~/.ssh/config 中的 qyhever。
if [ -n "${SERVER_HOST:-}" ] && [ -n "${SERVER_USER:-}" ]; then
    DEPLOY_TARGET="${SERVER_USER}@${SERVER_HOST}"
elif [ -z "${SERVER_HOST:-}" ] && [ -z "${SERVER_USER:-}" ]; then
    DEPLOY_TARGET="${DEPLOY_TARGET:-qyhever}"
else
    echo "❌ SERVER_HOST 和 SERVER_USER 必须同时设置"
    exit 1
fi

# 显式指定端口时同时应用于 rsync 和 ssh；未指定时沿用 SSH 配置。
SSH_CMD=(ssh)
RSYNC_CMD=(rsync)
if [ -n "${SERVER_PORT:-}" ]; then
    if ! [[ "$SERVER_PORT" =~ ^[0-9]{1,5}$ ]] || (( 10#$SERVER_PORT < 1 || 10#$SERVER_PORT > 65535 )); then
        echo "❌ SERVER_PORT 必须是 1 到 65535 之间的整数"
        exit 1
    fi
    SSH_CMD+=(-p "$SERVER_PORT")
    RSYNC_CMD+=(-e "ssh -p $SERVER_PORT")
fi

PROJECT_ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$PROJECT_ROOT"

export TZ="Asia/Shanghai"

echo "🚀 开始构建前端项目..."
echo "🎯 部署目标: $DEPLOY_TARGET"
echo "🕒 构建时区: $TZ"

pnpm build

echo "📤 开始上传前端构建产物..."
if [ "$DEPLOY_TARGET" = "qyhever" ]; then
    "${RSYNC_CMD[@]}" -avz --delete dist/ "${DEPLOY_TARGET}:/usr/share/nginx/html/ocopion/"
else
    "${RSYNC_CMD[@]}" -avz --delete dist/ "${DEPLOY_TARGET}:/var/www/html/ocopion/"
fi
echo "✅ 前端部署完成！"
