#!/usr/bin/env bash
# 亲友团（qingzhou）· 完整自动化测试脚本
#
# 把全仓验证串成一条可复现的流水线，并生成三份可归档的产物：
#   docs/test-report/test-cases.md       —— 测试用例清单（每个 Go 包 `-list` 的用例名 + 数量）
#   docs/test-report/result-<ts>.md      —— 当次验证结果（每步通过/失败、耗时、命令报文尾部）
#   docs/test-report/latest.md           —— 始终指向最近一次结果的软入口（README 可引用）
#
# 覆盖（与 .github/workflows/ci.yml 对齐，可作为本机/服务器跑的 CI 本地版）：
#   1) go build ./...                    全模块编译
#   2) go vet ./...                      静态检查（race 前置）
#   3) go test -race -count=1 ./...      全量单测含数据竞争检测（需 cgo）
#   4) 测试用例清单收集（go test -list ./...）
#   5) bash -n 语法：install.sh + install-singbox.sh + 本脚本
#   6) bash scripts/test-argo.sh         专项套件（Argo 端口/回源/冒烟）
#   7) 前端：npm test（node:test 单测）
#   8) 前端：npm run typecheck（vue-tsc 类型检查）
#   9) 前端：npm run build（vite 产物 = go embed 用的 dist）
#   10) 启动冒烟：临时库构建二进制 → /api/health 200 + 引导日志
#
# 用法：
#   bash scripts/test-full.sh                     # 全套（含前端，需 node/npm）
#   GO=/usr/local/go/bin/go QZ_PORT=18083 bash scripts/test-full.sh
#   bash scripts/test-full.sh --no-frontend       # 跳过前端步骤（纯后端快速回归）
#   bash scripts/test-full.sh --no-smoke          # 跳过启动冒烟（离线）
#   bash scripts/test-full.sh --keep-logs         # 失败时不删临时日志，便于排查
#
# 退出码：全部通过 = 0；任一失败 = 1。
# 依赖：bash、go（race 需 cgo + gcc）；前端步骤需 node/npm。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ---- 产物目录 ----
REPORT_DIR="docs/test-report"
mkdir -p "$REPORT_DIR"
TS="$(date +%Y%m%d-%H%M%S)"
RESULT_FILE="$REPORT_DIR/result-$TS.md"
CASES_FILE="$REPORT_DIR/test-cases.md"
LOG_DIR="$(mktemp -d)"

PASS=0; FAIL=0; SKIP=0
GREEN=$'\033[1;32m'; RED=$'\033[1;31m'; YELLOW=$'\033[1;33m'; CYAN=$'\033[1;36m'; RESET=$'\033[0m'
ok()    { PASS=$((PASS+1)); printf "${GREEN}✓${RESET} %s\n" "$1"; }
bad()   { FAIL=$((FAIL+1)); printf "${RED}✗${RESET} %s\n" "$1" >&2; }
skip()  { SKIP=$((SKIP+1)); printf "${YELLOW}−${RESET} %s\n" "$1"; }
step()  { printf "\n${CYAN}== %s ==${RESET}\n" "$1"; }

# ---- 命令行开关 ----
RUN_FRONTEND=1
RUN_SMOKE=1
KEEP_LOGS=0
for arg in "$@"; do
  case "$arg" in
    --no-frontend) RUN_FRONTEND=0 ;;
    --no-smoke)    RUN_SMOKE=0 ;;
    --keep-logs)   KEEP_LOGS=1 ;;
    *) echo "未知参数: $arg" >&2; exit 2 ;;
  esac
done

cleanup() {
  if [ "$KEEP_LOGS" = "1" ]; then
    echo "→ 保留临时日志：$LOG_DIR" >&2
  else
    rm -rf "$LOG_DIR"
  fi
}
trap cleanup EXIT

# ---- Go 定位 ----
if [ -z "${GO:-}" ]; then
  if command -v go >/dev/null 2>&1; then GO=go
  elif [ -x "/usr/local/go/bin/go" ]; then GO=/usr/local/go/bin/go
  elif [ -x "/c/Program Files/Go/bin/go.exe" ]; then GO="/c/Program Files/Go/bin/go.exe"
  else
    echo "error: 找不到 go（可用 GO=/path/to/go 指定）" >&2
    exit 1
  fi
fi

# race 需要 cgo；README 已在服务器安装 build-essential，这里显式开启，
# 避免"本机能跑、服务器默认 CGO_ENABLED=0 却报 requires cgo"的分叉。
export CGO_ENABLED="${CGO_ENABLED:-1}"

# 记录到 md
record() { printf '%s\n' "$1" >> "$RESULT_FILE"; }
record_ok()   { record "- [x] $1"; }
record_bad()  { record "- [ ] **$1**"; }
record_skip() { record "- (跳过) $1"; }

# ---- 首行模板 ----
cat > "$RESULT_FILE" <<EOF
# 亲友团 · 自动化测试结果

- 执行时间：$TS
- 主机：$(hostname 2>/dev/null || echo '?')
- Go：$("$GO" version 2>/dev/null | head -1)
- GOPROXY：${GOPROXY:-<默认>}
- 参数：$*

## 步骤结果

EOF

now() { date +%s; }
elapsed() {
  local end start
  end="$1"; start="$2"
  local s=$((end - start))
  printf "%dm%02ds" $((s / 60)) $((s % 60))
}

START_ALL="$(now)"

# ================= 1/10 全模块编译 =================
step "1/10 go build ./..."
B=$(now)
if "$GO" build -buildvcs=false ./... ; then ok "go build ./..."; record_ok "go build ./..."; else bad "go build ./..."; record_bad "go build ./..."; fi
record "  - 耗时：$(elapsed "$(now)" "$B")"

# ================= 2/10 静态检查 =================
step "2/10 go vet ./..."
B=$(now)
if "$GO" vet ./... > "$LOG_DIR/vet.log" 2>&1; then ok "go vet ./..."; record_ok "go vet ./..."; else bad "go vet ./..."; tail -20 "$LOG_DIR/vet.log"; record_bad "go vet ./... (log: $(tail -3 "$LOG_DIR/vet.log" | tr '\n' ' '))"; fi
record "  - 耗时：$(elapsed "$(now)" "$B")"

# ================= 3/10 全量测试（含 race） =================
step "3/10 go test -race -count=1 ./..."
B=$(now)
if "$GO" test -race -count=1 ./... > "$LOG_DIR/test.log" 2>&1; then
  ok "go test -race ./...";
  record_ok "go test -race -count=1 ./..."

  # 汇总统计：ok 的包数量 / 明确 ok 的个数
  local_pkgs=$(grep -c '^ok ' "$LOG_DIR/test.log" || true)
  cmd_pkg=$(grep -c '^? ' "$LOG_DIR/test.log" || true)
  record "  - ok 包 $local_pkgs 个；无测试文件包 $cmd_pkg 个；总耗时 $(elapsed "$(now)" "$B")"
else
  bad "go test -race ./...";
  grep -E '^(--- |FAIL|panic|WARNING: DATA RACE)' "$LOG_DIR/test.log" | head -30
  record_bad "go test -race ./...（失败详情见以下报文）"
  record "  - 总耗时 $(elapsed "$(now)" "$B")"
  tail -40 "$LOG_DIR/test.log" >> "$RESULT_FILE"
fi

# ================= 4/10 测试用例清单收集 =================
step "4/10 收集测试用例清单（go test -list）"
B=$(now)
{
  echo "# 亲友团 · Go 测试用例清单"
  echo
  echo "- 生成时间：$TS"
  echo "- 方式：go test -list 按包收集（仅列出名称，不执行）"
  echo
  TOTAL=0
  # go list 遍历所有包（排除无测试的），逐个 -list
  pkgs="$("$GO" list ./... 2>/dev/null)"
  for pkg in $pkgs; do
    pkg_short="${pkg##qingzhou/}"
    [ "$pkg_short" = "qingzhou" ] && pkg_short="."
    list_out="$("$GO" test -list '.*' "$pkg" 2>/dev/null | grep -v '^ok ' | grep -v '^?' || true)"
    if [ -n "$list_out" ]; then
      cnt=$(printf '%s\n' "$list_out" | grep -c '^Test' || true)
      TOTAL=$((TOTAL + cnt))
      echo "## $pkg_short"
      echo
      echo "共 $cnt 个测试"
      printf '%s\n' "$list_out" | sed 's/^/- /'
      echo
    fi
  done
  echo "---"
  echo "全仓合计约 $TOTAL 个测试用例"
} > "$CASES_FILE.tmp"
mv "$CASES_FILE.tmp" "$CASES_FILE"
ok "测试用例清单已写入 $CASES_FILE"
record_ok "测试用例清单已生成 → docs/test-report/test-cases.md"
record "  - 耗时：$(elapsed "$(now)" "$B")"

# ================= 5/10 shell 语法 =================
step "5/10 bash -n 语法检查"
B=$(now)
SHELL_OK=0
if command -v bash >/dev/null 2>&1; then
  if bash -n install.sh && bash -n internal/assets/install-singbox.sh && bash -n scripts/test-full.sh; then
    SHELL_OK=1; ok "bash -n install.sh / install-singbox.sh / test-full.sh"; record_ok "bash -n 三脚本语法"
  else
    bad "bash -n 语法检查失败"; record_bad "bash -n 语法检查"
  fi
else
  skip "无 bash，跳过"; record_skip "无 bash，跳过语法检查"
fi
record "  - 耗时：$(elapsed "$(now)" "$B")"

# ================= 6/10 专项套件（Argo） =================
step "6/10 bash scripts/test-argo.sh（Argo 专项）"
B=$(now)
if bash scripts/test-argo.sh --no-smoke > "$LOG_DIR/argo.log" 2>&1; then
  ok "test-argo.sh"; record_ok "test-argo.sh（Argo 专项，跳过其自带冒烟，统一走本脚本冒烟）"
else
  bad "test-argo.sh"; tail -25 "$LOG_DIR/argo.log"; record_bad "test-argo.sh"
fi
record "  - 耗时：$(elapsed "$(now)" "$B")"

# ================= 7/10 前端单测 =================
if [ "$RUN_FRONTEND" = "1" ]; then
  step "7/10 前端 node:test 单测"
  B=$(now)
  if command -v npm >/dev/null 2>&1 && command -v node >/dev/null 2>&1; then
    ( cd frontend && npm test > "$LOG_DIR/fe.test.log" 2>&1 )
    if [ $? -eq 0 ]; then
      ok "npm test（frontend/tests/*.test.mjs）"
      record_ok "前端 node:test 单测（frontend/tests）"
    else
      bad "npm test"; tail -25 "$LOG_DIR/fe.test.log"; record_bad "前端 node:test 单测"
    fi
  else
    skip "无 node/npm"; record_skip "无 node/npm，跳过前端"
  fi
  record "  - 耗时：$(elapsed "$(now)" "$B")"

  # ================= 8/10 前端类型检查 =================
  step "8/10 前端 vue-tsc 类型检查"
  B=$(now)
  if command -v npm >/dev/null 2>&1; then
    ( cd frontend && npm run typecheck > "$LOG_DIR/fe.ts.log" 2>&1 )
    if [ $? -eq 0 ]; then
      ok "npm run typecheck"; record_ok "前端 vue-tsc 类型检查（0 错误）"
    else
      bad "npm run typecheck"; tail -30 "$LOG_DIR/fe.ts.log"; record_bad "前端 vue-tsc 类型检查"
    fi
  else
    skip "无 npm"; record_skip "无 npm，跳过类型检查"
  fi
  record "  - 耗时：$(elapsed "$(now)" "$B")"

  # ================= 9/10 前端构建 =================
  step "9/10 前端构建（vite build → dist，供 go embed）"
  B=$(now)
  if command -v npm >/dev/null 2>&1; then
    ( cd frontend && npm run build > "$LOG_DIR/fe.build.log" 2>&1 )
    if [ $? -eq 0 ]; then
      ok "npm run build"; record_ok "前端构建（vite，dist 就绪）"
    else
      bad "npm run build"; tail -30 "$LOG_DIR/fe.build.log"; record_bad "前端构建"
    fi
  else
    skip "无 npm"; record_skip "无 npm，跳过前端构建"
  fi
  record "  - 耗时：$(elapsed "$(now)" "$B")"
else
  step "7-9/10 前端（已按 --no-frontend 跳过）"
  skip "--no-frontend"; record_skip "前端三步（--no-frontend 跳过）"
fi

# ================= 10/10 启动冒烟 =================
step "10/10 启动冒烟（临时库 + /api/health）"
B=$(now)
if [ "$RUN_SMOKE" != "1" ]; then
  skip "已按 --no-smoke 跳过启动冒烟"; record_skip "启动冒烟（--no-smoke 跳过）"
else
  TMPD="$(mktemp -d)"
  BIN="$TMPD/qzfull.exe"
  PORT="${QZ_PORT:-18083}"
  PID=""
  cleanup_smoke() {
    [ -n "$PID" ] && kill "$PID" 2>/dev/null || true
    [ -n "$PID" ] && wait "$PID" 2>/dev/null || true
    sleep 1
    rm -rf "$TMPD"
  }
  trap cleanup_smoke EXIT
  if ! "$GO" build -buildvcs=false -o "$BIN" . ; then
    bad "启动冒烟：构建失败"; record_bad "启动冒烟（构建失败）"
  else
    QZ_LISTEN="127.0.0.1:$PORT" \
    QZ_DB="$TMPD/qzfull.db" \
    QZ_SECRET_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
    QZ_ADMIN_PASS="TestPass123!" \
    "$BIN" >"$TMPD/out.log" 2>&1 &
    PID=$!
    ready=0
    for _ in $(seq 1 30); do
      if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; then ready=1; break; fi
      sleep 1
    done
    if [ "$ready" != "1" ]; then
      bad "启动冒烟：/api/health 30s 内未就绪"; tail -n 8 "$TMPD/out.log" >&2; record_bad "启动冒烟（health 未就绪）"
    else
      body="$(curl -fsS "http://127.0.0.1:$PORT/api/health")"
      case "$body" in
        *'"status":"ok"'*) ok "启动冒烟：/api/health 200 OK"; record_ok "启动冒烟（/api/health 200）" ;;
        *) bad "启动冒烟：health 响应异常: $body"; record_bad "启动冒烟（health 响应异常）" ;;
      esac
      if grep -q "listening on" "$TMPD/out.log"; then
        ok "启动冒烟：引导日志 'listening on'"; record_ok "启动冒烟（引导日志确认）"
      else
        bad "启动冒烟：未见 'listening on'"; record_bad "启动冒烟（引导日志缺失）"
      fi
    fi
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
    sleep 1
    PID=""
  fi
  rm -rf "$TMPD"
  trap cleanup EXIT
  record "  - 耗时：$(elapsed "$(now)" "$B")"
fi

# ================= 汇总 =================
TOTAL_ELAPSED="$(elapsed "$(now)" "$START_ALL")"
echo
if [ "$FAIL" = 0 ]; then
  printf "${GREEN}全部通过：${RESET}通过 ${PASS} 项"
  [ "$SKIP" != 0 ] && printf "，跳过 ${SKIP} 项"
  printf "（总耗时 %s）\n" "$TOTAL_ELAPSED"
else
  printf "${RED}存在失败：${RESET}通过 ${PASS} 项，失败 ${FAIL} 项"
  [ "$SKIP" != 0 ] && printf "，跳过 ${SKIP} 项"
  printf "（总耗时 %s）\n" "$TOTAL_ELAPSED"
fi

cat >> "$RESULT_FILE" <<EOF

## 汇总

- 通过：$PASS 项；失败：$FAIL 项；跳过：$SKIP 项
- 总耗时：$TOTAL_ELAPSED
- 测试用例清单：docs/test-report/test-cases.md
- 结论：$([ "$FAIL" = 0 ] && echo "✅ 全部通过" || echo "❌ 存在失败")
EOF

# latest.md 软入口（始终指向最近一次）
cp "$RESULT_FILE" "$REPORT_DIR/latest.md" 2>/dev/null || true
printf "${CYAN}产物：%s${RESET}\n" "$RESULT_FILE"

[ "$FAIL" = 0 ]