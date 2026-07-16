# DNSforVPN — TODO & Handoff

> 上次会话: 2026-07-17 (凌晨)  
> 状态: 全部功能已验证 (后端 + 5 面板 GUI + 自定义 DNS), 可日常运行  
> 入口: `/home/raybing/Desktop/github/DNSforVPN`

## 项目概览

五层架构的 DNS-over-HTTPS 代理，带 Wails v2 + Svelte 跨平台 GUI。

```
dnsforvpn/
├── cmd/dnsforvpn/main.go         # CLI 入口（无 GUI 模式）
├── main.go                       # Wails 入口 (含 //go:embed frontend/dist + AssetServer)
├── app.go                        # Wails App 结构体 (13 个 Bind 方法)
├── wails.json                    # Wails 项目配置
├── config.toml                   # 运行时配置 (当前 port=1553)
├── internal/
│   ├── dns/        ✅ 17 tests   # DNS wire-format 解析/构建
│   ├── router/     ✅ 16 tests   # GFWList 域名路由
│   ├── cache/      ✅ 20 tests   # BoltDB + hot cache + negative + CustomStore
│   ├── upstream/   ✅ 4 tests    # Resolver 接口 + DoH + UDP + Manager (reduce 去重)
│   ├── query/      ✅            # 查询管线编排 (inflight dedup + custom 覆盖)
│   └── transport/  ✅            # UDP DNS server
└── frontend/                     # Svelte 4 + Vite 5
    └── src/
        ├── App.svelte            # 主框架 (5 Tab)
        └── lib/
            ├── ConfigPanel.svelte   # 配置编辑
            ├── LatencyPanel.svelte  # 延迟监控
            ├── StatsPanel.svelte    # QPS/缓存仪表盘
            ├── CachePanel.svelte    # 缓存浏览
            └── CustomDNSPanel.svelte# 自定义域名→IP 覆盖
```

## 下次会话立即执行

```bash
cd /home/raybing/Desktop/github/DNSforVPN

# 运行 DNS 服务 (端口 1553, 免 sudo; 改回 53 需 setcap, 见运维要点)
go build -o dnsforvpn ./cmd/dnsforvpn/ && ./dnsforvpn

# 或 GUI 开发模式 (必须带 tag)
wails dev -tags webkit2_41

# 生产构建
wails build -tags webkit2_41   # 产物 build/bin/dnsforvpn (10MB, 动态链接 GTK/WebKit)
```

## 运维要点 (踩过的坑)

- **BoltDB 单句柄**: `data/dns_cache.db` 同一时刻只允许一个进程打开。
  CLI 服务运行时, GUI 的 Cache/Custom DNS 面板会报锁超时 — 要么用 GUI
  内 Start 启动服务, 要么改条目时先停 CLI。
- **自定义覆盖实时生效**: query 层每次查询都读 DB (CustomGet 无热缓存),
  面板改完立刻生效, 无需重启服务。
- **文件所有权**: 曾用 `sudo ./dnsforvpn` 导致 `data/` 变 root 所有, GUI
  打不开 DB。已 `sudo chown -R raybing:raybing data/` 修复, 勿再混用。
- **53 端口**: 绑 53 需特权。推荐 `sudo setcap 'cap_net_bind_service=+ep'
  ./dnsforvpn` (重编译后需重跑), 而非 sudo 运行。当前配置 1553 免特权。
- **wails build/dev 必须 `-tags webkit2_41`**: Ubuntu 24.04 只有
  webkit2gtk-4.1 (已装 libgtk-3-dev + libwebkit2gtk-4.1-dev)。
- **App Bind 方法返回 slice 必须 `make([]T, 0)`**: nil slice 序列化成
  JSON null 会崩 Svelte 前端 (已修 3 处)。
- **GUI 发行包运行时依赖**: 目标机需 `libgtk-3-0` + `libwebkit2gtk-4.1-0`;
  headless 场景用 CLI (纯 Go, 可静态编译)。

## 待完成任务

### 高优先级

- [ ] **修正自定义条目拼写** — 面板中 `porhub.com` 应为 `pornhub.com`
  (少个 n, 当前 dig pornhub.com 仍走真实上游)

### 中优先级

- [ ] **config.toml 热重载** — SaveConfig 写盘但需重启服务生效, 考虑 Restart 按钮
- [ ] **GFWList 定时刷新** — 后台 goroutine 定期重下规则文件
- [ ] **UDP server 优雅关闭** — Close() 等待 WaitGroup, 高负载下可能卡住
- [ ] **Windows/macOS 交叉编译测试**
- [ ] **gofmt 收尾** — 5 个遗留文件: cmd/dnsforvpn/main.go,
  internal/cache/boltdb.go, internal/dns/build.go, internal/dns/dns_test.go,
  internal/router/router_test.go (一次性 `gofmt -w .`)
- [ ] **Wails 版本对齐** — go.mod v2.9.2 vs CLI v2.13.0, build 有 warning

### 低优先级 / 未来

- [ ] **DoT (DNS over TLS)** — 实现 upstream/dot/ 包
- [ ] **DoQ (DNS over QUIC)** — 实现 upstream/doq/ 包
- [ ] **LRU 淘汰** — boltCache 的 hotEntry 已有 lastAccess, 可实现淘汰逻辑
- [ ] **Prometheus metrics** — 暴露 /metrics 端点
- [ ] **Svelte 深色/亮色主题切换**
- [ ] **日志查看器面板**
- [ ] **缓存面板删除/清空按钮** (Cache.Del 已存在, 只欠绑定+UI)
- [ ] **自定义 DNS 支持更多类型** (CNAME 等; 当前仅 A/AAAA, TTL 固定 300)

## 关键设计决策

| 决策 | 内容 |
|------|------|
| 存储引擎 | BoltDB (纯 Go, 无 CGo); bucket "dns" 缓存 + "custom" 自定义覆盖 |
| 热缓存 | sync.Map 懒加载, 首次 Get 触发一次读盘 |
| 编码格式 | gob (比 JSON 快, 无 protobuf 依赖) |
| 并发策略 | Manager.ResolveAll() 聚合全部成功响应 + 按 (Name,Type,RData) 去重 |
| 前端 | Wails v2 (稳定版) + Svelte 4 |
| 去重 | sync.Map + chan 实现的 inflight 请求合并 |
| 日志 | 标准库 log/slog |
| 自定义 DNS | 管线步骤 1b 最高优先级, 仅 A/AAAA, TTL=300, 每次查询实时读库 |

## Go 包依赖图

```
cmd/dnsforvpn/main.go
  └── internal/transport
        └── internal/query (Service.Handle)
              ├── internal/dns       (ParseQuery, BuildResponse)
              ├── internal/router    (Route)
              ├── internal/cache     (Get, Set, List, NegativeCache, CustomStore)
              └── internal/upstream  (Manager.ResolveAll)
                    ├── internal/upstream/doh  (Client.Resolve)
                    └── internal/upstream/udp  (Client.Resolve)

main.go / app.go (Wails)
  └── 同上, 通过 App.Start() 装配
```

## 本次会话变更 (2026-07-16 晚 ~ 17 凌晨)

- **删除遗留单体文件** `config.go` / `dns.go` / `doh.go` (旧架构, git 可恢复)
- **app.go**: 补 `Config` 顶层结构体; `toml.Marshal` 改为
  `toml.NewEncoder(&buf).Encode()` (BurntSushi 无 Marshal)
- **manager.go**: `reduce()` 按 (Name,Type,RData) 去重 + 最小 TTL,
  修复多上游聚合重复记录; 新增 4 测试
- **main.go**: 补 `//go:embed all:frontend/dist` + `AssetServer`
  (否则 wails.Run 报 AssetServer options invalid)
- **新增 Cache 浏览 + Custom DNS 面板**: cache.List + CustomStore
  (bucket "custom"); query 管线步骤 1b; app.go 5 个新绑定方法 +
  withStore helper (stopped 时临时开库); 前端 2 个新组件 + 2 Tab
- **修复 nil slice 前端崩溃**: GetCustomDNS/queryCache/CheckLatency
  空结果返回 null 崩 Svelte (tab 无法切换); make([]T,0) + 前端 ?? []
- **修复 CNAME 响应坏包**: encodeRData CNAME 分支漏根结束符 0x00,
  dig 报 "extra input data"; 修复 + wire 字节级测试
  (自家 ParseResponse 对缺终止符宽容, 必须直接断言字节)
- **端口改 1553** (用户): 免 sudo 运行, dig 实测自定义覆盖与上游均正常

## 测试结果

```
go test ./internal/... -count=1 -race
  internal/dns       ✅ 17 tests (含 CNAME rdata 终止符)
  internal/router    ✅ 16 tests
  internal/cache     ✅ 20 tests (含 List/CustomStore/Records)
  internal/upstream  ✅ 4 tests (reduce 去重)
  go vet             ✅ clean
```

## 上次 dig 验证 (2026-07-17, 用户亲测 @127.0.0.1 -p 1553)

```
dig youjizz.com  → 127.0.0.1, TTL 300, 1ms   (自定义覆盖命中)
dig pornhub.com  → 66.254.114.41, TTL 14000  (拼写未覆盖, 走代理上游)
dig youjizz1.com → CNAME plantwo.seihappy.com + 2×A (CNAME 修复后 dig 正常解析;
                    上游为 GFW 污染应答: 假 IP + TTL=1, 属预期)
```
