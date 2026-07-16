// Package udp implements plain UDP DNS resolution (primarily for bootstrap).
package udp

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"doh-dns-proxy/internal/dns"
	"doh-dns-proxy/internal/upstream"
)

// Client resolves DNS queries via a single UDP DNS server.
type Client struct {
	addr string // host:port

	totalQueries atomic.Uint64
	totalErrors  atomic.Uint64
}

// New creates a UDP resolver targeting addr (e.g. "223.5.5.5:53").
func New(addr string) *Client {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "53")
	}
	return &Client{addr: addr}
}

// Resolve sends a UDP DNS query and parses the response. Implements Resolver.
func (c *Client) Resolve(ctx context.Context, domain string, qtype uint16) (upstream.Result, error) {
	c.totalQueries.Add(1)

	queryPacket := dns.BuildQuery(domain, qtype)

	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", c.addr)
	if err != nil {
		c.totalErrors.Add(1)
		return upstream.Result{}, fmt.Errorf("udp: %s: %w", c.addr, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	} else {
		conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	if _, err := conn.Write(queryPacket); err != nil {
		c.totalErrors.Add(1)
		return upstream.Result{}, fmt.Errorf("udp: write: %w", err)
	}

	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		c.totalErrors.Add(1)
		return upstream.Result{}, fmt.Errorf("udp: read: %w", err)
	}

	parsed, err := dns.ParseResponse(buf[:n])
	if err != nil {
		// Best-effort: some servers return responses our parser can't
		// fully handle. Return what we got.
		return upstream.Result{Records: parsed.Records, TTL: parsed.TTL}, nil
	}

	if parsed.NXDomain {
		return upstream.Result{NXDomain: true}, nil
	}

	return upstream.Result{
		Records: parsed.Records,
		TTL:     parsed.TTL,
	}, nil
}

// Stats returns accumulated metrics. UDP resolver does not track latency
// percentiles (used only for bootstrap).
func (c *Client) Stats() upstream.StatsSnapshot {
	return upstream.StatsSnapshot{
		Protocol:     "udp",
		Server:       c.addr,
		TotalQueries: c.totalQueries.Load(),
		TotalErrors:  c.totalErrors.Load(),
	}
}

// HealthCheck probes the UDP server with a test query.
func (c *Client) HealthCheck(ctx context.Context) upstream.ServerLatency {
	start := time.Now()
	_, err := c.Resolve(ctx, "example.com", dns.QTypeA)
	elapsed := time.Since(start).Seconds() * 1000

	status := "ok"
	if err != nil {
		status = "error"
	}

	return upstream.ServerLatency{
		ServerURL: "udp://" + c.addr,
		LatencyMs: elapsed,
		Status:    status,
	}
}
