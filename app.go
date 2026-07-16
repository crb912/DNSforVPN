package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
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

// App is the Wails-bound application struct. All exported methods are
// callable from the Svelte frontend via the Wails JS bridge.
type App struct {
	ctx context.Context

	mu      sync.Mutex
	config  Config
	svc     *query.Service
	srv     *transport.UDPServer
	cache   cache.Cache       // live while running, nil when stopped
	custom  cache.CustomStore // live while running, nil when stopped
	cancel  context.CancelFunc
	running bool
}

// --- Config types (shared with cmd/dnsforvpn) ---

// Config mirrors the TOML structure of config.toml.
type Config struct {
	DOHServers DOHServers  `toml:"doh_servers" json:"doh_servers"`
	DNS        DNSConfig   `toml:"dns" json:"dns"`
	Cache      CacheConfig `toml:"cache" json:"cache"`
	Proxy      ProxyConfig `toml:"proxy" json:"proxy"`
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

type LogConfig struct {
	Level string `toml:"level" json:"level"`
}

// --- Bind methods (called from Svelte frontend) ---

// GetConfig returns the current configuration.
func (a *App) GetConfig() Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.config
}

// SaveConfig persists configuration to config.toml.
func (a *App) SaveConfig(cfg Config) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	if err := os.WriteFile("config.toml", buf.Bytes(), 0644); err != nil {
		return err
	}
	a.mu.Lock()
	a.config = cfg
	a.mu.Unlock()
	slog.Info("config saved", "servers", len(cfg.DOHServers.DirectServers))
	return nil
}

// GetStatus returns the server status: "running" or "stopped".
func (a *App) GetStatus() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return "running"
	}
	return "stopped"
}

// Start creates and starts the DNS server with the current configuration.
func (a *App) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return nil
	}

	cfg := a.config

	// Cache.
	c, err := cache.New(cfg.Cache.DBPath)
	if err != nil {
		return err
	}
	negCache := cache.NewNegativeCache(300 * time.Second)

	// Router.
	rtr, err := router.New(cfg.Proxy.RuleFile, cfg.Proxy.RuleFileURL)
	if err != nil {
		c.Close()
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
	customStore, _ := c.(cache.CustomStore)
	svc := query.New(c, negCache, rtr, mgr, customStore)

	// Transport.
	srv, err := transport.NewUDPServer(cfg.DNS.Host, cfg.DNS.Port, svc.Handle)
	if err != nil {
		c.Close()
		return err
	}

	ctx, cancel := context.WithCancel(a.ctx)
	a.svc = svc
	a.srv = srv
	a.cache = c
	a.custom = customStore
	a.cancel = cancel
	a.running = true

	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			slog.Error("server error", "err", err)
		}
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()

	slog.Info("DNS server started",
		"host", cfg.DNS.Host,
		"port", cfg.DNS.Port,
		"direct", len(cfg.DOHServers.DirectServers),
		"proxy", len(cfg.DOHServers.ProxyServers),
	)
	return nil
}

// Stop gracefully shuts down the DNS server.
func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return
	}

	a.cancel()
	if a.srv != nil {
		a.srv.Close()
	}
	if a.svc != nil {
		a.svc.Shutdown()
	}
	a.svc = nil
	a.srv = nil
	a.cache = nil
	a.custom = nil
	a.running = false
	slog.Info("DNS server stopped")
}

// CheckLatency performs health checks on all configured DoH servers and
// returns latency information for the UI panel.
func (a *App) CheckLatency() []upstream.ServerLatency {
	a.mu.Lock()
	cfg := a.config
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	all := make([]upstream.ServerLatency, 0, len(cfg.DOHServers.DirectServers)+len(cfg.DOHServers.ProxyServers))

	for _, s := range cfg.DOHServers.DirectServers {
		client := doh.New(s, "")
		all = append(all, client.HealthCheck(ctx))
	}

	proxyURL := ""
	if cfg.Proxy.EnableProxy {
		proxyURL = cfg.Proxy.HTTPS
		if proxyURL == "" {
			proxyURL = cfg.Proxy.HTTP
		}
	}
	for _, s := range cfg.DOHServers.ProxyServers {
		client := doh.New(s, proxyURL)
		all = append(all, client.HealthCheck(ctx))
	}

	return all
}

// GetCacheStats returns cache-layer statistics.
func (a *App) GetCacheStats() cache.Stats {
	a.mu.Lock()
	svc := a.svc
	a.mu.Unlock()

	if svc == nil {
		return cache.Stats{}
	}
	return svc.CacheStats()
}

// GetQueryStats returns query-layer statistics.
func (a *App) GetQueryStats() query.Stats {
	a.mu.Lock()
	svc := a.svc
	a.mu.Unlock()

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
func (a *App) ListCache() ([]CacheEntryView, error) {
	return a.queryCache("")
}

// QueryCache returns cached entries for one domain across all qtypes.
// An empty domain behaves like ListCache.
func (a *App) QueryCache(domain string) ([]CacheEntryView, error) {
	return a.queryCache(strings.ToLower(strings.TrimSpace(domain)))
}

func (a *App) queryCache(filter string) ([]CacheEntryView, error) {
	out := make([]CacheEntryView, 0)
	err := a.withStore(func(c cache.Cache, _ cache.CustomStore) error {
		for _, it := range c.List() {
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
func (a *App) GetCustomDNS() ([]CustomDNSEntry, error) {
	out := make([]CustomDNSEntry, 0)
	err := a.withStore(func(_ cache.Cache, cs cache.CustomStore) error {
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
func (a *App) SetCustomDNS(domain string, ips []string) error {
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
	return a.withStore(func(_ cache.Cache, cs cache.CustomStore) error {
		if cs == nil {
			return fmt.Errorf("custom DNS store unavailable")
		}
		cs.CustomSet(domain, clean)
		slog.Info("custom DNS set", "domain", domain, "ips", clean)
		return nil
	})
}

// DeleteCustomDNS removes the override for a domain.
func (a *App) DeleteCustomDNS(domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return fmt.Errorf("domain must not be empty")
	}
	return a.withStore(func(_ cache.Cache, cs cache.CustomStore) error {
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
func (a *App) withStore(fn func(c cache.Cache, cs cache.CustomStore) error) error {
	a.mu.Lock()
	c, cs := a.cache, a.custom
	dbPath := a.config.Cache.DBPath
	a.mu.Unlock()

	if c != nil {
		return fn(c, cs)
	}

	tmp, err := cache.New(dbPath)
	if err != nil {
		return fmt.Errorf("open cache db: %w", err)
	}
	defer tmp.Close()
	tmpCS, _ := tmp.(cache.CustomStore)
	return fn(tmp, tmpCS)
}
