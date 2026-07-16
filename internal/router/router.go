// Package router provides domain-based traffic routing using GFWList-style
// rule files. A Router decides whether a DNS query for a given domain should
// be sent to a direct (domestic) resolver or through a proxy (foreign)
// resolver.
package router

import (
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Decision tells the caller which upstream path to use.
type Decision int

const (
	RouteDirect Decision = iota
	RouteProxy
)

// RouteResult carries the routing decision and optional debug info.
type RouteResult struct {
	Decision  Decision
	MatchRule string // the GFWList rule that triggered the match (empty if direct)
}

// Router holds the parsed proxy-domain set and exposes Route(domain).
type Router struct {
	mu      sync.RWMutex
	domains map[string]bool // lowercased, normalized domain → true=proxy
	size    int
}

// New creates a Router, loading and parsing the GFWList rule file.
// If the local file is missing it will attempt to download from fileURL.
func New(filePath, fileURL string) (*Router, error) {
	r := &Router{
		domains: make(map[string]bool),
	}
	content, err := r.loadContent(filePath, fileURL)
	if err != nil {
		return nil, err
	}
	if content != "" {
		r.parseRules(content)
		r.size = len(r.domains)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return r, err // best-effort, rules are already loaded
		}
	}
	return r, nil
}

// Route returns the routing decision for a domain name. It performs
// case-insensitive exact and parent-domain matching against the loaded
// ruleset.
func (r *Router) Route(domain string) RouteResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	domain = normalize(domain)
	if r.domains[domain] {
		return RouteResult{Decision: RouteProxy, MatchRule: domain}
	}

	// Walk up parent domains: "a.b.c.example" → "b.c.example" → "c.example" → "example"
	parts := strings.Split(domain, ".")
	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[i:], ".")
		if r.domains[parent] {
			return RouteResult{Decision: RouteProxy, MatchRule: parent}
		}
	}

	return RouteResult{Decision: RouteDirect}
}

// Size returns the number of loaded proxy-domain rules.
func (r *Router) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

// Reload clears the current ruleset and re-loads from file/URL.
func (r *Router) Reload(filePath, fileURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.domains = make(map[string]bool)
	content, err := r.loadContent(filePath, fileURL)
	if err != nil {
		return err
	}
	r.parseRules(content)
	r.size = len(r.domains)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return err
	}
	return nil
}

// --- internal helpers ---

func (r *Router) loadContent(filePath, fileURL string) (string, error) {
	content := readFile(filePath)
	if content != "" {
		return content, nil
	}
	if fileURL != "" {
		return download(fileURL)
	}
	return "", nil
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// GFWList is base64-encoded; try decode, fall back to raw text.
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err == nil {
		return string(decoded)
	}
	return string(data)
}

func download(url string) (string, error) {
	resp, err := http.Get(url) //nolint:gosec // URL is user-configured
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	decoded, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// domainPattern matches domain names in AdBlock-style rules.
// Examples: "||example.com^", "|http://example.com", "example.com"
var domainPattern = regexp.MustCompile(
	`(?:\|\||\|)?([a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+(?:\.[a-zA-Z0-9_-]+)*)`,
)

func (r *Router) parseRules(content string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Stop at whitelist section.
		if strings.Contains(line, "Whitelist") {
			break
		}
		// Skip comments, empty lines, and metadata headers.
		if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") {
			continue
		}

		matches := domainPattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 {
				domain := normalize(match[1])
				r.domains[domain] = true
			}
		}
	}
}

func normalize(domain string) string {
	d := strings.ToLower(strings.TrimSpace(domain))
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "https://")
	return d
}
