#!/bin/bash
# XFeedSystem 服务器安装/更新脚本
# 用法: ./install.sh        # 新安装
#       ./install.sh update # 只更新二进制和配置（会先停服务）
# 需要 root 权限

set -e

APP_USER="xfeed"
APP_DIR="/opt/xfeed"
SERVICE_NAME="xfeed-api"
IS_UPDATE="${1:-}"

echo "=== XFeedSystem 服务器${IS_UPDATE:+更新} ==="

# 新安装时才创建用户
if [ "$IS_UPDATE" != "update" ]; then
    if ! id "$APP_USER" &>/dev/null; then
        useradd -r -s /bin/false "$APP_USER"
        echo "✓ 用户 $APP_USER 已创建"
    else
        echo "✓ 用户 $APP_USER 已存在"
    fi
fi

# 创建目录
mkdir -p "$APP_DIR"/{configs,migrations,scripts,logs}
echo "✓ 目录结构已创建"

# 更新时先停服务
if [ "$IS_UPDATE" = "update" ]; then
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        systemctl stop "$SERVICE_NAME"
        echo "✓ 已停止 $SERVICE_NAME"
    fi
fi

# 复制文件（用 mv 先删旧的再放新的，避免 Text file busy）
if [ -f xfeed-api ]; then
    mv xfeed-api "$APP_DIR/xfeed-api.new"
    chmod +x "$APP_DIR/xfeed-api.new"
    mv "$APP_DIR/xfeed-api.new" "$APP_DIR/xfeed-api"
fi
cp configs/config.yaml "$APP_DIR/configs/"
cp -r migrations/*.sql "$APP_DIR/migrations/" 2>/dev/null || true
cp scripts/docker-compose.yaml "$APP_DIR/scripts/" 2>/dev/null || true
cp .env.example "$APP_DIR/.env.example" 2>/dev/null || true
echo "✓ 文件已复制到 $APP_DIR"

# 新安装时的提示
if [ "$IS_UPDATE" != "update" ]; then
    if [ ! -f "$APP_DIR/.env" ]; then
        echo ""
        echo ">>> 请创建 $APP_DIR/.env 文件并填入真实配置："
        echo "  cp $APP_DIR/.env.example $APP_DIR/.env"
        echo "  vim $APP_DIR/.env"
    fi
fi

# systemd 服务
cp deploy/xfeed-api.service /etc/systemd/system/
systemctl daemon-reload
echo "✓ systemd 服务已安装"

# 权限
chown -R "$APP_USER:$APP_USER" "$APP_DIR"

# 更新完成后重启
if [ "$IS_UPDATE" = "update" ]; then
    systemctl start "$SERVICE_NAME"
    echo "✓ 已重启 $SERVICE_NAME"
fi

echo ""
echo "=== $([ "$IS_UPDATE" = "update" ] && echo '更新完成' || echo '安装完成') ==="
echo "  查看状态: systemctl status $SERVICE_NAME"
echo "  查看日志: journalctl -u $SERVICE_NAME -f"
