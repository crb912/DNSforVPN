package upstream

import (
	"testing"

	"doh-dns-proxy/internal/dns"
)

func TestReduceDeduplicatesRecords(t *testing.T) {
	records := []dns.RRRecord{
		{Name: "example.com", Type: dns.QTypeA, Class: dns.QClass, TTL: 300, RData: "192.0.2.1"},
		{Name: "example.com", Type: dns.QTypeA, Class: dns.QClass, TTL: 200, RData: "192.0.2.2"},
		// Same RR from a second server with a smaller TTL — must be merged,
		// keeping the smaller TTL.
		{Name: "example.com", Type: dns.QTypeA, Class: dns.QClass, TTL: 250, RData: "192.0.2.1"},
	}

	got := reduce(records)

	if len(got.Records) != 2 {
		t.Fatalf("expected 2 deduplicated records, got %d: %+v", len(got.Records), got.Records)
	}
	if got.Records[0].RData != "192.0.2.1" || got.Records[0].TTL != 250 {
		t.Errorf("record 0: want {192.0.2.1 ttl 250}, got {%s ttl %d}", got.Records[0].RData, got.Records[0].TTL)
	}
	if got.TTL != 200 {
		t.Errorf("Result.TTL: want min TTL 200, got %d", got.TTL)
	}
}

func TestReduceKeepsDistinctNames(t *testing.T) {
	// Same rdata but different owner names are distinct RRs (CNAME chains).
	records := []dns.RRRecord{
		{Name: "www.example.com", Type: dns.QTypeA, Class: dns.QClass, TTL: 300, RData: "192.0.2.1"},
		{Name: "example.com", Type: dns.QTypeA, Class: dns.QClass, TTL: 300, RData: "192.0.2.1"},
	}

	got := reduce(records)

	if len(got.Records) != 2 {
		t.Fatalf("expected both records kept, got %d", len(got.Records))
	}
}

func TestReduceEmpty(t *testing.T) {
	got := reduce(nil)

	if got.Records != nil {
		t.Errorf("expected nil records, got %+v", got.Records)
	}
	if got.TTL != dns.MinimumTTL {
		t.Errorf("expected fallback TTL %d, got %d", dns.MinimumTTL, got.TTL)
	}
}

func TestReduceZeroTTLFallsBack(t *testing.T) {
	records := []dns.RRRecord{
		{Name: "example.com", Type: dns.QTypeA, Class: dns.QClass, TTL: 0, RData: "192.0.2.1"},
	}

	got := reduce(records)

	if got.TTL != dns.MinimumTTL {
		t.Errorf("expected fallback TTL %d, got %d", dns.MinimumTTL, got.TTL)
	}
}
