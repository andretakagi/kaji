package caddy

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return json.RawMessage(data)
}

func unmarshalRoute(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal route: %v", err)
	}
	return m
}

func TestBuildRuleDomain_RootRule(t *testing.T) {
	rpCfg := mustMarshal(t, ReverseProxyConfig{Upstream: "localhost:3000"})
	rule := RuleBuildParams{
		RuleID:        "rule_abc123",
		MatchType:     "",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}

	result, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)

	if route["@id"] != "kaji_rule_abc123" {
		t.Errorf("@id = %v, want kaji_rule_abc123", route["@id"])
	}
	if route["terminal"] != true {
		t.Errorf("terminal = %v, want true", route["terminal"])
	}

	matches := route["match"].([]any)
	matchObj := matches[0].(map[string]any)
	hosts := matchObj["host"].([]any)
	if hosts[0] != "example.com" {
		t.Errorf("host = %v, want example.com", hosts[0])
	}

	handlers := route["handle"].([]any)
	lastHandler := handlers[len(handlers)-1].(map[string]any)
	if lastHandler["handler"] != "reverse_proxy" {
		t.Errorf("last handler = %v, want reverse_proxy", lastHandler["handler"])
	}
}

func TestBuildRuleDomain_SubdomainHost(t *testing.T) {
	rpCfg := mustMarshal(t, ReverseProxyConfig{Upstream: "localhost:8080"})
	rule := RuleBuildParams{
		RuleID:        "rule_sub1",
		MatchType:     "",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}

	result, err := BuildRuleDomain("api.example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	matches := route["match"].([]any)
	matchObj := matches[0].(map[string]any)
	hosts := matchObj["host"].([]any)
	if hosts[0] != "api.example.com" {
		t.Errorf("host = %v, want api.example.com", hosts[0])
	}
}

func TestBuildRuleDomain_PathPrefix(t *testing.T) {
	rpCfg := mustMarshal(t, ReverseProxyConfig{Upstream: "localhost:9000"})
	rule := RuleBuildParams{
		RuleID:        "rule_prefix1",
		MatchType:     "path",
		PathMatch:     "prefix",
		MatchValue:    "/api",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}

	result, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	matches := route["match"].([]any)
	matchObj := matches[0].(map[string]any)
	paths := matchObj["path"].([]any)
	if paths[0] != "/api/*" {
		t.Errorf("path = %v, want /api/*", paths[0])
	}
}

func TestBuildRuleDomain_PathExact(t *testing.T) {
	rpCfg := mustMarshal(t, ReverseProxyConfig{Upstream: "localhost:9000"})
	rule := RuleBuildParams{
		RuleID:        "rule_exact1",
		MatchType:     "path",
		PathMatch:     "exact",
		MatchValue:    "/health",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}

	result, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	matches := route["match"].([]any)
	matchObj := matches[0].(map[string]any)
	paths := matchObj["path"].([]any)
	if paths[0] != "/health" {
		t.Errorf("path = %v, want /health", paths[0])
	}
}

func TestBuildRuleDomain_PathRegex(t *testing.T) {
	rpCfg := mustMarshal(t, ReverseProxyConfig{Upstream: "localhost:9000"})
	rule := RuleBuildParams{
		RuleID:        "rule_regex1",
		MatchType:     "path",
		PathMatch:     "regex",
		MatchValue:    `^/users/\d+`,
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}

	result, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	matches := route["match"].([]any)
	matchObj := matches[0].(map[string]any)
	pathRegexp := matchObj["path_regexp"].(map[string]any)
	if pathRegexp["pattern"] != `^/users/\d+` {
		t.Errorf("path_regexp pattern = %v, want ^/users/\\d+", pathRegexp["pattern"])
	}
}

func TestBuildRuleDomain_WithToggles(t *testing.T) {
	rpCfg := mustMarshal(t, ReverseProxyConfig{Upstream: "localhost:3000"})
	rule := RuleBuildParams{
		RuleID:        "rule_toggles1",
		MatchType:     "",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}
	toggles := DomainToggles{
		ForceHTTPS:  true,
		Compression: true,
	}

	result, err := BuildRuleDomain("example.com", rule, toggles, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)

	if len(handlers) < 3 {
		t.Fatalf("expected at least 3 handlers, got %d", len(handlers))
	}

	// First handler: force HTTPS subroute
	first := handlers[0].(map[string]any)
	if first["handler"] != "subroute" {
		t.Errorf("first handler = %v, want subroute (force HTTPS)", first["handler"])
	}

	// Second handler: encode (compression)
	second := handlers[1].(map[string]any)
	if second["handler"] != "encode" {
		t.Errorf("second handler = %v, want encode (compression)", second["handler"])
	}

	// Last handler: reverse_proxy
	last := handlers[len(handlers)-1].(map[string]any)
	if last["handler"] != "reverse_proxy" {
		t.Errorf("last handler = %v, want reverse_proxy", last["handler"])
	}
}

func TestBuildRuleDomain_UnsupportedHandler(t *testing.T) {
	rule := RuleBuildParams{
		RuleID:        "rule_bad1",
		HandlerType:   "unknown",
		HandlerConfig: json.RawMessage(`{}`),
	}

	_, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for unsupported handler type")
	}

	want := `unsupported handler type: "unknown"`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestBuildRuleDomain_ReverseProxyConfig(t *testing.T) {
	rpCfg := mustMarshal(t, ReverseProxyConfig{
		Upstream:          "localhost:4000",
		TLSSkipVerify:     true,
		WebSocketPassthru: true,
		LoadBalancing: LoadBalancing{
			Enabled:   true,
			Strategy:  "least_conn",
			Upstreams: []string{"localhost:4001", "localhost:4002"},
		},
	})
	rule := RuleBuildParams{
		RuleID:        "rule_rp1",
		MatchType:     "",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}

	result, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	rp := handlers[len(handlers)-1].(map[string]any)

	// Check upstreams (primary + 2 extra)
	upstreams := rp["upstreams"].([]any)
	if len(upstreams) != 3 {
		t.Errorf("upstreams count = %d, want 3", len(upstreams))
	}

	// Check TLS skip verify
	transport := rp["transport"].(map[string]any)
	tls := transport["tls"].(map[string]any)
	if tls["insecure_skip_verify"] != true {
		t.Errorf("insecure_skip_verify = %v, want true", tls["insecure_skip_verify"])
	}

	// Check websocket passthrough
	flushInterval, ok := rp["flush_interval"]
	if !ok {
		t.Fatal("flush_interval not set")
	}
	if flushInterval.(float64) != -1 {
		t.Errorf("flush_interval = %v, want -1", flushInterval)
	}

	// Check load balancing
	lb := rp["load_balancing"].(map[string]any)
	sp := lb["selection_policy"].(map[string]any)
	if sp["policy"] != "least_conn" {
		t.Errorf("load balancing policy = %v, want least_conn", sp["policy"])
	}
}

// Error case tests

func TestBuildRuleDomain_EmptyDomainName(t *testing.T) {
	rpCfg := mustMarshal(t, ReverseProxyConfig{Upstream: "localhost:3000"})
	rule := RuleBuildParams{
		RuleID:        "rule_nodomain",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}

	_, err := BuildRuleDomain("", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for empty domain name")
	}

	want := "domain name is required"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestBuildRuleDomain_UnsupportedHandlerType(t *testing.T) {
	rule := RuleBuildParams{
		RuleID:        "rule_badhandler",
		HandlerType:   "unknown_handler",
		HandlerConfig: json.RawMessage(`{}`),
	}

	_, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for unsupported handler type")
	}

	want := `unsupported handler type: "unknown_handler"`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestBuildRuleDomain_MalformedReverseProxyConfig(t *testing.T) {
	rule := RuleBuildParams{
		RuleID:        "rule_badconfig",
		HandlerType:   "reverse_proxy",
		HandlerConfig: json.RawMessage(`{invalid json}`),
	}

	_, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for malformed config")
	}

	if !strings.Contains(err.Error(), "parsing reverse proxy config") {
		t.Errorf("error = %q, want to contain 'parsing reverse proxy config'", err.Error())
	}
}

func TestBuildRuleDomain_EmptyReverseProxyUpstream(t *testing.T) {
	rpCfg := mustMarshal(t, ReverseProxyConfig{Upstream: ""})
	rule := RuleBuildParams{
		RuleID:        "rule_noupstream",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}

	_, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for empty upstream")
	}

	want := "upstream is required"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestBuildRuleDomain_MalformedRedirectConfig(t *testing.T) {
	rule := RuleBuildParams{
		RuleID:        "rule_badredirect",
		HandlerType:   "redirect",
		HandlerConfig: json.RawMessage(`{bad`),
	}

	_, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for malformed redirect config")
	}

	if !strings.Contains(err.Error(), "parsing redirect config") {
		t.Errorf("error = %q, want to contain 'parsing redirect config'", err.Error())
	}
}

func TestBuildRuleDomain_EmptyRedirectTargetURL(t *testing.T) {
	rdCfg := mustMarshal(t, RedirectConfig{TargetURL: ""})
	rule := RuleBuildParams{
		RuleID:        "rule_notarget",
		HandlerType:   "redirect",
		HandlerConfig: rdCfg,
	}

	_, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for empty target URL")
	}

	want := "target URL is required"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestBuildRuleDomain_MalformedFileServerConfig(t *testing.T) {
	rule := RuleBuildParams{
		RuleID:        "rule_badfs",
		HandlerType:   "file_server",
		HandlerConfig: json.RawMessage(`{broken`),
	}

	_, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for malformed file server config")
	}

	if !strings.Contains(err.Error(), "parsing file server config") {
		t.Errorf("error = %q, want to contain 'parsing file server config'", err.Error())
	}
}

func TestBuildRuleDomain_EmptyFileServerRoot(t *testing.T) {
	fsCfg := mustMarshal(t, FileServerConfig{Root: ""})
	rule := RuleBuildParams{
		RuleID:        "rule_noroot",
		HandlerType:   "file_server",
		HandlerConfig: fsCfg,
	}

	_, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for empty root directory")
	}

	want := "root directory is required"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestBuildRuleDomain_MalformedStaticResponseConfig(t *testing.T) {
	rule := RuleBuildParams{
		RuleID:        "rule_badsr",
		HandlerType:   "static_response",
		HandlerConfig: json.RawMessage(`{malformed`),
	}

	_, err := BuildRuleDomain("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for malformed static response config")
	}

	if !strings.Contains(err.Error(), "parsing static response config") {
		t.Errorf("error = %q, want to contain 'parsing static response config'", err.Error())
	}
}
