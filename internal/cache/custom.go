package cache

import (
	"bytes"
	"encoding/gob"
	"net"
	"sort"

	"doh-dns-proxy/internal/dns"
)

// CustomEntry is a user-defined DNS override for a domain: a static list
// of IP addresses returned for A / AAAA queries instead of resolving
// upstream.
type CustomEntry struct {
	Domain string
	IPs    []string
}

// Records converts the entry's IP list into DNS records of the requested
// type: IPv4 addresses for A queries, IPv6 for AAAA. Returns nil when no
// address matches the requested family.
func (e CustomEntry) Records(qtype uint16) []dns.RRRecord {
	var out []dns.RRRecord
	for _, s := range e.IPs {
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		isV4 := ip.To4() != nil
		if (qtype == dns.QTypeA) != isV4 {
			continue
		}
		out = append(out, dns.RRRecord{
			Name:  e.Domain,
			Type:  qtype,
			Class: dns.QClass,
			TTL:   dns.MinimumTTL,
			RData: s,
		})
	}
	return out
}

// CustomStore persists user-defined DNS overrides. boltCache implements
// it using the "custom" bucket in the same BoltDB file as the cache.
type CustomStore interface {
	// CustomGet retrieves the override for a domain.
	CustomGet(domain string) (CustomEntry, bool)
	// CustomSet creates or replaces the override for a domain.
	CustomSet(domain string, ips []string)
	// CustomDel removes the override for a domain.
	CustomDel(domain string)
	// CustomList returns all overrides, sorted by domain.
	CustomList() []CustomEntry
}

// --- gob encoding for CustomEntry ---

func encodeCustom(e CustomEntry) []byte {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(e); err != nil {
		return nil
	}
	return buf.Bytes()
}

func decodeCustom(data []byte) (CustomEntry, bool) {
	var e CustomEntry
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&e); err != nil {
		return CustomEntry{}, false
	}
	return e, true
}

// sortCustomEntries orders entries by domain for stable UI display.
func sortCustomEntries(entries []CustomEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Domain < entries[j].Domain })
}
