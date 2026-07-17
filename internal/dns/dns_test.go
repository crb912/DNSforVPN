package dns

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

// hexToBytes converts a hex string like "00 01 02 03" to []byte.
func hexToBytes(s string) []byte {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			continue
		}
		hi := hexVal(s[i])
		lo := hexVal(s[i+1])
		b = append(b, hi<<4|lo)
		i++
	}
	return b
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// buildTestQuery creates a wire-format DNS query for "example.com A".
func buildTestQuery(domain string, qtype uint16, id uint16) []byte {
	q := BuildQuery(domain, qtype)
	binary.BigEndian.PutUint16(q[0:2], id)
	return q
}

// buildTestResponse creates a minimal wire-format DNS response with a
// single A record.
func buildTestAResponse(domain string, ip string, ttl uint32) []byte {
	query := buildTestQuery(domain, QTypeA, 0x1234)

	// Parse and build through our API for the correct result.
	q, err := ParseQuery(query)
	if err != nil {
		panic(err)
	}

	result := Result{
		Records: []RRRecord{
			{Name: domain, Type: QTypeA, Class: QClass, TTL: ttl, RData: ip},
		},
		TTL: ttl,
	}
	return BuildResponse(q, result)
}

func TestParseQuery_Domain(t *testing.T) {
	query := buildTestQuery("example.com", QTypeA, 0xABCD)

	q, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}

	if q.ID != 0xABCD {
		t.Errorf("ID = 0x%04X, want 0xABCD", q.ID)
	}
	if q.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", q.Domain, "example.com")
	}
	if q.Type != QTypeA {
		t.Errorf("Type = %d, want %d", q.Type, QTypeA)
	}
	if len(q.Packet) != len(query) {
		t.Errorf("Packet len = %d, want %d", len(q.Packet), len(query))
	}
}

func TestParseQuery_AAAA(t *testing.T) {
	query := buildTestQuery("ipv6.test.example", QTypeAAAA, 0xBEEF)

	q, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}

	if q.Type != QTypeAAAA {
		t.Errorf("Type = %d, want %d", q.Type, QTypeAAAA)
	}
	if q.Domain != "ipv6.test.example" {
		t.Errorf("Domain = %q", q.Domain)
	}
}

func TestParseQuery_Subdomain(t *testing.T) {
	query := buildTestQuery("www.sub.dom.example", QTypeA, 1)

	q, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if q.Domain != "www.sub.dom.example" {
		t.Errorf("Domain = %q, want %q", q.Domain, "www.sub.dom.example")
	}
}

func TestParseQuery_EmptyPacket(t *testing.T) {
	_, err := ParseQuery([]byte{})
	if err == nil {
		t.Error("expected error for empty packet")
	}
}

func TestParseQuery_TooShort(t *testing.T) {
	_, err := ParseQuery(make([]byte, 8))
	if err == nil {
		t.Error("expected error for short packet")
	}
}

func TestBuildResponse_ARecord(t *testing.T) {
	query := buildTestQuery("example.com", QTypeA, 0xF00D)
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}

	result := Result{
		Records: []RRRecord{
			{Name: "example.com", Type: QTypeA, Class: QClass, TTL: 300, RData: "93.184.216.34"},
		},
		TTL: 300,
	}

	resp := BuildResponse(q, result)

	// Parse the response back to verify.
	parsed, err := ParseResponse(resp)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if len(parsed.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(parsed.Records))
	}
	if parsed.Records[0].RData != "93.184.216.34" {
		t.Errorf("RData = %q, want %q", parsed.Records[0].RData, "93.184.216.34")
	}
}

func TestBuildResponse_CNAMERecord(t *testing.T) {
	query := buildTestQuery("www.example.com", QTypeCNAME, 0x1111)
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}

	result := Result{
		Records: []RRRecord{
			{Name: "www.example.com", Type: QTypeCNAME, Class: QClass, TTL: 600, RData: "example.com"},
		},
		TTL: 600,
	}

	resp := BuildResponse(q, result)
	parsed, err := ParseResponse(resp)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if len(parsed.Records) == 0 {
		t.Fatal("no records in response")
	}
	// CNAME may parse as "example.com" or "example.com." — both are fine.
	if parsed.Records[0].Type != QTypeCNAME {
		t.Errorf("Type = %d, want %d", parsed.Records[0].Type, QTypeCNAME)
	}
}

func TestBuildResponse_AAAARecord(t *testing.T) {
	query := buildTestQuery("ipv6.test", QTypeAAAA, 0x2222)
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}

	result := Result{
		Records: []RRRecord{
			{Name: "ipv6.test", Type: QTypeAAAA, Class: QClass, TTL: 3600,
				RData: "2001:db8::1"},
		},
		TTL: 3600,
	}

	resp := BuildResponse(q, result)
	parsed, err := ParseResponse(resp)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if len(parsed.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(parsed.Records))
	}
	if parsed.Records[0].RData != "2001:db8::1" {
		t.Errorf("RData = %q, want %q", parsed.Records[0].RData, "2001:db8::1")
	}
}

func TestBuildErrorResponse_NXDomain(t *testing.T) {
	query := buildTestQuery("nxdomain.example", QTypeA, 0x3333)
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}

	resp := BuildErrorResponse(q, RcodeNXDomain)

	if len(resp) < 12 {
		t.Fatal("response too short")
	}

	flags := binary.BigEndian.Uint16(resp[2:4])
	rcode := int(flags & 0xF)
	if rcode != RcodeNXDomain {
		t.Errorf("rcode = %d, want %d", rcode, RcodeNXDomain)
	}
}

func TestBuildErrorResponse_ServFail(t *testing.T) {
	query := buildTestQuery("fail.example", QTypeA, 0x4444)
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}

	resp := BuildErrorResponse(q, RcodeServFail)

	flags := binary.BigEndian.Uint16(resp[2:4])
	rcode := int(flags & 0xF)
	if rcode != RcodeServFail {
		t.Errorf("rcode = %d, want %d", rcode, RcodeServFail)
	}
}

func TestBuildQuery(t *testing.T) {
	q := BuildQuery("dns.google.com", QTypeA)

	if len(q) < 17 {
		t.Fatalf("query too short: %d bytes", len(q))
	}

	// It should parse cleanly.
	_, err := ParseQuery(q)
	if err != nil {
		t.Fatalf("ParseQuery on BuildQuery output: %v", err)
	}
}

func TestParseResponse_NXDomain(t *testing.T) {
	// Build a real NXDOMAIN response through our own builder.
	query := buildTestQuery("nxdomain.test", QTypeA, 0x5555)
	q, _ := ParseQuery(query)
	resp := BuildErrorResponse(q, RcodeNXDomain)

	result, err := ParseResponse(resp)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if !result.NXDomain {
		t.Error("expected NXDomain = true")
	}
}

func TestParseResponse_EmptyPacket(t *testing.T) {
	_, err := ParseResponse([]byte{})
	if err == nil {
		t.Error("expected error for empty response")
	}
}

func TestParseResponse_MultipleRecords(t *testing.T) {
	query := buildTestQuery("multi.test", QTypeA, 0x6666)
	q, _ := ParseQuery(query)

	result := Result{
		Records: []RRRecord{
			{Name: "multi.test", Type: QTypeA, Class: QClass, TTL: 100, RData: "1.2.3.4"},
			{Name: "multi.test", Type: QTypeA, Class: QClass, TTL: 200, RData: "5.6.7.8"},
			{Name: "multi.test", Type: QTypeAAAA, Class: QClass, TTL: 300, RData: "::1"},
		},
		TTL: 100,
	}

	resp := BuildResponse(q, result)
	parsed, err := ParseResponse(resp)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}

	wantIPs := map[string]bool{"1.2.3.4": true, "5.6.7.8": true, "::1": true}
	for _, r := range parsed.Records {
		if !wantIPs[r.RData] {
			t.Errorf("unexpected RData: %q", r.RData)
		}
		delete(wantIPs, r.RData)
	}
	if len(wantIPs) > 0 {
		t.Errorf("missing records: %v", wantIPs)
	}
}

func TestParseResponse_MinTTL(t *testing.T) {
	query := buildTestQuery("ttl.test", QTypeA, 0x7777)
	q, _ := ParseQuery(query)

	result := Result{
		Records: []RRRecord{
			{Name: "ttl.test", Type: QTypeA, Class: QClass, TTL: 3600, RData: "10.0.0.1"},
			{Name: "ttl.test", Type: QTypeA, Class: QClass, TTL: 60, RData: "10.0.0.2"},
		},
		TTL: 60,
	}

	resp := BuildResponse(q, result)
	parsed, err := ParseResponse(resp)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if parsed.TTL != 60 {
		t.Errorf("TTL = %d, want 60", parsed.TTL)
	}
}

func TestParseResponse_NoRecords(t *testing.T) {
	query := buildTestQuery("empty.test", QTypeA, 0x8888)
	q, _ := ParseQuery(query)

	result := Result{
		TTL: MinimumTTL,
	}
	resp := BuildResponse(q, result)

	parsed, err := ParseResponse(resp)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if len(parsed.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(parsed.Records))
	}
}

func TestRoundTrip_QueryAndResponse(t *testing.T) {
	// Simulate a complete flow: client query → parse → build response → parse response.
	query := buildTestQuery("roundtrip.example", QTypeA, 0x9999)

	// Step 1: Parse the client query.
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}

	// Step 2: Build a response (as if from cache or upstream).
	result := Result{
		Records: []RRRecord{
			{Name: "roundtrip.example", Type: QTypeA, Class: QClass, TTL: 1800, RData: "203.0.113.1"},
		},
		TTL: 1800,
	}
	resp := BuildResponse(q, result)

	// Step 3: The client would now receive `resp`. Verify it parses correctly.
	parsed, err := ParseResponse(resp)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}

	if parsed.NXDomain {
		t.Error("NXDomain should be false")
	}
	if len(parsed.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(parsed.Records))
	}
	r := parsed.Records[0]
	if r.Type != QTypeA {
		t.Errorf("type = %d, want %d", r.Type, QTypeA)
	}
	if r.RData != "203.0.113.1" {
		t.Errorf("RData = %q, want %q", r.RData, "203.0.113.1")
	}
	if r.TTL != 1800 {
		t.Errorf("TTL = %d, want 1800", r.TTL)
	}
}

func TestRoundTrip_RealWorldDomains(t *testing.T) {
	domains := []string{
		"google.com",
		"github.com",
		"a.b.c.d.example",
		"very-long-subdomain-name-that-is-still-valid.com",
	}

	for _, domain := range domains {
		t.Run(domain, func(t *testing.T) {
			query := buildTestQuery(domain, QTypeA, 0xAAAA)
			q, err := ParseQuery(query)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", domain, err)
			}
			if q.Domain != domain {
				t.Errorf("domain = %q, want %q", q.Domain, domain)
			}

			result := Result{
				Records: []RRRecord{
					{Name: domain, Type: QTypeA, Class: QClass, TTL: 600, RData: "203.0.113.99"},
				},
				TTL: 600,
			}
			resp := BuildResponse(q, result)
			parsed, err := ParseResponse(resp)
			if err != nil {
				t.Fatalf("ParseResponse(%q): %v", domain, err)
			}
			if len(parsed.Records) != 1 || parsed.Records[0].RData != "203.0.113.99" {
				t.Errorf("bad round-trip for %q: records=%v", domain, parsed.Records)
			}
		})
	}
}

func TestTTLRemaining(t *testing.T) {
	// Expired.
	got := TTLRemaining(0)
	if got != MinimumTTL {
		t.Errorf("expired: got %d, want %d", got, MinimumTTL)
	}

	// Far future.
	got = TTLRemaining(1<<31 - 1)
	if got < 3600 {
		t.Errorf("far future: got %d, want >= 3600", got)
	}
}

func TestEncodeDomainName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"a", "\x01a"},
		{"ab.cd", "\x02ab\x02cd"},
		{"example.com", "\x07example\x03com"},
	}

	for _, tt := range tests {
		got := string(encodeDomainName(tt.in))
		if got != tt.want {
			t.Errorf("encodeDomainName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildQuery_ProducesParsable(t *testing.T) {
	types := []uint16{QTypeA, QTypeAAAA, QTypeCNAME}
	for _, qtype := range types {
		q := BuildQuery("test.example", qtype)
		parsed, err := ParseQuery(q)
		if err != nil {
			t.Errorf("ParseQuery(BuildQuery(_, %d)): %v", qtype, err)
			continue
		}
		if parsed.Type != qtype {
			t.Errorf("type = %d, want %d", parsed.Type, qtype)
		}
	}
}

func TestParseResponse_RealGoogleDoHResponse(t *testing.T) {
	// Real captured DoH response for google.com A — verified with wireshark.
	// ID=0xee62, flags=0x8180, QD=1, AN=1, NS=0, AR=0
	// Question: google.com A
	// Answer: google.com 300 IN A 142.250.80.46
	raw := []byte{
		0xee, 0x62, // ID
		0x81, 0x80, // flags
		0x00, 0x01, // QDCOUNT = 1
		0x00, 0x01, // ANCOUNT = 1
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
		// Question section: google.com A IN
		0x06, 'g', 'o', 'o', 'g', 'l', 'e',
		0x03, 'c', 'o', 'm',
		0x00,       // end of name
		0x00, 0x01, // QTYPE = A
		0x00, 0x01, // QCLASS = IN
		// Answer section
		0xc0, 0x0c, // compression pointer to google.com
		0x00, 0x01, // TYPE = A
		0x00, 0x01, // CLASS = IN
		0x00, 0x00, 0x01, 0x2c, // TTL = 300
		0x00, 0x04, // RDLENGTH = 4
		0x8e, 0xfa, 0x50, 0x2e, // 142.250.80.46
	}

	result, err := ParseResponse(raw)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if result.NXDomain {
		t.Error("NXDomain should be false")
	}
	if len(result.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(result.Records))
	}
	r := result.Records[0]
	if r.Type != QTypeA {
		t.Errorf("Type = %d, want %d", r.Type, QTypeA)
	}
	ip := net.ParseIP("142.250.80.46")
	if r.RData != ip.String() {
		t.Errorf("RData = %q, want %q", r.RData, ip.String())
	}
	if r.TTL != 300 {
		t.Errorf("TTL = %d, want 300", r.TTL)
	}
}

func TestBuildResponse_CNAMERdataTerminated(t *testing.T) {
	q, err := ParseQuery(BuildQuery("youjizz1.com", QTypeCNAME))
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	resp := BuildResponse(q, Result{
		Records: []RRRecord{
			{Name: q.Domain, Type: QTypeCNAME, Class: QClass, TTL: 120, RData: "plantwo.seihappy.com"},
		},
		TTL: 120,
	})

	// Wire format requires the rdata domain name to end with a root
	// terminator (0x00) — dig rejects the packet otherwise.
	want := []byte("\x07plantwo\x08seihappy\x03com\x00")
	if !bytes.Contains(resp, want) {
		t.Errorf("CNAME rdata missing root terminator; response: %x", resp)
	}

	if _, err := ParseResponse(resp); err != nil {
		t.Errorf("ParseResponse(BuildResponse): %v", err)
	}
}
