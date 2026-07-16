package router

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRouter_ExactMatch(t *testing.T) {
	r := &Router{
		domains: map[string]bool{
			"google.com": true,
		},
	}

	result := r.Route("google.com")
	if result.Decision != RouteProxy {
		t.Error("expected RouteProxy for exact match")
	}
	if result.MatchRule != "google.com" {
		t.Errorf("MatchRule = %q, want %q", result.MatchRule, "google.com")
	}
}

func TestRouter_ParentDomainMatch(t *testing.T) {
	r := &Router{
		domains: map[string]bool{
			"google.com": true,
		},
	}

	result := r.Route("www.google.com")
	if result.Decision != RouteProxy {
		t.Error("expected RouteProxy for subdomain match")
	}
	if result.MatchRule != "google.com" {
		t.Errorf("MatchRule = %q, want %q", result.MatchRule, "google.com")
	}
}

func TestRouter_DeepSubdomainMatch(t *testing.T) {
	r := &Router{
		domains: map[string]bool{
			"example.com": true,
		},
	}

	result := r.Route("a.b.c.example.com")
	if result.Decision != RouteProxy {
		t.Error("expected RouteProxy for deep subdomain")
	}
}

func TestRouter_Direct(t *testing.T) {
	r := &Router{
		domains: map[string]bool{
			"google.com": true,
		},
	}

	result := r.Route("baidu.com")
	if result.Decision != RouteDirect {
		t.Error("expected RouteDirect for unmatched domain")
	}
	if result.MatchRule != "" {
		t.Errorf("MatchRule = %q, want empty", result.MatchRule)
	}
}

func TestRouter_CaseInsensitive(t *testing.T) {
	r := &Router{
		domains: map[string]bool{
			"google.com": true,
		},
	}

	result := r.Route("GOOGLE.COM")
	if result.Decision != RouteProxy {
		t.Error("expected RouteProxy for uppercase match")
	}
}

func TestRouter_HttpPrefix(t *testing.T) {
	r := &Router{
		domains: map[string]bool{
			"blocked.com": true,
		},
	}

	result := r.Route("http://blocked.com")
	if result.Decision != RouteProxy {
		t.Error("expected RouteProxy with http:// prefix")
	}
}

func TestRouter_HttpsPrefix(t *testing.T) {
	r := &Router{
		domains: map[string]bool{
			"blocked.com": true,
		},
	}

	result := r.Route("https://blocked.com")
	if result.Decision != RouteProxy {
		t.Error("expected RouteProxy with https:// prefix")
	}
}

func TestRouter_EmptyRuleset(t *testing.T) {
	r := &Router{
		domains: make(map[string]bool),
	}

	result := r.Route("google.com")
	if result.Decision != RouteDirect {
		t.Error("expected RouteDirect for empty ruleset")
	}
}

func TestRouter_Size(t *testing.T) {
	r := &Router{
		domains: map[string]bool{
			"a.com": true,
			"b.com": true,
			"c.com": true,
		},
		size: 3,
	}

	if r.Size() != 3 {
		t.Errorf("Size = %d, want 3", r.Size())
	}
}

func TestRouter_Normalize(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"google.com", "google.com"},
		{"GOOGLE.COM", "google.com"},
		{" Google.Com ", "google.com"},
		{"http://example.com", "example.com"},
		{"https://example.com", "example.com"},
		{"HTTPS://Example.Com", "example.com"},
	}

	for _, tt := range tests {
		got := normalize(tt.in)
		if got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRouter_ConcurrentAccess(t *testing.T) {
	r := &Router{
		domains: map[string]bool{
			"google.com":   true,
			"github.com":   true,
			"twitter.com":  true,
			"facebook.com": true,
		},
	}

	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				r.Route("google.com")
				r.Route("baidu.com")
				r.Route("www.github.com")
				r.Size()
			}
			done <- true
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestParseRules_GFWListSample(t *testing.T) {
	// Sample GFWList content with various rule formats.
	content := `
! This is a comment
[AutoProxy 0.2.9]
||google.com^
||twitter.com^
|http://facebook.com
||youtube.com^
||www.dropbox.com^
!Whitelist starts here
@@||cn.example.com^
`
	r := &Router{domains: make(map[string]bool)}
	r.parseRules(content)

	want := map[string]bool{
		"google.com":       true,
		"twitter.com":      true,
		"facebook.com":     true,
		"youtube.com":      true,
		"www.dropbox.com":  true, // captured as-is from rule
	}

	for domain, expected := range want {
		if r.domains[domain] != expected {
			t.Errorf("domains[%q] = %v, want %v", domain, r.domains[domain], expected)
		}
	}

	// Whitelist section should stop parsing — "cn.example.com" NOT added.
	if r.domains["cn.example.com"] {
		t.Error("cn.example.com should NOT be in rules (whitelist section)")
	}
}

func TestNew_EmptyRuleFile(t *testing.T) {
	dir := t.TempDir()
	rulePath := filepath.Join(dir, "nonexistent.txt")

	r, err := New(rulePath, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if r.Size() != 0 {
		t.Errorf("expected empty ruleset, got %d entries", r.Size())
	}
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	rulePath := filepath.Join(dir, "rules.txt")

	// Create initial rule file.
	initial := rawContent("||google.com^")
	if err := os.WriteFile(rulePath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := New(rulePath, "")
	if err != nil {
		t.Fatal(err)
	}

	// Verify initial rules.
	if r.Route("google.com").Decision != RouteProxy {
		t.Error("google.com should be proxy after initial load")
	}
	if r.Route("baidu.com").Decision != RouteDirect {
		t.Error("baidu.com should be direct")
	}

	// Update rule file.
	updated := rawContent("||baidu.com^")
	if err := os.WriteFile(rulePath, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	if err := r.Reload(rulePath, ""); err != nil {
		t.Fatal(err)
	}

	// Verify updated rules.
	if r.Route("google.com").Decision != RouteDirect {
		t.Error("google.com should be direct after reload (rule removed)")
	}
	if r.Route("baidu.com").Decision != RouteProxy {
		t.Error("baidu.com should be proxy after reload (rule added)")
	}
}

// rawContent returns the input unchanged — used for tests where GFWList file
// contains plain text rather than base64. The readFile helper will fall
// through to raw text when base64 decode fails.
func rawContent(s string) string {
	return s
}

func TestRouter_SubdomainNotPartialMatch(t *testing.T) {
	// "co.jp" should NOT match "example.co.jp" unless "co.jp" is explicitly in rules.
	r := &Router{
		domains: map[string]bool{
			"example.com": true,
		},
	}

	// "other.co" ends with "co" which is a suffix of "go.co" — no.
	result := r.Route("other.co")
	if result.Decision != RouteDirect {
		t.Error("other.co should be direct (no suffix match)")
	}

	// "notexample.com" contains "example.com" but is not a subdomain.
	result = r.Route("notexample.com")
	if result.Decision != RouteDirect {
		t.Error("notexample.com should be direct (not a real subdomain)")
	}
}
