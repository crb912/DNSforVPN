# DNSforVPN — TODO & Handoff

> 上次会话: 2026-07-17 (下午)
> 状态: **架构已迁移 — Wails 已移除, 全面转向"单二进制 + 内嵌 Web UI (PWA)"**,
> 桌面/路由器同一二进制, Model B 系统服务常驻。全部验证通过。
> 入口: `/home/raybing/Desktop/github/DNSforVPN` (Linux) / `C:/Users/Bing/Desktop/git/DNSforVPN` (Windows)

## 项目概览

五层 DNS-over-HTTPS 代理 + 内嵌 Web 管理界面 (Svelte 4 SPA + REST API)。
AdGuard Home 同款形态: 一个 Go 二进制 = DNS server + Web UI + 系统服务。

```
cmd/dnsforvpn/main.go   # 唯一入口: run (前台/服务) + service 子命令
configs/                # 单目录配置布局 (打包携带前两项)
  config.toml           #   主配置 (默认 --config 指向它)
  rules/gfwlist.txt     #   规则种子, 入库+随包携带 (离线可启动)
  data/                 #   运行时数据 (BoltDB), gitignore, 不打包
internal/
  dns/        ✅ 17 tests   # DNS wire-format
  router/     ✅ 16 tests   # GFWList 路由
  cache/      ✅ 20 tests   # BoltDB + hot + negative + CustomStore
  upstream/   ✅ 4 tests    # DoH/UDP Resolver + Manager (reduce 去重)
  query/                    # 查询管线 (inflight dedup + custom 覆盖)
  transport/                # UDP DNS server
  control/                  # 控制层 (ex-app.go 剥离 Wails): 配置 + 生命周期
  web/                      # REST API /api/* + embed SPA + Basic Auth
  browser/                  # xdg-open/open/rundll32 开浏览器
frontend/                 # Svelte 4 + Vite 5; embed.go 内嵌 dist
  src/api.js              # fetch 封装, 方法名沿用旧 Wails 绑定
  public/                 # manifest.webmanifest + sw.js + icons/ (PWA)
tools/                    # genicons(图标生成) / dnsprobe(UDP 探测) / mkipk(免 SDK 打 ipk)
deploy/
  openwrt/    # build-ipk.sh + control + procd init + postinst/prerm + 路由器 config
  windows/    # installer.nsi + dnsforvpn.ico (genicons 生成)
  linux/      # install.sh/uninstall.sh + dnsforvpn.desktop
  macos/      # make-app.sh (launcher .app)
```

## 下次会话立即执行

```bash
# 开发/日常运行 (自动开浏览器到 Web UI)
cd frontend && npm run build && cd ..
go build -o dnsforvpn ./cmd/dnsforvpn && ./dnsforvpn

# 无头模式 (路由器/服务器)
./dnsforvpn --no-browser

# 系统服务 (Windows SCM / launchd / systemd)
./dnsforvpn service install && ./dnsforvpn service start

# OpenWrt 打包
deploy/openwrt/build-ipk.sh arm64        # 或 mipsle / x86_64
```

## 运维要点 (踩过的坑)

- **BoltDB 单句柄**: 不变 — 同一时刻只允许一个进程打开 DB。Web UI 与服务
  同进程后此问题自然消失, 但别再起第二个进程打开同一 DB。
- **服务模式路径**: 配置里的相对路径 (db_path/rule_file) 相对**配置文件目录**
  解析 (control.resolve), 服务模式 cwd 无关。
- **Web UI 鉴权**: `[web] password` 非空即启用 Basic Auth (浏览器原生弹窗)。
  监听非回环地址 (路由器 0.0.0.0) 必须设密码。
- **Windows 测试坑**: Git Bash `kill $!` 杀不掉原生 .exe 后台进程 —
  用 `taskkill //F //IM dnsforvpn.exe`。nslookup 的 `-port=` 在 Windows 无效,
  用 `go run ./tools/dnsprobe -server 127.0.0.1:PORT domain` 验证。
- **53 端口**: 服务模式下由系统服务管理器提权, 无需 setcap/sudo。
- **前端构建顺序**: `npm run build` 必须先于 `go build` (embed 占位
  index.html 仅保证裸克隆可编译, 真正 UI 需 vite 产出)。
- **JSON 契约**: 空 slice 必须 `make([]T,0)` (序列化为 null 会崩前端);
  cache.Stats 已补 snake_case json tag。
- **PWA**: sw.js 只缓存静态资源, /api/* 永远走网络。改 CACHE 常量版本号
  以强制刷新 (当前 dnsforvpn-v1)。

## 待完成任务

### 高优先级

- [ ] **修正自定义条目拼写** — Linux 机器 `data/dns_cache.db` 里
  `porhub.com` 应为 `pornhub.com` (在跑该库的机器上用 Web UI 改即可)

### 中优先级

- [ ] **L3 系统托盘** (用户已排期, 暂缓) — Web UI 稳定后评估:
  getlantern/systray 系; 注意 Linux 会引入 GTK/AppIndicator CGO 依赖,
  与"纯 Go 零 CGO"原则冲突, 可考虑仅 Windows/macOS 启用 (build tags)
- [ ] **config.toml 热重载** — SaveConfig 写盘但需重启生效; 考虑 Web UI
  加 Restart 按钮 (control.Stop+Start 已具备条件)
- [ ] **GFWList 定时刷新** — 后台 goroutine 定期重下规则文件
- [ ] **UDP server 优雅关闭** — Close() 等待 WaitGroup, 高负载下可能卡住
- [ ] **OpenWrt 实机验证** — ipk 已能产出 (arm64/mipsle/x86_64 均编译过,
  ar/tar 结构校验过), 尚未在真路由器 opkg install + dnsmasq 转发实测;
  LuCI 跳转入口 (几行 JS 的菜单链接) 可做可不做
- [ ] **Windows NSIS 实机验证** — installer.nsi 未实跑 (本机无 NSIS);
  service install/start 逻辑已在 CLI 层验证
- [ ] **macOS 未验证** — 交叉编译通过; launchd 服务 + make-app.sh 未实跑

### 低优先级 / 未来

- [ ] **DoT (DNS over TLS)** — upstream/dot/ 包
- [ ] **DoQ (DNS over QUIC)** — upstream/doq/ 包
- [ ] **LRU 淘汰** — boltCache hotEntry 已有 lastAccess
- [ ] **Prometheus metrics** — web 层加 /metrics 很容易
- [ ] **Svelte 深色/亮色主题切换**
- [ ] **日志查看器面板**
- [ ] **缓存面板删除/清空按钮** (Cache.Del 已存在, 只欠 API+UI)
- [ ] **自定义 DNS 支持更多类型** (CNAME 等; 当前仅 A/AAAA, TTL 固定 300)
- [ ] **UCI 集成** — OpenWrt 配置映射 UCI (工作量大, 后置)

## 关键设计决策

| 决策 | 内容 |
|------|------|
| 交付形态 | 单二进制 = DNS + Web UI + 服务管理 (AdGuard Home 模式) |
| GUI 方案 | ~~Wails v2~~ → 内嵌 Web UI (embed Svelte SPA + REST); 已移除 Wails |
| 桌面入口 | L1 安装器快捷方式+启动开浏览器 ✅; L2 PWA ✅; L3 托盘 (TODO) |
| 常驻模型 | Model B: 系统服务保活 DNS, 图标只是 UI 启动器 |
| 服务管理 | kardianos/service (Windows SCM/launchd/systemd); OpenWrt 用 procd |
| Web 鉴权 | Basic Auth, `[web] password` 非空启用; subtle.ConstantTimeCompare |
| 存储引擎 | BoltDB (纯 Go 无 CGO); bucket "dns" 缓存 + "custom" 自定义覆盖 |
| 编码格式 | gob (缓存); JSON (Web API) |
| 并发策略 | Manager.ResolveAll() 聚合 + (Name,Type,RData) 去重; inflight dedup |
| 前端 | Svelte 4 + Vite 5; fetch API 封装沿用旧绑定方法名, 组件改动最小 |
| OpenWrt 打包 | 免 SDK: tools/mkipk 纯 Go 写 ar+tar.gz (Windows 构建机无 binutils) |
| 日志 | 标准库 log/slog |

## Go 包依赖图

```
cmd/dnsforvpn/main.go
  ├── internal/control      (配置 + DNS 生命周期)
  │     ├── internal/cache / router / query / transport / upstream ...
  ├── internal/web          (REST API + SPA)
  │     ├── internal/control
  │     └── frontend        (embed dist)
  ├── internal/browser      (开浏览器)
  └── kardianos/service     (系统服务)

tools/{genicons,dnsprobe,mkipk}  # 独立小工具, 不进入主二进制
```

## 后续会话变更 (2026-07-17 傍晚, configs/ 单目录布局)

- **configs/ 布局落地** (用户拍板): config.toml→configs/, gfwlist.txt→configs/rules/
  (入库+随包携带, 解决离线首启), data/→configs/data/ (gitignore, 不打包);
  CLI 默认 --config=configs/config.toml; mkipk 携带 rules 种子;
  NSIS 配置改装 %PROGRAMDATA%\DNSforVPN; linux install.sh 装 rules/
- **要点**: db 是运行时产物不进包; gfwlist 种子进包是刻意为之
  (raw.githubusercontent 部分网络不可达)

### 延迟面板连接复用 (同日)

- **CheckLatency 复用 DoH 客户端** (control.go): healthClients map
  按 `serverURL|proxyURL` 缓存, 配置变更时 prune; 轮询测到热连接延迟
- **LatencyPanel 标题** → "DNS Server Latency"; 语义 = 每 5s 主动探测
  example.com 的合成指标, 真实查询延迟在 StatsPanel AvgLatencyMs
- **诊断结论** (httptrace 探针验证): doh.pub/alidns 均支持 H2 连接复用
  (5s 空闲不断); 复用后实测 doh.pub ~230→~75ms, alidns ~220→~65ms,
  代理链路 dns.google ~1.5s→~205ms, 1.1.1.1 ~1s→~210ms。
  此前"复用无效"是用了旧二进制实测的假象

## 本次会话变更 (2026-07-17 下午, 架构迁移)

- **移除 Wails**: 删 app.go/main.go/wails.json/frontend/wailsjs/package.json.md5;
  go.mod 清掉 wails 及 20+ 间接依赖
- **新增 internal/control**: ex-app.go 全部业务逻辑 (13 方法), 去 Wails 上下文;
  增加 WebConfig、cfgPath/cfgDir 相对路径解析、Start 自动 MkdirAll(db 目录)
- **新增 internal/web**: 12 个 REST 端点 + embed SPA (含 SPA fallback,
  /api/* 404 保护) + Basic Auth 中间件; `.webmanifest` MIME 注册
- **新增 internal/browser**: 三平台开浏览器, 无依赖
- **CLI 重写**: kardianos/service — 前台运行 / `service` 子命令
  (install/uninstall/start/stop/restart/status); 交互模式自动开浏览器
- **前端**: 新增 api.js (fetch), 6 个组件去 window.go; vite dev /api 代理;
  ConfigPanel 新增 Web UI 段 (onMount 自动加载)
- **PWA**: manifest + sw.js (app shell 缓存, API 不缓存) + 程序化图标
  (tools/genicons: 4x 超采样, 192/512/maskable/ICO)
- **OpenWrt**: build-ipk.sh + tools/mkipk (纯 Go ar/tar.gz) + procd init +
  postinst/prerm + 路由器默认配置 (DNS 127.0.0.1:1553, Web 0.0.0.0:8080)
- **桌面打包**: NSIS installer.nsi (服务注册 + .url 快捷方式 + 升级保留 config)、
  linux install/uninstall.sh + .desktop、macos make-app.sh
- **cache.Stats 补 json tag** (snake_case, Web API 契约)
- **README 整体重写** (旧内容是上一个单体项目, 完全过时)

## 测试结果 (2026-07-17, Windows)

```
go build ./... + go vet ./...      ✅
go test ./internal/... -count=1    ✅ (dns 17 / router 16 / cache 20 / upstream 4)
vite build                         ✅
端到端 (.e2e 隔离配置):
  12 个 API 端点全部 curl 验证      ✅
  SPA fallback / /api 404 / MIME    ✅
  Basic Auth 401/401/200/静态保护    ✅
  dnsprobe 自定义覆盖 A+AAAA        ✅ (mytest.local → 1.2.3.4 / ::1)
  真实上游 + 缓存命中 + 统计        ✅ (baidu.com, hits=3 miss=1)
  Start/Stop 控制                   ✅
  service status (未安装)           ✅
交叉编译: linux amd64/arm64/mipsle(sf), darwin amd64/arm64, windows amd64 ✅
ipk 组装 + ar/tar 结构校验           ✅
```
