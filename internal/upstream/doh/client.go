// Package doh implements DNS-over-HTTPS resolution as a Resolver.
package doh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"doh-dns-proxy/internal/dns"
	"doh-dns-proxy/internal/upstream"
)

// Client resolves DNS queries via a single DoH server endpoint.
type Client struct {
	serverURL  string
	proxyURL   string // optional HTTP proxy
	httpClient *http.Client

	// Metrics.
	totalQueries atomic.Uint64
	totalErrors  atomic.Uint64
	latencies    *latencyTracker // rolling window for percentile computation
}

// latencyTracker stores a fixed-size window of recent latencies for
// computing approximate percentiles.
type latencyTracker struct {
	buf    []float64
	idx    int
	filled bool
}

func newLatencyTracker(size int) *latencyTracker {
	return &latencyTracker{buf: make([]float64, size)}
}

func (lt *latencyTracker) add(ms float64) {
	lt.buf[lt.idx] = ms
	lt.idx++
	if lt.idx >= len(lt.buf) {
		lt.idx = 0
		lt.filled = true
	}
}

// percentiles returns (p50, p99, avg) from the current window. Returns
// zeroes when no data is available.
func (lt *latencyTracker) percentiles() (p50, p99, avg float64) {
	n := lt.idx
	if lt.filled {
		n = len(lt.buf)
	}
	if n == 0 {
		return 0, 0, 0
	}

	// Copy and sort a small slice for percentiles. Simple insertion sort
	// is fine for window sizes ≤ 1024.
	sorted := make([]float64, n)
	copy(sorted, lt.buf[:n])
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	var sum float64
	for _, v := range sorted {
		sum += v
	}
	avg = sum / float64(n)

	p50Idx := int(float64(n) * 0.50)
	p99Idx := int(float64(n) * 0.99)
	if p50Idx >= n {
		p50Idx = n - 1
	}
	if p99Idx >= n {
		p99Idx = n - 1
	}
	p50 = sorted[p50Idx]
	p99 = sorted[p99Idx]
	return
}

// New creates a DoH Client targeting a single server. If proxyURL is
// non-empty, all HTTP requests will be routed through it.
func New(serverURL, proxyURL string) *Client {
	c := &Client{
		serverURL: serverURL,
		proxyURL:  proxyURL,
		latencies: newLatencyTracker(1024),
	}
	c.httpClient = c.buildClient()
	return c
}

func (c *Client) buildClient() *http.Client {
	transport := &http.Transport{
		IdleConnTimeout: 90 * time.Second,
	}

	if c.proxyURL != "" {
		u, err := url.Parse(c.proxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
	}
}

// Resolve sends a single DNS-over-HTTPS POST query and parses the wire-format
// response. This is a single-request, single-server operation — concurrent
// multi-server orchestration is the Manager's responsibility.
func (c *Client) Resolve(ctx context.Context, domain string, qtype uint16) (upstream.Result, error) {
	c.totalQueries.Add(1)

	// Build wire-format query.
	queryPacket := dns.BuildQuery(domain, qtype)
	body := bytes.NewReader(queryPacket)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL, body)
	if err != nil {
		c.totalErrors.Add(1)
		return upstream.Result{}, fmt.Errorf("doh: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	elapsed := time.Since(start).Seconds() * 1000 // ms
	c.latencies.add(elapsed)

	if err != nil {
		c.totalErrors.Add(1)
		return upstream.Result{}, fmt.Errorf("doh: %s: %w", c.serverURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.totalErrors.Add(1)
		return upstream.Result{}, fmt.Errorf("doh: %s returned %d", c.serverURL, resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.totalErrors.Add(1)
		return upstream.Result{}, fmt.Errorf("doh: read body: %w", err)
	}

	parsed, err := dns.ParseResponse(respBody)
	if err != nil {
		c.totalErrors.Add(1)
		return upstream.Result{}, fmt.Errorf("doh: parse response: %w", err)
	}

	if parsed.NXDomain {
		return upstream.Result{NXDomain: true}, nil
	}

	return upstream.Result{
		Records: parsed.Records,
		TTL:     parsed.TTL,
	}, nil
}

// Stats returns a snapshot of accumulated metrics.
func (c *Client) Stats() upstream.StatsSnapshot {
	p50, p99, avg := c.latencies.percentiles()
	return upstream.StatsSnapshot{
		Protocol:     "doh",
		Server:       c.serverURL,
		TotalQueries: c.totalQueries.Load(),
		TotalErrors:  c.totalErrors.Load(),
		AvgLatencyMs: avg,
		P50LatencyMs: p50,
		P99LatencyMs: p99,
	}
}

// HealthCheck sends a test query for "example.com" A and measures latency.
func (c *Client) HealthCheck(ctx context.Context) upstream.ServerLatency {
	start := time.Now()
	_, err := c.Resolve(ctx, "example.com", dns.QTypeA)
	elapsed := time.Since(start).Seconds() * 1000

	if err != nil {
		// Try TCP ping as fallback.
		if tcpMs := tcpPingLatency(c.serverURL, 3*time.Second); tcpMs > 0 {
			return upstream.ServerLatency{
				ServerURL: c.serverURL,
				LatencyMs: float64(tcpMs),
				Status:    "ok",
			}
		}
		return upstream.ServerLatency{
			ServerURL: c.serverURL,
			LatencyMs: elapsed,
			Status:    "error",
		}
	}

	return upstream.ServerLatency{
		ServerURL: c.serverURL,
		LatencyMs: elapsed,
		Status:    "ok",
	}
}
