// Package transport implements the DNS wire-format network listeners.
package transport

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Handler processes a raw DNS packet from a client and returns a wire-format
// reply for the transport to send back.
type Handler func(ctx context.Context, packet []byte, addr net.Addr) ([]byte, error)

// UDPServer listens for DNS queries over UDP and dispatches each to the
// Handler. It supports graceful shutdown via context cancellation.
type UDPServer struct {
	addr    *net.UDPAddr
	conn    *net.UDPConn
	handler Handler
	wg      sync.WaitGroup
}

// NewUDPServer creates a UDP DNS listener. It does NOT start accepting
// connections — call Start for that.
func NewUDPServer(host string, port int, handler Handler) (*UDPServer, error) {
	addr := &net.UDPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	return &UDPServer{
		addr:    addr,
		conn:    conn,
		handler: handler,
	}, nil
}

// Start begins accepting DNS queries. It blocks until ctx is cancelled.
func (s *UDPServer) Start(ctx context.Context) error {
	slog.Info("transport: UDP server started", "addr", s.addr.String())

	buf := make([]byte, 512)

	for {
		select {
		case <-ctx.Done():
			slog.Info("transport: shutting down UDP server")
			s.wg.Wait()
			return ctx.Err()
		default:
		}

		s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("transport: UDP read error", "err", err)
			continue
		}

		// Copy to avoid data race with the next read.
		packet := make([]byte, n)
		copy(packet, buf[:n])

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			resp, err := s.handler(context.Background(), packet, addr)
			if err != nil {
				return // handler already logged
			}
			if _, err := s.conn.WriteToUDP(resp, addr); err != nil {
				slog.Warn("transport: UDP write error", "err", err)
			}
		}()
	}
}

// Addr returns the listening address.
func (s *UDPServer) Addr() net.Addr {
	return s.addr
}

// Close stops the server and closes the underlying socket.
func (s *UDPServer) Close() error {
	s.wg.Wait()
	return s.conn.Close()
}
