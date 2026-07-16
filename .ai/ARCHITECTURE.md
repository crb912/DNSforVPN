# DNSforVPN — Architecture Reference

> 完整架构文档，供大模型下次会话加载。读取此文件即可获得全部上下文。

## 项目结构

```
dnsforvpn/                          # module: doh-dns-proxy, go 1.22
├── cmd/dnsforvpn/main.go           # CLI 入口（无 GUI 模式）
├── main.go                         # Wails 入口 (wails.Run)
├── app.go                          # Wails App struct, Bind 方法
├── wails.json                      # Wails v2 项目配置
├── config.toml                     # 运行时配置
├── TODO.md                         # 待办清单
├── ARCHITECTURE.md                 # ← 本文件
│
├── internal/
│   ├── dns/                        # DNS wire-format (纯函数, 零状态)
│   │   ├── types.go                #   常量 + Query, Result, RRRecord
│   │   ├── parse.go                #   ParseQuery, ParseResponse (+ 内部解析器)
│   │   ├── build.go                #   BuildResponse, BuildErrorResponse, BuildQuery
│   │   └── dns_test.go             #   16 tests
│   │
│   ├── router/                     # 域名路由决策 (横向层)
│   │   ├── router.go               #   Router struct + GFWList 解析
│   │   └── router_test.go          #   16 tests
│   │
│   ├── cache/                      # 缓存层
│   │   ├── cache.go                #   Cache 接口 + Entry, Stats, CacheItem
│   │   ├── boltdb.go               #   boltCache (BoltDB + sync.Map hot cache), 实现 CustomStore
│   │   ├── custom.go               #   CustomEntry + CustomStore 接口 (自定义 DNS 覆盖)
│   │   ├── negative.go             #   NegativeCache (NXDOMAIN)
│   │   └── cache_test.go           #   20 tests
│   │
│   ├── upstream/                   # 上游协议层
│   │   ├── resolver.go             #   Resolver 接口 + Result, StatsSnapshot, ServerLatency
│   │   ├── manager.go              #   Manager: 并发多服务器编排 (方案B)
│   │   ├── doh/
│   │   │   ├── client.go           #   DoH Client (single-server POST)
│   │   │   └── tcp_ping.go         #   TCP ping 延迟探测
│   │   └── udp/
│   │       └── client.go           #   UDP Client (bootstrap)
│   │
│   ├── query/                      # 查询管线编排
│   │   └── service.go              #   Service + inflight dedup
│   │
│   └── transport/                  # 网络传输层
│       └── udp.go                  #   UDPServer
│
└── frontend/                       # Wails 前端 (Svelte 4 + Vite 5)
    ├── package.json
    ├── vite.config.js
    ├── index.html
    └── src/
        ├── main.js
        ├── App.svelte              # 主框架 (Tab: Status/Config/Stats/Cache/Custom DNS)
        └── lib/
            ├── ConfigPanel.svelte  # 服务器/缓存/代理配置
            ├── LatencyPanel.svelte # 延迟监控 (5s 轮询)
            ├── StatsPanel.svelte   # QPS/缓存仪表盘
            ├── CachePanel.svelte   # 缓存浏览 (搜索/刷新, 含过期状态)
            └── CustomDNSPanel.svelte # 自定义域名→IP 覆盖管理
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

type Service struct { ... }        // + custom cache.CustomStore (可选, nil 禁用)
func New(c cache.Cache, nc *cache.NegativeCache, r *router.Router, u *upstream.Manager, cs cache.CustomStore) *Service
func (s *Service) Handle(ctx, packet, clientAddr) ([]byte, error)  // 完整管线
func (s *Service) Stats() Stats
func (s *Service) CacheStats() cache.Stats
func (s *Service) Shutdown() error
```

**Handle 管线流程**:
```
1.  dns.ParseQuery(packet)
1b. custom.CustomGet(domain) → 用户自定义覆盖命中直接返回 (A/AAAA, 最高优先级)
2.  router.Route(q.Domain) → 选择 direct/proxy []Resolver
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

## 二、Wails App 绑定方法

```go
// app.go
type App struct {
    ctx     context.Context
    mu      sync.Mutex
    config  Config
    svc     *query.Service
    srv     *transport.UDPServer
    cancel  context.CancelFunc
    running bool
}

// 以下方法被 Wails Bind 到前端 (window.go.main.App.*)
func (a *App) GetConfig() Config
func (a *App) SaveConfig(cfg Config) error          // 写入 config.toml
func (a *App) GetStatus() string                     // "running" | "stopped"
func (a *App) Start() error                          // 装配全部依赖 + 启动 DNS 服务器
func (a *App) Stop()                                 // 优雅停止
func (a *App) CheckLatency() []upstream.ServerLatency // 所有 DoH 服务器延迟
func (a *App) GetCacheStats() cache.Stats
func (a *App) GetQueryStats() query.Stats

// 缓存浏览 (running 用活句柄, stopped 临时打开 DB)
func (a *App) ListCache() ([]CacheEntryView, error)         // 全量, cap 500, 按域名排序
func (a *App) QueryCache(domain string) ([]CacheEntryView, error)

// 自定义 DNS 覆盖
func (a *App) GetCustomDNS() ([]CustomDNSEntry, error)
func (a *App) SetCustomDNS(domain string, ips []string) error  // 校验 IP 格式
func (a *App) DeleteCustomDNS(domain string) error
```

---

## 三、config.toml 格式

```toml
[doh_servers]
direct_servers = ["https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"]
proxy_servers = ["https://dns.google/dns-query", "https://1.1.1.1/dns-query"]
bootstrap_server = "223.5.5.5"

[dns]
host = "0.0.0.0"
port = 1553

[cache]
db_path = "data/dns_cache.db"
max_hot_size = 100000
save_interval = 72

[proxy]
enable_proxy = true
http = "http://192.168.5.8:7899"
https = "http://192.168.5.8:7899"
rule_file = "gfwlist.txt"
rule_file_url = "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt"

[logging]
level = "info"
```

---

## 四、Go 依赖图

```
go.mod:
  github.com/BurntSushi/toml v1.3.2     # 配置解析
  github.com/wailsapp/wails/v2 v2.9.2   # GUI 框架
  go.etcd.io/bbolt v1.3.11              # 缓存存储
```

**包依赖图**:
```
cmd/dnsforvpn/main.go
  └── Config → transport.UDPServer
                 └── query.Service.Handle()
                       ├── dns.ParseQuery / BuildResponse / BuildErrorResponse
                       ├── router.Route() → RouteResult
                       ├── cache.Get / Set / NegativeCache
                       └── upstream.Manager.ResolveAll()
                             ├── upstream/doh.Client.Resolve()
                             └── upstream/udp.Client.Resolve()  (bootstrap)

main.go → wails.Run()
  └── app.go → App.Start() → 同上装配链
```

---

## 五、关键设计决策记录

| 决策 | 理由 |
|------|------|
| BoltDB 而非 LMDB | 纯 Go 无 CGo, Wails 交叉编译友好 |
| gob 编码而非 JSON/protobuf | 比 JSON 快, 无 protobuf 依赖 |
| sync.Map 热缓存 | 读多写少, 懒加载: 首次 Get 触发一次读盘 |
| 并发编排在 Manager | 方案B: 协议层单一职责, Manager 控制并发策略 |
| Wails v2 而非 v3 | v3 alpha, v2 稳定生产就绪 |
| 标准库 slog 替代 logrus | 减少外部依赖, Go 1.21+ 内置 |
| inflight dedup | sync.Map + chan: 无锁竞争, 5 个并发请求只发 1 次上游 |

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
- Load Config 按钮 → GetConfig()
- DNS Listen: Host + Port
- Direct DoH Servers: 可编辑列表 (+/-)
- Proxy DoH Servers: 可编辑列表 (+/-)
- Bootstrap server
- Proxy toggle + HTTP/HTTPS URL
- Cache: DB path, hot size, save interval
- Logging level dropdown
- Save Config → SaveConfig() → 写入 config.toml

### LatencyPanel
- 每 5 秒轮询 CheckLatency()
- 每条 server 一行: URL | 延迟柱状图 | ms | status
- 柱状图颜色: <20ms 绿色, <100ms 橙色, >100ms 红色

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
