package cache

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"doh-dns-proxy/internal/dns"

	bolt "go.etcd.io/bbolt"
)

const (
	// bucketName is the BoltDB bucket for DNS cache entries.
	bucketName = "dns"
	// customBucketName is the BoltDB bucket for user-defined DNS overrides.
	customBucketName = "custom"
)

// boltCache implements Cache backed by BoltDB on disk and a sync.Map hot
// cache in memory. Hot entries are loaded lazily on first access.
type boltCache struct {
	db     *bolt.DB
	hot    sync.Map      // map[string]*hotEntry
	hits   atomic.Uint64 // total cache hits
	misses atomic.Uint64 // total cache misses
	size   atomic.Int64  // current entry count (approximate)
}

type hotEntry struct {
	value      Entry
	lastAccess atomic.Int64 // Unix nano — for potential LRU eviction
}

// New opens or creates a BoltDB-backed cache at dbPath. The hot cache
// starts empty — entries are loaded from disk on first Get.
func New(dbPath string) (Cache, error) {
	dir := dbPath[:max(0, lastSlash(dbPath))]
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("cache: mkdir %s: %w", dir, err)
		}
	}

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("cache: open boltdb: %w", err)
	}

	// Ensure the buckets exist.
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists([]byte(customBucketName))
		return err
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: create bucket: %w", err)
	}

	c := &boltCache{
		db: db,
	}

	// Count existing entries for initial size estimate.
	_ = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b != nil {
			c.size.Store(int64(b.Stats().KeyN))
		}
		return nil
	})

	return c, nil
}

// key builds the BoltDB key: "domain|qtype".
func key(domain string, qtype uint16) string {
	return fmt.Sprintf("%s|%d", domain, qtype)
}

func lastSlash(p string) int {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return i
		}
	}
	return -1
}

// Get checks hot cache, then disk. On a disk hit the entry is promoted to
// the hot cache. Returns (Entry, true) when present regardless of expiry.
func (c *boltCache) Get(domain string, qtype uint16) (Entry, bool) {
	k := key(domain, qtype)

	// 1. Hot cache.
	if v, ok := c.hot.Load(k); ok {
		he := v.(*hotEntry)
		he.lastAccess.Store(time.Now().UnixNano())
		c.hits.Add(1)
		return he.value, true
	}

	// 2. BoltDB.
	var entry Entry
	found := false
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		data := b.Get([]byte(k))
		if data == nil {
			return nil
		}
		entry, found = decode(data)
		return nil
	})

	if found {
		// 3. Promote to hot cache.
		c.hot.Store(k, &hotEntry{
			value:      entry,
			lastAccess: atomic.Int64{},
		})
		c.hits.Add(1)
		return entry, true
	}

	c.misses.Add(1)
	return Entry{}, false
}

// Set writes to both the hot cache and BoltDB.
func (c *boltCache) Set(domain string, qtype uint16, entry Entry) {
	k := key(domain, qtype)

	// Hot cache.
	c.hot.Store(k, &hotEntry{value: entry})

	// Disk.
	data := encode(entry)
	_ = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		return b.Put([]byte(k), data)
	})
}

// Del removes an entry from both layers.
func (c *boltCache) Del(domain string) {
	// Remove from hot cache (iterate over all qtype keys for this domain).
	prefix := domain + "|"
	c.hot.Range(func(k, _ interface{}) bool {
		if len(k.(string)) >= len(prefix) && k.(string)[:len(prefix)] == prefix {
			c.hot.Delete(k)
		}
		return true
	})

	// Delete from disk — BoltDB range delete over prefix.
	_ = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		cursor := b.Cursor()
		for k, _ := cursor.Seek([]byte(prefix)); k != nil && bytes.HasPrefix(k, []byte(prefix)); k, _ = cursor.Next() {
			_ = b.Delete(k)
		}
		return nil
	})
}

// List returns all entries stored on disk, including expired ones.
// The disk bucket is authoritative: Set writes through to it.
func (c *boltCache) List() []CacheItem {
	var items []CacheItem
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			parts := strings.SplitN(string(k), "|", 2)
			if len(parts) != 2 {
				return nil // skip malformed keys
			}
			var qtype uint16
			if _, err := fmt.Sscanf(parts[1], "%d", &qtype); err != nil {
				return nil
			}
			entry, ok := decode(v)
			if !ok {
				return nil
			}
			items = append(items, CacheItem{
				Domain: parts[0],
				QType:  qtype,
				Entry:  entry,
			})
			return nil
		})
	})
	return items
}

// Stats returns hit rate and approximate sizes.
func (c *boltCache) Stats() Stats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	// Memory estimate: count of hot entries ≈ 200 bytes each.
	memEstimate := int64(0)
	c.hot.Range(func(_, _ interface{}) bool {
		memEstimate += 200
		return true
	})

	var diskSize int64
	if fi, err := os.Stat(c.db.Path()); err == nil {
		diskSize = fi.Size()
	}

	return Stats{
		Size:      c.size.Load(),
		HitRate:   hitRate,
		MemBytes:  memEstimate,
		DiskBytes: diskSize,
	}
}

// Close flushes and closes the BoltDB.
func (c *boltCache) Close() error {
	// Sync hot cache to disk (already done on Set, but belt-and-suspenders).
	c.hot.Range(func(k, v interface{}) bool {
		he := v.(*hotEntry)
		data := encode(he.value)
		_ = c.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(bucketName))
			if b != nil {
				return b.Put([]byte(k.(string)), data)
			}
			return nil
		})
		return true
	})
	return c.db.Close()
}

// --- CustomStore implementation (bucket "custom", no hot cache) ---

// CustomGet retrieves the user-defined override for a domain.
func (c *boltCache) CustomGet(domain string) (CustomEntry, bool) {
	var entry CustomEntry
	found := false
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(customBucketName))
		if b == nil {
			return nil
		}
		data := b.Get([]byte(domain))
		if data == nil {
			return nil
		}
		entry, found = decodeCustom(data)
		return nil
	})
	return entry, found
}

// CustomSet creates or replaces the user-defined override for a domain.
func (c *boltCache) CustomSet(domain string, ips []string) {
	data := encodeCustom(CustomEntry{Domain: domain, IPs: ips})
	_ = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(customBucketName))
		if b == nil {
			return nil
		}
		return b.Put([]byte(domain), data)
	})
}

// CustomDel removes the user-defined override for a domain.
func (c *boltCache) CustomDel(domain string) {
	_ = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(customBucketName))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(domain))
	})
}

// CustomList returns all user-defined overrides, sorted by domain.
func (c *boltCache) CustomList() []CustomEntry {
	var entries []CustomEntry
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(customBucketName))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			if entry, ok := decodeCustom(v); ok {
				entries = append(entries, entry)
			}
			return nil
		})
	})
	sortCustomEntries(entries)
	return entries
}

// --- gob encoding ---

func init() {
	gob.Register(dns.RRRecord{})
	gob.Register([]dns.RRRecord{})
}

func encode(e Entry) []byte {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(e); err != nil {
		// Should never fail for Entry / RRRecord.
		return nil
	}
	return buf.Bytes()
}

func decode(data []byte) (Entry, bool) {
	var e Entry
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&e); err != nil {
		return Entry{}, false
	}
	return e, true
}
