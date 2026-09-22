#!/bin/bash
# 安装看门狗：把 qz-watchdog.sh 放入 /usr/local/bin，创建 systemd service + 每小时 timer。
set -e
sudo install -m 755 ./qz-watchdog.sh /usr/local/bin/qz-watchdog.sh

sudo tee /etc/systemd/system/qz-watchdog.service > /dev/null <<'UNIT'
[Unit]
Description=QinYouTuan watchdog (nginx + panel health)
After=network-online.target
Wants=network-online.target
[Service]
Type=oneshot
ExecStart=/usr/local/bin/qz-watchdog.sh
UNIT

sudo tee /etc/systemd/system/qz-watchdog.timer > /dev/null <<'UNIT'
[Unit]
Description=QinYouTuan watchdog hourly run
Requires=qz-watchdog.service
[Timer]
OnCalendar=hourly
Persistent=true
Unit=qz-watchdog.service
[Install]
WantedBy=timers.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable --now qz-watchdog.timer
systemctl list-timers qz-watchdog.timer --no-pager -l
echo "--- 手动跑一次验证（服务都在时不应发通知） ---"
sudo /usr/local/bin/qz-watchdog.sh
echo "done"