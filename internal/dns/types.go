// Package dns provides pure functions for parsing and building DNS wire-format
// messages. It holds no state and has no external dependencies.
package dns

// DNS query type constants.
const (
	QTypeA     = 1
	QTypeAAAA  = 28
	QTypeCNAME = 5
	QTypeSOA   = 6
	QTypeOPT   = 41
	QClass     = 1 // IN (Internet)
)

// MinimumTTL is the floor TTL returned to clients when a cached entry has
// expired.
const MinimumTTL = 300

// Rcode constants.
const (
	RcodeOK       = 0
	RcodeFormErr  = 1
	RcodeServFail = 2
	RcodeNXDomain = 3
	RcodeNotImp   = 4
	RcodeRefused  = 5
)

// Query represents a parsed DNS query from a client.
type Query struct {
	ID     uint16 // transaction ID from the original packet
	Domain string // requested domain name
	Type   uint16 // QTypeA, QTypeAAAA, etc.
	Packet []byte // original wire-format query bytes (for BuildResponse)
}

// Result carries the upstream DNS resolution result — either from cache or
// a live resolver.
type Result struct {
	Records  []RRRecord
	TTL      uint32 // minimum TTL across all returned records
	NXDomain bool
}

// RRRecord is a single DNS resource record.
type RRRecord struct {
	Name  string
	Type  uint16
	Class uint16
	TTL   uint32
	RData string // presentation format: "192.0.2.1", "example.com.", etc.
}
