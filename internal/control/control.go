// Package control provides the platform-neutral control layer for dnsforvpn.
// It owns the runtime configuration and the DNS server lifecycle, and is
// consumed by the web API (internal/web) and the CLI (cmd/dnsforvpn).
package control

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"doh-dns-proxy/internal/cache"
	"doh-dns-proxy/internal/dns"
	"doh-dns-proxy/internal/query"
	"doh-dns-proxy/internal/router"
	"doh-dns-proxy/internal/transport"
	"doh-dns-proxy/internal/upstream"
	"doh-dns-proxy/internal/upstream/doh"
	"doh-dns-proxy/internal/upstream/udp"

	"github.com/BurntSushi/toml"
)

// Config mirrors the TOML structure of config.toml.
type Config struct {
	DOHServers DOHServers  `toml:"doh_servers" json:"doh_servers"`
	DNS        DNSConfig   `toml:"dns" json:"dns"`
	Cache      CacheConfig `toml:"cache" json:"cache"`
	Proxy      ProxyConfig `toml:"proxy" json:"proxy"`
	Web        WebConfig   `toml:"web" json:"web"`
	Logging    LogConfig   `toml:"logging" json:"logging"`
}

type DOHServers struct {
	DirectServers   []string `toml:"direct_servers" json:"direct_servers"`
	ProxyServers    []string `toml:"proxy_servers" json:"proxy_servers"`
	BootstrapServer string   `toml:"bootstrap_server" json:"bootstrap_server"`
}

type DNSConfig struct {
	Host string `toml:"host" json:"host"`
	Port int    `toml:"port" json:"port"`
	// Mode selects the upstream pool for all queries: "direct" (direct
	// servers only), "proxy" (proxy servers only) or "rules" (GFWList
	// routing; default when empty).
	Mode string `toml:"mode" json:"mode"`
}

type CacheConfig struct {
	DBPath       string `toml:"db_path" json:"db_path"`
	MaxHotSize   int    `toml:"max_hot_size" json:"max_hot_size"`
	SaveInterval int    `toml:"save_interval" json:"save_interval"`
}

type ProxyConfig struct {
	EnableProxy bool   `toml:"enable_proxy" json:"enable_proxy"`
	HTTP        string `toml:"http" json:"http"`
	HTTPS       string `toml:"https" json:"https"`
	RuleFile    string `toml:"rule_file" json:"rule_file"`
	RuleFileURL string `toml:"rule_file_url" json:"rule_file_url"`
}

// WebConfig controls the built-in web UI / REST API listener.
// Username and Password enable HTTP Basic auth when Password is non-empty.
type WebConfig struct {
	Host     string `toml:"host" json:"host"`
	Port     int    `toml:"port" json:"port"`
	Username string `toml:"username" json:"username"`
	Password string `toml:"password" json:"password"`
}

type LogConfig struct {
	Level string `toml:"level" json:"level"`
}

// Control manages the DNS server lifecycle and exposes everything the
// web UI needs. It is safe for concurrent use.
type Control struct {
	cfgPath string // absolute path of the config file
	cfgDir  string // directory of cfgPath; base for relative paths in Config

	mu      sync.Mutex
	config  Config
	svc     *query.Service
	srv     *transport.UDPServer
	cache   cache.Cache       // live while running, nil when stopped
	custom  cache.CustomStore // live while running, nil when stopped
	cancel  context.CancelFunc
	running bool

	// healthClients 缓存延迟探测用的 DoH 客户端 (key: serverURL|proxyURL),
	// 复用其底层 keep-alive 连接, 使轮询测到的是热连接延迟而非冷启动全程。
	healthClients map[string]*doh.Client
}

// New loads the configuration from cfgPath and returns a Control.
func New(cfgPath string) (*Control, error) {
	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if _, err := toml.DecodeFile(abs, &cfg); err != nil {
		return nil, err
	}
	if cfg.Web.Host == "" {
		cfg.Web.Host = "127.0.0.1"
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = 8080
	}
	return &Control{
		cfgPath:       abs,
		cfgDir:        filepath.Dir(abs),
		config:        cfg,
		healthClients: make(map[string]*doh.Client),
	}, nil
}

// resolve makes relative paths in cfg absolute against the config file's
// directory, so running as a system service (arbitrary working directory)
// behaves the same as running from the config directory.
func (c *Control) resolve(cfg Config) Config {
	if !filepath.IsAbs(cfg.Cache.DBPath) {
		cfg.Cache.DBPath = filepath.Join(c.cfgDir, cfg.Cache.DBPath)
	}
	if cfg.Proxy.RuleFile != "" && !filepath.IsAbs(cfg.Proxy.RuleFile) {
		cfg.Proxy.RuleFile = filepath.Join(c.cfgDir, cfg.Proxy.RuleFile)
	}
	return cfg
}

// GetConfig returns the current configuration as stored in the config file.
func (c *Control) GetConfig() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.config
}

// SaveConfig persists configuration to the config file.
func (c *Control) SaveConfig(cfg Config) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	if err := os.WriteFile(c.cfgPath, buf.Bytes(), 0644); err != nil {
		return err
	}
	c.mu.Lock()
	c.config = cfg
	c.mu.Unlock()
	slog.Info("config saved", "servers", len(cfg.DOHServers.DirectServers))
	return nil
}

// GetStatus returns the server status: "running" or "stopped".
func (c *Control) GetStatus() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return "running"
	}
	return "stopped"
}

// Start creates and starts the DNS server with the current configuration.
// Calling Start on an already running server is a no-op.
func (c *Control) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	cfg := c.resolve(c.config)

	// Cache.
	if dir := filepath.Dir(cfg.Cache.DBPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	cc, err := cache.New(cfg.Cache.DBPath)
	if err != nil {
		return err
	}
	negCache := cache.NewNegativeCache(300 * time.Second)

	// Router.
	rtr, err := router.New(cfg.Proxy.RuleFile, cfg.Proxy.RuleFileURL)
	if err != nil {
		cc.Close()
		return err
	}

	// Upstream resolvers.
	proxyURL := ""
	if cfg.Proxy.EnableProxy {
		proxyURL = cfg.Proxy.HTTPS
		if proxyURL == "" {
			proxyURL = cfg.Proxy.HTTP
		}
	}

	directResolvers := make([]upstream.Resolver, 0, len(cfg.DOHServers.DirectServers))
	for _, s := range cfg.DOHServers.DirectServers {
		directResolvers = append(directResolvers, doh.New(s, ""))
	}
	proxyResolvers := make([]upstream.Resolver, 0, len(cfg.DOHServers.ProxyServers))
	for _, s := range cfg.DOHServers.ProxyServers {
		proxyResolvers = append(proxyResolvers, doh.New(s, proxyURL))
	}
	var bootResolver upstream.Resolver
	if cfg.DOHServers.BootstrapServer != "" {
		bootResolver = udp.New(cfg.DOHServers.BootstrapServer)
	}
	mgr := upstream.NewManager(directResolvers, proxyResolvers, bootResolver)

	// Query service.
	customStore, _ := cc.(cache.CustomStore)
	svc := query.New(cc, negCache, rtr, mgr, customStore, cfg.DNS.Mode)

	// Transport.
	srv, err := transport.NewUDPServer(cfg.DNS.Host, cfg.DNS.Port, svc.Handle)
	if err != nil {
		cc.Close()
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.svc = svc
	c.srv = srv
	c.cache = cc
	c.custom = customStore
	c.cancel = cancel
	c.running = true

	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			slog.Error("server error", "err", err)
		}
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	slog.Info("DNS server started",
		"host", cfg.DNS.Host,
		"port", cfg.DNS.Port,
		"mode", cfg.DNS.Mode,
		"direct", len(cfg.DOHServers.DirectServers),
		"proxy", len(cfg.DOHServers.ProxyServers),
	)
	return nil
}

// Stop gracefully shuts down the DNS server.
func (c *Control) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	c.cancel()
	if c.srv != nil {
		c.srv.Close()
	}
	if c.svc != nil {
		c.svc.Shutdown()
	}
	c.svc = nil
	c.srv = nil
	c.cache = nil
	c.custom = nil
	c.running = false
	slog.Info("DNS server stopped")
}

// LatencyTarget names one DoH server to probe. ViaProxy routes the probe
// through the configured proxy (for the "Proxy" group in the UI).
type LatencyTarget struct {
	URL      string `json:"url"`
	ViaProxy bool   `json:"via_proxy"`
}

// CheckLatency performs health checks on all configured DoH servers.
func (c *Control) CheckLatency() []upstream.ServerLatency {
	cfg := c.GetConfig()
	targets := make([]LatencyTarget, 0, len(cfg.DOHServers.DirectServers)+len(cfg.DOHServers.ProxyServers))
	for _, s := range cfg.DOHServers.DirectServers {
		targets = append(targets, LatencyTarget{URL: s})
	}
	for _, s := range cfg.DOHServers.ProxyServers {
		targets = append(targets, LatencyTarget{URL: s, ViaProxy: true})
	}
	return c.CheckServersLatency(targets)
}

// CheckServersLatency probes the given targets concurrently. DoH clients
// are cached across calls (keyed by serverURL|proxyURL) so the underlying
// keep-alive HTTP connections are reused — after the first (cold) probe,
// results reflect warm-connection latency. Targets marked ViaProxy are
// probed through the configured proxy; when no proxy is configured they
// immediately report an error instead of silently going direct.
func (c *Control) CheckServersLatency(targets []LatencyTarget) []upstream.ServerLatency {
	c.mu.Lock()
	cfg := c.config

	proxyURL := ""
	if cfg.Proxy.EnableProxy {
		proxyURL = cfg.Proxy.HTTPS
		if proxyURL == "" {
			proxyURL = cfg.Proxy.HTTP
		}
	}

	// Prune clients no longer referenced by config or this request.
	want := make(map[string]bool)
	for _, s := range cfg.DOHServers.DirectServers {
		want[s+"|"] = true
	}
	for _, s := range cfg.DOHServers.ProxyServers {
		want[s+"|"+proxyURL] = true
	}
	for _, t := range targets {
		want[t.URL+"|"+proxyFor(t, proxyURL)] = true
	}
	for k := range c.healthClients {
		if !want[k] {
			delete(c.healthClients, k)
		}
	}

	type probe struct {
		cl  *doh.Client
		url string // set when cl is nil (proxy requested but not configured)
	}
	probes := make([]probe, 0, len(targets))
	seen := make(map[string]bool)
	for _, t := range targets {
		if t.URL == "" {
			continue
		}
		if t.ViaProxy && proxyURL == "" {
			probes = append(probes, probe{url: t.URL})
			continue
		}
		p := proxyFor(t, proxyURL)
		k := t.URL + "|" + p
		if seen[k] {
			continue
		}
		seen[k] = true
		cl, ok := c.healthClients[k]
		if !ok {
			cl = doh.New(t.URL, p)
			c.healthClients[k] = cl
		}
		probes = append(probes, probe{cl: cl})
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	all := make([]upstream.ServerLatency, len(probes))
	var wg sync.WaitGroup
	for i, pr := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if pr.cl == nil {
				all[i] = upstream.ServerLatency{ServerURL: pr.url, Status: "error"}
				return
			}
			all[i] = pr.cl.HealthCheck(ctx)
		}()
	}
	wg.Wait()
	return all
}

func proxyFor(t LatencyTarget, proxyURL string) string {
	if t.ViaProxy {
		return proxyURL
	}
	return ""
}

// GetCacheStats returns cache-layer statistics.
func (c *Control) GetCacheStats() cache.Stats {
	c.mu.Lock()
	svc := c.svc
	c.mu.Unlock()

	if svc == nil {
		return cache.Stats{}
	}
	return svc.CacheStats()
}

// GetQueryStats returns query-layer statistics.
func (c *Control) GetQueryStats() query.Stats {
	c.mu.Lock()
	svc := c.svc
	c.mu.Unlock()

	if svc == nil {
		return query.Stats{}
	}
	return svc.Stats()
}

// --- Cache browser ---

// CacheEntryView is the JSON view of one cached DNS entry for the UI.
type CacheEntryView struct {
	Domain    string   `json:"domain"`
	QType     string   `json:"qtype"`
	Records   []string `json:"records"`
	TTLRemain uint32   `json:"ttl_remaining"`
	Expired   bool     `json:"expired"`
}

const maxCacheListItems = 500

// ListCache returns all cached DNS entries (including expired ones),
// sorted by domain and capped at maxCacheListItems.
func (c *Control) ListCache() ([]CacheEntryView, error) {
	return c.queryCache("")
}

// QueryCache returns cached entries for one domain across all qtypes.
// An empty domain behaves like ListCache.
func (c *Control) QueryCache(domain string) ([]CacheEntryView, error) {
	return c.queryCache(strings.ToLower(strings.TrimSpace(domain)))
}

func (c *Control) queryCache(filter string) ([]CacheEntryView, error) {
	out := make([]CacheEntryView, 0)
	err := c.withStore(func(cc cache.Cache, _ cache.CustomStore) error {
		for _, it := range cc.List() {
			if filter != "" && it.Domain != filter {
				continue
			}
			out = append(out, toCacheEntryView(it))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].QType < out[j].QType
	})
	if len(out) > maxCacheListItems {
		out = out[:maxCacheListItems]
	}
	return out, nil
}

func toCacheEntryView(it cache.CacheItem) CacheEntryView {
	recs := make([]string, 0, len(it.Entry.Records))
	for _, r := range it.Entry.Records {
		recs = append(recs, r.RData)
	}
	var ttl uint32
	if !it.Entry.Expired() {
		ttl = dns.TTLRemaining(it.Entry.ExpireAt)
	}
	return CacheEntryView{
		Domain:    it.Domain,
		QType:     qtypeName(it.QType),
		Records:   recs,
		TTLRemain: ttl,
		Expired:   it.Entry.Expired(),
	}
}

func qtypeName(q uint16) string {
	switch q {
	case dns.QTypeA:
		return "A"
	case dns.QTypeAAAA:
		return "AAAA"
	case dns.QTypeCNAME:
		return "CNAME"
	case dns.QTypeSOA:
		return "SOA"
	default:
		return strconv.Itoa(int(q))
	}
}

// --- Custom DNS overrides ---

// CustomDNSEntry is the JSON view of a user-defined DNS override.
type CustomDNSEntry struct {
	Domain string   `json:"domain"`
	IPs    []string `json:"ips"`
}

// GetCustomDNS returns all user-defined DNS overrides, sorted by domain.
func (c *Control) GetCustomDNS() ([]CustomDNSEntry, error) {
	out := make([]CustomDNSEntry, 0)
	err := c.withStore(func(_ cache.Cache, cs cache.CustomStore) error {
		if cs == nil {
			return nil
		}
		for _, e := range cs.CustomList() {
			out = append(out, CustomDNSEntry{Domain: e.Domain, IPs: e.IPs})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetCustomDNS creates or replaces the override for a domain. Every IP is
// validated; the whole call fails on the first invalid one.
func (c *Control) SetCustomDNS(domain string, ips []string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return fmt.Errorf("domain must not be empty")
	}
	clean := make([]string, 0, len(ips))
	for _, s := range ips {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if net.ParseIP(s) == nil {
			return fmt.Errorf("invalid IP address: %s", s)
		}
		clean = append(clean, s)
	}
	if len(clean) == 0 {
		return fmt.Errorf("at least one valid IP is required")
	}
	return c.withStore(func(_ cache.Cache, cs cache.CustomStore) error {
		if cs == nil {
			return fmt.Errorf("custom DNS store unavailable")
		}
		cs.CustomSet(domain, clean)
		slog.Info("custom DNS set", "domain", domain, "ips", clean)
		return nil
	})
}

// DeleteCustomDNS removes the override for a domain.
func (c *Control) DeleteCustomDNS(domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return fmt.Errorf("domain must not be empty")
	}
	return c.withStore(func(_ cache.Cache, cs cache.CustomStore) error {
		if cs == nil {
			return nil
		}
		cs.CustomDel(domain)
		slog.Info("custom DNS deleted", "domain", domain)
		return nil
	})
}

// withStore runs fn against the live cache handles while the server is
// running, or against a short-lived handle opened just for the call when
// it is stopped (BoltDB allows only one open handle per file).
func (c *Control) withStore(fn func(cc cache.Cache, cs cache.CustomStore) error) error {
	c.mu.Lock()
	cc, cs := c.cache, c.custom
	dbPath := c.resolve(c.config).Cache.DBPath
	c.mu.Unlock()

	if cc != nil {
		return fn(cc, cs)
	}

	tmp, err := cache.New(dbPath)
	if err != nil {
		return fmt.Errorf("open cache db: %w", err)
	}
	defer tmp.Close()
	tmpCS, _ := tmp.(cache.CustomStore)
	return fn(tmp, tmpCS)
}
