// Package web exposes the control layer over HTTP: a JSON REST API under
// /api plus the embedded single-page UI built from frontend/dist.
package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"doh-dns-proxy/frontend"
	"doh-dns-proxy/internal/control"
)

func init() {
	// Not in Go's built-in table on all platforms (needed for the PWA manifest).
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// Server is the web UI + REST API server.
type Server struct {
	ctl  *control.Control
	cfg  control.WebConfig
	http *http.Server
	ln   net.Listener
}

// New creates a Server. cfg.Host/cfg.Port select the listen address;
// HTTP Basic auth is enforced when cfg.Password is non-empty.
func New(ctl *control.Control, cfg control.WebConfig) *Server {
	return &Server{ctl: ctl, cfg: cfg}
}

// Start binds the listener and serves in the background.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handleSaveConfig)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/start", s.handleStart)
	mux.HandleFunc("POST /api/stop", s.handleStop)
	mux.HandleFunc("GET /api/latency", s.handleLatency)
	mux.HandleFunc("GET /api/stats/cache", s.handleCacheStats)
	mux.HandleFunc("GET /api/stats/query", s.handleQueryStats)
	mux.HandleFunc("GET /api/cache", s.handleCache)
	mux.HandleFunc("GET /api/custom-dns", s.handleGetCustomDNS)
	mux.HandleFunc("PUT /api/custom-dns", s.handleSetCustomDNS)
	mux.HandleFunc("DELETE /api/custom-dns", s.handleDeleteCustomDNS)
	mux.Handle("GET /", s.staticHandler())

	s.http = &http.Server{
		Handler:           s.withAuth(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port)))
	if err != nil {
		return err
	}
	s.ln = ln
	go func() { _ = s.http.Serve(ln) }()
	return nil
}

// Shutdown stops the HTTP server gracefully.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// URL returns the browseable base URL, mapping wildcard listen hosts to
// loopback so it can be handed to a browser.
func (s *Server) URL() string {
	host := s.cfg.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(s.cfg.Port))
}

// --- API handlers ---

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl.GetConfig())
}

func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var cfg control.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, err)
		return
	}
	if err := s.ctl.SaveConfig(cfg); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl.GetStatus())
}

func (s *Server) handleStart(w http.ResponseWriter, _ *http.Request) {
	if err := s.ctl.Start(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, s.ctl.GetStatus())
}

func (s *Server) handleStop(w http.ResponseWriter, _ *http.Request) {
	s.ctl.Stop()
	writeJSON(w, s.ctl.GetStatus())
}

func (s *Server) handleLatency(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl.CheckLatency())
}

func (s *Server) handleCacheStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl.GetCacheStats())
}

func (s *Server) handleQueryStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl.GetQueryStats())
}

func (s *Server) handleCache(w http.ResponseWriter, r *http.Request) {
	entries, err := s.ctl.QueryCache(r.URL.Query().Get("domain"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, entries)
}

func (s *Server) handleGetCustomDNS(w http.ResponseWriter, _ *http.Request) {
	entries, err := s.ctl.GetCustomDNS()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, entries)
}

func (s *Server) handleSetCustomDNS(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Domain string   `json:"domain"`
		IPs    []string `json:"ips"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, err)
		return
	}
	if err := s.ctl.SetCustomDNS(body.Domain, body.IPs); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteCustomDNS(w http.ResponseWriter, r *http.Request) {
	if err := s.ctl.DeleteCustomDNS(r.URL.Query().Get("domain")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// --- static files (embedded SPA) ---

func (s *Server) staticHandler() http.Handler {
	dist, err := fs.Sub(frontend.Dist, "dist")
	if err != nil {
		// embed guarantees dist exists; this is defensive only.
		return http.NotFoundHandler()
	}
	fileServer := http.FileServerFS(dist)
	index, _ := fs.ReadFile(dist, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasPrefix(p, "api/") {
			http.NotFound(w, r)
			return
		}
		if p != "" {
			if f, err := dist.Open(p); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback: unknown paths render the app shell.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}

// --- helpers ---

func (s *Server) withAuth(next http.Handler) http.Handler {
	if s.cfg.Password == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		userOK := s.cfg.Username == "" || subtle.ConstantTimeCompare([]byte(user), []byte(s.cfg.Username)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(s.cfg.Password)) == 1
		if !ok || !userOK || !passOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="dnsforvpn"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
