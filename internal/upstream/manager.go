package upstream

import (
	"context"
	"fmt"
	"time"

	"doh-dns-proxy/internal/dns"
)

// Manager orchestrates concurrent DNS queries across multiple Resolver
// instances. It does NOT import concrete protocol packages — it consumes the
// Resolver interface, keeping layers cleanly separated.
type Manager struct {
	direct []Resolver
	proxy  []Resolver
	boot   Resolver // optional UDP resolver for bootstrap
}

// NewManager creates a Manager. Use nil for bootstrap if not needed.
func NewManager(direct, proxy []Resolver, boot Resolver) *Manager {
	return &Manager{
		direct: direct,
		proxy:  proxy,
		boot:   boot,
	}
}

// Direct returns the direct resolvers.
func (m *Manager) Direct() []Resolver { return m.direct }

// Proxy returns the proxy resolvers.
func (m *Manager) Proxy() []Resolver { return m.proxy }

// Bootstrap returns the UDP bootstrap resolver, or nil.
func (m *Manager) Bootstrap() Resolver { return m.boot }

// ResolveAll fans out the query to all given resolvers concurrently.
// Returns the deduplicated aggregate of records from all successful
// responses and the minimum TTL. If all fail, returns the last error.
func (m *Manager) ResolveAll(ctx context.Context, domain string, qtype uint16, resolvers []Resolver) (Result, error) {
	if len(resolvers) == 0 {
		return Result{}, fmt.Errorf("upstream: no resolvers configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	type rpcResult struct {
		records []dns.RRRecord
		ttl     uint32
		err     error
	}

	ch := make(chan rpcResult, len(resolvers))
	for _, r := range resolvers {
		go func(r Resolver) {
			res, err := r.Resolve(ctx, domain, qtype)
			if err != nil {
				ch <- rpcResult{err: err}
				return
			}
			ch <- rpcResult{records: res.Records, ttl: res.TTL}
		}(r)
	}

	var allRecords []dns.RRRecord
	var firstErr error
	ok := 0

	for i := 0; i < len(resolvers); i++ {
		select {
		case rr := <-ch:
			if rr.err != nil {
				if firstErr == nil {
					firstErr = rr.err
				}
				continue
			}
			ok++
			allRecords = append(allRecords, rr.records...)
		case <-ctx.Done():
			if ok > 0 {
				return reduce(allRecords), nil
			}
			return Result{}, ctx.Err()
		}
	}

	if ok > 0 {
		return reduce(allRecords), nil
	}
	if firstErr != nil {
		return Result{}, firstErr
	}
	return Result{}, fmt.Errorf("upstream: all resolvers failed")
}

// HealthCheckAll probes every resolver and returns latency info.
func (m *Manager) HealthCheckAll(ctx context.Context) []ServerLatency {
	all := append(m.direct, m.proxy...)
	ch := make(chan ServerLatency, len(all))
	for _, r := range all {
		go func(r Resolver) { ch <- r.HealthCheck(ctx) }(r)
	}
	out := make([]ServerLatency, 0, len(all))
	for i := 0; i < len(all); i++ {
		out = append(out, <-ch)
	}
	return out
}

// StatsAll returns stats for every resolver.
func (m *Manager) StatsAll() []StatsSnapshot {
	out := make([]StatsSnapshot, 0, len(m.direct)+len(m.proxy))
	for _, r := range m.direct {
		out = append(out, r.Stats())
	}
	for _, r := range m.proxy {
		out = append(out, r.Stats())
	}
	return out
}

// --- helpers ---

// reduce merges records from all successful responses into one Result.
// Identical records (same name, type and rdata) returned by multiple
// servers are deduplicated, keeping the smallest TTL seen; Result.TTL is
// the minimum TTL across all records.
func reduce(records []dns.RRRecord) Result {
	if len(records) == 0 {
		return Result{TTL: dns.MinimumTTL}
	}
	type key struct {
		name  string
		typ   uint16
		rdata string
	}
	seen := make(map[key]int, len(records))
	deduped := make([]dns.RRRecord, 0, len(records))
	for _, r := range records {
		k := key{r.Name, r.Type, r.RData}
		if i, dup := seen[k]; dup {
			if r.TTL < deduped[i].TTL {
				deduped[i].TTL = r.TTL
			}
			continue
		}
		seen[k] = len(deduped)
		deduped = append(deduped, r)
	}
	ttl := uint32(0xFFFFFFFF)
	for _, r := range deduped {
		if r.TTL > 0 && r.TTL < ttl {
			ttl = r.TTL
		}
	}
	if ttl == 0xFFFFFFFF {
		ttl = dns.MinimumTTL
	}
	return Result{Records: deduped, TTL: ttl}
}
