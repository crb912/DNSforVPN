// Package cache provides the caching layer for DNS query results.
// It defines the Cache interface that query layer consumes, plus concrete
// BoltDB-backed and negative-cache implementations.
package cache

import (
	"time"

	"doh-dns-proxy/internal/dns"
)

// Entry is a single cached DNS resolution result.
type Entry struct {
	Records  []dns.RRRecord
	TTL      uint32
	ExpireAt int64 // Unix timestamp when this entry expires
}

// Stats provides runtime statistics for the cache.
type Stats struct {
	Size      int64   // number of entries currently cached
	HitRate   float64 // cache hit ratio over the last window (0.0 - 1.0)
	MemBytes  int64   // estimated hot-cache memory usage
	DiskBytes int64   // on-disk database file size
}

// Expired returns true if the entry's TTL has passed.
func (e Entry) Expired() bool {
	return time.Now().Unix() >= e.ExpireAt
}

// CacheItem is a single cached entry paired with its lookup key, as
// returned by Cache.List.
type CacheItem struct {
	Domain string
	QType  uint16
	Entry  Entry
}

// Cache is the interface consumed by the query layer.
type Cache interface {
	// Get retrieves a cached entry. The second bool indicates whether the
	// entry was present in the cache (regardless of expiry). Callers must
	// check Expired() on the returned Entry.
	Get(domain string, qtype uint16) (Entry, bool)

	// Set stores or updates a cached entry.
	Set(domain string, qtype uint16, entry Entry)

	// Del removes an entry from the cache.
	Del(domain string)

	// List returns all entries currently stored, including expired ones
	// (callers must check Expired per entry).
	List() []CacheItem

	// Stats returns runtime statistics.
	Stats() Stats

	// Close flushes pending writes and releases resources.
	Close() error
}
