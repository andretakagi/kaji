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

func TestParseCaddyfileAdminAddr(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:  "no global block",
			input: "example.com {\n\treverse_proxy localhost:8080\n}",
			want:  "",
		},
		{
			name:  "admin with address",
			input: "{\n\tadmin 127.0.0.1:2019\n}\n\nexample.com {\n\treverse_proxy localhost:8080\n}",
			want:  "127.0.0.1:2019",
		},
		{
			name:  "admin off is ignored",
			input: "{\n\tadmin off\n}\n",
			want:  "",
		},
		{
			name:  "admin with sub-block brace is ignored",
			input: "{\n\tadmin {\n\t\torigins localhost\n\t}\n}\n",
			want:  "",
		},
		{
			name:  "admin at custom port",
			input: "{\n\tadmin :9999\n}\n",
			want:  ":9999",
		},
		{
			name:  "comment-only lines are skipped",
			input: "{\n\t# this is a comment\n\tadmin 0.0.0.0:2019\n}\n",
			want:  "0.0.0.0:2019",
		},
		{
			name:  "blank lines are skipped",
			input: "{\n\n\n\tadmin 10.0.0.1:2020\n}\n",
			want:  "10.0.0.1:2020",
		},
		{
			name:  "non-brace first non-empty line means no global block",
			input: "example.com\nreverse_proxy localhost:8080\n",
			want:  "",
		},
		{
			name:  "admin inside nested block at depth > 1 is ignored",
			input: "{\n\tlog {\n\t\tadmin localhost:2019\n\t}\n}\n",
			want:  "",
		},
		{
			name:  "admin address after other directives",
			input: "{\n\temail admin@example.com\n\tadmin unix//run/caddy.sock\n}\n",
			want:  "unix//run/caddy.sock",
		},
		{
			name:  "global block closes before admin found",
			input: "{\n\temail admin@example.com\n}\n",
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCaddyfileAdminAddr(tc.input)
			if got != tc.want {
				t.Errorf("ParseCaddyfileAdminAddr() = %q, want %q", got, tc.want)
			}
		})
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
		Toggles: RouteToggles{Headers: HeadersConfig{
			Enabled:  true,
			Response: ResponseHeaders{Security: true},
		}},
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
			Headers: HeadersConfig{Enabled: true, Response: ResponseHeaders{CORS: true, CORSOrigins: []string{}}},
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
			Headers: HeadersConfig{
				Enabled:  true,
				Response: ResponseHeaders{CORS: true, CORSOrigins: []string{"https://frontend.example.com"}},
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
			Compression: true,
			Headers: HeadersConfig{
				Enabled:  true,
				Response: ResponseHeaders{Security: true},
			},
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

// buildFullConfigWithLogging extends buildFullConfig by injecting a logging
// section into the top-level Caddy JSON config.
func buildFullConfigWithLogging(t *testing.T, base json.RawMessage, logging map[string]any) json.RawMessage {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(base, &m); err != nil {
		t.Fatalf("failed to unmarshal base config: %v", err)
	}
	m["logging"] = map[string]any{"logs": logging}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal config with logging: %v", err)
	}
	return json.RawMessage(data)
}

// buildMultiServerConfig creates a Caddy JSON config with multiple named
// servers, each containing one route.
func buildMultiServerConfig(t *testing.T, servers map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	srvMap := make(map[string]any)
	for name, route := range servers {
		srvMap[name] = map[string]any{
			"routes": []json.RawMessage{route},
		}
	}
	cfg := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": srvMap,
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal multi-server config: %v", err)
	}
	return json.RawMessage(data)
}

// TestGenerateCaddyfileLogFileWriter verifies that a named logger with a file
// writer and roll settings produces the correct global log block.
func TestGenerateCaddyfileLogFileWriter(t *testing.T) {
	base := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "", false, false)
	cfg := buildFullConfigWithLogging(t, base, map[string]any{
		"mylog": map[string]any{
			"writer": map[string]any{
				"output":        "file",
				"filename":      "/var/log/caddy/app.log",
				"roll_size_mb":  50,
				"roll_keep":     3,
				"roll_keep_for": 3600000000000 * 24 * 7, // 7 days in nanoseconds
			},
			"level": "WARN",
		},
	})

	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "log mylog {", "named logger block")
	assertContains(t, out, "output file /var/log/caddy/app.log {", "file output with roll block")
	assertContains(t, out, "roll_size 50MiB", "roll_size")
	assertContains(t, out, "roll_keep 3", "roll_keep")
	assertContains(t, out, "roll_keep_for 168h", "roll_keep_for 7 days")
	assertContains(t, out, "level WARN", "log level")
}

// TestGenerateCaddyfileLogStdout verifies that a logger with stdout output
// produces `output stdout` without a nested block.
func TestGenerateCaddyfileLogStdout(t *testing.T) {
	base := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "", false, false)
	cfg := buildFullConfigWithLogging(t, base, map[string]any{
		"console": map[string]any{
			"writer": map[string]any{
				"output": "stdout",
			},
			"encoder": map[string]any{
				"format": "console",
			},
			"level": "DEBUG",
		},
	})

	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "log console {", "named logger block")
	assertContains(t, out, "output stdout", "stdout output")
	assertContains(t, out, "format console", "console encoder")
	assertContains(t, out, "level DEBUG", "debug level")
}

// TestGenerateCaddyfileLogIncludeExclude verifies that include/exclude
// filters appear in the global log block.
func TestGenerateCaddyfileLogIncludeExclude(t *testing.T) {
	base := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "", false, false)
	cfg := buildFullConfigWithLogging(t, base, map[string]any{
		"filtered": map[string]any{
			"writer": map[string]any{
				"output": "stderr",
			},
			"level":   "INFO",
			"include": []string{"http.log.access"},
			"exclude": []string{"http.log.access.srv0"},
		},
	})

	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "log filtered {", "named logger block")
	assertContains(t, out, "output stderr", "stderr output")
	assertContains(t, out, "include http.log.access", "include filter")
	assertContains(t, out, "exclude http.log.access.srv0", "exclude filter")
}

// TestGenerateCaddyfileLogDefaultWithExtras verifies that a default logger
// with extras (not just DEBUG level) gets written as a full log block.
func TestGenerateCaddyfileLogDefaultWithExtras(t *testing.T) {
	base := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "", false, false)
	cfg := buildFullConfigWithLogging(t, base, map[string]any{
		"default": map[string]any{
			"writer": map[string]any{
				"output":   "file",
				"filename": "/var/log/caddy/default.log",
			},
			"level": "DEBUG",
		},
	})

	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	// Default logger with extras should use bare "log {" (no name)
	assertContains(t, out, "log {", "default logger block (no name)")
	assertContains(t, out, "output file /var/log/caddy/default.log", "default file output")
}

// TestGenerateCaddyfileLogFileNoRollSettings verifies that a file writer
// without roll settings produces a simple one-line output directive.
func TestGenerateCaddyfileLogFileNoRollSettings(t *testing.T) {
	base := buildFullConfig(t, []json.RawMessage{minimalRoute(t)}, "", "", false, false)
	cfg := buildFullConfigWithLogging(t, base, map[string]any{
		"simple": map[string]any{
			"writer": map[string]any{
				"output":   "file",
				"filename": "/var/log/caddy/simple.log",
			},
			"level": "ERROR",
		},
	})

	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "output file /var/log/caddy/simple.log\n", "simple file output (no roll block)")
	assertNotContains(t, out, "roll_size", "no roll_size")
	assertNotContains(t, out, "roll_keep", "no roll_keep")
}

// TestGenerateCaddyfileCORSMultiOrigin verifies that multiple allowed origins
// produce per-origin matchers with Vary headers.
func TestGenerateCaddyfileCORSMultiOrigin(t *testing.T) {
	raw, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			Headers: HeadersConfig{
				Enabled:  true,
				Response: ResponseHeaders{CORS: true, CORSOrigins: []string{"https://a.example.com", "https://b.example.com"}},
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

	assertContains(t, out, "@cors0 header Origin https://a.example.com", "cors0 matcher")
	assertContains(t, out, "@cors1 header Origin https://b.example.com", "cors1 matcher")
	assertContains(t, out, `header @cors0 Access-Control-Allow-Origin "https://a.example.com"`, "cors0 origin header")
	assertContains(t, out, `header @cors1 Access-Control-Allow-Origin "https://b.example.com"`, "cors1 origin header")
	assertContains(t, out, `header @cors0 Vary "Origin"`, "cors0 vary")
	assertContains(t, out, `header @cors1 Vary "Origin"`, "cors1 vary")
	assertContains(t, out, "Access-Control-Allow-Methods", "CORS methods")
	assertContains(t, out, "Access-Control-Allow-Headers", "CORS headers")

	// Should NOT contain wildcard origin
	assertNotContains(t, out, `Allow-Origin "*"`, "no wildcard when multi-origin")
}

// TestGenerateCaddyfileMultipleServers verifies that routes from multiple
// servers all appear in the output in deterministic (alphabetical) order.
func TestGenerateCaddyfileMultipleServers(t *testing.T) {
	route1, err := BuildRoute(RouteParams{
		Domain:   "alpha.example.com",
		Upstream: "localhost:8001",
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}
	route2, err := BuildRoute(RouteParams{
		Domain:   "beta.example.com",
		Upstream: "localhost:8002",
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	// Name servers in reverse alphabetical order to verify sorting
	cfg := buildMultiServerConfig(t, map[string]json.RawMessage{
		"srv1": route2,
		"srv0": route1,
	})

	out, err := GenerateCaddyfile(cfg, "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "alpha.example.com {", "srv0 route")
	assertContains(t, out, "beta.example.com {", "srv1 route")

	// srv0 (alpha) must appear before srv1 (beta) due to alphabetical sort
	alphaIdx := strings.Index(out, "alpha.example.com {")
	betaIdx := strings.Index(out, "beta.example.com {")
	if alphaIdx < 0 || betaIdx < 0 {
		t.Fatal("expected both server routes in output")
	}
	if alphaIdx > betaIdx {
		t.Error("srv0 (alpha) should appear before srv1 (beta) due to sorted server names")
	}
}

// TestGenerateCaddyfileAccessLog verifies that a route with AccessLog set
// and a kaji_access logger produces a per-site log block with the writer.
func TestGenerateCaddyfileAccessLog(t *testing.T) {
	route, err := BuildRoute(RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
	})
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}

	srvWithLogs := map[string]any{
		"routes": []json.RawMessage{route},
		"logs": map[string]any{
			"logger_names": map[string]string{
				"example.com": "kaji_access",
			},
		},
	}
	cfg := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": srvWithLogs,
				},
			},
		},
		"logging": map[string]any{
			"logs": map[string]any{
				"kaji_access": map[string]any{
					"writer": map[string]any{
						"output":   "file",
						"filename": "/var/log/caddy/access.log",
					},
				},
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	out, err := GenerateCaddyfile(json.RawMessage(data), "")
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}

	assertContains(t, out, "example.com {", "site block")
	assertContains(t, out, "reverse_proxy localhost:8080", "reverse_proxy")

	// Per-site access log block should appear inside the site block
	assertContains(t, out, "log {", "per-site log block")
	assertContains(t, out, "output file /var/log/caddy/access.log", "access log file path")
	assertContains(t, out, "format json", "access log json format")

	// kaji_access should NOT appear as a global log block
	assertNotContains(t, out, "log kaji_access", "kaji_access not in global blocks")
}
