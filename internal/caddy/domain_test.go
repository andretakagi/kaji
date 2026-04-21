package caddy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateDomainID(t *testing.T) {
	id := GenerateDomainID()
	if !strings.HasPrefix(id, "dom_") {
		t.Errorf("GenerateDomainID() = %q, want dom_ prefix", id)
	}
	// dom_ + 16 hex chars = 20 total
	if len(id) != 20 {
		t.Errorf("GenerateDomainID() length = %d, want 20", len(id))
	}
}

func TestGenerateDomainID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := GenerateDomainID()
		if seen[id] {
			t.Fatalf("duplicate ID on iteration %d: %s", i, id)
		}
		seen[id] = true
	}
}

func TestGenerateRuleID(t *testing.T) {
	id := GenerateRuleID()
	if !strings.HasPrefix(id, "rule_") {
		t.Errorf("GenerateRuleID() = %q, want rule_ prefix", id)
	}
	// rule_ + 16 hex chars = 21 total
	if len(id) != 21 {
		t.Errorf("GenerateRuleID() length = %d, want 21", len(id))
	}
}

func TestCaddyDomainID(t *testing.T) {
	cases := []struct {
		ruleID string
		want   string
	}{
		{"rule_abc123", "kaji_rule_abc123"},
		{"rule_0000000000000000", "kaji_rule_0000000000000000"},
	}
	for _, c := range cases {
		got := CaddyDomainID(c.ruleID)
		if got != c.want {
			t.Errorf("CaddyDomainID(%q) = %q, want %q", c.ruleID, got, c.want)
		}
	}
}

func TestMergeToggles_NilOverrides(t *testing.T) {
	defaults := DomainToggles{
		ForceHTTPS:  true,
		Compression: true,
	}
	result := MergeToggles(defaults, nil)
	if !result.ForceHTTPS {
		t.Error("expected ForceHTTPS from defaults")
	}
	if !result.Compression {
		t.Error("expected Compression from defaults")
	}
}

func TestMergeToggles_WithOverrides(t *testing.T) {
	defaults := DomainToggles{
		ForceHTTPS:  true,
		Compression: true,
	}
	overrides := &DomainToggles{
		ForceHTTPS:  false,
		Compression: false,
		AccessLog:   "custom.log",
	}
	result := MergeToggles(defaults, overrides)
	if result.ForceHTTPS {
		t.Error("expected ForceHTTPS=false from override")
	}
	if result.Compression {
		t.Error("expected Compression=false from override")
	}
	if result.AccessLog != "custom.log" {
		t.Errorf("expected AccessLog=%q from override, got %q", "custom.log", result.AccessLog)
	}
}

func TestParseReverseProxyConfig(t *testing.T) {
	original := ReverseProxyConfig{
		Upstream:          "localhost:8080",
		TLSSkipVerify:     true,
		WebSocketPassthru: true,
		LoadBalancing: LoadBalancing{
			Enabled:   true,
			Strategy:  "round_robin",
			Upstreams: []string{"localhost:8081", "localhost:8082"},
		},
	}

	data, err := MarshalReverseProxyConfig(original)
	if err != nil {
		t.Fatalf("MarshalReverseProxyConfig: %v", err)
	}

	parsed, err := ParseReverseProxyConfig(json.RawMessage(data))
	if err != nil {
		t.Fatalf("ParseReverseProxyConfig: %v", err)
	}

	if parsed.Upstream != original.Upstream {
		t.Errorf("Upstream = %q, want %q", parsed.Upstream, original.Upstream)
	}
	if !parsed.TLSSkipVerify {
		t.Error("expected TLSSkipVerify = true")
	}
	if !parsed.WebSocketPassthru {
		t.Error("expected WebSocketPassthru = true")
	}
	if !parsed.LoadBalancing.Enabled {
		t.Error("expected LoadBalancing.Enabled = true")
	}
	if parsed.LoadBalancing.Strategy != "round_robin" {
		t.Errorf("Strategy = %q, want round_robin", parsed.LoadBalancing.Strategy)
	}
	if len(parsed.LoadBalancing.Upstreams) != 2 {
		t.Fatalf("Upstreams length = %d, want 2", len(parsed.LoadBalancing.Upstreams))
	}
}

func TestParseStaticResponseConfig(t *testing.T) {
	original := StaticResponseConfig{
		StatusCode: "200",
		Body:       "Hello, World!",
		Headers: map[string][]string{
			"Content-Type": {"text/plain"},
			"X-Custom":     {"value1", "value2"},
		},
		Close: true,
	}

	data, err := MarshalStaticResponseConfig(original)
	if err != nil {
		t.Fatalf("MarshalStaticResponseConfig: %v", err)
	}

	parsed, err := ParseStaticResponseConfig(json.RawMessage(data))
	if err != nil {
		t.Fatalf("ParseStaticResponseConfig: %v", err)
	}

	if parsed.StatusCode != original.StatusCode {
		t.Errorf("StatusCode = %q, want %q", parsed.StatusCode, original.StatusCode)
	}
	if parsed.Body != original.Body {
		t.Errorf("Body = %q, want %q", parsed.Body, original.Body)
	}
	if !parsed.Close {
		t.Error("expected Close = true")
	}
	if len(parsed.Headers) != 2 {
		t.Fatalf("Headers length = %d, want 2", len(parsed.Headers))
	}
	if val, ok := parsed.Headers["Content-Type"]; !ok || len(val) != 1 || val[0] != "text/plain" {
		t.Errorf("Content-Type header mismatch")
	}
	if val, ok := parsed.Headers["X-Custom"]; !ok || len(val) != 2 {
		t.Errorf("X-Custom header mismatch")
	}
}

func TestParseStaticResponseConfigInvalid(t *testing.T) {
	_, err := ParseStaticResponseConfig(json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseRedirectConfig(t *testing.T) {
	original := RedirectConfig{
		TargetURL:    "https://example.com/new",
		StatusCode:   "301",
		PreservePath: true,
	}

	data, err := MarshalRedirectConfig(original)
	if err != nil {
		t.Fatalf("MarshalRedirectConfig: %v", err)
	}

	parsed, err := ParseRedirectConfig(json.RawMessage(data))
	if err != nil {
		t.Fatalf("ParseRedirectConfig: %v", err)
	}

	if parsed.TargetURL != original.TargetURL {
		t.Errorf("TargetURL = %q, want %q", parsed.TargetURL, original.TargetURL)
	}
	if parsed.StatusCode != original.StatusCode {
		t.Errorf("StatusCode = %q, want %q", parsed.StatusCode, original.StatusCode)
	}
	if !parsed.PreservePath {
		t.Error("expected PreservePath = true")
	}
}

func TestParseRedirectConfigInvalid(t *testing.T) {
	_, err := ParseRedirectConfig(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseFileServerConfig(t *testing.T) {
	original := FileServerConfig{
		Root:       "/var/www",
		Browse:     true,
		IndexNames: []string{"index.html", "index.htm"},
		Hide:       []string{".git", ".env"},
	}

	data, err := MarshalFileServerConfig(original)
	if err != nil {
		t.Fatalf("MarshalFileServerConfig: %v", err)
	}

	parsed, err := ParseFileServerConfig(json.RawMessage(data))
	if err != nil {
		t.Fatalf("ParseFileServerConfig: %v", err)
	}

	if parsed.Root != original.Root {
		t.Errorf("Root = %q, want %q", parsed.Root, original.Root)
	}
	if !parsed.Browse {
		t.Error("expected Browse = true")
	}
	if len(parsed.IndexNames) != 2 {
		t.Fatalf("IndexNames length = %d, want 2", len(parsed.IndexNames))
	}
	if parsed.IndexNames[0] != "index.html" || parsed.IndexNames[1] != "index.htm" {
		t.Errorf("IndexNames = %v, want [index.html index.htm]", parsed.IndexNames)
	}
	if len(parsed.Hide) != 2 {
		t.Fatalf("Hide length = %d, want 2", len(parsed.Hide))
	}
	if parsed.Hide[0] != ".git" || parsed.Hide[1] != ".env" {
		t.Errorf("Hide = %v, want [.git .env]", parsed.Hide)
	}
}

func TestParseFileServerConfigInvalid(t *testing.T) {
	_, err := ParseFileServerConfig(json.RawMessage(`{malformed`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
