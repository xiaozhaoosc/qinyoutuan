# 亲友团 (QinYouTuan / QingZhou) 开发&运维交接文档

> 面向团队：换设备/换机器/换人接手本项目的**一站式指引**。
> 最后更新：2026-09-23

---

## 0. 一页速览

| 项 | 值/位置 |
|---|---|
| 项目 | sing-box 订阅管理面板（Go 1.25 + Vue3 + SQLite 单体） |
| 代码仓库 | `git@github.com:xiaozhaoosc/qinyoutuan.git`，**主分支 `dev`**（非 main） |
| 上游/参考 | `git@github.com:mllt992/qing-zhou.git`（origin，只读参考） |
| 生产服务器 | Oracle Cloud ARM64 | IP `64.181.244.178`，域名 `myservs.rouroujuxuan.top` |
| 服务器 SSH | 私钥：本地 `oracle/ssh-key-2026-09-20.key`（**禁止入 git**，见 §8) |
| 面板 | nginx 反代，地址 `https://myservs.rouroujuxuan.top/qyt/` |
| 已被植入 | **Cloudflare Argo 隧道接入**（阶段 A–G 见 §5） |
| 自动化测试 | `scripts/test-full.sh`（完整 10 步流水线，产物落 `docs/test-report/`）见 §7 |
| CI | `.github/workflows/ci.yml`（`argo-suite` + `frontend`） |

---

## 1. 仓库与分支约定（团队必读）

- **开发在 `dev` 分支**，不要直接推 `main`。上线/合并前在 GitHub 上开 PR。
- 提交信息用**中文 + 前缀**，例如 `feat(argo): ...`、`fix(api): ...`、`docs: ...`。
- 每次改动尽量**一个主题一个提交**；改动前后跑通 §7 的测试。
- **敏感信息绝不入库**：
  - `oracle/`（含 SSH 私钥）已在 `.gitignore` 锁定 `/oracle/`，**不要** `git add oracle/`。
  - 数据库、`.env`、`.key`、`.pem` 均被 `.gitignore` 忽略。
  - 换人或给新人前，建议在团队密钥库（如 1Password/Vault/GitHub Secrets）存证，不要靠这个 md 里的明文传递。

---

## 2. 环境准备（换新机器后）

### 2.1 本地开发
```bash
# 需要 Go 1.25+、Node 20+
git clone git@github.com:xiaozhaoosc/qinyoutuan.git
cd qinyoutuan && git checkout dev
cd frontend && npm install && npx vite build && cd ..
QZ_LISTEN=127.0.0.1:8081 go run .   # Windows 可 ./start.ps1
```
- 前端产物**不入库**（占位符）；`go run` 前必须先 `npx vite build`。
- 改前端用 `cd frontend && npm run dev`（热更新，`/api` 代理到后端）。

### 2.2 跑自动化测试（必跑）
见 §7。

### 2.3 本机 Docker（可选）
`docker compose up -d` 用官方 GHCR 镜像跑面板。注意：**本机镜像源曾失效**，需可用镜像源/代理后再 `docker build`（详见 §6.3）。

---

## 3. 生产服务器现状（接手必看）

> ⚠️ 以下是**运维基线**，任何改动前先确认其中契约（端口/认证），避免改出互斥配置。

- **面板进程**：`/home/ubuntu/qinyoutuan`（root 运行，端口 `8081`）。数据库 `/home/ubuntu/qinyoutuan.db`，主密钥 `~/qz-secret`（**勿丢**，丢了 aes 加密内容解不开）。
- **启动环境**（面板重启时用它，别漏）：
  ```
  QZ_LISTEN=0.0.0.0:8081  QZ_DB=/home/ubuntu/qinyoutuan.db
  QZ_SECRET_KEY=$(cat ~/qz-secret)  QZ_ADMIN_PASS=已改
  QZ_PUBLIC_BASE=http://myservs.rouroujuxuan.top/qyt
  ```
- **sing-box**：`/usr/local/bin/sing-box`（带 v2ray_api），systemd `sing-box`，配置 `/etc/sing-box/config.json`。
- **cloudflared**：`/usr/local/bin/cloudflared`，系统临时隧道由面板拉起（每入站一个 systemd 单元 `cloudflared-<tag>.service`），日志 `/etc/cloudflared/*.log`。
- **nginx 1.28.3**：反代 `8081`（`/qyt` + 根路径），监听 80/443。站点文件 `/etc/nginx/sites-available/qyt`（软链到 enabled）。
- **HTTPS**：Let's Encrypt（HTTP-01）证书 `/etc/letsencrypt/live/myservs.rouroujuxuan.top/*`，certbot 自动续期 timer 已启用。
- **防火墙**：
  - 实例 iptables：已开 `22`（SSH）、`80`、`443`（并已持久化到 `/etc/iptables/rules.v4`）。
  - Oracle **NSG**：需自行在控制台确认（见 §6.1「端口开放清单」）。
- **面板账号**：admin 口令已从弱口令改掉（见团队密钥库/交付时单独下发，勿用公开文档里的旧值）。

SSH 连服务器（新机器）：
```bash
ssh -i oracle/ssh-key-2026-09-20.key ubuntu@64.181.244.178
```

---

## 4. 当前代码里有什么（按目录）

- `internal/store/` 数据层。`SbInbound` 已含 Argo 字段（`argo_enabled/argo_mode/argo_auth/argo_domain/argo_host`），`singbox.go` 含 Argo 出站（`argoVariantLinks`：temporary 只发 443、fixed 才展开 80系×7+443系×6；`argoCDNPorts`）与 **`argoOriginInbound` 派生无 TLS 明文 ws 回源入站**（`127.0.0.1:<ListenPort+10000>`，cloudflared 回源目标）；`migrate.go` 有幂等迁移（含 argo 列、`migrateEncryptArgoAuth` 加密 token）。
- `internal/sbproc/` **Argo 驱动器**（本移植核心）：`cloudflared.go` 含
  `ArgoOriginPortDelta`、`ArgoSpec`、`ArgoArgs`、`ParseTemporaryHostname`、`CloudflaredServiceUnit`、`FindCloudflaredBin`、
  `EnsureLocalArgo`(本机)、`RemoteEnsureScript`/`EnsureRemoteArgo`(远端，阶段E)、`ArgoHost` 缓存。
- `internal/api/`：入站 API（`sb_admin.go` 含 argo 校验与列表 argo 字段）；`argo_remote.go` = `api.StartArgoSync`（远端落地机 Argo 编排）。
- `main.go`：`startLocalArgoSync`(本机) + `StartArgoSync`(远端)。
- `frontend/src/views/AdminSingbox.vue`：入站页「Argo 隧道」配置区（临时/固定、token/域名、当前域名）。
- `internal/assets/install-singbox.sh`：`--with-argo` 装 sing-box(v2ray_api)+cloudflared（**其余节点/裸机用**）。
- `scripts/test-argo.sh`：Argo 专项自动化测试。

---

## 5. 移植进度（阶段 A–G）

| 阶段 | 状态 | 说明 |
|---|---|---|
| A 数据模型+迁移 | ✅ | `SbInbound.argo_*` 字段、`ALTER TABLE` 幂等迁移、`argo_auth` AES 加密+老明文迁移 |
| B cloudflared 交付链 | ✅ | `install-singbox.sh --with-argo` 拉官方 cloudflared（不自建）；定 arm/amd 架构名 |
| C 进程管理+本机胶水 | ✅ | sbproc 驱动器 + `startLocalArgoSync`（本机 systemd 托管，优于 pgrep） |
| D 订阅 13 端口出站 | ✅ | `argoVariantLinks`（**temporary 只发 443**；fixed 才 80系×7 + 443系×6）|
| E 远端落地机 Argo | ◐ 核心+编排完成 | `api.StartArgoSync` 经 sshctl；**尚未真机联调** |
| F API + 前端 | ◐ 后端✅；前端已改待 CI 验证 | 入站页 Argo 配置 + 状态；`node_host_override`/分组授权见 §6.2 |
| G 端到端+文档 | ✅ 本机+订阅+sing-box 客户端真连验证过 | `scripts/test-argo.sh` 6/6；argo-443 端到端实测 204；**§6.3-1/6.3-2 已修复** |

---

## 6. 已知问题 / 待办（接手最重要）

### 6.1 端口开放清单（生产）
| 端口 | 是否需公开 | 说明 |
|---|---|---|
| 22 | 已开 | SSH |
| 80/443 | 已开 | nginx/HTTPS |
| **20086** | **待开(NSG)** | **直连 vmess-ws 节点需要**；argo 走 CF 不需要。若只走 argo 可不开 |
| 其余 | 不开 | argo 客户端连 Cloudflare 边缘，非直连服务器 |

> 注意 Oracle：**同时有安全列表(子网) 和 NSG(实例)，两处都要配**。之前 80/443 公网不通就是 NSG 漏了。

### 6.2 订阅"节点授权/显示"要点（排查过，根因已定位）
- 自建节点要能出现在订阅，需满足：① `node_host_override` 设置好（`BuildSelfBuiltLinks` 的 `host` 来源，为空=空订阅）；② 免费分组 `free_group_id` 或「plan 套餐绑分组」被用户桶覆盖。
- 生产已设 `node_host_override=64.181.244.178`、`free_group_id=1`。

### 6.3 已知技术债 / 待办（按优先级）
> ✅ **2026-09-21 已解决**（提交 `ea0f6fe` / `3a677f3`）：
> 1. 临时隧道 13 端口错配 —— 根因：quick tunnel 只服务 443(80)。已改：`temporary` 只下发 `443` 一个 argo 节点，`fixed` 才展开 13 端口（`argoVariantLinks` 按 `mode` 区分）。
> 2. argo 回源 502/EOF —— 根因：cloudflared 回源到 Reality(TLS) 入站，明文 HTTP 被 sing-box 秒断（`Unable to reach the origin service: EOF`）。已改：config 生成时派生**无 TLS 明文 ws 回源入站** `127.0.0.1:<ListenPort+10000>`（`argoOriginInbound`/`ArgoOriginPortDelta`），cloudflared 回源指向它；sing-box 客户端经 argo-443 端到端实测 204。生产已部署，回源日志无 EOF。
> 3. vmess+Reality 直连节点失败 —— 根因：`BuildShareLink` 的 vmess 分支不输出 Reality 参数（`security/pbk/sid`），且 subconv 渲染时硬编码 `sbTLS(p,"tls")`、用 `param()` 读不到 vmess JSON 里的 pbk/sid。已改：vmess 链接 JSON 补 `security=reality/pbk/sid`，subconv 透传 security 并改用 `tlsParam` 读 pbk/sid；argo 变体清空 Reality（CF 边缘终结 TLS）。直连 20086 端到端实测 204。
>
> **遗留待办**：
3. 移植项②「多 IP 存活兜底订阅」**后端+前端已完成**（`sb_inbounds.fallback_hosts` 列 + liveness 探测选主 + 备选 `-ipN` 变体；入站表单「备选 IP/域名」标签编辑）。**待真实第二公网 IP 端到端联调**。
4. 固定隧道(token)实机未联调（配好后 13 端口才成立）。
6. 本机 Docker `build` 依赖可用镜像源/代理（曾 403/525；`Dockerfile` 已去掉 `# syntax` 前端镜像依赖减轻此问题）。

> ✅ **已修复**：
> 7. `TestRFC5987Escape` 既有失败 —— 已随品牌统一为"亲友团"（服务器 `go test -race ./...` 全绿确认通过），非遗留。
> 8. 监控详情图长范围静默截断 —— `ListMetrics` 5000 行上限在 30s 采样下使 7d/30d 只显示最近约 1.7 天；改为分时下采样覆盖全窗口（提交 `c48f154`）。
> 9. 流量采样默认周期 10→5 min —— 缩小换 IP 触发 sing-box 重启导致的瞬态丢数窗口（提交 `68b5e52`）。

---

## 7. 测试与 CI

```bash
# 完整自动化流水线（10 步：Go build/vet/test-race + 用例清单 + shell 语法 +
# Argo 专项 + 前端 test/typecheck/build + 启动冒烟），产物落盘 docs/test-report/
bash scripts/test-full.sh
# 后端快速回归（跳过前端三步）
bash scripts/test-full.sh --no-frontend
# 或指定 go：GO=/path/to/go bash scripts/test-full.sh；跳过冒烟 --no-smoke
# 用法详见 docs/test-report/README.md

# Legacy：Argo 专项（编译+vet+单测+语法+启动冒烟=6 项）
bash scripts/test-argo.sh
```
CI（`.github/workflows/ci.yml`）：`go` job（race）、`shell`（脚本语法）、`frontend`（npm test+build）、`argo-suite`（跑 test-argo.sh）。
> 注：`test-full.sh` 与本仓 CI 对齐，可作为本机/服务器跑的本地 CI；`test-argo.sh` 是其中第 6 步子套件。两者均用 `-buildvcs=false`（Git for Windows 的 VCS 探测问题）、临时库启动冒烟（/api/health）。race 需 cgo，本机 Windows 建议到服务器(Linux 有 systemd+gcc)跑，才能覆盖 sbctl 相关测试。

---

## 8. 协作红线（团队）

1. **绝不提交 `oracle/` 私钥**；push 前 `git status` 确认无 `.key/.pem/.db`。
2. **拿到代码先跑 `scripts/test-argo.sh`** 再动手。
3. 改 store/迁移：`migrate.go` 里**幂等追加**（列只增不减），勿破坏旧库。
4. 改 Argo/cloudflared：**临时=1端口、固定=13端口** 的语义要对齐（见 §6.3-1）。
5. 生产改配置/迁移前，**SSH 进服务器确认**；面板重启请用 §3 的启动命令（root，QZ_PUBLIC_BASE 别丢）。
6. 账号/密钥走团队密钥库，别写在公开文档/聊天明文长期保留。

---

## 9. 接手后的第一个 15 分钟

1. clone + `git checkout dev` + 跑 `bash scripts/test-argo.sh` 全绿。
2. SSH 上生产：`ps aux|grep qinyoutuan`、`curl -k https://127.0.0.1/qyt/api/health -H 'Host: myservs.rouroujuxuan.top'`。
3. 浏览器开 `https://myservs.rouroujuxuan.top/qyt/` 登录（口令在密钥库）。
4. 订阅页复制订阅链接，客户端导入，确认至少**直连**或 **argo-443**能用。
5. 有意识地推进 §6.3 遗留待办（6.3-1/6.3-2 已于 2026-09-21 修复，见 §6.3）。

---

## 10. 附录：常用命令速查

```bash
# 面板重启（root）
sudo -n pkill -f qinyoutuan; sleep 1
sudo -n bash -c 'nohup env QZ_LISTEN=0.0.0.0:8081 QZ_DB=/home/ubuntu/qinyoutuan.db \
  QZ_SECRET_KEY="'"$(cat ~/qz-secret)"'" QZ_ADMIN_USER=admin \
  QZ_PUBLIC_BASE=http://myservs.rouroujuxuan.top/qyt \
  /home/ubuntu/qinyoutuan >/home/ubuntu/qz.out 2>&1 &'

# nginx / vhost 测试
sudo nginx -t && sudo systemctl reload nginx

# 看隧道日志/当前临时域名
sudo tail -n 20 /etc/cloudflared/argo-test.log | grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' | tail -1

# 订阅（admin 为例，token 从 DB 读）
sudo sqlite3 /home/ubuntu/qinyoutuan.db "select sub_token from users where username='admin'"
# 订阅地址 = https://myservs.rouroujuxuan.top/qyt/sub/<token>
```