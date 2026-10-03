#!/bin/bash
# 清理游离的 qinyoutuan 进程（不受 systemd 托管、占用 8081 的孤儿/nohup 实例）。
# 由 qinyoutuan.service 的 ExecStartPre 与看门狗在 restart 前调用。
#
# 背景：历史上曾用 nohup 手动启动过面板，重启机器后这类进程 PPID=1、脱离
# systemd 管理，但继续占着 8081。systemd 托管实例每次启动都会
# "bind: address already in use" 失败并无限 auto-restart（曾循环 1280 次）。
#
# 判定游离：exe 可执行文件 == /home/ubuntu/qinyoutuan，且 cgroup 不含
# qinyoutuan.service。用 exe 路径而非 cmdline：真实游离进程与托管进程的
# cmdline 完全相同（都是 /home/ubuntu/qinyoutuan），只有 exe + cgroup 能区分。
set -u

log() { echo "$(date '+%F %T') $*" >> /var/log/qz-watchdog.log; }

killed=0
for pid in /proc/[0-9]*; do
  pid="${pid#/proc/}"
  # cgroup 属于 systemd 单元 → 托管实例，跳过
  if grep -q 'qinyoutuan\.service' "/proc/$pid/cgroup" 2>/dev/null; then
    continue
  fi
  # exe 解析失败（内核线程/权限）→ 跳过
  exe="$(readlink -f "/proc/$pid/exe" 2>/dev/null)" || continue
  [ "$exe" = "/home/ubuntu/qinyoutuan" ] || continue
  log "clearing stray qinyoutuan pid=$pid (not systemd-managed)"
  kill -9 "$pid" 2>/dev/null || true
  killed=$((killed + 1))
done

if [ "$killed" -gt 0 ]; then
  log "cleared $killed stray qinyoutuan process(es)"
  sleep 2  # 等 socket 释放
fi
exit 0