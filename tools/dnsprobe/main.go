// dnsprobe sends a single DNS query over UDP and prints the decoded
// response. It exists because stock nslookup/dig variants (Windows,
// busybox) cannot reliably target a custom port.
//
// Usage:
//
//	go run ./tools/dnsprobe -server 127.0.0.1:5553 -type A example.com
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"doh-dns-proxy/internal/dns"
)

func main() {
	server := flag.String("server", "127.0.0.1:5553", "DNS server host:port")
	qtype := flag.String("type", "A", "query type: A, AAAA or CNAME")
	timeout := flag.Duration("timeout", 3*time.Second, "UDP deadline")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: dnsprobe [-server host:port] [-type A|AAAA|CNAME] domain")
		os.Exit(2)
	}
	domain := flag.Arg(0)

	var qt uint16 = dns.QTypeA
	switch strings.ToUpper(*qtype) {
	case "A":
	case "AAAA":
		qt = dns.QTypeAAAA
	case "CNAME":
		qt = dns.QTypeCNAME
	default:
		fmt.Fprintln(os.Stderr, "unsupported qtype:", *qtype)
		os.Exit(2)
	}

	conn, err := net.DialTimeout("udp", *server, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(*timeout))

	if _, err := conn.Write(dns.BuildQuery(domain, qt)); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}

	res, err := dns.ParseResponse(buf[:n])
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		os.Exit(1)
	}
	if res.NXDomain {
		fmt.Println("NXDOMAIN")
		return
	}
	for _, r := range res.Records {
		fmt.Printf("%s\ttype=%d\tttl=%d\t%s\n", r.Name, r.Type, r.TTL, r.RData)
	}
}
