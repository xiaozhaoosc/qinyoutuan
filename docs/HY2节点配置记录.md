# HY2 节点配置记录（UDP 8082）

> 2026-09-29。在亲友团面板（`64.181.244.178`，`myservs.rouroujuxuan.top/qyt/`）新增一条
> Hysteria2 节点，用于与现有 vless+Reality 直连节点做延迟/速度对比。

## 一、端口决策

- **443**：已被 nginx 占用（TCP，Cloudflare 回源）。HY2 走 UDP/QUIC，技术上与 TCP 443
  不冲突，但 UDP 443 出网常被运营商 QoS/封禁，且与 CDN 回源混淆，故不采用。
- **8082**：探测确认空闲（未被任何服务占用），符合端口策略（新端口），**确定使用 8082**。

| 端口 | 协议 | 用途 | 占用方 |
|---|---|---|---|
| 443 | TCP | HTTPS 回源 | nginx |
| 8081 | TCP | 面板 | qinyoutuan |
| 8082 | **UDP** | **HY2 节点（新增）** | sing-box |
| 20086 | TCP | vless 直连 | sing-box |

## 二、防火墙放行

### 1. 服务器 iptables（已执行并持久化）
```bash
sudo iptables -I INPUT -p udp --dport 8082 -j ACCEPT
sudo iptables-save > /etc/iptables/rules.v4   # netfilter-persistent 已启用，重启不丢
```

### 2. Oracle NSG（需在控制台确认）⚠️
服务器侧 iptables 已放行，但 Oracle 实例还有 **NSG（安全组）** 一层。若 NSG 未放行
UDP 8082，公网仍不可达。**请在 Oracle 控制台 → 实例 → 安全列表/NSG 放行 UDP 8082**
（若整机已放行大范围端口则无需操作）。

## 三、面板配置（已完成，含 API 步骤）

1. **TLS**：`POST /api/admin/sb/tls/quick-selfsigned` → 生成自签证书
   `hy2-selfsigned`（SAN=`64.181.244.178`，insecure=1 客户端）
2. **入站**：`POST /api/admin/sb/inbounds`
   ```json
   {
     "type": "hysteria2",
     "tag": "hy2-8082",
     "listen": "::",
     "listen_port": 8082,
     "tls_id": 2,
     "server_id": 0,
     "enabled": true,
     "options": "{\"up_mbps\":100,\"down_mbps\":100,\"obfs\":{\"type\":\"salamander\",\"password\":\"qz-hy2-obfs-2026\"},\"bbr_profile\":\"standard\"}"
   }
   ```
   > 注意：`options` 是 **JSON 字符串**（不是对象），否则解码报「请求格式错误」。
3. **逻辑节点 + 分组授权**：`POST /api/admin/nodes` 创建 self_built 节点
   `hy2-8082`（inbound_tag=`hy2-8082`，protocol=`hysteria2`，group_ids=[1] 美国-主节点）。
   > 这是节点进入用户订阅的关键：新建入站**不会**自动生成 self_built 节点行，
   > 必须手动建并挂到用户可访问的分组（free_group_id=1）。

## 四、生成结果（已实测确认）

- **订阅 base64**（`myservs.../qyt/sub/<token>`）已含 3 节点：
  - vless 直连（20086）
  - vless argo-443
  - **hysteria2 `hy2-8082`**
- **HY2 链接示例**：
  ```
  hysteria2://<client_secret>@64.181.244.178:8082?security=tls&insecure=1&fp=chrome&sni=64.181.244.178&obfs=salamander&obfs-password=qz-hy2-obfs-2026&upmbps=100&downmbps=100
  ```
- **sing-box 侧**：8082 UDP 监听中（`*:8082`），`users` 数组含全部 bucket 凭据
  （admin 及付费套餐用户的 client_secret 均已自动注入），obfs=salamander 注入，
  bbr_profile=standard，TLS 自签 enabled。

## 五、实测对比（待用户客户端验证）

| 节点 | 协议/端口 | 传输 | 预期延迟 | 预期速度 | 适用 |
|---|---|---|---|---|---|
| 直连 | vless+Reality 20086 | TCP | 低（雷厉） | 高（弱网易崩） | 隐蔽优先 |
| argo-443 | vless+ws 443 | TCP/CDN | 中（走 CF 边缘） | 受 CF 限制 | 落地 IP 被墙时兜底 |
| **HY2** | hysteria2 8082 | **UDP/QUIC** | 中（QUIC 握手） | **弱网最高** | 看重速度/高丢包链路 |

**建议实测**：客户端导入订阅后，分别测速/测延迟：
- Clash/Mihomo：`clash verge` 或 `mihomo`，HY2 节点测速
- sing-box：官方客户端直接订阅
- 记录 3 节点在同一网络下的下载 Mbps / Ping，填入下方表格

| 节点 | Ping(ms) | 下载(Mbps) | 上传(Mbps) | 备注 |
|---|---|---|---|---|
| 直连 vless | | | | |
| argo-443 | | | | |
| **HY2 8082** | | | | |

## 六、回滚 / 停用

- 停用节点：面板「sing-box 配置 → 入站」停用 `hy2-8082`，或删除 self_built 节点 id=3
- 关闭端口：`sudo iptables -D INPUT -p udp --dport 8082 -j ACCEPT && sudo iptables-save > /etc/iptables/rules.v4`
- 删除入站：面板删除 `hy2-8082` 入站（自动连带删除 self_built 节点行与分组）

## 七、已知限制

- HY2 依赖 UDP 通路：被 QoS/封 UDP 的网络不可用（此时直连兜底）
- 自签证书：客户端需 `insecure=1` 或 pinSHA256（订阅已自动带 insecure）
- Oracle NSG 未放行 UDP 8082 时外部不可达（见 §二.2）
