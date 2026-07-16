// Package upstream provides DNS resolution via external servers. Each
// protocol (DoH, DoT, UDP) implements the Resolver interface; the Manager
// orchestrates concurrent queries across multiple servers.
package upstream

import (
	"context"
	"doh-dns-proxy/internal/dns"
)

// Result is the resolved answer from a single upstream query.
type Result struct {
	Records  []dns.RRRecord
	TTL      uint32
	NXDomain bool
}

// StatsSnapshot captures per-resolver performance counters.
type StatsSnapshot struct {
	Protocol     string  `json:"protocol"`
	Server       string  `json:"server"`
	TotalQueries uint64  `json:"total_queries"`
	TotalErrors  uint64  `json:"total_errors"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`
}

// ServerLatency is a health-check result for a single server.
type ServerLatency struct {
	ServerURL string  `json:"server_url"`
	LatencyMs float64 `json:"latency_ms"`
	Status    string  `json:"status"` // "ok", "timeout", "error"
}

// Resolver is the interface consumed by the query layer. Each implementation
// is responsible for a single protocol (DoH, DoT, UDP).
type Resolver interface {
	// Resolve sends a DNS query to the resolver and returns the result.
	Resolve(ctx context.Context, domain string, qtype uint16) (Result, error)

	// Stats returns accumulated performance counters.
	Stats() StatsSnapshot

	// HealthCheck probes the resolver and returns latency information.
	HealthCheck(ctx context.Context) ServerLatency
}
