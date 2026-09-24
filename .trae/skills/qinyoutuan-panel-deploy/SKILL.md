---
name: "qinyoutuan-panel-deploy"
description: "Deploys/upgrades the QinYouTuan (qinyoutuan/qingzhou) proxy-subscription panel on a remote Linux server: SSH password auth, in-server build (Node+Go), in-place binary swap, sets QZ_PUBLIC_BASE, restarts systemd, verifies health. Invoke when the user asks to deploy/update/redeploy the panel or to a remote server."
---

# QinYouTuan Panel — Remote Deploy & Configure

一键把「亲友团面板」源码构建并部署到远程 Linux 服务器，原地升级、保留数据库与主密钥。

## 什么时候用
- 用户要**部署/更新/重新部署**面板到远程服务器（本项目 `qinyoutuan` / 旧名 `qingzhou`）。
- 用户给了服务器 SSH 信息并希望由 Agent 自动构建 + 换二进制 + 改配置 + 重启。

## 前置：连接信息（全部用占位符，勿内置真实值）
| 变量 | 说明 |
|---|---|
| `HOST` | 服务器 IP |
| `USER` | SSH 用户名 |
| `PORT` | SSH 端口 |
| 密码 | 某个文件路径，或由用户提供 |

> 本项目已废弃 `qinyoutuan` / `qingzhou` 两个历史名都可能出现：**别想当然**，先连上去探测实际运行方式（见步骤 2）。

## 0) 非交互密码登录（Windows）
Windows OpenSSH 不直接吃密码。用 `SSH_ASKPASS` 喂密码（避免密码进 argv）：

1. 写一个最小 Go 助手指着密码文件并编译（`go build` 在本机即可）：
   ```go
   package main
   import ("os";"strings")
   func main(){ b,_:=os.ReadFile(`<PASSWORD_FILE>`); os.Stdout.WriteString(strings.TrimRight(string(b),"\r\n")+"\n") }
   ```
2. 执行任一远端命令时设置：
   ```
   SSH_ASKPASS=<askpass.exe>  SSH_ASKPASS_REQUIRE=force  DISPLAY=:0
   ssh -o StrictHostKeyChecking=accept-new <USER>@<HOST> -p <PORT> '<cmd>'
   ```
   > Linux 端若用 `sshpass`：`sshpass -f <pwfile> ssh <USER>@<HOST> ...`。
3. 传二进制/大文件时 `scp` 常不可靠：用 `.NET Process` 把文件 bytes 写到 `ssh ... "cat > /p"` 的 stdin（二进制安全），或 base64 + `base64 -d`。

## 1) 先盘点远端现状（不要假设路径，可能两种名字并存）
```bash
ss -ltnp | grep -E ':8086|:8081'
pgrep -fa -i 'qin'                      # 看进程名
systemctl list-units --type=service | grep -iE 'qin'
for p in /opt/qingzhou /opt/qinyoutuan /etc/*.env; do [ -e "$p" ] && ls -ld "$p"; done
```
典型两套：
- 旧版 install.sh 部署：`/opt/qingzhou/qingzhou` + `qingzhou.service` + `/opt/qingzhou/qingzhou.env` + 同目录 `qingzhou.db`
- 新版：`/opt/qinyoutuan/...` + `qinyoutuan.service` + `/etc/qinyoutuan.env`

## 2) 确认数据与密钥（必须保留，否则丢用户/解不开加密）
- **`QZ_DB`**：新旧二进制默认 db 名不同，务必沿用现有的 DB 路径。
- **`QZ_SECRET_KEY`**：绝不能换，否则已加密的 SMTP/Reality 密钥解不开（会拒绝下发）。
- 读取 env 时对 `QZ_SECRET_KEY`/`QZ_ADMIN_PASS` **脱敏**：`sed -E 's/^(QZ_(SECRET_KEY|ADMIN_PASS))=[^$]*/\1=<redacted>/' <env>`。

## 3) 服务器端工具链（缺则装）
- Node 20：`apt-get update && apt-get install -y nodejs npm`
- Go **1.25+**（Debian apt 只有 1.24，不够）→ 下载官方 tarball，镜像回退顺序：
  `https://dl.google.com/go/go1.25.x.linux-amd64.tar.gz` → `go.dev` → `mirrors.aliyun.com/golang` → `golang.google.cn`
  `tar -C /usr/local -xzf go*.tgz`；`export PATH=/usr/local/go/bin:$PATH`

## 4) 上传源码并在服务器构建
- 本地打包：`tar -czf src.tgz --exclude=.git --exclude=.deploytmp --exclude=node_modules -C <repo> .`
- 传到 `/opt/qz-src.tar.gz` → `tar -xzpf ... -C /opt/qz-src`
- 构建（顺序重要：先前端才有 `dist` 给 go embed）：
  ```bash
  cd /opt/qz-src/frontend
  npm ci --no-audit --no-fund            # 失败可加 --registry=https://registry.npmmirror.com
  npx vite build
  cd /opt/qz-src
  export PATH=/usr/local/go/bin:$PATH
  go build -trimpath -ldflags "-s -w -X qingzhou/internal/version.Version=<VERSION>" -o /opt/qinyoutuan.new .
  ```
  > 注：模块路径目前仍是 `qingzhou`（`go.mod` 未改），ldflags 用 `qingzhou/internal/version.Version`。

## 5) 原地升级（备份→换→设域名→重启→验证）
```bash
TS=$(date +%Y%m%d%H%M%S)
cp -a <BIN> <BIN>.old.$TS                       # 备份旧二进制
cp -a <ENV> <ENV>.bak.$TS                       # 备份旧 env
# 设置对外地址（源地址，不含 #/ hash 路由）；沿用现有 DB/SECRET
sed -i 's|^QZ_PUBLIC_BASE=.*|QZ_PUBLIC_BASE=<https://your.domain>|' <ENV> \
  || echo 'QZ_PUBLIC_BASE=<https://your.domain>' >> <ENV>
install -m 755 /opt/qinyoutuan.new <BIN>        # 原子替换
systemctl daemon-reload && systemctl restart <SERVICE>
sleep 4 && systemctl is-active <SERVICE>
curl -s http://127.0.0.1:8086/api/health        # 期望返回新版本
```
> `QZ_PUBLIC_BASE` 填**源地址**即可（`#/` 是前端 hash 路由，不是服务器 base）。优先级：env 变量 > 设置页 > 请求 Host。

## 6) 验证清单
- `systemctl is-active <SERVICE>` = active，`ss -ltnp | grep 8086` 进程为新二进制（启动日志含 `qinyoutuan listening`）
- 本机 8086 / 直连 IP:8086 / 反代 https 域名 `/api/health` 均返回新版本号
- env 脱敏回显确认 `QZ_PUBLIC_BASE` 已切换

## 7) 回滚
```bash
install -m755 <BIN>.old.$TS <BIN> && systemctl restart <SERVICE>
```

## 注意事项 / 陷阱
- **online update 与 install.sh 产物名**：改名后 release 资产为 `qinyoutuan-linux-*`；**已发布的旧 release 仍是 `qingzhou-linux-*`**。未发新 release 前，`install.sh` 和面板内「在线更新」会因产物名不匹配而下载 404。要让其可用，须在改名后的代码上**发布新 release**。
- **版本号**：注入的 `<VERSION>` 若低于 GitHub 最新 release，面板会提示可更新但可能下载失败。可注入不小于最新 release 的版本规避误导。
- 面板默认更新仓库为 `mllt992/qing-zhou`（外部仓库），`QZ_UPDATE_REPO` 可覆盖。
- 服务器上若还留旧名二进制/服务，别一并删，确保新服务起来、健康后再清理。

## 建议的 Agent 执行顺序（幂等）
盘点现状 → 确认 DB/SECRET → 补工具链 → 传源码 → 构建 → 备份 → 换二进制 → 改 `QZ_PUBLIC_BASE` → 重启 → 脱敏验证 → 汇报；条件不符时停下询问用户，不要盲目覆盖生产。