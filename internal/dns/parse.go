package dns

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

// ParseQuery decodes a DNS query from wire format and returns the parsed
// Query. The original packet is retained for later response building.
func ParseQuery(packet []byte) (*Query, error) {
	if len(packet) < 12 {
		return nil, fmt.Errorf("dns: packet too short: %d bytes", len(packet))
	}

	id := binary.BigEndian.Uint16(packet[0:2])
	// flags   := binary.BigEndian.Uint16(packet[2:4])
	qdcount := binary.BigEndian.Uint16(packet[4:6])
	if qdcount == 0 {
		return nil, fmt.Errorf("dns: no question in query")
	}

	domain, offset, err := unpackName(packet, 12, true)
	if err != nil {
		return nil, fmt.Errorf("dns: question name: %w", err)
	}

	if offset+4 > len(packet) {
		return nil, fmt.Errorf("dns: question section truncated")
	}

	qtype := binary.BigEndian.Uint16(packet[offset : offset+2])
	// qclass := binary.BigEndian.Uint16(packet[offset+2 : offset+4])

	return &Query{
		ID:     id,
		Domain: domain,
		Type:   qtype,
		Packet: packet,
	}, nil
}

// ParseResponse decodes a DNS response from wire format and returns the
// extracted resource records. This is used for upstream DoH/UDP responses.
func ParseResponse(packet []byte) (Result, error) {
	if len(packet) < 12 {
		return Result{}, fmt.Errorf("dns: response packet too short: %d bytes", len(packet))
	}

	flags := binary.BigEndian.Uint16(packet[2:4])
	ancount := binary.BigEndian.Uint16(packet[6:8])
	nscount := binary.BigEndian.Uint16(packet[8:10])
	arcount := binary.BigEndian.Uint16(packet[10:12])

	rcode := int(flags & 0x000F)
	if rcode != 0 && rcode != RcodeNXDomain {
		return Result{}, fmt.Errorf("dns: response rcode=%d", rcode)
	}

	// Find where the question section ends so we can start parsing answers.
	qEndOffset, err := skipQuestion(packet, 12)
	if err != nil {
		return Result{}, fmt.Errorf("dns: response question: %w", err)
	}

	records := make([]RRRecord, 0)

	// Parse answer, authority, and additional sections.
	for _, count := range []uint16{ancount, nscount, arcount} {
		var rr []RRRecord
		rr, qEndOffset, err = unpackRRSection(packet, int(count), qEndOffset)
		if err != nil {
			// Best-effort: continue on parse errors in trailing sections.
			continue
		}
		records = append(records, rr...)
	}

	if rcode == RcodeNXDomain && len(records) == 0 {
		return Result{NXDomain: true}, nil
	}

	ttl := minTTLFrom(records)
	return Result{Records: records, TTL: ttl}, nil
}

// skipQuestion reads past the question section (domain name + QType + QClass)
// and returns the offset of the first byte after it.
func skipQuestion(packet []byte, start int) (int, error) {
	_, offset, err := unpackName(packet, start, true)
	if err != nil {
		return start, fmt.Errorf("skip question name: %w", err)
	}
	if offset+4 > len(packet) {
		return offset, fmt.Errorf("skip question: truncated at offset %d", offset)
	}
	return offset + 4, nil
}

// unpackRRSection parses `count` resource records starting at `offset`.
func unpackRRSection(packet []byte, count, start int) ([]RRRecord, int, error) {
	records := make([]RRRecord, 0)
	offset := start

	for i := 0; i < count; i++ {
		if offset >= len(packet) {
			return records, offset, fmt.Errorf("dns: rr section truncated")
		}

		name, newOffset, err := unpackName(packet, offset, false)
		if err != nil {
			return records, offset, err
		}
		offset = newOffset

		if offset+10 > len(packet) {
			return records, offset, fmt.Errorf("dns: rr header truncated at offset %d", offset)
		}

		rrType := binary.BigEndian.Uint16(packet[offset : offset+2])
		class := binary.BigEndian.Uint16(packet[offset+2 : offset+4])
		ttl := binary.BigEndian.Uint32(packet[offset+4 : offset+8])
		rdLen := binary.BigEndian.Uint16(packet[offset+8 : offset+10])
		offset += 10

		if offset+int(rdLen) > len(packet) {
			return records, offset, fmt.Errorf("dns: rdata truncated at offset %d, rdlen=%d", offset, rdLen)
		}

		rdataBytes := packet[offset : offset+int(rdLen)]
		rdata := parseRData(packet, rrType, rdataBytes, offset)
		offset += int(rdLen)

		if rdata != "" {
			records = append(records, RRRecord{
				Name:  name,
				Type:  rrType,
				Class: class,
				TTL:   ttl,
				RData: rdata,
			})
		}
	}

	return records, offset, nil
}

// parseRData converts raw RDATA bytes to a human-readable string.
func parseRData(packet []byte, rrType uint16, data []byte, dataOffset int) string {
	switch rrType {
	case QTypeA:
		if len(data) == 4 {
			return net.IP(data).String()
		}
	case QTypeAAAA:
		if len(data) == 16 {
			return net.IP(data).String()
		}
	case QTypeCNAME:
		// Try uncompressed first (data may be self-contained).
		if name := parseUncompressedName(data); name != "" {
			return name
		}
		// Fall back to compression-pointer resolution.
		if dataOffset >= 0 && dataOffset < len(packet) {
			if name, _, err := unpackName(packet, dataOffset, false); err == nil && name != "" {
				return name
			}
		}
	case QTypeSOA:
		return fmt.Sprintf("%d", MinimumTTL)
	}
	return ""
}

// parseUncompressedName parses a domain name from raw bytes that do not
// contain compression pointers (e.g., RDATA for a simple CNAME).
func parseUncompressedName(data []byte) string {
	parts := make([]string, 0)
	offset := 0
	for offset < len(data) {
		length := int(data[offset])
		offset++
		if length == 0 {
			break
		}
		if length >= 192 { // compression pointer — bail
			return ""
		}
		if offset+length > len(data) {
			return ""
		}
		parts = append(parts, string(data[offset:offset+length]))
		offset += length
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ".")
}

// unpackName reads a domain name starting at `start` in `packet`. When
// qSection is true, compression pointers are rejected (RFC 1035 §4.1.4).
func unpackName(packet []byte, start int, qSection bool) (string, int, error) {
	if start < 0 || start >= len(packet) {
		return "", start, fmt.Errorf("dns: name start %d out of range [0,%d)", start, len(packet))
	}

	offset := start
	parts := make([]string, 0)
	visited := make(map[int]bool)
	const maxJumps = 10

	for jumps := 0; offset < len(packet); {
		if offset >= len(packet) {
			return "", offset, fmt.Errorf("dns: unexpected end of name at offset %d", offset)
		}

		length := int(packet[offset])
		offset++

		if length == 0 {
			// End of name.
			return strings.Join(parts, "."), offset, nil
		}

		if qSection && length > 63 {
			return "", offset, fmt.Errorf("dns: label length %d > 63 in question section", length)
		}

		// Compression pointer (top two bits set).
		if !qSection && length >= 192 {
			if offset >= len(packet) {
				return "", offset, fmt.Errorf("dns: compression pointer truncated")
			}
			pointer := int(length&0x3F)<<8 | int(packet[offset])
			offset++

			if pointer < 0 || pointer >= len(packet) {
				return "", offset, fmt.Errorf("dns: invalid compression pointer %d", pointer)
			}
			if pointer >= start {
				return "", offset, fmt.Errorf("dns: forward compression pointer %d", pointer)
			}
			if visited[pointer] {
				return "", offset, fmt.Errorf("dns: compression pointer loop at %d", pointer)
			}
			visited[pointer] = true
			jumps++
			if jumps > maxJumps {
				return "", offset, fmt.Errorf("dns: too many compression jumps")
			}

			jumpedName, _, err := unpackName(packet, pointer, false)
			if err != nil {
				if len(parts) > 0 {
					return strings.Join(parts, "."), offset, nil
				}
				return "", offset, err
			}
			if jumpedName != "" {
				parts = append(parts, jumpedName)
			}
			return strings.Join(parts, "."), offset, nil
		}

		// Normal label.
		if offset+length > len(packet) {
			return "", offset, fmt.Errorf("dns: label runs past end of packet")
		}
		parts = append(parts, string(packet[offset:offset+length]))
		offset += length
	}

	return strings.Join(parts, "."), offset, nil
}

// minTTLFrom returns the smallest non-zero TTL from a set of records, or
// MinimumTTL if none have a valid TTL.
func minTTLFrom(records []RRRecord) uint32 {
	if len(records) == 0 {
		return MinimumTTL
	}
	min := uint32(0xFFFFFFFF)
	for _, r := range records {
		if r.TTL > 0 && r.TTL < min {
			min = r.TTL
		}
	}
	if min == 0xFFFFFFFF {
		return MinimumTTL
	}
	return min
}
