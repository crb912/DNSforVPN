# DNSforVPN — TODO & Handoff

> 上次会话: 2026-07-18
> 状态: **分发模式简化落地 — 全平台统一"二进制 + install/uninstall 脚本", Makefile 构建,
> NSIS/ipk/.app/.desktop 全部删除。DNS 默认端口 5553 (上游定位, 不抢 53)。**
> 入口: `/home/raybing/Desktop/github/DNSforVPN` (Linux) / `C:/Users/Bing/Desktop/git/DNSforVPN` (Windows)


## 下次会话立即执行

```bash
# 开发/日常运行 (识别当前系统, 构建并前台运行, 自动开浏览器)
make run

# 构建分发目录 build/<os>/ (二进制 + config + rules + install/uninstall 脚本)
make linux          # 或 windows / macos / openwrt
make openwrt ARCH=mipsle    # openwrt 默认 arm64; 其他平台默认宿主架构, ARCH= 可覆盖

# 安装 (目标机器上): 整个 build/<os>/ 拷过去, 跑 install 脚本
#   linux:  sudo ./install.sh      → /opt/dnsforvpn       (systemd)
#   windows: 管理员运行 install.bat → C:\Program Files\DNSforVPN (SCM)
#   macos:  sudo ./install.sh      → /usr/local/dnsforvpn (launchd)
#   openwrt: scp 后 sh install.sh  → /usr/bin + /etc/dnsforvpn (procd)
# 卸载: 对应 uninstall 脚本 = 停服务 + 注销 + 全删 (二进制/配置/缓存)
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
  index.html 仅保证裸克隆可编译, 真正 UI 需 vite 产出); make 目标已自动前置。
- **JSON 契约**: 空 slice 必须 `make([]T,0)` (序列化为 null 会崩前端);
  cache.Stats 已补 snake_case json tag。
- **PWA**: sw.js 只缓存静态资源, /api/* 永远走网络。改 CACHE 常量版本号
  以强制刷新 (当前 dnsforvpn-v3)。注意: 前端任何改动发布时都必须 bump,
  否则已装 SW 的浏览器永远吃旧缓存 — 曾因此误判"改动没生效"。
- **"页面没变化"排查两步**: ① 确认浏览器连的是新进程 —
  `ss -tlnp | grep :8080` 看 PID/启动时间; 注意 web 端口被占时进程
  **只 slog.Error 不退出** (main.go work()), 旧进程继续服务旧 UI。
  ② 前端改动 bump CACHE 后浏览器需**刷新两次** (第一次后台装新 SW 清旧
  缓存, 第二次才吃新壳); 或 DevTools → Application → Service Workers →
  Unregister 后一次刷新。2026-07-17 深夜实例: 无残留旧进程 (唯一进程
  持 127.0.0.1:8080 且为新二进制), 纯 SW 缓存所致。

## 待完成任务

### 高优先级

暂无

### 中优先级

- [ ] **L3 系统托盘** (用户已排期, 暂缓) — Web UI 稳定后评估:
  getlantern/systray 系; 注意 Linux 会引入 GTK/AppIndicator CGO 依赖,
  与"纯 Go 零 CGO"原则冲突, 可考虑仅 Windows/macOS 启用 (build tags)
- [ ] **config.toml 热重载** — SaveConfig 写盘但需重启生效; 考虑 Web UI
  加 Restart 按钮 (control.Stop+Start 已具备条件)
- [ ] **GFWList 定时刷新** — 后台 goroutine 定期重下规则文件
- [ ] **UDP server 优雅关闭** — Close() 等待 WaitGroup, 高负载下可能卡住
- [ ] **OpenWrt 实机验证** — 分发改为 build/openwrt/ (二进制 + install.sh +
  procd init) 后尚未在真路由器 scp + sh install.sh + dnsmasq 转发实测;
  LuCI 跳转入口已明确不做 (用户拍板: Web UI 即配置界面)
- [ ] **Windows install.bat 实机验证** — NSIS 已删, 分发为 build/windows/
  (dnsforvpn.exe + install.bat); service install/start 逻辑已在 CLI 层验证,
  bat 脚本本身未在 Windows 实跑
- [ ] **macOS 未验证** — 交叉编译通过; install.sh (launchd) 未实跑

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
| 分发形态 | 二进制 + 各平台 install/uninstall 脚本, Makefile 产出 build/<os>/; 无 NSIS/ipk/.app (2026-07-18 起) |
| GUI 方案 | ~~Wails v2~~ → 内嵌 Web UI (embed Svelte SPA + REST); 已移除 Wails |
| 桌面入口 | L1 安装脚本+启动开浏览器 ✅; L2 PWA ✅; L3 托盘 (TODO) |
| 常驻模型 | Model B: 系统服务保活 DNS, 图标只是 UI 启动器 |
| 服务管理 | kardianos/service (Windows SCM/launchd/systemd); OpenWrt 用 procd |
| DNS 端口 | 默认 5553, 上游定位 (dnsmasq/系统 DNS 手动转发); 不抢 53 |
| Web 鉴权 | Basic Auth, `[web] password` 非空启用; subtle.ConstantTimeCompare |
| 存储引擎 | BoltDB (纯 Go 无 CGO); bucket "dns" 缓存 + "custom" 自定义覆盖 |
| 编码格式 | gob (缓存); JSON (Web API) |
| 并发策略 | Manager.ResolveAll() 聚合 + (Name,Type,RData) 去重; inflight dedup |
| 前端 | Svelte 4 + Vite 5; fetch API 封装沿用旧绑定方法名, 组件改动最小 |
| 构建 | Makefile: make linux/windows/macos/openwrt/run/clean; 纯 Go 交叉编译 |
| 日志 | 标准库 log/slog |


## 本次会话变更 (2026-07-18, 分发简化 + Makefile + 端口 5553)

- **背景**: 用户拍板三件事 — ① DNS 不做 53 端口方案, 默认 5553 作为其他
  DNS 服务的上游 (dnsmasq/系统 DNS 由用户手动接线); ② 全平台统一"二进制 +
  install/uninstall 脚本"分发, 放弃 NSIS 安装器 / OpenWrt ipk / macOS .app
  (ipk 与 LuCI 页面均不做: Web UI 即配置界面, 路由器浏览器直达 :8080);
  ③ 构建统一走 Makefile
- **新增 Makefile**: 目标 linux/windows/macos/openwrt/run/clean; 每个平台
  目标产出 build/<os>/ (二进制 + config.toml + rules/ + 该平台 install/
  uninstall 脚本); 前端 npm build 为前置; 配置/规则 cp -n 不覆盖 (保住
  Web UI 修改); 默认架构=宿主, openwrt 默认 arm64, `ARCH=` 可覆盖;
  mips/mipsle 自动 GOMIPS=softfloat (坑: recipe 里须用 `env $SF go build`,
  直接 `$SF go build` 会被 dash 当作命令名报 not found)
- **deploy/ 重写**: linux/macos install.sh (二进制+uninstall脚本+config
  +rules → /opt/dnsforvpn 与 /usr/local/dnsforvpn, 不覆盖已有 config,
  `service install --config` 注册+启动) + uninstall.sh (停+注销+全删,
  不再有 --purge 分支); windows install.bat/uninstall.bat (%ProgramFiles%\
  DNSforVPN, 选 .bat 避开 ps1 执行策略); openwrt install.sh/uninstall.sh
  (busybox sh 兼容, 用 cp/chmod 不用 install; 二进制→/usr/bin 对齐既有
  dnsforvpn.init, 配置→/etc/dnsforvpn; 输出含设密码警告 + dnsmasq 转发
  三行 uci 含 noresolv)
- **删除**: deploy/windows/installer.nsi + dnsforvpn.ico, tools/mkipk/,
  deploy/openwrt/build-ipk.sh + control + postinst + prerm,
  deploy/macos/make-app.sh, deploy/linux/dnsforvpn.desktop
- **端口 1553→5553**: deploy/openwrt/config.toml, tools/dnsprobe 默认值,
  test/dns_client.py; configs/config.toml 用户自己已改 5553
- **.gitignore**: 加 /build/, 删 ipk/.app 旧条目; README 整体重写
- **验证**: go build/vet/test 全绿; make 四目标产出 file 校验 (ELF x86-64 /
  PE32+ / Mach-O / ELF aarch64, mipsle 软浮点 ELF32 验证); make run 实测
  DNS 5553 dnsprobe 解析正常 (Web UI 8080 冲突系用户残留 sudo 进程, 预期内)
- **启动器图标** (同日追加): install 装图标, uninstall 卸载 — linux: icon.png
  (Makefile 从 frontend/public/icons/icon-512.png 拷入 build/linux/) +
  内联写 .desktop → /usr/share/applications + hicolor 图标; macos: 内联生成
  /Applications/DNSforVPN.app (Info.plist + launch 脚本 open :8080, 无 icns
  自定义图标); windows: 桌面 DNSforVPN.url 快捷方式 (无 .ico); openwrt 无
- **注意**: 用户另有 sudo ./dnsforvpn 残留进程 (PID 5418, 53 端口旧配置),
  与本改动无关, 未动


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
- **LatencyPanel 标题** → "DNS Server Latency"; 语义 = 手动触发的主动探测
  example.com 的合成指标, 真实查询延迟在 StatsPanel AvgLatencyMs
- **诊断结论** (httptrace 探针验证): doh.pub/alidns 均支持 H2 连接复用
  (5s 空闲不断); 复用后实测 doh.pub ~230→~75ms, alidns ~220→~65ms,
  代理链路 dns.google ~1.5s→~205ms, 1.1.1.1 ~1s→~210ms。
  此前"复用无效"是用了旧二进制实测的假象

## 本次会话变更 (2026-07-17 晚②, DNS 模式 + Save 按钮上移)

- **[dns] mode 配置**: "direct" (全部查询走直连组) / "proxy" (全部走代理组)
  / "rules" (gfwlist 分流, 默认; 空或未知值回退 rules)。实现在
  query.Service.Handle 的选池分支 (新增 mode 字段 + ModeDirect/ModeProxy/
  ModeRules 常量, New 签名加 mode 参数); control.DNSConfig.Mode 透传,
  Start 日志带 mode; configs/ 与 deploy/openwrt 的 config.toml 均加
  mode = "rules"
- **ConfigPanel**: Save Config 上移到 Load Config 旁 (footer 删除,
  状态提示同排); DNS Listen 下新增 DNS Mode 三选一 radio (横向并排,
  bind:group, 旧配置 load 后自动补 "rules")
- **测试**: 新增 internal/query/service_test.go (4 个 mode 用例:
  direct 只打直连 / proxy 只打代理 / rules 按域名分流 / 空 mode 回退)
- 验证: go test ./internal/... 全绿 (dns 17 / router 16 / cache 20 /
  upstream 4 / query 4) / vet / gofmt ✅; vite build ✅; SW bump v3

## 本次会话变更 (2026-07-17 晚, 延迟面板 + 配置面板改版)

- **LatencyPanel 手动刷新**: 去掉 5s 轮询 — onMount 自动测一次 + Refresh
  按钮; 只显示当前激活 (已配置) 服务器, 探测走 `GET /api/latency`
  (CheckLatency → CheckServersLatency, 按 server|proxy 复用 keep-alive
  客户端); 无激活服务器时显示提示
- **分组展示**: Direct (闪电) / Proxy (盾牌) 图标分组; 名称命中
  doh-catalog.js 目录显示目录名, 否则 hostname
- **颜色阈值重校准** (热连接): <100ms 绿 / <300ms 橙 / 其余红
- **共享候选目录**: `frontend/src/doh-catalog.js` (CATALOG), LatencyPanel
  (名称查找) 与 ConfigPanel (候选列表) 共用
- **ConfigPanel DoH 段改版**: 文本输入列表 → 候选列表 (目录 + 配置中自定义
  URL); 勾选=写入 config (Save 生效); 全部行 × 删除 — 自定义行移出 config,
  目录行移出 config 并隐藏 (localStorage `config.hiddenCatalog.v1`,
  + Add 重加同 URL 可恢复); 每组底部输入框 + Add/Enter 添加 (添加即勾选)
- **注**: 曾为候选探测加的 `POST /api/latency` 随 UI 定版移除 (无消费者)
- 验证: vite build ✅ / go build ./... ✅ / go vet ./... ✅
  (gofmt 残留 cmd/dnsforvpn/main.go + tools/genicons/main.go 对齐问题,
  系 HEAD 既有, 未动)

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
