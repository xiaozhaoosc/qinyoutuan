# 自动化测试 · 使用说明

亲友团（qingzhou）提供一套**完整、可复现**的自动化测试流水线，可在本机、远程服务器或 CI
中直接运行，并把验证产物（测试用例清单、结果报告）落盘归档。它复用了 `.github/workflows/ci.yml`
的既有检查项，作为可在任意环境跑的"本地 CI"。

## 入口脚本

| 脚本 | 作用 |
|---|---|
| `scripts/test-full.sh` | **完整流水线**：Go 全量（build/vet/test-race）+ 用例清单 + shell 语法 + Argo 专项 + 前端（test/typecheck/build）+ 启动冒烟，并生成三份报告 |
| `scripts/test-argo.sh` | Argo 隧道专项套件（被 test-full.sh 调用） |

## 快速开始

```bash
# 完整跑一遍（需要 go + node/npm + bash + gcc；race 需 cgo）
bash scripts/test-full.sh

# 只用后端（快速回归，跳过前端三步）
bash scripts/test-full.sh --no-frontend

# 跳过启动冒烟（离线或无空闲端口）
bash scripts/test-full.sh --no-smoke

# 失败时保留临时日志便于排查
bash scripts/test-full.sh --keep-logs
```

### 常用环境变量

| 变量 | 用途 | 示例 |
|---|---|---|
| `GO` | 指定 Go 可执行文件 | `GO=/usr/local/go/bin/go` |
| `QZ_PORT` | 启动冒烟使用的端口（默认 `18083`） | `QZ_PORT=19083` |
| `GOPROXY` | 依赖代理（服务器常用 `goproxy.cn`） | `GOPROXY=https://goproxy.cn,direct` |
| `CGO_ENABLED` | race 依赖 cgo；脚本默认置 `1` | 一般无需设置 |

## 覆盖内容（10 步）

1. **go build ./...** 全模块编译
2. **go vet ./...** 静态检查
3. **go test -race -count=1 ./...** 全量单测（含数据竞争检测）
4. **测试用例清单收集**（`go test -list` 按包列出用例名与数量）
5. **bash -n 语法**：`install.sh` / `internal/assets/install-singbox.sh` / 本脚本
6. **Argo 专项**：`scripts/test-argo.sh`（端口/回源/规格映射，跳过其自带冒烟）
7. **前端单测**：`frontend/tests/*.test.mjs`（node:test）
8. **前端类型检查**：`vue-tsc`（npm run typecheck）
9. **前端构建**：`vite build` → `frontend/dist`（供 go embed）
10. **启动冒烟**：临时库构建二进制 → `/api/health` 200 + 引导日志 `listening on`

## 产物（docs/test-report/）

| 文件 | 内容 | 生成时机 |
|---|---|---|
| `test-cases.md` | 每个 Go 包的测试用例清单（用例名 + 数量 + 全仓合计） | 每次运行第 4 步 |
| `result-<时间戳>.md` | 当次验证结果（每步通过/失败、耗时、失败报文） | 每次运行 |
| `latest.md` | 始终指向最近一次结果的副本，便于稳定引用 | 每次运行 |

## 退出码

- `0` = 全部通过
- `1` = 存在失败（会自动在 stderr 打印失败项 + 相关日志尾部）
- `2` = 参数非法

本机为 Windows 时若无 go/node，建议将源码同步到服务器（Linux 有 systemd + gcc，race
与 sbctl 相关测试才能完整通过）后执行。CI 中网关 `.github/workflows/ci.yml` 已覆盖同一套
检查，二者保持一致，避免"本机能跑、CI 失败"漂移。