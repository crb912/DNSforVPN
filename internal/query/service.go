// Package query implements the DNS query orchestration pipeline. The Service
// coordinates cache, routing, and upstream resolution for each incoming DNS
// request.
package query

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"doh-dns-proxy/internal/cache"
	"doh-dns-proxy/internal/dns"
	"doh-dns-proxy/internal/router"
	"doh-dns-proxy/internal/upstream"
)

// Stats captures query-layer performance counters exposed to the UI.
type Stats struct {
	TotalQueries uint64  `json:"total_queries"`
	CacheHits    uint64  `json:"cache_hits"`
	CacheMisses  uint64  `json:"cache_misses"`
	TotalErrors  uint64  `json:"total_errors"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// Service orchestrates the full DNS query pipeline.
type Service struct {
	cache    cache.Cache
	negCache *cache.NegativeCache
	router   *router.Router
	upstream *upstream.Manager
	custom   cache.CustomStore // optional user-defined DNS overrides (may be nil)

	// In-flight deduplication: domain|qtype → chan closed when upstream
	// resolve completes. Subsequent queries for the same key wait on
	// this channel instead of spawning duplicate upstream requests.
	inflight sync.Map // map[string] chan struct{}

	// Metrics.
	totalQueries atomic.Uint64
	cacheHits    atomic.Uint64
	cacheMisses  atomic.Uint64
	totalErrors  atomic.Uint64
	latencySum   atomic.Int64 // sum of microseconds
	latencyCount atomic.Int64
}

// New creates a Service with all required dependencies. The custom store
// is optional — pass nil to disable user-defined DNS overrides.
func New(
	c cache.Cache,
	nc *cache.NegativeCache,
	r *router.Router,
	u *upstream.Manager,
	cs cache.CustomStore,
) *Service {
	return &Service{
		cache:    c,
		negCache: nc,
		router:   r,
		upstream: u,
		custom:   cs,
	}
}

// Handle processes a single incoming DNS query packet from a client.
// It returns the wire-format reply bytes for the transport layer to send.
func (s *Service) Handle(ctx context.Context, packet []byte, clientAddr net.Addr) ([]byte, error) {
	s.totalQueries.Add(1)
	start := time.Now()
	defer func() {
		s.latencySum.Add(time.Since(start).Microseconds())
		s.latencyCount.Add(1)
	}()

	// 1. Parse the DNS query.
	q, err := dns.ParseQuery(packet)
	if err != nil {
		s.totalErrors.Add(1)
		slog.Warn("query: failed to parse", "addr", clientAddr.String(), "err", err)
		return nil, err
	}

	// 1b. User-defined DNS overrides take precedence over everything.
	if s.custom != nil && (q.Type == dns.QTypeA || q.Type == dns.QTypeAAAA) {
		if ce, ok := s.custom.CustomGet(strings.ToLower(q.Domain)); ok {
			if recs := ce.Records(q.Type); len(recs) > 0 {
				s.cacheHits.Add(1)
				return dns.BuildResponse(q, dns.Result{Records: recs, TTL: dns.MinimumTTL}), nil
			}
		}
	}

	// 2. Route: pick which resolver pool to use.
	decision := s.router.Route(q.Domain)
	var resolvers []upstream.Resolver
	switch decision.Decision {
	case router.RouteProxy:
		resolvers = s.upstream.Proxy()
	default:
		resolvers = s.upstream.Direct()
	}

	// 2b. In-flight deduplication: if another goroutine is already
	// resolving this exact (domain, qtype) pair, wait for it to finish
	// and then retry the cache instead of launching a duplicate upstream
	// request.
	dedupKey := q.Domain + "|" + strconv.FormatUint(uint64(q.Type), 10)
	ch := make(chan struct{})
	if prev, loaded := s.inflight.LoadOrStore(dedupKey, ch); loaded {
		// Another request is already in-flight — wait for it.
		<-prev.(chan struct{})

		// Re-check cache (first request should have populated it).
		if entry, ok := s.cache.Get(q.Domain, q.Type); ok {
			s.cacheHits.Add(1)
			result := dns.Result{
				Records: entry.Records,
				TTL:     dns.TTLRemaining(entry.ExpireAt),
			}
			return dns.BuildResponse(q, result), nil
		}
		// Fall through: if cache still empty, resolve normally.
	} else {
		// We own the channel — clean up on exit.
		defer func() {
			close(ch)
			s.inflight.Delete(dedupKey)
		}()
	}

	// 3. Check negative cache (NXDOMAIN).
	if s.negCache.Get(q.Domain) {
		s.cacheHits.Add(1)
		return dns.BuildErrorResponse(q, dns.RcodeNXDomain), nil
	}

	// 4. Check main cache.
	if entry, ok := s.cache.Get(q.Domain, q.Type); ok {
		if !entry.Expired() {
			// Hot hit.
			s.cacheHits.Add(1)
			result := dns.Result{
				Records: entry.Records,
				TTL:     dns.TTLRemaining(entry.ExpireAt),
			}
			return dns.BuildResponse(q, result), nil
		}

		// Stale cache: return old value immediately, refresh in background.
		s.cacheHits.Add(1)
		go s.refreshInBackground(q, resolvers)

		result := dns.Result{
			Records: entry.Records,
			TTL:     dns.MinimumTTL,
		}
		return dns.BuildResponse(q, result), nil
	}

	// 5. Cache miss — resolve via upstream.
	s.cacheMisses.Add(1)

	// Bootstrap: if the requested domain is a DoH server hostname, resolve
	// via UDP first.
	if s.upstream.Bootstrap() != nil && s.isBootstrapHost(q.Domain) {
		return s.bootstrapResolve(ctx, q)
	}

	result, err := s.upstream.ResolveAll(ctx, q.Domain, q.Type, resolvers)
	if err != nil {
		s.totalErrors.Add(1)
		slog.Warn("query: upstream failed", "domain", q.Domain, "err", err)
		return dns.BuildErrorResponse(q, dns.RcodeServFail), nil
	}

	// 6. Write to cache and negative cache.
	if result.NXDomain {
		s.negCache.Set(q.Domain)
		return dns.BuildErrorResponse(q, dns.RcodeNXDomain), nil
	}

	ttl := result.TTL
	if ttl < dns.MinimumTTL {
		ttl = dns.MinimumTTL
	}
	s.cache.Set(q.Domain, q.Type, cache.Entry{
		Records:  result.Records,
		TTL:      ttl,
		ExpireAt: time.Now().Unix() + int64(ttl),
	})

	return dns.BuildResponse(q, dns.Result{
		Records: result.Records,
		TTL:     ttl,
	}), nil
}

// Stats returns aggregated query-layer metrics.
func (s *Service) Stats() Stats {
	sum := s.latencySum.Load()
	count := s.latencyCount.Load()
	avgMs := float64(0)
	if count > 0 {
		avgMs = float64(sum) / float64(count) / 1000.0
	}

	return Stats{
		TotalQueries: s.totalQueries.Load(),
		CacheHits:    s.cacheHits.Load(),
		CacheMisses:  s.cacheMisses.Load(),
		TotalErrors:  s.totalErrors.Load(),
		AvgLatencyMs: avgMs,
	}
}

// CacheStats returns the underlying cache layer statistics.
func (s *Service) CacheStats() cache.Stats {
	return s.cache.Stats()
}

// Shutdown gracefully closes resources.
func (s *Service) Shutdown() error {
	return s.cache.Close()
}

// --- private helpers ---

func (s *Service) refreshInBackground(q *dns.Query, resolvers []upstream.Resolver) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := s.upstream.ResolveAll(ctx, q.Domain, q.Type, resolvers)
	if err != nil {
		return
	}

	ttl := result.TTL
	if ttl < dns.MinimumTTL {
		ttl = dns.MinimumTTL
	}
	s.cache.Set(q.Domain, q.Type, cache.Entry{
		Records:  result.Records,
		TTL:      ttl,
		ExpireAt: time.Now().Unix() + int64(ttl),
	})
}

func (s *Service) bootstrapResolve(ctx context.Context, q *dns.Query) ([]byte, error) {
	boot := s.upstream.Bootstrap()
	if boot == nil {
		return dns.BuildErrorResponse(q, dns.RcodeServFail), nil
	}

	result, err := boot.Resolve(ctx, q.Domain, q.Type)
	if err != nil {
		s.totalErrors.Add(1)
		slog.Warn("query: bootstrap failed", "domain", q.Domain, "err", err)
		return dns.BuildErrorResponse(q, dns.RcodeServFail), nil
	}

	ttl := result.TTL
	if ttl < dns.MinimumTTL {
		ttl = dns.MinimumTTL
	}
	s.cache.Set(q.Domain, q.Type, cache.Entry{
		Records:  result.Records,
		TTL:      ttl,
		ExpireAt: time.Now().Unix() + int64(ttl),
	})

	return dns.BuildResponse(q, dns.Result{
		Records: result.Records,
		TTL:     ttl,
	}), nil
}

// isBootstrapHost checks if this domain is one of the configured DoH server
// hostnames that needs to be resolved via plain UDP before DoH can work.
func (s *Service) isBootstrapHost(domain string) bool {
	for _, r := range s.upstream.Direct() {
		if srv := r.Stats().Server; srv != "" {
			if hostFromURL(srv) == domain {
				return true
			}
		}
	}
	for _, r := range s.upstream.Proxy() {
		if srv := r.Stats().Server; srv != "" {
			if hostFromURL(srv) == domain {
				return true
			}
		}
	}
	return false
}

// hostFromURL extracts the hostname from a URL string like
// "https://dns.google/dns-query".
func hostFromURL(raw string) string {
	// Fast path: strip scheme and path.
	s := raw
	if len(s) > 8 && s[:8] == "https://" {
		s = s[8:]
	} else if len(s) > 7 && s[:7] == "http://" {
		s = s[7:]
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '/' || s[i] == ':' {
			return s[:i]
		}
	}
	return s
}
