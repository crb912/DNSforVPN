package query

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"doh-dns-proxy/internal/cache"
	"doh-dns-proxy/internal/dns"
	"doh-dns-proxy/internal/router"
	"doh-dns-proxy/internal/upstream"
)

// --- fakes ---

type fakeCache struct{}

func (fakeCache) Get(string, uint16) (cache.Entry, bool) { return cache.Entry{}, false }
func (fakeCache) Set(string, uint16, cache.Entry)        {}
func (fakeCache) Del(string)                             {}
func (fakeCache) List() []cache.CacheItem                { return nil }
func (fakeCache) Stats() cache.Stats                     { return cache.Stats{} }
func (fakeCache) Close() error                           { return nil }

type fakeResolver struct {
	calls atomic.Int32
}

func (f *fakeResolver) Resolve(context.Context, string, uint16) (upstream.Result, error) {
	f.calls.Add(1)
	return upstream.Result{
		Records: []dns.RRRecord{{Name: "x", Type: dns.QTypeA, Class: 1, TTL: 300, RData: "192.0.2.1"}},
		TTL:     300,
	}, nil
}

func (f *fakeResolver) Stats() upstream.StatsSnapshot { return upstream.StatsSnapshot{} }

func (f *fakeResolver) HealthCheck(context.Context) upstream.ServerLatency {
	return upstream.ServerLatency{}
}

// --- helpers ---

func newTestService(r *router.Router, mode string) (*Service, *fakeResolver, *fakeResolver) {
	direct := &fakeResolver{}
	proxy := &fakeResolver{}
	mgr := upstream.NewManager([]upstream.Resolver{direct}, []upstream.Resolver{proxy}, nil)
	svc := New(fakeCache{}, cache.NewNegativeCache(time.Minute), r, mgr, nil, mode)
	return svc, direct, proxy
}

func emptyRouter(t *testing.T) *router.Router {
	t.Helper()
	r, err := router.New("", "")
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	return r
}

func handle(t *testing.T, svc *Service, domain string) {
	t.Helper()
	if _, err := svc.Handle(context.Background(), dns.BuildQuery(domain, dns.QTypeA), nil); err != nil {
		t.Fatalf("Handle(%s): %v", domain, err)
	}
}

// --- mode tests ---

func TestModeDirectPinsAllQueries(t *testing.T) {
	svc, direct, proxy := newTestService(emptyRouter(t), ModeDirect)
	handle(t, svc, "example.com")
	if direct.calls.Load() == 0 {
		t.Error("direct resolver not called in direct mode")
	}
	if proxy.calls.Load() != 0 {
		t.Error("proxy resolver called in direct mode")
	}
}

func TestModeProxyPinsAllQueries(t *testing.T) {
	svc, direct, proxy := newTestService(emptyRouter(t), ModeProxy)
	handle(t, svc, "example.com")
	if proxy.calls.Load() == 0 {
		t.Error("proxy resolver not called in proxy mode")
	}
	if direct.calls.Load() != 0 {
		t.Error("direct resolver called in proxy mode")
	}
}

func TestModeRulesRoutesByDomain(t *testing.T) {
	ruleFile := filepath.Join(t.TempDir(), "rules.txt")
	if err := os.WriteFile(ruleFile, []byte("||proxied.example^\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := router.New(ruleFile, "")
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}

	svc, direct, proxy := newTestService(r, ModeRules)
	handle(t, svc, "proxied.example") // rule hit → proxy pool
	handle(t, svc, "plain.example")   // no rule → direct pool

	if proxy.calls.Load() != 1 {
		t.Errorf("proxy resolver calls = %d, want 1 (ruled domain only)", proxy.calls.Load())
	}
	if direct.calls.Load() != 1 {
		t.Errorf("direct resolver calls = %d, want 1 (unruled domain only)", direct.calls.Load())
	}
}

func TestModeEmptyFallsBackToRules(t *testing.T) {
	svc, direct, proxy := newTestService(emptyRouter(t), "")
	handle(t, svc, "example.com")
	if direct.calls.Load() == 0 {
		t.Error("direct resolver not called: empty mode should behave as rules (no rule → direct)")
	}
	if proxy.calls.Load() != 0 {
		t.Error("proxy resolver called with empty mode and empty ruleset")
	}
}
