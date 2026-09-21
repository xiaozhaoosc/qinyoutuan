# 亲游团移植方案：Argo 隧道接入 + 多 IP 存活兜底订阅

> 目标项目：`qing-zhou`（亲友团 / QingZhou，Go + Vue 的 sing-box 订阅管理面板）
> 移植来源：`sing-box-yg`（甬哥 sing-box-yg，shell 脚本实现）
> 适用范围：仅移植以下两项，其余已被亲游团现有能力覆盖，**不在本方案内**。

---

## 摘要

sing-box-yg 的核心脚本价值在于**对抗"落地 IP 被墙"与"走 CDN 抗封"的实战手段**，而非它的部署/保活（这部分亲游团的 systemd + sbproc 自愈更优）。本方案聚焦两个最具移植价值的点：

1. **Argo / Quick Tunnel 隧道接入** —— 让入站可一键挂 Cloudflare Argo（临时 / 固定隧道），走 CDN 优选 IP 与 13 个 80/443 系端口，落地 IP 被墙依旧可用，且无需自有域名。
2. **多 IP 存活兜底订阅** —— 让一个节点在订阅里输出同一服务的多个 IP/域名备选节点，复用现有探测能力自动挑"当前可达（未被墙）"的 IP 作主节点，被封时自动/手动切换。

本文档提供：① 两张详细移植表格；② 每个移植项的代码触点与改动清单；③ 分阶段实施步骤；④ 排期与验收标准；⑤ 明确"不做的事"边界。

---

## 〇、实施进度（实时更新）

> 状态：`□ 未开始`　`◐ 进行中`　`✔ 已完成`

### 移植项①：Argo / Quick Tunnel 隧道接入

| 阶段 | 状态 | 说明 / 关键决策与改动 |
|---|---|---|
| A 数据模型 + 迁移 | ✔ 已完成 | `SbInbound` 加 4 字段（`argo_enabled/argo_mode/argo_auth/argo_domain`）；`ALTER TABLE sb_inbounds` 幂等追加 4 列（照抄 egress/upstream 惯例，不动 CREATE TABLE）；`singbox.go` 的 4 处 SELECT + INSERT + UPDATE 全部对齐。编译 + vet + store 测试通过。注：`argo_auth`(CF token) 目前明文存储，加密延到 API 阶段 |
| B cloudflared 交付链 | ✔ 已完成 | 在 `internal/assets/install-singbox.sh` 加 `--with-argo`：`cf_arch_tag()` + `maybe_install_cloudflared()`，直接拉 **Cloudflare 官方** latest 按架构原子装到 `/usr/local/bin/cloudflared`。**关键决策**：cloudflared 是官方开源静态二进制，不自建/不进自己 release（与探针/sing-box 自建交付链不同），故交付源改为官方。bash -n + go build + 断言测试通过 |
| C sbproc 子进程管理 + 胶水 | ✔ 已完成 | `internal/sbproc/cloudflared.go`：`ArgoSpec/ArgoArgs/ParseTemporaryHostname/CloudflaredServiceUnit/FindCloudflaredBin` + 运行时缓存 `SetArgoHost/ArgoHost` + `EnsureLocalArgo`（写 unit→daemon-reload→enable→restart，仅参数变化才重启）。**胶水**：`main.go` 新增 `startLocalArgoSync`/`reconcileLocalArgo` 协程（每 2min），对**面板本机(server_id=0)** 的 argo 入站确保 cloudflared 运行并回填临时域名；远端服务器显式留阶段 E。**关键决策**：沿用 systemd 托管（优于 pgrep）；**已知边界**：远端落地机 Argo 未接（E）；`argo_auth` 明文存（F 加密） |
| D 订阅 13 端口输出 | ✔ 已完成 | `store/singbox.go` 新增 `argoCDNPorts`(80系×7+443系×6)、`SbInbound.argoSpec()`、`argoVariantLinks()`；在 `BuildSelfBuiltLinks` 中对 argo(vmess-ws) 入站额外批量产出 13 个 CDN 出站（Host/SNI/ws-Host=隧道域名，TLS 按端口开关），与直连节点并存。域名来源：固定→`ArgoDomain`；临时→`sbproc.ArgoHost`(运行时缓存)。单测覆盖 13 变体/标签后缀/空 host 兜底 |
| E sshctl 落地集成 | □ 未开始 | cloudflared 在远端落地机时，复用 `sshctl` 推送/管理 |
| F API + 前端 | ◐ 进行中（后端✔，前端待构建验证） | **后端✔**：① `argo_auth` 落库 AES-256-GCM 加密（`SaveSbInbound` 加密、4 条读路径解密、`migrateEncryptArgoAuth` 幂等迁移老明文）；② `SbInbound.ArgoHost` 只读字段向 UI 暴露当前隧道域名；③ API 校验：仅 vmess 可用、fixed 必须带 token+域名、关闭时清残留。**前端（已改，待 CI 构建验证）**：`AdminSingbox.vue` 入站抽屉加「Argo 隧道」区（开关/临时·固定/token·域名/当前域名标签）+ 列表行 Argo 徽标 + 保存体带 argo_*。**验证路径**：本地无 node + 本机 Docker 镜像源全失效，前端构建由 CI `frontend` 与 `argo-suite` job 把关 |
| G 端到端测试 + 文档 | ✔ 实机联调通过 | **Oracle ARM64 实机验证**（`ubuntu@64.181.244.178`）：面板 health 200；install-singbox.sh `--with-argo` 装好 sing-box 1.14.0(带 v2ray_api) + cloudflared 2026.9.1；API 建 vmess-ws(20086)+Argo 临时隧道 → argosync 拉起 systemd `cloudflared-<tag>.service` → 拿到 `*.trycloudflare.com` → `argo_host` 回填 API → 公网 `https://<host>/ws` 返回 sing-box 400（全链路 CN 边缘→隧道→origin 通）。**真机抓到的 3 个 bug**：① 临时隧道 CLI 是顶层 `tunnel --url`（`tunnel run --url` 被 cloudflared 拒绝，单测只验了自己预期）；② API 入站列表 DTO 漏 argo 字段（前端看不到、状态不回填）；③ `ParseTemporaryHostname` 取首个匹配，追加式日志里旧域名永远抢占 → 改取最后一个。另有进程归属坑：sudo 启动的面板 pkill 需 sudo，否则"重启"无效。`scripts/test-argo.sh` 6/6 + store 全量测试通过。**未完**：固定隧道(token)实机、13 端口真实客户端握手（需 v2rayN/sing-box 客户端） |

### 移植项②：多 IP 存活兜底订阅

| 阶段 | 状态 | 说明 |
|---|---|---|
| A 建模 / B 存活判定 / C-D 订阅 / E-F 前端 | □ 未开始 | 待移植项① 阶段 C 收尾后推进 |

## 一、移植项 ①：Argo / Quick Tunnel 隧道接入

### 1.1 原理（一句话）

面板在节点上起一个 `cloudflared`（Cloudflare Tunnel 客户端）子进程，把本地的一个 **vmess-ws 入站**通过公网隧道映射出去；客户端订阅里的出站不再直连落地 IP，而是连到 `trycloudflare.com`（临时）或你的固定域名（固定隧道）经过的 **CDN 优选 IP : 端口**。这样即使落地 IP 被墙，CDN 前置也能进得来；临时隧道免费、免域名。

两种模式：
- **临时隧道**：无需 token/域名，面板启动时向 CF 请求，返回一个 `xxx.trycloudflare.com`，无法固定域名，重启会变。
- **固定隧道**：用 CF 官方给的 `token` + `域名`，域名稳定，适合长期对外。

### 1.2 详细移植表格

| 环节 | 现状触点（代码） | 移植要改什么 | 涉及文件 / 函数 | 预估工作量（人日） |
|---|---|---|---|---|
| 数据模型 | `SbInbound` 无 argo 字段 | 入站新增 argo 配置区：`argo_enabled`、`argo_mode(临时/固定)`、`argo_auth(固定 token)`、`argo_domain(固定域名)`；幂等迁移 | `internal/store/nodesingbox.go`、`nodes.go`、`migrate.go` | 0.5 – 1 |
| 二进制/依赖 | 面板未内置 cloudflared | 引入 `cloudflared` 静态二进制（arm/amd），走与探针相同的发布/下载机制；落地机安装脚本同步 | 复用 `cmd/probe` 的发布机制、`deploy/install-singbox.sh`、`.github/workflows/release.yml` | 1 |
| config 生成 | `Inbound` 结构体 + 生成器只生成本地入站 | 为 argo 入站生成回源 vmess-ws 入站；argo 出站写入**订阅**而非服务端 config | `internal/singbox/generate.go`（`Inbound`、生成函数） | 0.5 |
| 进程管理 | `sbproc/manager.go` 只管 sing-box 写→check→原子替换→reload | 新增 `cloudflared` 子进程生命周期：启动/保活(`pgrep` 自愈)/临时隧道解析 trycloudflare 域名/固定隧道用 token/重启重置 | `internal/sbproc/manager.go`（新增子进程管理） | 1 – 1.5 |
| 订阅输出 | `subconv/output.go` 出站 URL 转换 | 对 argo 入站批量生成 **13 个 CDN 优选端口**节点（80 系 7 个 + 443 系 6 个）+ 不死优选 IP，回源 `host` 填 argo 域名、`path` 填 uuid-vm | `internal/subconv/output.go`（URL/clash/singbox/surge 输出） | 0.5 – 1 |
| 远程落地 | `sshctl` 多机下发 | cloudflared 在落地机运行时，用 `sshctl` 推送二进制 + 注册 systemd 单位并保活 | `internal/sshctl/`、落地相关 handler | 0.5 – 1 |
| API | admin `sb/*` 入站路由 | 入站读写带上 argo 字段；新增 argo 状态/重启端点 | `internal/api/`（nodesingbox / sb 处理器） | 0.5 |
| 前端 | `AdminNodes.vue` 节点表单 | 入站页加"Argo 隧道"配置区（临时/固定、token、域名）+ 状态/域名展示 | `frontend/src/views/AdminNodes.vue` 及表单组件 | 1 – 1.5 |
| 测试/联调 | — | 回源连通、隧道域名 404 判定、端口可测、被墙下可用、临时转固定 | 端到端 | 0.5 – 1 |

### 1.3 分阶段实施步骤

- **阶段 A：建模与迁移** —— 在 `nodesingbox.go` 的 `SbInbound` 增加 argo 字段，`migrate.go` 以幂等方式追加列（列只增不减），跑 `go build ./...` 验证。
- **阶段 B：cloudflared 交付链** —— 参照探针 `cmd/probe` 的构建/上传/下载链路，增加 cloudflared 二进制产物；落地机一键脚本能装它。
- **阶段 C：进程管理**（工作量最大）—— 在 `sbproc` 新增一个受管子进程类型：启动、自愈（进程没了自动拉起）、临时隧道域名解析（从 `boot.log`/`--url` 输出取 `trycloudflare.com`）、固定隧道 token 模式。
- **阶段 D：订阅输出** —— 在 `subconv/output.go` 为 argo 入站批量拼 13 个 CDN 端口节点 + 不死优选 IP，任一端到 Clash/sing-box/Surge/base64 都正确。
- **阶段 E：SSH 落地集成** —— `sshctl` 把 cloudflared 推到远端落地机并挂 systemd。
- **阶段 F：API + 前端** —— 入站页可配、可看隧道域名与运行状态、可手动重启隧道。
- **阶段 G：文档 + 端到端验收** —— 更新部署手册与订阅指南。

### 1.4 风险与权衡

| 风险/权衡 | 说明与对策 |
|---|---|
| 临时隧道域名会变 | 客户端订阅的 `host` 用域名；域名变了需刷新订阅或改回源。固定隧道无此问题 |
| CDN 中转有速度/延迟损耗 | 适合"被墙兜底"而非追求极致速度；对外文案要写清楚 |
| cloudflared 多一个常驻进程 | 1H1G 可接受；进程自愈必须做，否则隧道挂掉节点变不可达 |
| fail 行为 | 建议 Argo 节点做成**独立备选**而非替代直连，保持直连不动 |
| 安全 | 隧道等同把入站暴露到公网，必须确认入站本身已配好认证（uuid）+ 私网出口阻断沿用现有配置 |

---

## 二、移植项 ②：多 IP 存活兜底订阅

### 2.1 原理（一句话）

一个节点当前绑定单个 server/IP；若该 IP 被墙则整条失效。移植后让一个节点维护**多个出口 IP/域名**（例如同机/同服务的 A/B/C 三 IP），订阅生成时用现有探测能力判定各 IP 是否可达，**自动挑当前可达的作主节点**，并把其余作为备选节点 + 自动优选组一起下发，用户被封时客户端 urltest 自动切，或手动换 IP。

现有可复用的探测源：`internal/api/nodehost_admin.go` 的 `detectEgressIPs`（并发探测多 IP 回显服务，按 v4/v6 各选候选用）。

### 2.2 详细移植表格

| 环节 | 现状触点（代码） | 移植要改什么 | 涉及文件 / 函数 | 预估工作量（人日） |
|---|---|---|---|---|
| 数据模型 | `Server` 只有单 `Host/Port`（`servers.go`）；节点无多 server 字段 | 节点/入站支持**多出口 IP/域名列表**（备选 server 数组）；幂等迁移 | `internal/store/nodes.go`、`servers.go`、`migrate.go` | 0.5 – 1 |
| 存活判定 | `detectEgressIPs`（`nodehost_admin.go`）并发探测回显 IP；另有探针/测速 | 复用探测逻辑，扩展为**对候选 `IP:port` 做可达性/延迟判定**，得到"被墙/可用"状态并缓存 | `internal/api/nodehost_admin.go`、`internal/sysmetrics`、`cmd/probe` | 0.5 – 1 |
| 订阅生成 | `subconv/output.go` 的 `Render` + `groups.go` 的优选组语义（`fixed/fallback/ai` + `urltestURL`） | 对每个协议输出 N 个同服务不同 IP 的备选节点；主节点指向当前可达 IP；构建自动优选组，支持锁定切 IP | `internal/subconv/output.go`、`groups.go` | 1 – 2 |
| 用户订阅 | `api/user.go` 的 `handleSubscription`（鉴权→收集节点→渲染） | 收集节点时按存活判定挑主；返回备选节点 + 优选组 | `internal/api/user.go` | 0.5 |
| 自动/手动切换 | 现有"智能优选（auto / 锁定服务器 / 节点选择）" | 把"自动换可用 IP"并入现有优选语义，区分"切协议锁 IP"与"换 IP"两种操作 | `internal/subconv/groups.go`、前端订阅页 | 0.5 – 1 |
| 前端 | 节点编辑页、订阅管理页 | 节点页多 IP 列表编辑；订阅信息页展示当前 IP + 备选切换入口 | `frontend/src/views/*`（节点编辑、订阅管理） | 1 – 1.5 |
| 监控联动（可选） | `internal/api/monitor` | 把"被墙判定"上报到监控热力图/告警，主动通知换 IP | `internal/api/monitor.go` | 0.5（可选） |
| 测试/联调 | — | 封掉一个 IP 验证自动换到另一 IP；多端格式下备选节点均正确 | 端到端 | 0.5 – 1 |

### 2.3 分阶段实施步骤

- **阶段 A：建模** —— 节点/入站增加多出口 IP/域名（备选 server），`migrate.go` 幂等追加。
- **阶段 B：存活判定服务** —— 封装"候选 IP 可达性探测"，复用 `detectEgressIPs` + 探针，输出带缓存的"被墙/可用"集合（避免每次订阅都实时探测，控频）。
- **阶段 C：订阅生成** —— `subconv` 对每个协议输出 N 个备选节点 + 一个自动 urltest 优选组；主节点由存活判定决定。
- **阶段 D：用户订阅接入** —— `handleSubscription` 使用存活判定选主并下发育选组。
- **阶段 E：前端** —— 节点多 IP 编辑；订阅信息页展示当前 IP 与手动切换。
- **阶段 F：测试 + 文档** —— 封 IP 验证自动切换；更新订阅指南。

### 2.4 风险与权衡

| 风险/权衡 | 说明与对策 |
|---|---|
| 与被封 IP 恢复的时序 | 探测结果要带 **TTL 缓存**，避免频繁探测；被墙判定过快切走也可能误判偶发超时，需容忍窗口 |
| 多 IP 订阅会"变长" | 备选节点增多会让订阅变大，对免费/高级分组可用开关控制是否启用兜底 |
| 与现有"锁定服务器"语义冲突 | 要区分"切协议"与"换 IP"两种锁定动作，UI 上说清楚 |
| 探测可用性 | 复用 `nodehost_admin` 的回显探测更省事，但那是"出口 IP"探测；真正判定"被墙"需主动测个 `IP:port` 握手，需新增轻量探测（可复用探针/测速通道） |

---

## 三、总体排期（估算，非承诺）

> 以下为**人力工作量（人日）估算**，未计入真实开发中断、需求变更。前后端可并行。
> 单人**串行**约 **2 – 3 周**；前后端并行约 **1.5 – 2 周**。

### 3.1 甘特式阶段表

| 阶段 | 日期范围（估算） | 交付物 | 前置 |
|---|---|---|---|
| 移植项① 阶段 A+B：建模 + cloudflared 交付链 | 第 1–2 天 | 字段/迁移 + 二进制发布链路 | — |
| 移植项① 阶段 C：sbproc 子进程管理 | 第 3–4 天 | cloudflared 启动/自愈/域名解析 | A+B |
| 移植项① 阶段 D：订阅 13 端口输出 | 第 4–5 天 | argo 多端口节点出站 | A |
| 移植项① 阶段 E：sshctl 落地集成 | 第 5 天 | 远端 cloudflared + systemd | A+B |
| 移植项① 阶段 F：API + 前端 | 第 5–7 天 | 入站页可配/可看状态 | C/D |
| 移植项① 阶段 G：端到端测试 + 文档 | 第 7 天 | 验收 + 手册更新 | F |
| 移植项② 阶段 A+B：多 server 建模 + 存活判定 | 第 6–7 天 | 字段/迁移 + 探测服务 | 可与①并行 |
| 移植项② 阶段 C+D：订阅备选 + 用户订阅接入 | 第 8–10 天 | 多 IP 备选 + 优选组 + 自动选主 | ②A+B |
| 移植项② 阶段 E+F：前端 + 测试/文档 | 第 10–12 天 | 切换入口 + 验收 | ②C |

> 注：上表为**串行**口径，两移植项之间有共享模块（`store`、`subconv`），并行时注意合并冲突。

### 3.2 关键里程碑

1. **M1**：cloudflared 在单机落地跑通，临时隧道域名可解析、客户端能连（①C+D 通过）。
2. **M2**：入站在面板里可配 Argo 并在界面看到域名/状态（①E/F 通过）。
3. **M3**：多 IP 备选订阅生成，封一个 IP 客户端自动切到另一 IP（②C/D 通过）。
4. **M4**：前端入口与文档齐备，两功能走完整验收（①②全部通过）。

---

## 四、验收标准（两移植项通用）

- 数据库迁移幂等：`go build ./... && go vet ./...` 通过；`npx vite build` 通过。
- 新列/新表在旧版本二进制下被安全忽略（结构只增不减）。
- 功能向前兼容：未启用 Argo / 未配置多 IP 的旧节点行为完全不变。
- 无人值守可自愈：cloudflared / 探测缓存进程挂掉后自动恢复。
- 前端：移动端自适应，入站/订阅页新控件可用。

---

## 五、明确"不做的事"（边界，防过度工程）

- **不移植**：sing-box-yg 的 Serv00 保活体系（`serv00keep.sh`、`app.js` 的 `/up /re /rp /jc`）——亲游团已有 systemd + sbproc 自愈，更强更稳。
- **不移植**：GitHub Actions 定时访问网页保活、`kp.sh` 多平台远程批量部署——`sshctl` 已覆盖多机，真机面板无需外部定时唤醒。
- **不移植**：端口跳跃、proxyip、五协议一键脚本这类"锦上添花/已被覆盖"项；仅保留在本文摘要中作为后续可选增强。
- 两个移植项均以**独立可选能力**落地，不改变现有直连节点的默认行为。