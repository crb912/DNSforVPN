package cache

import (
	"path/filepath"
	"testing"
	"time"

	"doh-dns-proxy/internal/dns"
)

func newTestCache(t *testing.T) Cache {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	c, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func sampleEntry(ip string, ttl uint32) Entry {
	return Entry{
		Records: []dns.RRRecord{
			{Name: "example.com", Type: dns.QTypeA, Class: dns.QClass, TTL: ttl, RData: ip},
		},
		TTL:      ttl,
		ExpireAt: time.Now().Unix() + int64(ttl),
	}
}

func TestCache_SetAndGet(t *testing.T) {
	c := newTestCache(t)

	entry := sampleEntry("93.184.216.34", 300)
	c.Set("example.com", dns.QTypeA, entry)

	got, ok := c.Get("example.com", dns.QTypeA)
	if !ok {
		t.Fatal("Get returned false after Set")
	}
	if len(got.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(got.Records))
	}
	if got.Records[0].RData != "93.184.216.34" {
		t.Errorf("RData = %q, want %q", got.Records[0].RData, "93.184.216.34")
	}
	if got.TTL != 300 {
		t.Errorf("TTL = %d, want 300", got.TTL)
	}
}

func TestCache_Miss(t *testing.T) {
	c := newTestCache(t)

	_, ok := c.Get("nonexistent.com", dns.QTypeA)
	if ok {
		t.Error("Get returned true for missing key")
	}
}

func TestCache_MultipleTypes(t *testing.T) {
	c := newTestCache(t)

	c.Set("dual.test", dns.QTypeA, sampleEntry("1.1.1.1", 100))
	c.Set("dual.test", dns.QTypeAAAA, sampleEntry("::1", 200))

	a, ok := c.Get("dual.test", dns.QTypeA)
	if !ok || a.Records[0].RData != "1.1.1.1" {
		t.Error("A record mismatch")
	}
	aaaa, ok := c.Get("dual.test", dns.QTypeAAAA)
	if !ok || aaaa.Records[0].RData != "::1" {
		t.Error("AAAA record mismatch")
	}
	if aaaa.TTL != 200 {
		t.Errorf("AAAA TTL = %d, want 200", aaaa.TTL)
	}
}

func TestCache_Overwrite(t *testing.T) {
	c := newTestCache(t)

	c.Set("example.com", dns.QTypeA, sampleEntry("1.1.1.1", 100))
	c.Set("example.com", dns.QTypeA, sampleEntry("2.2.2.2", 200))

	got, ok := c.Get("example.com", dns.QTypeA)
	if !ok {
		t.Fatal("entry missing after overwrite")
	}
	if got.Records[0].RData != "2.2.2.2" {
		t.Errorf("RData = %q, want %q", got.Records[0].RData, "2.2.2.2")
	}
}

func TestCache_Del(t *testing.T) {
	c := newTestCache(t)

	c.Set("example.com", dns.QTypeA, sampleEntry("1.1.1.1", 100))
	c.Set("example.com", dns.QTypeAAAA, sampleEntry("::1", 200))

	c.Del("example.com")

	_, ok := c.Get("example.com", dns.QTypeA)
	if ok {
		t.Error("A record still present after Del")
	}
	_, ok = c.Get("example.com", dns.QTypeAAAA)
	if ok {
		t.Error("AAAA record still present after Del")
	}
}

func TestCache_Del_NonExistent(t *testing.T) {
	c := newTestCache(t)
	c.Del("nonexistent.com") // must not panic
}

func TestCache_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	// Write.
	c1, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	c1.Set("example.com", dns.QTypeA, sampleEntry("93.184.216.34", 300))
	c1.Close()

	// Read back.
	c2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	got, ok := c2.Get("example.com", dns.QTypeA)
	if !ok {
		t.Fatal("entry not found after reopen")
	}
	if got.Records[0].RData != "93.184.216.34" {
		t.Errorf("RData = %q after reopen", got.Records[0].RData)
	}
}

func TestCache_Stats(t *testing.T) {
	c := newTestCache(t)

	c.Set("a.com", dns.QTypeA, sampleEntry("1.1.1.1", 100))
	c.Set("b.com", dns.QTypeA, sampleEntry("2.2.2.2", 100))

	// Two hits.
	c.Get("a.com", dns.QTypeA)
	c.Get("a.com", dns.QTypeA)

	// One miss.
	c.Get("nosuch.com", dns.QTypeA)

	stats := c.Stats()
	if stats.HitRate <= 0 {
		t.Errorf("HitRate = %f, want > 0", stats.HitRate)
	}
	if stats.MemBytes <= 0 {
		t.Errorf("MemBytes = %d, want > 0", stats.MemBytes)
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := newTestCache(t)
	c.Set("concurrent.test", dns.QTypeA, sampleEntry("10.0.0.1", 600))

	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.Get("concurrent.test", dns.QTypeA)
				c.Set("write.test", dns.QTypeA, sampleEntry("10.0.0.2", 300))
				c.Get("write.test", dns.QTypeA)
			}
			done <- true
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestCache_LazyLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lazy.db")

	// Seed disk with data, then close.
	c1, _ := New(path)
	c1.Set("disk.only", dns.QTypeA, sampleEntry("99.99.99.99", 600))
	c1.Close()

	// Reopen — hot cache is empty.
	c2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	// First Get triggers disk read + hot promotion.
	got, ok := c2.Get("disk.only", dns.QTypeA)
	if !ok {
		t.Fatal("lazy-load: entry not found")
	}
	if got.Records[0].RData != "99.99.99.99" {
		t.Errorf("lazy-load: RData = %q", got.Records[0].RData)
	}

	// Second Get is a pure hot-cache hit.
	got, ok = c2.Get("disk.only", dns.QTypeA)
	if !ok {
		t.Fatal("lazy-load: second Get failed (hot cache)")
	}

	stats := c2.Stats()
	if stats.HitRate != 1.0 {
		t.Errorf("lazy-load: HitRate = %f after two hits, want 1.0", stats.HitRate)
	}
}

func TestNegativeCache_GetSet(t *testing.T) {
	nc := NewNegativeCache(time.Second)

	if nc.Size() != 0 {
		t.Error("initial size should be 0")
	}

	nc.Set("nxdomain.test")
	if !nc.Get("nxdomain.test") {
		t.Error("Get returned false after Set")
	}
	if nc.Size() != 1 {
		t.Errorf("size = %d, want 1", nc.Size())
	}

	// Not yet expired.
	if !nc.Get("nxdomain.test") {
		t.Error("should still be valid immediately after Set")
	}
}

func TestNegativeCache_Expiry(t *testing.T) {
	nc := NewNegativeCache(50 * time.Millisecond)
	nc.Set("expire.test")

	time.Sleep(100 * time.Millisecond)

	if nc.Get("expire.test") {
		t.Error("entry should have expired")
	}
}

func TestNegativeCache_Miss(t *testing.T) {
	nc := NewNegativeCache(300 * time.Second)

	if nc.Get("unset.domain") {
		t.Error("Get returned true for unset domain")
	}
}

func TestNegativeCache_Sweep(t *testing.T) {
	nc := NewNegativeCache(50 * time.Millisecond)
	nc.Set("a.com")
	nc.Set("b.com")

	time.Sleep(100 * time.Millisecond)

	nc.Sweep()
	if nc.Size() != 0 {
		t.Errorf("size after sweep = %d, want 0", nc.Size())
	}
}

func TestNegativeCache_Concurrent(t *testing.T) {
	nc := NewNegativeCache(10 * time.Second)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				nc.Set("concurrent.test")
				nc.Get("concurrent.test")
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	if !nc.Get("concurrent.test") {
		t.Error("entry lost under concurrent access")
	}
}

func TestCache_EmptyHotCacheAfterReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hot.db")

	// Write 5 entries.
	c1, _ := New(path)
	for i := 0; i < 5; i++ {
		c1.Set("domain"+string(rune('a'+i))+".com", dns.QTypeA, sampleEntry("1.2.3.4", 300))
	}
	c1.Close()

	// Reopen — verify hot cache is lazy (all entries still accessible).
	c2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	for i := 0; i < 5; i++ {
		domain := "domain" + string(rune('a'+i)) + ".com"
		_, ok := c2.Get(domain, dns.QTypeA)
		if !ok {
			t.Errorf("lazy-load failed for %s", domain)
		}
	}
}

func TestCache_CNAMERecord(t *testing.T) {
	c := newTestCache(t)

	entry := Entry{
		Records: []dns.RRRecord{
			{Name: "www.example.com", Type: dns.QTypeCNAME, Class: dns.QClass, TTL: 600, RData: "example.com"},
		},
		TTL:      600,
		ExpireAt: time.Now().Unix() + 600,
	}

	c.Set("www.example.com", dns.QTypeCNAME, entry)

	got, ok := c.Get("www.example.com", dns.QTypeCNAME)
	if !ok {
		t.Fatal("CNAME entry not found")
	}
	if got.Records[0].RData != "example.com" {
		t.Errorf("CNAME RData = %q", got.Records[0].RData)
	}
}

func TestCache_List(t *testing.T) {
	c := newTestCache(t)

	c.Set("example.com", dns.QTypeA, sampleEntry("93.184.216.34", 300))
	c.Set("example.com", dns.QTypeAAAA, sampleEntry("2606:2800:220:1:248:1893:25c8:1946", 300))
	c.Set("other.org", dns.QTypeA, sampleEntry("192.0.2.1", 300))

	items := c.List()
	if len(items) != 3 {
		t.Fatalf("List returned %d items, want 3", len(items))
	}

	type dkey struct {
		domain string
		qtype  uint16
	}
	seen := make(map[dkey]bool)
	for _, it := range items {
		seen[dkey{it.Domain, it.QType}] = true
		if len(it.Entry.Records) == 0 {
			t.Errorf("List item %s|%d has no records", it.Domain, it.QType)
		}
	}
	for _, want := range []dkey{
		{"example.com", dns.QTypeA},
		{"example.com", dns.QTypeAAAA},
		{"other.org", dns.QTypeA},
	} {
		if !seen[want] {
			t.Errorf("List missing %s|%d", want.domain, want.qtype)
		}
	}
}

func newTestCustomStore(t *testing.T) CustomStore {
	t.Helper()
	cs, ok := newTestCache(t).(CustomStore)
	if !ok {
		t.Fatal("cache does not implement CustomStore")
	}
	return cs
}

func TestCustomStore_CRUD(t *testing.T) {
	cs := newTestCustomStore(t)

	// Create.
	cs.CustomSet("myapp.local", []string{"10.0.0.1", "10.0.0.2"})
	got, ok := cs.CustomGet("myapp.local")
	if !ok {
		t.Fatal("CustomGet returned false after CustomSet")
	}
	if len(got.IPs) != 2 || got.IPs[0] != "10.0.0.1" {
		t.Errorf("IPs = %v", got.IPs)
	}
	if got.Domain != "myapp.local" {
		t.Errorf("Domain = %q", got.Domain)
	}

	// Update.
	cs.CustomSet("myapp.local", []string{"10.0.0.3"})
	got, _ = cs.CustomGet("myapp.local")
	if len(got.IPs) != 1 || got.IPs[0] != "10.0.0.3" {
		t.Errorf("after update IPs = %v", got.IPs)
	}

	// List (sorted by domain).
	cs.CustomSet("aaa.local", []string{"1.1.1.1"})
	list := cs.CustomList()
	if len(list) != 2 || list[0].Domain != "aaa.local" || list[1].Domain != "myapp.local" {
		t.Errorf("CustomList = %+v", list)
	}

	// Delete.
	cs.CustomDel("myapp.local")
	if _, ok := cs.CustomGet("myapp.local"); ok {
		t.Error("CustomGet returned true after CustomDel")
	}
}

func TestCustomEntry_Records(t *testing.T) {
	e := CustomEntry{
		Domain: "mixed.local",
		IPs:    []string{"192.0.2.1", "2001:db8::1", "not-an-ip"},
	}

	aRecs := e.Records(dns.QTypeA)
	if len(aRecs) != 1 || aRecs[0].RData != "192.0.2.1" {
		t.Fatalf("A records = %+v", aRecs)
	}
	if aRecs[0].Type != dns.QTypeA || aRecs[0].Class != dns.QClass {
		t.Errorf("A record type/class wrong: %+v", aRecs[0])
	}
	if aRecs[0].TTL != dns.MinimumTTL {
		t.Errorf("A record TTL = %d, want %d", aRecs[0].TTL, dns.MinimumTTL)
	}
	if aRecs[0].Name != "mixed.local" {
		t.Errorf("A record Name = %q", aRecs[0].Name)
	}

	aaaaRecs := e.Records(dns.QTypeAAAA)
	if len(aaaaRecs) != 1 || aaaaRecs[0].RData != "2001:db8::1" {
		t.Fatalf("AAAA records = %+v", aaaaRecs)
	}
	if aaaaRecs[0].Type != dns.QTypeAAAA {
		t.Errorf("AAAA record type wrong: %+v", aaaaRecs[0])
	}
}
