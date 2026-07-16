package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"doh-dns-proxy/internal/cache"
	"doh-dns-proxy/internal/query"
	"doh-dns-proxy/internal/router"
	"doh-dns-proxy/internal/transport"
	"doh-dns-proxy/internal/upstream"
	"doh-dns-proxy/internal/upstream/doh"
	"doh-dns-proxy/internal/upstream/udp"

	"github.com/BurntSushi/toml"
)

// Config mirrors the TOML structure.
type Config struct {
	DOHServers DOHServers `toml:"doh_servers"`
	DNS        DNSConfig  `toml:"dns"`
	Cache      CacheConfig `toml:"cache"`
	Proxy      ProxyConfig `toml:"proxy"`
	Logging    LogConfig   `toml:"logging"`
}

type DOHServers struct {
	DirectServers   []string `toml:"direct_servers"`
	ProxyServers    []string `toml:"proxy_servers"`
	BootstrapServer string   `toml:"bootstrap_server"`
}

type DNSConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type CacheConfig struct {
	DBPath       string `toml:"db_path"`
	MaxHotSize   int    `toml:"max_hot_size"`
	SaveInterval int    `toml:"save_interval"`
}

type ProxyConfig struct {
	EnableProxy bool   `toml:"enable_proxy"`
	HTTP        string `toml:"http"`
	HTTPS       string `toml:"https"`
	RuleFile    string `toml:"rule_file"`
	RuleFileURL string `toml:"rule_file_url"`
}

type LogConfig struct {
	Level string `toml:"level"`
}

func main() {
	// --- Load config ---
	cfg, err := loadConfig("config.toml")
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	setupLogging(cfg.Logging)

	// --- Cache layer ---
	c, err := cache.New(cfg.Cache.DBPath)
	if err != nil {
		slog.Error("failed to open cache", "err", err)
		os.Exit(1)
	}
	defer c.Close()

	negCache := cache.NewNegativeCache(300 * time.Second)

	// --- Router ---
	rtr, err := router.New(cfg.Proxy.RuleFile, cfg.Proxy.RuleFileURL)
	if err != nil {
		slog.Error("failed to create router", "err", err)
		os.Exit(1)
	}
	slog.Info("router loaded", "rules", rtr.Size())

	// --- Upstream ---
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

	// --- Query service ---
	customStore, _ := c.(cache.CustomStore)
	svc := query.New(c, negCache, rtr, mgr, customStore)

	// --- Transport ---
	srv, err := transport.NewUDPServer(cfg.DNS.Host, cfg.DNS.Port, svc.Handle)
	if err != nil {
		slog.Error("failed to start UDP server", "err", err)
		os.Exit(1)
	}
	defer srv.Close()

	slog.Info("dnsforvpn starting",
		"host", cfg.DNS.Host,
		"port", cfg.DNS.Port,
		"direct_servers", len(cfg.DOHServers.DirectServers),
		"proxy_servers", len(cfg.DOHServers.ProxyServers),
	)

	// --- Signal handling ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		slog.Info("shutting down...")
		cancel()
	}()

	// --- Run ---
	if err := srv.Start(ctx); err != nil && err != context.Canceled {
		slog.Error("server error", "err", err)
	}
	slog.Info("dnsforvpn stopped")
}

func loadConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setupLogging(logCfg LogConfig) {
	level := slog.LevelInfo
	switch logCfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))
}
