# DNSforVPN — Architecture Reference

> 完整架构文档，供大模型下次会话加载。读取此文件即可获得全部上下文。

## 项目结构

```
dnsforvpn/                          # module: doh-dns-proxy, go 1.22+
├── cmd/dnsforvpn/main.go           # 唯一入口: run (前台/服务) + service 子命令
├── configs/                        # 配置根目录 (单目录布局, 打包携带前两项)
│   ├── config.toml                 #   主配置 (含 [web] 段)
│   ├── rules/gfwlist.txt           #   规则种子 (随包携带, 离线可启动)
│   └── data/                       #   运行时数据 (BoltDB; gitignore, 不打包)
│
├── internal/
│   ├── dns/                        # DNS wire-format (纯函数, 零状态)
│   │   ├── types.go                #   常量 + Query, Result, RRRecord
│   │   ├── parse.go                #   ParseQuery, ParseResponse
│   │   ├── build.go                #   BuildResponse, BuildErrorResponse, BuildQuery
│   │   └── dns_test.go             #   17 tests
│   │
│   ├── router/                     # 域名路由决策 (横向层)
│   │   ├── router.go               #   Router struct + GFWList 解析
│   │   └── router_test.go          #   16 tests
│   │
│   ├── cache/                      # 缓存层
│   │   ├── cache.go                #   Cache 接口 + Entry, Stats(带 json tag), CacheItem
│   │   ├── boltdb.go               #   boltCache (BoltDB + sync.Map hot cache)
│   │   ├── custom.go               #   CustomEntry + CustomStore 接口
│   │   ├── negative.go             #   NegativeCache (NXDOMAIN)
│   │   └── cache_test.go           #   20 tests
│   │
│   ├── upstream/                   # 上游协议层
│   │   ├── resolver.go             #   Resolver 接口 + Result, StatsSnapshot, ServerLatency
│   │   ├── manager.go              #   Manager: 并发多服务器编排 (方案B)
│   │   ├── doh/                    #   DoH Client + TCP ping
│   │   └── udp/                    #   UDP Client (bootstrap)
│   │
│   ├── query/                      # 查询管线编排
│   │   └── service.go              #   Service + inflight dedup
│   │
│   ├── transport/                  # 网络传输层
│   │   └── udp.go                  #   UDPServer
│   │
│   ├── control/                    # 控制层 (平台无关, ex-app.go 剥离 Wails)
│   │   └── control.go              #   Control: Config + DNS 生命周期 + 全部业务方法
│   │
│   ├── web/                        # Web 层
│   │   └── server.go               #   REST API (/api/*) + embed SPA + Basic Auth
│   │
│   └── browser/                    # 浏览器启动
│       └── browser.go              #   Open(url): rundll32/open/xdg-open
│
├── frontend/                       # Web 前端 (Svelte 4 + Vite 5)
│   ├── embed.go                    #   //go:embed all:dist (产物内嵌进二进制)
│   ├── package.json / vite.config.js / index.html
│   ├── public/
│   │   ├── manifest.webmanifest    #   PWA manifest
│   │   ├── sw.js                   #   service worker (静态缓存, /api 直通)
│   │   └── icons/                  #   genicons 生成的 PNG 图标
│   └── src/
│       ├── main.js                 #   入口 + 注册 service worker
│       ├── api.js                  #   fetch 封装 (方法名沿用旧 Wails 绑定)
│       ├── App.svelte              #   主框架 (5 Tab)
│       └── lib/
│           ├── ConfigPanel.svelte  #   配置编辑 (含 Web UI 段)
│           ├── LatencyPanel.svelte #   延迟监控 (5s 轮询)
│           ├── StatsPanel.svelte   #   QPS/缓存仪表盘
│           ├── CachePanel.svelte   #   缓存浏览
│           └── CustomDNSPanel.svelte # 自定义域名→IP 覆盖
│
├── tools/
│   ├── genicons/main.go            # 程序化生成图标 (PNG + ICO, 4x 超采样)
│   └── dnsprobe/main.go            # UDP DNS 探测 (替代不可靠的 nslookup -port)
│
├── Makefile                        # 唯一构建入口: linux/windows/macos/openwrt/run/clean
│
└── deploy/                         # 各平台安装脚本 (make 时拷进 build/<os>/ 随二进制分发)
    ├── openwrt/                    # install.sh/uninstall.sh + dnsforvpn.init (procd) + 路由版 config
    ├── windows/                    # install.bat / uninstall.bat
    ├── linux/                      # install.sh / uninstall.sh
    └── macos/                      # install.sh / uninstall.sh
```

---

## 一、包级 API 签名

### 1.1 `internal/dns` — DNS wire-format

```go
// === types.go ===
const QTypeA=1, QTypeAAAA=28, QTypeCNAME=5, QTypeSOA=6, QTypeOPT=41, QClass=1
const MinimumTTL = 300
const RcodeOK=0, RcodeNXDomain=3, RcodeServFail=2

type Query struct {
    ID     uint16
    Domain string
    Type   uint16
    Packet []byte          // 原始 wire-format 查询包
}

type Result struct {
    Records  []RRRecord
    TTL      uint32
    NXDomain bool
}

type RRRecord struct {
    Name  string
    Type  uint16
    Class uint16
    TTL   uint32
    RData string           // 展示格式: "192.0.2.1", "example.com"
}

// === parse.go ===
func ParseQuery(packet []byte) (*Query, error)
func ParseResponse(packet []byte) (Result, error)

// === build.go ===
func BuildResponse(q *Query, r Result) []byte
func BuildErrorResponse(q *Query, rcode int) []byte
func BuildQuery(domain string, qtype uint16) []byte
func BuildQueryWithID(domain string, qtype uint16, id uint16) []byte
func TTLRemaining(expireAt int64) uint32
```

### 1.2 `internal/router` — GFWList 路由

```go
type Decision int
const (
    RouteDirect Decision = iota   // → 直连 DoH 服务器
    RouteProxy                     // → 代理 DoH 服务器
)

type RouteResult struct {
    Decision  Decision
    MatchRule string              // 命中的 GFWList 规则 (调试用)
}

func New(filePath, fileURL string) (*Router, error)
func (r *Router) Route(domain string) RouteResult
func (r *Router) Size() int
func (r *Router) Reload(filePath, fileURL string) error
```

### 1.3 `internal/cache` — 缓存层

```go
// === cache.go ===
type Entry struct {
    Records  []dns.RRRecord
    TTL      uint32
    ExpireAt int64               // Unix timestamp
}
func (e Entry) Expired() bool    // time.Now().Unix() >= ExpireAt

type Stats struct {
    Size      int64
    HitRate   float64
    MemBytes  int64
    DiskBytes int64
}

type Cache interface {
    Get(domain string, qtype uint16) (Entry, bool)
    Set(domain string, qtype uint16, entry Entry)
    Del(domain string)
    List() []CacheItem              // 全量遍历 (含过期), domain/qtype 从 key 解析
    Stats() Stats
    Close() error
}

type CacheItem struct {
    Domain string
    QType  uint16
    Entry  Entry
}

// === custom.go ===
type CustomEntry struct {
    Domain string
    IPs    []string                // v4/v6 混合, Records() 按 qtype 过滤
}
func (e CustomEntry) Records(qtype uint16) []dns.RRRecord  // TTL=MinimumTTL

type CustomStore interface {       // boltCache 实现, bucket "custom"
    CustomGet(domain string) (CustomEntry, bool)
    CustomSet(domain string, ips []string)
    CustomDel(domain string)
    CustomList() []CustomEntry     // 按域名排序
}

// === boltdb.go ===
func New(dbPath string) (Cache, error)
// 内部实现: boltCache
//   - db: *bolt.DB (bucket: "dns", key: "domain|qtype", value: gob(Entry))
//   - hot: sync.Map (map[string]*hotEntry), 懒加载: 首次 Get 触发读盘
//   - 编码: encoding/gob (无外部依赖)

// === negative.go ===
type NegativeCache struct { ... }
func NewNegativeCache(ttl time.Duration) *NegativeCache
func (n *NegativeCache) Get(domain string) bool
func (n *NegativeCache) Set(domain string)
func (n *NegativeCache) Size() int
func (n *NegativeCache) Sweep()
```

### 1.4 `internal/upstream` — 上游协议

```go
// === resolver.go ===
type Result struct {
    Records  []dns.RRRecord
    TTL      uint32
    NXDomain bool
}

type StatsSnapshot struct {
    Protocol     string  `json:"protocol"`
    Server       string  `json:"server"`
    TotalQueries uint64  `json:"total_queries"`
    TotalErrors  uint64  `json:"total_errors"`
    AvgLatencyMs float64 `json:"avg_latency_ms"`
    P50LatencyMs float64 `json:"p50_latency_ms"`
    P99LatencyMs float64 `json:"p99_latency_ms"`
}

type ServerLatency struct {
    ServerURL string  `json:"server_url"`
    LatencyMs float64 `json:"latency_ms"`
    Status    string  `json:"status"`   // "ok", "timeout", "error"
}

type Resolver interface {
    Resolve(ctx context.Context, domain string, qtype uint16) (Result, error)
    Stats() StatsSnapshot
    HealthCheck(ctx context.Context) ServerLatency
}

// === manager.go ===
type Manager struct { ... }
func NewManager(direct, proxy []Resolver, boot Resolver) *Manager
func (m *Manager) Direct() []Resolver
func (m *Manager) Proxy() []Resolver
func (m *Manager) Bootstrap() Resolver
func (m *Manager) ResolveAll(ctx, domain, qtype, resolvers) (Result, error)  // 并发 fan-out
func (m *Manager) HealthCheckAll(ctx) []ServerLatency
func (m *Manager) StatsAll() []StatsSnapshot

// === doh/client.go ===
type Client struct { ... }
func New(serverURL, proxyURL string) *Client
func (c *Client) Resolve(ctx, domain, qtype) (upstream.Result, error)
func (c *Client) Stats() upstream.StatsSnapshot
func (c *Client) HealthCheck(ctx) upstream.ServerLatency

// === udp/client.go ===
type Client struct { ... }
func New(addr string) *Client          // addr: "223.5.5.5:53"
// 实现 Resolver 接口同上
```

### 1.5 `internal/query` — 查询管线

```go
type Stats struct {
    TotalQueries uint64  `json:"total_queries"`
    CacheHits    uint64  `json:"cache_hits"`
    CacheMisses  uint64  `json:"cache_misses"`
    TotalErrors  uint64  `json:"total_errors"`
    AvgLatencyMs float64 `json:"avg_latency_ms"`
}

type Service struct { ... }        // + custom cache.CustomStore (可选, nil 禁用), mode (见下)
// DNS 模式: ModeDirect="direct" (全部直连组) / ModeProxy="proxy" (全部代理组)
//   / ModeRules="rules" (gfwlist 分流; 空或未知值按 rules)
func New(c cache.Cache, nc *cache.NegativeCache, r *router.Router, u *upstream.Manager, cs cache.CustomStore, mode string) *Service
func (s *Service) Handle(ctx, packet, clientAddr) ([]byte, error)  // 完整管线
func (s *Service) Stats() Stats
func (s *Service) CacheStats() cache.Stats
func (s *Service) Shutdown() error
```

**Handle 管线流程**:
```
1.  dns.ParseQuery(packet)
1b. custom.CustomGet(domain) → 用户自定义覆盖命中直接返回 (A/AAAA, 最高优先级)
2.  mode 选池: direct→Direct() / proxy→Proxy() / rules→router.Route(q.Domain) 决定
3.  inflight dedup: sync.Map.LoadOrStore → 等待或接管
4.  negCache.Get(q.Domain) → NXDOMAIN 命中直接返回
5.  cache.Get(q.Domain, q.Type) → 热命中返回 / 过期返回+后台刷新
6.  缓存未命中 → manager.ResolveAll(ctx, q.Domain, q.Type, resolvers)
7.  NXDOMAIN → negCache.Set / 正常 → cache.Set + BuildResponse
```

**Inflight Dedup 实现**:
```go
// Service 结构体中:
inflight sync.Map  // map[string] chan struct{}

// Handle 中:
dedupKey := q.Domain + "|" + strconv.FormatUint(uint64(q.Type), 10)
ch := make(chan struct{})
if prev, loaded := s.inflight.LoadOrStore(dedupKey, ch); loaded {
    <-prev.(chan struct{})         // 等待第一个请求完成
    // 重新查缓存 (第一个请求已写入)
    if entry, ok := s.cache.Get(q.Domain, q.Type); ok {
        return BuildResponse(...)
    }
} else {
    defer func() {
        close(ch)                  // 唤醒所有等待者
        s.inflight.Delete(dedupKey)
    }()
}
```

### 1.6 `internal/transport` — 传输层

```go
type Handler func(ctx context.Context, packet []byte, addr net.Addr) ([]byte, error)

type UDPServer struct { ... }
func NewUDPServer(host string, port int, handler Handler) (*UDPServer, error)
func (s *UDPServer) Start(ctx context.Context) error    // 阻塞, 直到 ctx cancel
func (s *UDPServer) Close() error
```

---

## 二、控制层与 Web API

### 2.1 `internal/control` — 平台无关控制层

```go
// control.go — 全部业务逻辑, 被 CLI 和 web 层共用
type Control struct { /* cfgPath, cfgDir, mu, config, svc, srv, cache, custom, cancel, running */ }

func New(cfgPath string) (*Control, error)  // 加载配置, 记录配置文件目录
func (c *Control) GetConfig() Config
func (c *Control) SaveConfig(cfg Config) error          // 写入原配置文件
func (c *Control) GetStatus() string                     // "running" | "stopped"
func (c *Control) Start() error                          // 装配全部依赖 + 启动 DNS
func (c *Control) Stop()                                 // 优雅停止
func (c *Control) CheckLatency() []upstream.ServerLatency   // 探测客户端按 server|proxy 缓存复用 (keep-alive 热连接)
func (c *Control) GetCacheStats() cache.Stats
func (c *Control) GetQueryStats() query.Stats
func (c *Control) ListCache() ([]CacheEntryView, error)         // cap 500
func (c *Control) QueryCache(domain string) ([]CacheEntryView, error)
func (c *Control) GetCustomDNS() ([]CustomDNSEntry, error)
func (c *Control) SetCustomDNS(domain string, ips []string) error
func (c *Control) DeleteCustomDNS(domain string) error
```

要点: 配置中的相对路径 (db_path/rule_file) 相对**配置文件目录**解析
(`resolve()`), 系统服务任意 cwd 下行为一致; `Start()` 自动 `MkdirAll` DB 目录;
`withStore` 在 stopped 时临时开库 (BoltDB 单句柄约束)。

### 2.2 REST API (`internal/web`, Go 1.22 ServeMux)

```
GET    /api/config              → control.Config (JSON)
PUT    /api/config              body=Config → 保存写盘
GET    /api/status              → "running" | "stopped"
POST   /api/start               → 启动 DNS → status
POST   /api/stop                → 停止 DNS → status
GET    /api/latency             → []ServerLatency (全部已配置 DoH 服务器)
GET    /api/stats/cache         → cache.Stats
GET    /api/stats/query         → query.Stats
GET    /api/cache?domain=       → []CacheEntryView (domain 为空 = 全量, cap 500)
GET    /api/custom-dns          → []CustomDNSEntry
PUT    /api/custom-dns          body={domain, ips} → 校验 IP 后写入
DELETE /api/custom-dns?domain=  → 删除
GET    /*                       → embed SPA (frontend/dist); 未知路径回退 index.html;
                                  未匹配的 /api/* 返回 404
```

- **鉴权**: `[web] password` 非空 → 全部请求要求 HTTP Basic Auth
  (username 可空=不校验; subtle.ConstantTimeCompare 比对)
- **错误**: 500 + 纯文本错误信息; 空集合一律返回 `[]` (非 null)
- **静态**: `http.FileServerFS` + SPA fallback; `.webmanifest` 显式注册 MIME

---

## 三、config.toml 格式

位于 `configs/config.toml`; 其中相对路径 (db_path/rule_file) 相对 configs/ 解析。

```toml
[doh_servers]
direct_servers = ["https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"]
proxy_servers = ["https://dns.google/dns-query", "https://1.1.1.1/dns-query"]
bootstrap_server = "223.5.5.5"

[dns]
host = "0.0.0.0"
port = 5553
mode = "rules"   # direct=全部直连组 / proxy=全部代理组 / rules=gfwlist 分流 (默认)

[cache]
db_path = "data/dns_cache.db"
max_hot_size = 100000
save_interval = 72

[proxy]
enable_proxy = true
http = "http://192.168.5.8:7899"
https = "http://192.168.5.8:7899"
rule_file = "rules/gfwlist.txt"
rule_file_url = "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt"

[web]
host = "127.0.0.1"   # 桌面默认仅回环; 路由器改 0.0.0.0 并设密码
port = 8080
username = ""
password = ""        # 非空 → HTTP Basic Auth

[logging]
level = "info"
```

---

## 四、Go 依赖图

```
go.mod:
  github.com/BurntSushi/toml v1.3.2      # 配置解析
  go.etcd.io/bbolt v1.3.11               # 缓存存储
  github.com/kardianos/service v1.2.2    # 系统服务 (SCM/launchd/systemd)
```

**包依赖图**:
```
cmd/dnsforvpn/main.go
  ├── internal/control  (Config + DNS 生命周期)
  │     ├── internal/cache     (Get, Set, List, NegativeCache, CustomStore)
  │     ├── internal/router    (Route)
  │     ├── internal/query     (Service.Handle)
  │     │     └── internal/dns (ParseQuery, BuildResponse)
  │     ├── internal/transport (UDPServer)
  │     └── internal/upstream  (Manager.ResolveAll)
  │           ├── internal/upstream/doh
  │           └── internal/upstream/udp
  ├── internal/web      (REST API + SPA)
  │     ├── internal/control
  │     └── frontend          (//go:embed all:dist)
  ├── internal/browser  (打开默认浏览器)
  └── github.com/kardianos/service
```

---

## 五、关键设计决策记录

| 决策 | 理由 |
|------|------|
| 单二进制 + 内嵌 Web UI | AdGuard Home 模式: 桌面/路由器同一形态, 无显示环境依赖 |
| ~~Wails GUI~~ → REST + SPA | 路由器无窗口系统; 浏览器即 UI; 砍掉 GTK/WebKit 依赖与版本对齐负担 |
| Model B 服务常驻 | DNS 停了=断网, 必须由系统服务保活; 桌面图标只是 UI 启动器 |
| kardianos/service | 纯 Go 封装 SCM/launchd/systemd; OpenWrt 另用 procd (其不支持) |
| 路径按配置文件目录解析 | 服务模式 cwd 不确定, 保证行为一致 |
| Basic Auth 而非自研登录页 | 浏览器原生弹窗, 前端零代码; 局域网场景足够 |
| BoltDB 而非 LMDB | 纯 Go 无 CGO, 交叉编译友好 |
| gob 编码而非 JSON/protobuf | 比 JSON 快, 无 protobuf 依赖 |
| sync.Map 热缓存 | 读多写少, 懒加载: 首次 Get 触发一次读盘 |
| 并发编排在 Manager | 方案B: 协议层单一职责, Manager 控制并发策略 |
| 标准库 slog 替代 logrus | 减少外部依赖, Go 1.21+ 内置 |
| inflight dedup | sync.Map + chan: 无锁竞争, 5 个并发请求只发 1 次上游 |
| PWA | manifest+sw.js 几十行换"准原生"桌面形态; API 不走缓存 |
| mkipk 纯 Go 打 ipk | ~~构建机 (Windows) 无 binutils/OpenWrt SDK~~ 已废弃: 2026-07-18 起统一"二进制+脚本"分发 |
| 二进制+install脚本分发 | 删 NSIS/ipk/.app/.desktop: 四个平台同一形态, 服务注册本就在二进制内 (kardianos), 脚本只做复制+调用; PWA 已覆盖桌面图标场景 |
| Makefile 构建 | make linux/windows/macos/openwrt → build/<os>/ (二进制+config+rules+脚本); make run 按 uname 选宿主目标; 配置 cp -n 不覆盖保住 Web UI 修改 |
| DNS 默认 5553 上游定位 | 不抢 53; 由 dnsmasq/系统 DNS 转发, 用户手动接线 (dnsmasq: server=127.0.0.1#5553 + noresolv=1) |

---

## 六、数据流 (完整的一次 DNS 查询)

```
dig @127.0.0.1 google.com A

  Transport.UDPServer
    │  read 28 byte DNS query from UDP
    ▼
  query.Service.Handle(packet, addr)
    │
    ├── dns.ParseQuery(packet) → Query{ID:0x1234, Domain:"google.com", Type:A}
    │
    ├── custom.CustomGet("google.com") → miss (自定义覆盖优先级最高)
    │
    ├── inflight dedup check: "google.com|1" not in progress → we own it
    │
    ├── router.Route("google.com") → RouteResult{Decision:RouteProxy}
    │     → pick servers = Manager.Proxy()
    │
    ├── negCache.Get("google.com") → false
    │
    ├── cache.Get("google.com", A) → miss
    │
    ├── manager.ResolveAll(ctx, "google.com", A, proxyResolvers)
    │     │  goroutine 1: doh.Client("https://dns.google/...").Resolve()
    │     │  goroutine 2: doh.Client("https://1.1.1.1/...").Resolve()
    │     │
    │     └── aggregate all OK responses, dedup same RRs, min TTL
    │         → Result{Records: [A:142.251.x.x, ...], TTL:300}
    │
    ├── cache.Set("google.com", A, Entry{Records, TTL, ExpireAt})
    │
    ├── close(dedup channel)  ← wake any waiters
    │
    └── dns.BuildResponse(query, Result)
          → wire-format response bytes

  Transport.UDPServer
    │  write response bytes back to client
    ▼
  dig receives response
```

---

## 七、前端面板设计

### ConfigPanel
- 顶部工具栏: Load Config + Save Config (✓ Saved / error 提示同排)
- DNS Listen: Host + Port; DNS Mode 单选 (radio: direct/proxy/rules,
  bind:group=config.dns.mode, 旧配置缺 mode 时 load 后补 "rules")
- Direct/Proxy DoH Servers: 候选列表 (共享 doh-catalog.js CATALOG + 配置中
  目录外的自定义 URL); 勾选=写入 config (Save 后生效); 全部行可 × 删除 —
  自定义行移出 config, 目录行移出 config 并隐藏 (localStorage
  `config.hiddenCatalog.v1`, + Add 重新添加同 URL 可恢复); 每组底部输入框
  (+ Add / Enter) 添加自定义 DoH, 添加即勾选
- Bootstrap server
- Proxy toggle + HTTP/HTTPS URL
- Cache: DB path, hot size, save interval
- Web UI: host/port/username/password
- Logging level dropdown
- Save Config → SaveConfig() → 写入 config.toml (需重启生效)

### LatencyPanel
- 标题 "DNS Server Latency"; 手动刷新: onMount 自动测一次 + Refresh 按钮,
  无轮询
- 只显示当前激活 (已配置) 服务器, 探测走 GET /api/latency (CheckLatency →
  CheckServersLatency, 按 server|proxy 缓存 DoH 客户端复用 keep-alive
  热连接); 主动合成探测 (每服务器查 example.com A), 与真实查询无关;
  真实查询延迟看 StatsPanel 的 AvgLatencyMs
- 分组展示 Direct/Proxy 带 SVG 图标; 名称命中 doh-catalog.js 目录显示目录名,
  否则 hostname; 每条 server 一行: name | URL | 延迟柱状图 | ms | status;
  无激活服务器时显示提示
- 柱状图颜色 (按热连接校准): <100ms 绿, <300ms 橙, 其余红
  (direct 热 ~50-100ms, proxy 热 ~200-300ms)

### StatsPanel
- 每 5 秒轮询 GetQueryStats() + GetCacheStats()
- 4 卡片: Total Queries, Hit Rate %, Avg Latency, Errors
- Hits/Misses 比例柱状图
- 4 卡片: Cache Entries, Memory, Disk, Cache Hit Rate

### CachePanel
- onMount + Refresh 按钮 → ListCache() (cap 500)
- 搜索框本地过滤域名 (大小写不敏感子串)
- 表格: Domain | Type | Records | 剩余 TTL | 状态 (Active/Expired)

### CustomDNSPanel
- onMount → GetCustomDNS(); 表单 (domain + 逗号分隔 IPs) → SetCustomDNS
- 每行 domain + IP chips + 删除 → DeleteCustomDNS; 后端校验 IP 格式
- 覆盖语义: 仅 A/AAAA, TTL 固定 300, 管线最高优先级, 实时读库即改即生效
