#!/bin/bash
# 服务看门狗：检测 nginx / qinyoutuan，异常自动重启并飞书 webhook 通知。
# 由 systemd timer 每小时触发（见 install_watchdog.sh）。安装到 /usr/local/bin/qz-watchdog.sh。
HOOK="https://open.feishu.cn/open-apis/bot/v2/hook/ad7c26d6-4cd5-44d7-bf64-a4689445fd1a"
IP="64.181.244.178"
LOG="/var/log/qz-watchdog.log"

log() { echo "$(date '+%F %T') $*" >> "$LOG"; }

# 发飞书文本消息（python3 做 JSON 转义，避免引号/换行破坏 payload）
send() {
  local text="$1"
  local payload
  payload="$(python3 -c 'import json,sys;print(json.dumps({"msg_type":"text","content":{"text":sys.argv[1]}}))' "$text")" \
    || { log "feishu json build failed: $text"; return; }
  curl -s -m 10 -X POST -H 'Content-Type: application/json' -d "$payload" "$HOOK" >/dev/null 2>&1 \
    || log "feishu send failed: $text"
}

check() {
  local svc="$1"
  if systemctl is-active --quiet "$svc"; then
    return 0
  fi
  local t="$(date '+%Y-%m-%d %H:%M:%S')"
  log "service $svc DOWN, restarting"
  send "服务器异常：${svc} 服务中止，请关注 IP：${IP}, 时间：${t}"
  # 加固：restart 前先清游离进程（历史上 nohup 遗留的 qinyoutuan 占着 8081，
  # 导致 systemd 实例 bind 失败无限重启；见 qz-clear-stray-qinyoutuan.sh）。
  if [ "$svc" = "qinyoutuan" ]; then
    if [ -x /usr/local/bin/qz-clear-stray-qinyoutuan.sh ]; then
      /usr/local/bin/qz-clear-stray-qinyoutuan.sh 2>>"$LOG" || true
    else
      # 兜底：cgroup 非 qinyoutuan.service、exe 为面板二进制的进程全清
      for pid in /proc/[0-9]*; do
        p="${pid#/proc/}"
        grep -q 'qinyoutuan\.service' "/proc/$p/cgroup" 2>/dev/null && continue
        exe="$(readlink -f "/proc/$p/exe" 2>/dev/null)" || continue
        [ "$exe" = "/home/ubuntu/qinyoutuan" ] || continue
        log "watchdog killing stray qinyoutuan pid=$p"
        kill -9 "$p" 2>/dev/null || true
      done
      sleep 2
    fi
  fi
  systemctl restart "$svc" 2>>"$LOG" || true
  sleep 3
  if systemctl is-active --quiet "$svc"; then
    send "${svc} 服务已自动启动，请关注 IP：${IP}, 时间：$(date '+%Y-%m-%d %H:%M:%S')"
    log "service $svc restarted OK"
  else
    send "服务器异常：${svc} 服务重启失败，请关注 IP：${IP}, 时间：$(date '+%Y-%m-%d %H:%M:%S')"
    log "service $svc restart FAILED"
  fi
}

check nginx
check qinyoutuan
exit 0