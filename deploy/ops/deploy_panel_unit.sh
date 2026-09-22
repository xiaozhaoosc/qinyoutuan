#!/bin/bash
# 部署亲友团面板为 systemd 服务（开机自启 + 崩溃自动重启）。
# 依赖：/home/ubuntu/qinyoutuan 二进制、/home/ubuntu/qz-secret 主密钥。
# 用法：sudo bash deploy_panel_unit.sh
set -e
SECRET=$(cat /home/ubuntu/qz-secret)

# 环境文件（含主密钥，权限 600）
sudo tee /etc/qinyoutuan.env > /dev/null <<EOF
QZ_LISTEN=0.0.0.0:8081
QZ_DB=/home/ubuntu/qinyoutuan.db
QZ_SECRET_KEY=${SECRET}
QZ_ADMIN_USER=admin
QZ_PUBLIC_BASE=http://myservs.rouroujuxuan.top/qyt
EOF
sudo chmod 600 /etc/qinyoutuan.env

# systemd 单元：进程级自动重启，防死循环激荡
sudo tee /etc/systemd/system/qinyoutuan.service > /dev/null <<'UNIT'
[Unit]
Description=QinYouTuan Panel
After=network-online.target sing-box.service
Wants=network-online.target
[Service]
Type=simple
User=root
EnvironmentFile=/etc/qinyoutuan.env
ExecStart=/home/ubuntu/qinyoutuan
Restart=always
RestartSec=5
StartLimitIntervalSec=30
StartLimitBurst=5
[Install]
WantedBy=multi-user.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable qinyoutuan.service
sudo systemctl restart qinyoutuan.service
sleep 3
sudo systemctl is-active qinyoutuan.service
sudo systemctl is-enabled qinyoutuan.service
curl -s -o /dev/null -w 'health_http=%{http_code}\n' -m 10 -k https://127.0.0.1/qyt/api/health -H 'Host: myservs.rouroujuxuan.top'