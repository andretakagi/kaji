package caddy

import (
	"encoding/json"
	"strings"
	"testing"
)

// buildFullConfig wraps a slice of route JSON blobs in a full Caddy config
// structure, optionally setting an ACME email, auto_https, and metrics.
func buildFullConfig(t *testing.T, routes []json.RawMessage, acmeEmail string, autoHTTPS string, metrics bool, perHost bool) json.RawMessage {
	t.Helper()

	var autoHTTPSObj any
	switch autoHTTPS {
	case "off":
		autoHTTPSObj = map[string]bool{"disable": true}
	case "disable_redirects":
		autoHTTPSObj = map[string]bool{"disable_redirects": true}
	default:
		autoHTTPSObj = nil
	}

	srv := map[string]any{
		"routes": routes,
	}
	if autoHTTPSObj != nil {
		srv["automatic_https"] = autoHTTPSObj
	}
	if metrics {
		metricsObj := map[string]any{}
		if perHost {
			metricsObj["per_host"] = true
		}
		srv["metrics"] = metricsObj
	}

	cfg := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": srv,
				},
			},
		},
	}

	if acmeEmail != "" {
		cfg["apps"].(map[string]any)["tls"] = map[string]any{
			"automation": map[string]any{
				"policies": []map[string]any{
					{
						"issuers": []map[string]any{
							{"module": "acme", "email": acmeEmail},
						},
					},
				},
			},
		}
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}
	return json.RawMessage(data)
}

func minimalRoute(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}
	return raw
}

// assertContains fails the test if s does not contain sub.
func assertContains(t *testing.T, s, sub, label string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%s: expected output to contain %q\nFull output:\n%s", label, sub, s)
	}
}

func assertNotContains(t *testing.T, s, sub, label string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Errorf("%s: expected output NOT to contain %q\nFull output:\n%s", label, sub, s)
	}
}

// TestGenerateCaddyfileMinimal verifies a route with no toggles produces a
// minimal site block with only a reverse_proxy directive.
func TestGenerateCaddyfileMinimal(t *testing.T) {
	cfg := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "example.com {", "minimal")
	assertContains(t, out, "reverse_proxy localhost:8080", "minimal")
	assertNotContains(t, out, "encode gzip", "minimal")
	assertNotContains(t, out, "header {", "minimal")
}

// TestGenerateCaddyfileGlobalOptions verifies that the global options block is
// produced correctly when ACME email, auto_https off, and metrics are set.
func TestGenerateCaddyfileGlobalOptions(t *testing.T) {
	cfg := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "admin@example.com", "off", true, true)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "{\n", "global options block open")
	assertContains(t, out, "email admin@example.com", "ACME email")
	assertContains(t, out, "auto_https off", "auto_https off")
	assertContains(t, out, "metrics {", "metrics block")
	assertContains(t, out, "per_host", "per_host metrics")
}

// TestGenerateCaddyfileGlobalOptionsNoEmail verifies that the global options
// block is still written when auto_https is off but no ACME email is set.
func TestGenerateCaddyfileGlobalOptionsNoEmail(t *testing.T) {
	cfg := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "auto_https off", "auto_https off")
	assertNotContains(t, out, "email ", "no ACME email")
}

// TestGenerateCaddyfileMetricsNoPerHost verifies that the metrics directive
// appears without a nested block when per_host is false.
func TestGenerateCaddyfileMetricsNoPerHost(t *testing.T) {
	cfg := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "off", true, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "\tmetrics\n", "metrics bare directive")
	assertNotContains(t, out, "per_host", "no per_host")
}

// TestGenerateCaddyfileCompression verifies that enable gzip+zstd encoding
// produces the encode directive.
func TestGenerateCaddyfileCompression(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{Compression: true},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "encode gzip zstd", "compression")
}

// TestGenerateCaddyfileSecurityHeaders verifies that security header
// directives are written for the SecurityHeaders toggle.
func TestGenerateCaddyfileSecurityHeaders(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{SecurityHeaders: true},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "header {", "security headers block")
	assertContains(t, out, "Strict-Transport-Security", "HSTS")
	assertContains(t, out, "X-Content-Type-Options", "X-Content-Type-Options")
	assertContains(t, out, "X-Frame-Options", "X-Frame-Options")
	assertContains(t, out, "Referrer-Policy", "Referrer-Policy")
}

// TestGenerateCaddyfileCORSWildcard verifies that CORS with no origins
// produces a wildcard Access-Control-Allow-Origin header block.
func TestGenerateCaddyfileCORSWildcard(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			CORS: CORSOpts{Enabled: true, AllowedOrigins: []string{}},
		},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, `Access-Control-Allow-Origin "*"`, "CORS wildcard origin")
	assertContains(t, out, "Access-Control-Allow-Methods", "CORS methods")
}

// TestGenerateCaddyfileCORSSingleOrigin verifies that a single allowed origin
// is written into the header block.
func TestGenerateCaddyfileCORSSingleOrigin(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			CORS: CORSOpts{
				Enabled:        true,
				AllowedOrigins: []string{"https://frontend.example.com"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, `Access-Control-Allow-Origin "https://frontend.example.com"`, "CORS single origin")
}

// TestGenerateCaddyfileBasicAuth verifies that basic_auth block is written
// with username and password hash.
func TestGenerateCaddyfileBasicAuth(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			BasicAuth: BasicAuth{
				Enabled:      true,
				Username:     "admin",
				PasswordHash: "$2a$14$somehash",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "basic_auth {", "basic_auth block")
	assertContains(t, out, "admin $2a$14$somehash", "basic_auth credentials")
}

// TestGenerateCaddyfileTLSSkipVerify verifies that TLSSkipVerify produces
// the transport http block with tls_insecure_skip_verify.
func TestGenerateCaddyfileTLSSkipVerify(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8443",
		Toggles:  RouteToggles{TLSSkipVerify: true},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "reverse_proxy localhost:8443 {", "reverse_proxy block for TLS skip")
	assertContains(t, out, "transport http {", "transport http block")
	assertContains(t, out, "tls_insecure_skip_verify", "tls_insecure_skip_verify")
}

// TestGenerateCaddyfileWebSocket verifies that WebSocketPassthru produces
// flush_interval -1 in the reverse_proxy block.
func TestGenerateCaddyfileWebSocket(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{WebSocketPassthru: true},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "reverse_proxy localhost:8080 {", "reverse_proxy block for WS")
	assertContains(t, out, "flush_interval -1", "flush_interval for websocket")
}

// TestGenerateCaddyfileLoadBalancing verifies that load balancing produces
// the correct lb_policy directive and includes all upstreams.
func TestGenerateCaddyfileLoadBalancing(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			LoadBalancing: LoadBalancing{
				Enabled:   true,
				Strategy:  "round_robin",
				Upstreams: []string{"localhost:8081", "localhost:8082"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "localhost:8080", "primary upstream")
	assertContains(t, out, "localhost:8081", "lb upstream 1")
	assertContains(t, out, "localhost:8082", "lb upstream 2")
	assertContains(t, out, "lb_policy round_robin", "lb_policy")
}

// TestGenerateCaddyfileLoadBalancingFirst verifies that strategy "first"
// also emits fail_duration and max_fails.
func TestGenerateCaddyfileLoadBalancingFirst(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			LoadBalancing: LoadBalancing{
				Enabled:   true,
				Strategy:  "first",
				Upstreams: []string{"localhost:8081"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "lb_policy first", "lb_policy first")
	assertContains(t, out, "fail_duration 30s", "fail_duration")
	assertContains(t, out, "max_fails 3", "max_fails")
}

// TestGenerateCaddyfileForceHTTPS verifies that ForceHTTPS produces a
// separate http:// redirect block before the main site block.
func TestGenerateCaddyfileForceHTTPS(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{ForceHTTPS: true},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "", "off", false, false)
	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "http://example.com {", "http redirect block")
	assertContains(t, out, "redir https://{host}{uri} 301", "redirect directive")
	assertContains(t, out, "example.com {", "main site block")
	assertContains(t, out, "reverse_proxy localhost:8080", "reverse_proxy in main block")

	// http:// redirect block must appear before the main site block
	httpIdx := strings.Index(out, "http://example.com {")
	mainIdx := strings.Index(out, "\nexample.com {")
	if httpIdx < 0 {
		t.Error("http redirect block not found")
	} else if mainIdx < 0 {
		t.Error("main site block not found")
	} else if httpIdx > mainIdx {
		t.Error("http redirect block should appear before the main site block")
	}
}

// TestExtractCaddyfileSettingsMinimal verifies that a minimal config is
// parsed with correct defaults.
func TestExtractCaddyfileSettingsMinimal(t *testing.T) {
	cfg := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "", false, false)
	settings, err := ExtractCaddyfileSettings(cfg)
	if err != nil {
		t.Fatalf("ExtractCaddyfileSettings failed: %v", err)
	}

	if settings.ACMEEmail != "" {
		t.Errorf("ACMEEmail = %q, want empty", settings.ACMEEmail)
	}
	if settings.Toggles.AutoHTTPS != "on" {
		t.Errorf("AutoHTTPS = %q, want on", settings.Toggles.AutoHTTPS)
	}
	if settings.Toggles.PrometheusMetrics {
		t.Error("PrometheusMetrics should be false")
	}
	if settings.RouteCount != 1 {
		t.Errorf("RouteCount = %d, want 1", settings.RouteCount)
	}
}

// TestExtractCaddyfileSettingsACMEEmail verifies that the ACME email is
// parsed from TLS policies.
func TestExtractCaddyfileSettingsACMEEmail(t *testing.T) {
	cfg := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "admin@example.com", "", false, false)
	settings, err := ExtractCaddyfileSettings(cfg)
	if err != nil {
		t.Fatalf("ExtractCaddyfileSettings failed: %v", err)
	}

	if settings.ACMEEmail != "admin@example.com" {
		t.Errorf("ACMEEmail = %q, want admin@example.com", settings.ACMEEmail)
	}
}

// TestExtractCaddyfileSettingsAutoHTTPSOff verifies that auto_https off is
// parsed correctly.
func TestExtractCaddyfileSettingsAutoHTTPSOff(t *testing.T) {
	cfg := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "off", false, false)
	settings, err := ExtractCaddyfileSettings(cfg)
	if err != nil {
		t.Fatalf("ExtractCaddyfileSettings failed: %v", err)
	}

	if settings.Toggles.AutoHTTPS != "off" {
		t.Errorf("AutoHTTPS = %q, want off", settings.Toggles.AutoHTTPS)
	}
}

// TestExtractCaddyfileSettingsMetrics verifies that prometheus metrics flags
// are parsed correctly.
func TestExtractCaddyfileSettingsMetrics(t *testing.T) {
	cfg := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "off", true, true)
	settings, err := ExtractCaddyfileSettings(cfg)
	if err != nil {
		t.Fatalf("ExtractCaddyfileSettings failed: %v", err)
	}

	if !settings.Toggles.PrometheusMetrics {
		t.Error("PrometheusMetrics should be true")
	}
	if !settings.Toggles.PerHostMetrics {
		t.Error("PerHostMetrics should be true")
	}
}

// TestExtractCaddyfileSettingsRouteCount verifies that multiple routes are
// counted correctly.
func TestExtractCaddyfileSettingsRouteCount(t *testing.T) {
	route2, err := BuildRoute(RouteParams{
		Domain:   "other.example.com",
		Upstream: "localhost:9090",
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{minimalRoute(t), route2}, "", "off", false, false)
	settings, err := ExtractCaddyfileSettings(cfg)
	if err != nil {
		t.Fatalf("ExtractCaddyfileSettings failed: %v", err)
	}

	if settings.RouteCount != 2 {
		t.Errorf("RouteCount = %d, want 2", settings.RouteCount)
	}
}

// TestExtractCaddyfileSettingsTogglesParsed verifies that route-level toggles
// do not influence the global settings struct (which only tracks global state).
func TestExtractCaddyfileSettingsTogglesParsed(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			Compression:     true,
			SecurityHeaders: true,
		},
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	cfg := buildFullConfig(t, []json.RawMessage{raw}, "me@example.com", "disable_redirects", false, false)
	settings, err := ExtractCaddyfileSettings(cfg)
	if err != nil {
		t.Fatalf("ExtractCaddyfileSettings failed: %v", err)
	}

	if settings.ACMEEmail != "me@example.com" {
		t.Errorf("ACMEEmail = %q, want me@example.com", settings.ACMEEmail)
	}
	if settings.Toggles.AutoHTTPS != "disable_redirects" {
		t.Errorf("AutoHTTPS = %q, want disable_redirects", settings.Toggles.AutoHTTPS)
	}
	if settings.RouteCount != 1 {
		t.Errorf("RouteCount = %d, want 1", settings.RouteCount)
	}
}
