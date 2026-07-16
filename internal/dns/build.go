package dns

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"time"
)

// qDomainPtr is a DNS compression pointer to offset 12 — the domain name
// in the question section. It saves space in the answer section by reusing
// the question's domain encoding.
var qDomainPtr = []byte{0xc0, 0x0c}

// wireFlags builds the DNS header flags field.
const (
	flagQR     = 0x8000 // Query Response
	flagAA     = 0x0400 // Authoritative Answer (unset)
	flagRD     = 0x0100 // Recursion Desired
	flagRA     = 0x0080 // Recursion Available
	flagStdRsp = flagQR | flagRD | flagRA // 0x8180
)

// BuildResponse constructs a wire-format DNS response from a parsed Query
// and a resolution Result. The query's original Packet provides the question
// section bytes.
func BuildResponse(q *Query, r Result) []byte {
	var buf bytes.Buffer

	// Header.
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], q.ID)
	// Find qEndOffset in the original packet so we can slice the question.
	qEndOffset := questionEndOffset(q.Packet)
	binary.BigEndian.PutUint16(header[2:4], flagStdRsp)
	binary.BigEndian.PutUint16(header[4:6], 1)                 // QDCOUNT
	binary.BigEndian.PutUint16(header[6:8], answerCount(r, q)) // ANCOUNT
	buf.Write(header)

	// Question section: copy from the original query packet.
	buf.Write(q.Packet[12:qEndOffset])

	// Answer section.
	for _, rec := range r.Records {
		buf.Write(qDomainPtr)
		buf.Write(encodeU16(rec.Type))
		buf.Write(encodeU16(rec.Class))
		buf.Write(encodeU32(rec.TTL))
		rDataBytes := encodeRData(rec.Type, rec.RData)
		buf.Write(encodeU16(uint16(len(rDataBytes))))
		buf.Write(rDataBytes)
	}

	return buf.Bytes()
}

// BuildErrorResponse constructs a wire-format DNS error response (e.g.
// NXDomain or ServFail) for the given Query.
func BuildErrorResponse(q *Query, rcode int) []byte {
	var buf bytes.Buffer

	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], q.ID)

	flags := flagQR | flagRD | flagRA | uint16(rcode&0xF)
	binary.BigEndian.PutUint16(header[2:4], flags)
	binary.BigEndian.PutUint16(header[4:6], 1) // QDCOUNT
	binary.BigEndian.PutUint16(header[6:8], 0) // ANCOUNT
	buf.Write(header)

	qEndOffset := questionEndOffset(q.Packet)
	buf.Write(q.Packet[12:qEndOffset])

	return buf.Bytes()
}

// BuildQuery constructs a wire-format DNS query for the given domain and
// query type. Used for bootstrap (plain UDP DNS) and health checks.
func BuildQuery(domain string, qtype uint16) []byte {
	var buf bytes.Buffer

	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], 0)       // ID = 0 (caller should randomize)
	binary.BigEndian.PutUint16(header[2:4], 0x0100)  // standard query, RD=1
	binary.BigEndian.PutUint16(header[4:6], 1)        // QDCOUNT
	buf.Write(header)

	buf.Write(encodeDomainName(domain))
	buf.WriteByte(0) // root label terminator

	buf.Write(encodeU16(qtype))
	buf.Write(encodeU16(QClass))

	return buf.Bytes()
}

// BuildQueryWithID is like BuildQuery but allows setting the transaction ID.
func BuildQueryWithID(domain string, qtype uint16, id uint16) []byte {
	packet := BuildQuery(domain, qtype)
	binary.BigEndian.PutUint16(packet[0:2], id)
	return packet
}

// questionEndOffset finds the byte offset of the end of the question section
// in the original query packet. Returns 12 if parsing fails (safe fallback).
func questionEndOffset(packet []byte) int {
	if len(packet) < 12 {
		return 12
	}
	_, offset, err := unpackName(packet, 12, true)
	if err != nil {
		return 12
	}
	if offset+4 > len(packet) {
		return offset
	}
	return offset + 4
}

// answerCount returns the number of answer records to emit. For NXDomain it
// returns 0; for CNAME results it returns the count; for A/AAAA with an
// expired TTL it returns 1 with a clamped TTL.
func answerCount(r Result, q *Query) uint16 {
	if r.NXDomain {
		return 0
	}
	return uint16(len(r.Records))
}

// encodeRData converts a presentation-format RData string to wire format
// based on the record type.
func encodeRData(rrType uint16, rdata string) []byte {
	switch rrType {
	case QTypeA:
		ip := net.ParseIP(rdata)
		if ip != nil {
			if v4 := ip.To4(); v4 != nil {
				return v4
			}
		}
	case QTypeAAAA:
		ip := net.ParseIP(rdata)
		if ip != nil {
			if v6 := ip.To16(); v6 != nil {
				return v6
			}
		}
	case QTypeCNAME:
		// encodeDomainName emits only the labels; a wire-format name in
		// rdata must end with the root label terminator (0x00).
		return append(encodeDomainName(rdata), 0)
	}
	return nil
}

// encodeDomainName encodes a dotted domain name into wire format.
// "www.example.com" → \x03www\x07example\x03com
func encodeDomainName(domain string) []byte {
	labels := strings.Split(domain, ".")
	var buf bytes.Buffer
	for _, label := range labels {
		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}
	return buf.Bytes()
}

// encodeU16 encodes a uint16 in big-endian order.
func encodeU16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

// encodeU32 encodes a uint32 in big-endian order.
func encodeU32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// TTLRemaining calculates the remaining TTL in seconds given an absolute
// expiration time. Returns MinimumTTL if the TTL has already expired.
func TTLRemaining(expireAt int64) uint32 {
	remaining := expireAt - time.Now().Unix()
	if remaining < MinimumTTL {
		return MinimumTTL
	}
	return uint32(remaining)
}
