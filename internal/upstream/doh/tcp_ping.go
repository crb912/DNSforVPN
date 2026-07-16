package doh

import (
	"fmt"
	"net"
	"net/url"
	"time"
)

// tcpPingLatency attempts a TCP connection to host:port and returns the
// round-trip time in milliseconds. Returns 0 on failure.
func tcpPingLatency(serverURL string, timeout time.Duration) int {
	u, err := url.Parse(serverURL)
	if err != nil {
		return 0
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return 0
	}
	defer conn.Close()
	return int(time.Since(start).Milliseconds())
}

// extractHostPort returns host:port from a server URL string.
func extractHostPort(serverURL string, defaultPort string) string {
	u, err := url.Parse(serverURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = defaultPort
	}
	return fmt.Sprintf("%s:%s", host, port)
}
