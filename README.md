# DNSforVPN

DNS-over-HTTPS 代理：GFWList 智能分流 + BoltDB 持久缓存 + 自定义域名覆盖，
**单二进制**交付，内嵌 Web 管理界面（PWA），可跑在桌面与 OpenWrt 路由器上。

## 特性

- **DoH 上游**：国内外多服务器并发解析、按 (Name,Type,RData) 去重聚合
- **智能分流**：GFWList 规则决定直连 / 代理上游
- **缓存**：BoltDB 持久化 + sync.Map 热缓存 + NXDOMAIN 负缓存 + inflight 合并
- **自定义 DNS**：域名→IP 覆盖（A/AAAA），管线最高优先级，即改即生效
- **Web UI**：配置、延迟、统计、缓存浏览、自定义 DNS 五个面板；PWA 可"安装"为桌面应用
- **常驻服务**：Windows (SCM) / macOS (launchd) / Linux (systemd) / OpenWrt (procd) 一键安装卸载

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
tools/                # genicons (图标) / dnsprobe (DNS 探测)
deploy/               # 各平台 install/uninstall 脚本 (openwrt 另含 procd init 与路由版配置)
configs/              # config.toml + rules/(种子) + data/(运行时, 忽略)
Makefile              # 全部构建入口
```

## 构建

```bash
make linux      # → build/linux/    二进制 + config + install.sh / uninstall.sh
make windows    # → build/windows/  dnsforvpn.exe + config + install.bat / uninstall.bat
make macos      # → build/macos/
make openwrt    # → build/openwrt/  默认 arm64; make openwrt ARCH=mipsle / x86_64
make run        # 识别当前系统, 构建并前台运行 build/<os>/ 下的二进制
make clean
```

- 纯 Go 零 CGO，交叉编译无需任何额外工具链；前端产物 (embed) 由 Makefile 自动先构建
- 每个 `build/<os>/` 目录就是完整的分发单元：整个拷到目标机器，跑里面的 install 脚本即可
- 换架构：`make linux ARCH=arm64`、`make macos ARCH=arm64` 等
- 已构建目录里的 `config.toml`/`rules/` 不会被重复构建覆盖（Web UI 里的修改得以保留），`make clean` 重置

## 安装 / 卸载（系统服务常驻, Model B）

| 平台 | 安装 | 安装目录 |
|------|------|----------|
| Linux   | `sudo ./install.sh` | `/opt/dnsforvpn` |
| Windows | 右键 → 以管理员身份运行 `install.bat` | `C:\Program Files\DNSforVPN` |
| macOS   | `sudo ./install.sh` | `/usr/local/dnsforvpn` |
| OpenWrt | 目录 scp 到路由器后 `sh install.sh` | 二进制 `/usr/bin`，配置 `/etc/dnsforvpn` |

- install = 复制文件 + 注册并启动系统服务（SCM / launchd / systemd / procd）+ 装启动器图标（Windows 桌面 .url 快捷方式 / Linux 应用菜单 .desktop / macOS /Applications 启动器，均指向 Web UI）
- uninstall = 停止并注销服务 + 删除二进制、配置、缓存数据库和启动器图标
- 重装不覆盖已有 `config.toml`（Web UI 里的修改保留）

手动管理服务（install 脚本就是对这两步的封装）：

```bash
dnsforvpn service install --config /path/to/config.toml
dnsforvpn service start|stop|restart|status|uninstall
```

## 运行（开发）

```bash
make run                                # 构建并前台运行, 自动打开浏览器到 Web UI
./build/linux/dnsforvpn --no-browser    # 不打开浏览器 (无头环境)
```

- DNS 默认监听 `0.0.0.0:5553` —— 定位是**其他 DNS 服务的上游**（由 dnsmasq / 系统 DNS
  转发过来），不抢 53 端口
- Web UI 默认 `127.0.0.1:8080`（见 config.toml）
- **目录布局**（单目录，安装/分发时携带前两样）：

  ```
  config.toml        # 配置
  rules/gfwlist.txt  # 规则种子 (缺失时自动下载; 随包携带, 离线可启动)
  data/              # 运行时生成 (BoltDB), gitignore, 不打包
  ```

- 配置中的相对路径（db_path、rule_file）一律相对**配置文件所在目录**解析，
  服务模式任意工作目录下行为一致

## OpenWrt

```bash
make openwrt ARCH=mipsle        # 或 arm64 (默认) / x86_64
scp -r build/openwrt router:/tmp/dnsforvpn
ssh router 'cd /tmp/dnsforvpn && sh install.sh'
```

- 路由版默认 DNS 监听 `127.0.0.1:5553`（避开 dnsmasq 的 53），Web UI 监听 `0.0.0.0:8080`
- **先设 `[web] password`**（Basic Auth），再对局域网开放 8080
- 让全网 DNS 走 dnsforvpn（dnsmasq 转发）：

```bash
uci add_list dhcp.@dnsmasq[0].server='127.0.0.1#5553'
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
port = 5553          # 上游监听口; 不抢 53, 由 dnsmasq/系统 DNS 转发

[cache]
db_path = "data/dns_cache.db"      # 相对配置文件目录解析
max_hot_size = 100000
save_interval = 72

[proxy]
enable_proxy = true
http  = "http://192.168.5.8:7899"
https = "http://192.168.5.8:7899"
rule_file = "rules/gfwlist.txt"    # 相对配置文件目录解析; 缺失时按 url 下载
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
go run ./tools/dnsprobe -server 127.0.0.1:5553 -type A example.com   # DNS 探测
go run ./tools/genicons                                              # 重生成图标

# 裸 go 构建 (Makefile 之外; 需先 cd frontend && npm run build)
go build -o dnsforvpn ./cmd/dnsforvpn
# Windows 注意: -o 要显式带 .exe —— go build -o dnsforvpn.exe ./cmd/dnsforvpn
```

## 安全注意

- 不要把 DNS 端口直接暴露到公网
- Web UI 监听非回环地址时务必设置 `[web] password`
- BoltDB 单句柄：服务运行期间不要再用第二个进程打开同一 DB 文件
