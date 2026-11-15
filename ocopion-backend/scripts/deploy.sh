#!/bin/bash

# 发生错误、使用未定义变量或管道命令失败时退出
set -euo pipefail

# GitHub Actions 通过环境变量指定服务器；本地继续兼容 ~/.ssh/config 中的 name。
if [ -n "${SERVER_HOST:-}" ] && [ -n "${SERVER_USER:-}" ]; then
    DEPLOY_TARGET="${SERVER_USER}@${SERVER_HOST}"
elif [ -z "${SERVER_HOST:-}" ] && [ -z "${SERVER_USER:-}" ]; then
    DEPLOY_TARGET="${DEPLOY_TARGET:-qyhever}" # 默认值
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

# 获取脚本所在目录的上一级目录（项目根目录）
PROJECT_ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$PROJECT_ROOT"

export TZ="Asia/Shanghai"

echo "🚀 开始构建项目..."
echo "🎯 部署目标: $DEPLOY_TARGET"
echo "🕒 构建时区: $TZ"

# 1. 检查并创建 public 目录
if [ ! -d "public" ]; then
    echo "📂 创建 public 目录..."
    mkdir -p public
fi

# 2. 生成 meta.json
echo "📄 生成 public/meta.json..."
CURRENT_TIME=$(date '+%Y-%m-%d %H:%M:%S')
echo "{\"deployTime\": \"$CURRENT_TIME\"}" > public/meta.json

# 3. 生成 Swagger 文档
echo "📚 生成 Swagger 文档..."
go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g ./cmd/main.go -o ./docs

# 4. 编译项目
echo "🔨 正在编译..."
# 设置环境变量进行交叉编译（如需在本机运行可去掉这些变量，这里保留原有逻辑）
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64

go build -o ocopion ./cmd/main.go

echo "✅ 构建成功！"
echo "📍 输出文件: $PROJECT_ROOT/ocopion"
echo "📅 部署时间: $CURRENT_TIME"

echo "📤 开始上传文件到服务器..."
"${RSYNC_CMD[@]}" -avz --progress --partial ./ocopion "${DEPLOY_TARGET}:/opt/apps/ocopion-backend"
"${RSYNC_CMD[@]}" -avz --progress --partial ./public "${DEPLOY_TARGET}:/opt/apps/ocopion-backend"
"${RSYNC_CMD[@]}" -avz --delete --progress --partial ./docs "${DEPLOY_TARGET}:/opt/apps/ocopion-backend"
"${RSYNC_CMD[@]}" -avz --progress --partial ./internal/config/app.yml "${DEPLOY_TARGET}:/opt/apps/ocopion-backend"
# 本地部署时才上传 prod.yml，因为远程仓库没有 prod.yml
if [ -z "${SERVER_HOST:-}" ] && [ -z "${SERVER_USER:-}" ]; then
    "${RSYNC_CMD[@]}" -avz --progress --partial ./internal/config/prod.yml "${DEPLOY_TARGET}:/opt/apps/ocopion-backend"
fi
# ssh "$DEPLOY_TARGET" 'mkdir -p /opt/apps/ocopion-backend/data'
# rsync -avz --progress --partial ./tmp/clash-template.yaml "$DEPLOY_TARGET":/opt/apps/ocopion-backend/data/
echo "✅ 上传完成！"

echo "🔄 重启远程 ocopion 服务..."
"${SSH_CMD[@]}" "$DEPLOY_TARGET" 'systemctl restart ocopion'

echo "📋 查看远程 ocopion 服务状态..."
"${SSH_CMD[@]}" "$DEPLOY_TARGET" 'systemctl status ocopion'
