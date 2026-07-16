package cache

import (
	"sync"
	"time"
)

// NegativeCache stores NXDOMAIN responses with a fixed TTL to avoid
// repeatedly querying upstream for domains that definitively do not exist.
type NegativeCache struct {
	mu    sync.RWMutex
	ttl   time.Duration
	items map[string]int64 // domain → expireAt (Unix)
}

// NewNegativeCache creates a negative cache with the given TTL. A typical
// value is 300s to match the SOA MINIMUM from NXDOMAIN responses.
func NewNegativeCache(ttl time.Duration) *NegativeCache {
	if ttl <= 0 {
		ttl = 300 * time.Second
	}
	return &NegativeCache{
		ttl:   ttl,
		items: make(map[string]int64),
	}
}

// Get returns true if the domain is cached and not yet expired.
func (n *NegativeCache) Get(domain string) bool {
	n.mu.RLock()
	expireAt, ok := n.items[domain]
	n.mu.RUnlock()
	if !ok {
		return false
	}
	return time.Now().Unix() < expireAt
}

// Set stores a negative entry for the domain, expiring after the configured
// TTL.
func (n *NegativeCache) Set(domain string) {
	n.mu.Lock()
	n.items[domain] = time.Now().Add(n.ttl).Unix()
	n.mu.Unlock()
}

// Size returns the current number of cached entries (including expired ones
// that have not yet been swept).
func (n *NegativeCache) Size() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.items)
}

// Sweep removes all expired entries. Call this periodically from a
// background goroutine.
func (n *NegativeCache) Sweep() {
	n.mu.Lock()
	defer n.mu.Unlock()
	now := time.Now().Unix()
	for domain, expireAt := range n.items {
		if now >= expireAt {
			delete(n.items, domain)
		}
	}
}
