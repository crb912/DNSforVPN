# DNSforVPN

DNS-over-HTTPS 代理：GFWList 智能分流 + BoltDB 持久缓存 + 自定义域名覆盖，
**单二进制**交付，内嵌 Web 管理界面（PWA），可跑在桌面与 OpenWrt 路由器上。

## 特性

- **DoH 上游**：国内外多服务器并发解析、按 (Name,Type,RData) 去重聚合
- **智能分流**：GFWList 规则决定直连 / 代理上游
- **缓存**：BoltDB 持久化 + sync.Map 热缓存 + NXDOMAIN 负缓存 + inflight 合并
- **自定义 DNS**：域名→IP 覆盖（A/AAAA），管线最高优先级，即改即生效
- **Web UI**：配置、延迟、统计、缓存浏览、自定义 DNS 五个面板；PWA 可"安装"为桌面应用
- **常驻服务**：Windows / macOS(launchd) / Linux(systemd) 一键注册系统服务
- **OpenWrt**：免 SDK 打出 .ipk，procd 守护

## 架构

```
cmd/dnsforvpn/        # 唯一入口: 前台运行 / service 子命令
internal/
  dns/                # DNS wire-format 解析/构建
  router/             # GFWList 路由
  cache/              # BoltDB + 热缓存 + 负缓存 + CustomStore
  upstream/           # Resolver 接口 + DoH/UDP Client + Manager
  query/              # 查询管线编排
  transport/          # UDP DNS server
  control/            # 控制层: 配置 + DNS 生命周期 (平台无关)
  web/                # REST API (/api/*) + 内嵌 SPA + Basic Auth
  browser/            # 调系统默认浏览器
frontend/             # Svelte 4 + Vite 5, embed.go 内嵌 dist
tools/                # genicons (图标) / dnsprobe (DNS 探测) / mkipk (免 SDK 打 ipk)
deploy/               # openwrt / windows(NSIS) / linux / macos 打包
configs/              # config.toml + rules/(种子) + data/(运行时, 忽略)
```

## 构建

```bash
cd frontend && npm ci && npm run build        # 先构建前端 (产物被 embed)
go build -o dnsforvpn ./cmd/dnsforvpn         # 再出二进制 (~9MB, 纯 Go 无 CGO)
# Windows 注意: -o 要显式带 .exe —— go build -o dnsforvpn.exe ./cmd/dnsforvpn
# (无扩展名的文件 Windows 无法直接执行)
```

## 运行

```bash
./dnsforvpn                    # 启动 DNS + Web UI, 并自动打开浏览器
./dnsforvpn --no-browser       # 不打开浏览器 (路由器/无头环境)
./dnsforvpn --config /path/to/config.toml     # 默认: configs/config.toml
```

- DNS 默认监听 `0.0.0.0:1553`，Web UI 默认 `127.0.0.1:8080`（见 configs/config.toml）
- **目录布局** (`configs/` 单目录, 安装包携带前两样):

  ```
  configs/
  ├── config.toml        # 配置
  ├── rules/gfwlist.txt  # 规则种子 (缺失时自动下载; 随包携带, 离线可启动)
  └── data/              # 运行时生成 (BoltDB), gitignore, 不打包
  ```

- 配置中的相对路径（db_path、rule_file）一律相对**配置文件所在目录**解析，
  服务模式任意工作目录下行为一致

## 系统服务（常驻，Model B）

```bash
dnsforvpn service install      # 注册为系统服务 (Windows SCM / launchd / systemd)
dnsforvpn service start|stop|restart|status|uninstall
```

桌面安装器（`deploy/` 下各平台脚本）会自动注册服务；桌面图标/开始菜单项
只是打开 `http://127.0.0.1:8080` 的启动器，DNS 由服务保活。

## OpenWrt

```bash
deploy/openwrt/build-ipk.sh arm64        # 或 mipsle / x86_64
# 产出 dnsforvpn_0.2.0_aarch64_generic.ipk, scp 到路由器:
opkg install dnsforvpn_0.2.0_aarch64_generic.ipk
```

- 包内默认 DNS 监听 `127.0.0.1:1553`（避开 dnsmasq 的 53），Web UI 监听 `0.0.0.0:8080`
- **先设 `[web] password`**（Basic Auth），再对局域网开放 8080
- 让全网 DNS 走 dnsforvpn（dnsmasq 转发）：

```bash
uci add_list dhcp.@dnsmasq[0].server='127.0.0.1#1553'
uci set dhcp.@dnsmasq[0].noresolv='1'
uci commit dhcp && /etc/init.d/dnsmasq restart
```

## 配置 (configs/config.toml)

```toml
[doh_servers]
direct_servers = ["https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"]
proxy_servers  = ["https://dns.google/dns-query", "https://1.1.1.1/dns-query"]
bootstrap_server = "223.5.5.5"

[dns]
host = "0.0.0.0"
port = 1553          # 绑 53 需特权 (服务模式下天然有权限)

[cache]
db_path = "data/dns_cache.db"      # 相对 configs/ 解析
max_hot_size = 100000
save_interval = 72

[proxy]
enable_proxy = true
http  = "http://192.168.5.8:7899"
https = "http://192.168.5.8:7899"
rule_file = "rules/gfwlist.txt"    # 相对 configs/ 解析; 缺失时按 url 下载
rule_file_url = "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt"

[web]
host = "127.0.0.1"   # 路由器改 0.0.0.0 并设密码
port = 8080
username = ""
password = ""        # 非空即启用 HTTP Basic Auth

[logging]
level = "info"
```

## 开发

```bash
# 后端
go test ./internal/...
go vet ./...

# 前端热更 (vite dev, /api 自动代理到 127.0.0.1:8080)
cd frontend && npm run dev

# 工具
go run ./tools/dnsprobe -server 127.0.0.1:1553 -type A example.com   # DNS 探测
go run ./tools/genicons                                             # 重生成图标

# 交叉编译 (纯 Go, 全平台一条命令)
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" ./cmd/dnsforvpn
GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build ./cmd/dnsforvpn  # 硬路由
```

## 安全注意

- 不要把 DNS 端口直接暴露到公网
- Web UI 监听非回环地址时务必设置 `[web] password`
- BoltDB 单句柄：服务运行期间不要再用第二个进程打开同一 DB 文件
