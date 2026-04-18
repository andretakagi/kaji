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

func TestCaddyRouteID(t *testing.T) {
	cases := []struct {
		ruleID string
		want   string
	}{
		{"rule_abc123", "kaji_rule_abc123"},
		{"rule_0000000000000000", "kaji_rule_0000000000000000"},
	}
	for _, c := range cases {
		got := CaddyRouteID(c.ruleID)
		if got != c.want {
			t.Errorf("CaddyRouteID(%q) = %q, want %q", c.ruleID, got, c.want)
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
