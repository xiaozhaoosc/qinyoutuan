#!/usr/bin/env bash
# 亲友团 · Argo 隧道移植（阶段 A–D）自动化测试脚本
#
# 覆盖：
#   1) go build ./...                    —— 全模块编译
#   2) go vet ./internal/store ./internal/sbproc ./internal/api
#                                        —— 静态检查
#   3) 新增单测：store 的 Argo 变体/规格映射；sbproc 的
#      ArgoArgs / 临时域名解析 / systemd 单元 / 运行时域名缓存 / 规格解析
#   4) bash -n 语法：internal/assets/install-singbox.sh（--with-argo 交付链）
#   5) 启动冒烟：临时库构建二进制 → 冷启动 → /api/health 200 + 引导日志
#
# 用法：
#   bash scripts/test-argo.sh              # 默认
#   GO=/path/to/go QZ_PORT=19083 bash scripts/test-argo.sh
#   bash scripts/test-argo.sh --no-smoke   # 跳过启动冒烟（离线/CI 无端口）
#
# 退出码：全部通过 = 0；任一失败 = 1。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PASS=0; FAIL=0
GREEN=$'\033[1;32m'; RED=$'\033[1;31m'; YELLOW=$'\033[1;33m'; RESET=$'\033[0m'
ok()   { PASS=$((PASS+1)); printf "${GREEN}✓${RESET} %s\n" "$1"; }
bad()  { FAIL=$((FAIL+1)); printf "${RED}✗${RESET} %s\n" "$1" >&2; }
skip() { printf "${YELLOW}−${RESET} %s\n" "$1"; }

# ---- Go 定位：环境变量优先，其次 PATH，再兜底常见 Windows 安装路径 ----
if [ -z "${GO:-}" ]; then
  if command -v go >/dev/null 2>&1; then
    GO=go
  elif [ -x "/c/Program Files/Go/bin/go.exe" ]; then
    GO="/c/Program Files/Go/bin/go.exe"
  else
    echo "error: 找不到 go（可用 GO=/path/to/go 指定）" >&2
    exit 1
  fi
fi

step() { printf "\n${YELLOW}== %s ==${RESET}\n" "$1"; }

step "1/5 全模块编译 go build ./..."
# -buildvcs=false：部分环境（如 Git for Windows 的 bash，git 报 safe.directory）
# 在 build 时做 VCS 探测会失败，该标志跳过 VCS 刻印，对产物无影响。
if "$GO" build -buildvcs=false ./... ; then ok "go build ./..."; else bad "go build ./..."; fi

step "2/5 静态检查 go vet（store / sbproc / api）"
if "$GO" vet ./internal/store ./internal/sbproc ./internal/api ; then ok "go vet"; else bad "go vet"; fi

step "3/5 新增功能单测（阶段 A–D 的核心逻辑）"
(
  "$GO" test -count=1 -v \
    -run 'TestArgo|TestCloudflared|TestParseTemporary|TestQuoteWorld|TestArgoArgs|TestArgoSpec' \
    ./internal/store/ ./internal/sbproc/ 2>&1
) | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok )' | grep -E -- '-- (PASS|FAIL)' 
if "$GO" test -count=1 \
    -run 'TestArgo|TestCloudflared|TestParseTemporary|TestQuoteWorld' \
    ./internal/store/ ./internal/sbproc/ >/dev/null 2>&1; then
  ok "新增单测全部通过（store + sbproc）"
else
  bad "新增单测存在失败"
fi

step "4/5 install-singbox.sh 语法（--with-argo 交付链）"
if command -v bash >/dev/null 2>&1; then
  if bash -n internal/assets/install-singbox.sh && bash -n scripts/test-argo.sh; then
    ok "bash -n internal/assets/install-singbox.sh"
  else
    bad "bash -n install-singbox.sh"
  fi
else
  skip "无 bash，跳过语法检查"
fi

SMOKE="${SMOKE:-1}"
if [ "${1:-}" = "--no-smoke" ]; then SMOKE=0; fi

step "5/5 启动冒烟（临时库 + /api/health）"
if [ "$SMOKE" != "1" ]; then
  skip "已按 --no-smoke 跳过启动冒烟"
else
  TMPD="$(mktemp -d)"
  # 命名带 .exe：Windows（Git Bash）下 Go 产物必须带扩展名才能执行，
  # Linux 下带 .exe 也照常可跑，故跨平台安全。
  BIN="$TMPD/qztest.exe"
  PORT="${QZ_PORT:-18083}"
  PID=""
  # wait 让进程真正退出、释放 SQLite 的 -wal/-shm 文件后再删临时目录。
  # Windows 上 kill 是异步的，不等就直接 rm 会报 Device or resource busy。
  cleanup() {
    [ -n "$PID" ] && kill "$PID" 2>/dev/null || true
    [ -n "$PID" ] && wait "$PID" 2>/dev/null || true
    sleep 1
    rm -rf "$TMPD" 2>/dev/null || true
  }
  trap cleanup EXIT

  if ! "$GO" build -buildvcs=false -o "$BIN" . ; then bad "启动冒烟：构建失败"; else
    # 64-hex 的固定密钥，仅测试用
    QZ_LISTEN="127.0.0.1:$PORT" \
    QZ_DB="$TMPD/qztest.db" \
    QZ_SECRET_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
    QZ_ADMIN_PASS="TestPass123!" \
    "$BIN" >"$TMPD/out.log" 2>&1 &
    PID=$!

    # 等 /api/health 就绪（最多 30s）
    ready=0
    for _ in $(seq 1 30); do
      if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; then ready=1; break; fi
      sleep 1
    done
    if [ "$ready" != "1" ]; then
      bad "启动冒烟：/api/health 30s 内未就绪"
      tail -n 5 "$TMPD/out.log" >&2 || true
    else
      body="$(curl -fsS "http://127.0.0.1:$PORT/api/health")"
      case "$body" in
        *'"status":"ok"'*) ok "启动冒烟：/api/health 200 OK" ;;
        *) bad "启动冒烟：/api/health 响应异常: $body" ;;
      esac
      if grep -q "listening on" "$TMPD/out.log"; then
        ok "启动冒烟：引导日志确认 'listening on'"
      else
        bad "启动冒烟：未找到 'listening on' 引导日志"
      fi
    fi
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
    sleep 1
    PID=""
  fi
fi

echo
printf "${GREEN}通过 %d 项，${RED}失败 %d 项${RESET}\n" "$PASS" "$FAIL"
[ "$FAIL" = 0 ]