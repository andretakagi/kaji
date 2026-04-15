package caddy

import (
	"encoding/json"
	"testing"
)

func TestGenerateRouteID(t *testing.T) {
	cases := []struct {
		domain string
		want   string
	}{
		{"example.com", "kaji_example_com"},
		{"my-app.example.com", "kaji_my-app_example_com"},
		{"foo.bar.baz", "kaji_foo_bar_baz"},
		{"already_safe", "kaji_already_safe"},
		{"with spaces", "kaji_with_spaces"},
		{"special!@#chars", "kaji_special___chars"},
		{"", "kaji_"},
	}

	for _, c := range cases {
		got := GenerateRouteID(c.domain)
		if got != c.want {
			t.Errorf("GenerateRouteID(%q) = %q, want %q", c.domain, got, c.want)
		}
	}
}

func TestBuildRouteMinimal(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
	}
	raw, err := BuildRoute(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var route struct {
		ID       string            `json:"@id"`
		Match    []map[string]any  `json:"match"`
		Handle   []json.RawMessage `json:"handle"`
		Terminal bool              `json:"terminal"`
	}
	if err := json.Unmarshal(raw, &route); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if route.ID != "kaji_example_com" {
		t.Errorf("ID = %q, want kaji_example_com", route.ID)
	}
	if !route.Terminal {
		t.Error("terminal should be true")
	}
	if len(route.Match) == 0 {
		t.Fatal("match is empty")
	}
	if len(route.Handle) == 0 {
		t.Fatal("handle is empty")
	}

	// Only handler should be reverse_proxy
	var rp struct {
		Handler   string `json:"handler"`
		Upstreams []struct {
			Dial string `json:"dial"`
		} `json:"upstreams"`
	}
	if err := json.Unmarshal(route.Handle[len(route.Handle)-1], &rp); err != nil {
		t.Fatalf("failed to parse last handler: %v", err)
	}
	if rp.Handler != "reverse_proxy" {
		t.Errorf("last handler = %q, want reverse_proxy", rp.Handler)
	}
	if len(rp.Upstreams) != 1 || rp.Upstreams[0].Dial != "localhost:8080" {
		t.Errorf("upstreams = %v, want [{localhost:8080}]", rp.Upstreams)
	}
}

func TestBuildRouteErrorsOnEmptyFields(t *testing.T) {
	_, err := BuildRoute(RouteParams{Domain: "", Upstream: "localhost:8080"})
	if err == nil {
		t.Error("expected error for empty domain")
	}

	_, err = BuildRoute(RouteParams{Domain: "example.com", Upstream: ""})
	if err == nil {
		t.Error("expected error for empty upstream")
	}
}

func buildAndUnmarshalHandlers(t *testing.T, p RouteParams) []json.RawMessage {
	t.Helper()
	raw, err := BuildRoute(p)
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}
	var route struct {
		Handle []json.RawMessage `json:"handle"`
	}
	if err := json.Unmarshal(raw, &route); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return route.Handle
}

func handlerNames(t *testing.T, handlers []json.RawMessage) []string {
	t.Helper()
	names := make([]string, 0, len(handlers))
	for _, h := range handlers {
		var obj struct {
			Handler string `json:"handler"`
		}
		if err := json.Unmarshal(h, &obj); err != nil {
			t.Fatalf("failed to parse handler: %v", err)
		}
		names = append(names, obj.Handler)
	}
	return names
}

func findHandler(t *testing.T, handlers []json.RawMessage, name string) json.RawMessage {
	t.Helper()
	for _, h := range handlers {
		var obj struct {
			Handler string `json:"handler"`
		}
		if err := json.Unmarshal(h, &obj); err != nil {
			continue
		}
		if obj.Handler == name {
			return h
		}
	}
	t.Fatalf("handler %q not found", name)
	return nil
}

func TestBuildRouteForceHTTPS(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{ForceHTTPS: true},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	names := handlerNames(t, handlers)
	if names[0] != "subroute" {
		t.Errorf("first handler = %q, want subroute for ForceHTTPS", names[0])
	}

	// Verify subroute contains protocol:http match
	var sub struct {
		Routes []struct {
			Match []struct {
				Protocol string `json:"protocol"`
			} `json:"match"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(handlers[0], &sub); err != nil {
		t.Fatalf("failed to parse subroute: %v", err)
	}
	if len(sub.Routes) == 0 || len(sub.Routes[0].Match) == 0 || sub.Routes[0].Match[0].Protocol != "http" {
		t.Error("ForceHTTPS subroute should match protocol:http")
	}
}

func TestBuildRouteCompression(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{Compression: true},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "encode")
	var enc struct {
		Encodings map[string]any `json:"encodings"`
		Prefer    []string       `json:"prefer"`
	}
	if err := json.Unmarshal(h, &enc); err != nil {
		t.Fatalf("failed to parse encode handler: %v", err)
	}
	if _, ok := enc.Encodings["gzip"]; !ok {
		t.Error("encode handler missing gzip")
	}
	if _, ok := enc.Encodings["zstd"]; !ok {
		t.Error("encode handler missing zstd")
	}
	wantPrefer := []string{"zstd", "gzip"}
	if len(enc.Prefer) != len(wantPrefer) {
		t.Fatalf("prefer length = %d, want %d", len(enc.Prefer), len(wantPrefer))
	}
	for i, v := range wantPrefer {
		if enc.Prefer[i] != v {
			t.Errorf("prefer[%d] = %q, want %q", i, enc.Prefer[i], v)
		}
	}
}

func TestBuildRouteSecurityHeaders(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{SecurityHeaders: true},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")
	var hdr struct {
		Response struct {
			Set map[string][]string `json:"set"`
		} `json:"response"`
	}
	if err := json.Unmarshal(h, &hdr); err != nil {
		t.Fatalf("failed to parse headers handler: %v", err)
	}
	for _, name := range []string{
		"Strict-Transport-Security",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
	} {
		if _, ok := hdr.Response.Set[name]; !ok {
			t.Errorf("missing %s header", name)
		}
	}
}

func TestBuildRouteCORSSingleOrigin(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			CORS: CORSOpts{
				Enabled:        true,
				AllowedOrigins: []string{"https://frontend.example.com"},
			},
		},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")
	var hdr struct {
		Response struct {
			Set map[string][]string `json:"set"`
		} `json:"response"`
	}
	if err := json.Unmarshal(h, &hdr); err != nil {
		t.Fatalf("failed to parse headers handler: %v", err)
	}
	origins, ok := hdr.Response.Set["Access-Control-Allow-Origin"]
	if !ok {
		t.Fatal("missing Access-Control-Allow-Origin header")
	}
	if len(origins) == 0 || origins[0] != "https://frontend.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %v, want [https://frontend.example.com]", origins)
	}
}

func TestBuildRouteCORSWildcard(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			CORS: CORSOpts{
				Enabled:        true,
				AllowedOrigins: []string{},
			},
		},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")
	var hdr struct {
		Response struct {
			Set map[string][]string `json:"set"`
		} `json:"response"`
	}
	if err := json.Unmarshal(h, &hdr); err != nil {
		t.Fatalf("failed to parse headers handler: %v", err)
	}
	origins, ok := hdr.Response.Set["Access-Control-Allow-Origin"]
	if !ok {
		t.Fatal("missing Access-Control-Allow-Origin header")
	}
	if len(origins) == 0 || origins[0] != "*" {
		t.Errorf("Access-Control-Allow-Origin = %v, want [*]", origins)
	}
}

func TestBuildRouteCORSMultiOrigin(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			CORS: CORSOpts{
				Enabled:        true,
				AllowedOrigins: []string{"https://a.example.com", "https://b.example.com"},
			},
		},
	}
	handlers := buildAndUnmarshalHandlers(t, p)

	// Multi-origin uses a subroute handler
	var sub json.RawMessage
	for _, h := range handlers {
		var obj struct {
			Handler string `json:"handler"`
			Routes  []struct {
				Match []struct {
					Header map[string][]string `json:"header"`
				} `json:"match"`
			} `json:"routes"`
		}
		if err := json.Unmarshal(h, &obj); err != nil {
			continue
		}
		if obj.Handler == "subroute" && len(obj.Routes) > 0 {
			if _, ok := obj.Routes[0].Match[0].Header["Origin"]; ok {
				sub = h
				break
			}
		}
	}
	if sub == nil {
		t.Fatal("no CORS subroute found for multi-origin")
	}

	var subRoute struct {
		Routes []struct {
			Match []struct {
				Header map[string][]string `json:"header"`
			} `json:"match"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(sub, &subRoute); err != nil {
		t.Fatalf("failed to parse CORS subroute: %v", err)
	}
	if len(subRoute.Routes) != 2 {
		t.Fatalf("expected 2 CORS routes, got %d", len(subRoute.Routes))
	}
	wantOrigins := []string{"https://a.example.com", "https://b.example.com"}
	for i, route := range subRoute.Routes {
		got := route.Match[0].Header["Origin"]
		if len(got) != 1 || got[0] != wantOrigins[i] {
			t.Errorf("route[%d] Origin = %v, want [%s]", i, got, wantOrigins[i])
		}
	}
}

func TestBuildRouteBasicAuth(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			BasicAuth: BasicAuth{
				Enabled:      true,
				Username:     "admin",
				PasswordHash: "$2a$14$hashedpassword",
			},
		},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "authentication")
	var auth struct {
		Providers struct {
			HTTPBasic struct {
				Accounts []struct {
					Username string `json:"username"`
					Password string `json:"password"`
				} `json:"accounts"`
			} `json:"http_basic"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(h, &auth); err != nil {
		t.Fatalf("failed to parse authentication handler: %v", err)
	}
	accounts := auth.Providers.HTTPBasic.Accounts
	if len(accounts) == 0 {
		t.Fatal("no accounts in authentication handler")
	}
	if accounts[0].Username != "admin" {
		t.Errorf("username = %q, want admin", accounts[0].Username)
	}
	if accounts[0].Password != "$2a$14$hashedpassword" {
		t.Errorf("password hash = %q, want $2a$14$hashedpassword", accounts[0].Password)
	}
}

func TestBuildRouteTLSSkipVerify(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8443",
		Toggles:  RouteToggles{TLSSkipVerify: true},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "reverse_proxy")
	var rp struct {
		Transport struct {
			TLS struct {
				InsecureSkipVerify bool `json:"insecure_skip_verify"`
			} `json:"tls"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(h, &rp); err != nil {
		t.Fatalf("failed to parse reverse_proxy: %v", err)
	}
	if !rp.Transport.TLS.InsecureSkipVerify {
		t.Error("insecure_skip_verify should be true")
	}
}

func TestBuildRouteWebSocket(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{WebSocketPassthru: true},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "reverse_proxy")
	var rp struct {
		FlushInterval json.Number `json:"flush_interval"`
	}
	if err := json.Unmarshal(h, &rp); err != nil {
		t.Fatalf("failed to parse reverse_proxy: %v", err)
	}
	if v, _ := rp.FlushInterval.Int64(); v != -1 {
		t.Errorf("flush_interval = %v, want -1", rp.FlushInterval)
	}
}

func TestBuildRouteLoadBalancingRoundRobin(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			LoadBalancing: LoadBalancing{
				Enabled:   true,
				Strategy:  "round_robin",
				Upstreams: []string{"localhost:8081", "localhost:8082"},
			},
		},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "reverse_proxy")
	var rp struct {
		Upstreams []struct {
			Dial string `json:"dial"`
		} `json:"upstreams"`
		LoadBalancing struct {
			SelectionPolicy struct {
				Policy string `json:"policy"`
			} `json:"selection_policy"`
		} `json:"load_balancing"`
		HealthChecks *json.RawMessage `json:"health_checks,omitempty"`
	}
	if err := json.Unmarshal(h, &rp); err != nil {
		t.Fatalf("failed to parse reverse_proxy: %v", err)
	}
	if len(rp.Upstreams) != 3 {
		t.Errorf("upstreams count = %d, want 3", len(rp.Upstreams))
	}
	if rp.LoadBalancing.SelectionPolicy.Policy != "round_robin" {
		t.Errorf("policy = %q, want round_robin", rp.LoadBalancing.SelectionPolicy.Policy)
	}
	if rp.HealthChecks != nil {
		t.Error("round_robin should not have health_checks")
	}
}

func TestBuildRouteLoadBalancingFirst(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			LoadBalancing: LoadBalancing{
				Enabled:   true,
				Strategy:  "first",
				Upstreams: []string{"localhost:8081"},
			},
		},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "reverse_proxy")
	var rp struct {
		LoadBalancing struct {
			SelectionPolicy struct {
				Policy string `json:"policy"`
			} `json:"selection_policy"`
		} `json:"load_balancing"`
		HealthChecks struct {
			Passive struct {
				FailDuration string `json:"fail_duration"`
				MaxFails     int    `json:"max_fails"`
			} `json:"passive"`
		} `json:"health_checks"`
	}
	if err := json.Unmarshal(h, &rp); err != nil {
		t.Fatalf("failed to parse reverse_proxy: %v", err)
	}
	if rp.LoadBalancing.SelectionPolicy.Policy != "first" {
		t.Errorf("policy = %q, want first", rp.LoadBalancing.SelectionPolicy.Policy)
	}
	if rp.HealthChecks.Passive.FailDuration != "30s" {
		t.Errorf("fail_duration = %q, want 30s", rp.HealthChecks.Passive.FailDuration)
	}
	if rp.HealthChecks.Passive.MaxFails != 3 {
		t.Errorf("max_fails = %d, want 3", rp.HealthChecks.Passive.MaxFails)
	}
}

// round-trip helpers

func buildAndParse(t *testing.T, p RouteParams) RouteParams {
	t.Helper()
	raw, err := BuildRoute(p)
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}
	result, err := ParseRouteParams(raw)
	if err != nil {
		t.Fatalf("ParseRouteParams failed: %v", err)
	}
	return result
}

func TestParseRouteParamsMinimal(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
	}
	got := buildAndParse(t, p)
	if got.Domain != p.Domain {
		t.Errorf("Domain = %q, want %q", got.Domain, p.Domain)
	}
	if got.Upstream != p.Upstream {
		t.Errorf("Upstream = %q, want %q", got.Upstream, p.Upstream)
	}
	if got.ID != "kaji_example_com" {
		t.Errorf("ID = %q, want kaji_example_com", got.ID)
	}
}

func TestParseRouteParamsForceHTTPS(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{ForceHTTPS: true},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.ForceHTTPS {
		t.Error("ForceHTTPS should round-trip to true")
	}
	if got.Domain != p.Domain {
		t.Errorf("Domain = %q, want %q", got.Domain, p.Domain)
	}
	if got.Upstream != p.Upstream {
		t.Errorf("Upstream = %q, want %q", got.Upstream, p.Upstream)
	}
}

func TestParseRouteParamsCompression(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{Compression: true},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.Compression {
		t.Error("Compression should round-trip to true")
	}
}

func TestParseRouteParamsSecurityHeaders(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{SecurityHeaders: true},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.SecurityHeaders {
		t.Error("SecurityHeaders should round-trip to true")
	}
}

func TestParseRouteParamsCORSSingleOrigin(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			CORS: CORSOpts{
				Enabled:        true,
				AllowedOrigins: []string{"https://frontend.example.com"},
			},
		},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.CORS.Enabled {
		t.Error("CORS.Enabled should round-trip to true")
	}
	if len(got.Toggles.CORS.AllowedOrigins) != 1 || got.Toggles.CORS.AllowedOrigins[0] != "https://frontend.example.com" {
		t.Errorf("AllowedOrigins = %v, want [https://frontend.example.com]", got.Toggles.CORS.AllowedOrigins)
	}
}

func TestParseRouteParamsCORSWildcard(t *testing.T) {
	// Wildcard origin (*) does not round-trip AllowedOrigins - only CORS.Enabled does
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			CORS: CORSOpts{
				Enabled:        true,
				AllowedOrigins: []string{},
			},
		},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.CORS.Enabled {
		t.Error("CORS.Enabled should round-trip to true for wildcard")
	}
	// AllowedOrigins is intentionally not set for wildcard
	if len(got.Toggles.CORS.AllowedOrigins) != 0 {
		t.Errorf("AllowedOrigins should be empty for wildcard, got %v", got.Toggles.CORS.AllowedOrigins)
	}
}

func TestParseRouteParamsCORSMultiOrigin(t *testing.T) {
	origins := []string{"https://a.example.com", "https://b.example.com"}
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			CORS: CORSOpts{
				Enabled:        true,
				AllowedOrigins: origins,
			},
		},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.CORS.Enabled {
		t.Error("CORS.Enabled should round-trip to true")
	}
	if len(got.Toggles.CORS.AllowedOrigins) != len(origins) {
		t.Fatalf("AllowedOrigins count = %d, want %d", len(got.Toggles.CORS.AllowedOrigins), len(origins))
	}
	for i, o := range origins {
		if got.Toggles.CORS.AllowedOrigins[i] != o {
			t.Errorf("AllowedOrigins[%d] = %q, want %q", i, got.Toggles.CORS.AllowedOrigins[i], o)
		}
	}
}

func TestParseRouteParamsBasicAuth(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			BasicAuth: BasicAuth{
				Enabled:      true,
				Username:     "admin",
				PasswordHash: "$2a$14$hashedpassword",
			},
		},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.BasicAuth.Enabled {
		t.Error("BasicAuth.Enabled should round-trip to true")
	}
	if got.Toggles.BasicAuth.Username != "admin" {
		t.Errorf("Username = %q, want admin", got.Toggles.BasicAuth.Username)
	}
	if got.Toggles.BasicAuth.PasswordHash != "$2a$14$hashedpassword" {
		t.Errorf("PasswordHash = %q, want $2a$14$hashedpassword", got.Toggles.BasicAuth.PasswordHash)
	}
}

func TestParseRouteParamsTLSSkipVerify(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8443",
		Toggles:  RouteToggles{TLSSkipVerify: true},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.TLSSkipVerify {
		t.Error("TLSSkipVerify should round-trip to true")
	}
}

func TestParseRouteParamsWebSocket(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles:  RouteToggles{WebSocketPassthru: true},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.WebSocketPassthru {
		t.Error("WebSocketPassthru should round-trip to true")
	}
}

func TestParseRouteParamsLoadBalancingRoundRobin(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			LoadBalancing: LoadBalancing{
				Enabled:   true,
				Strategy:  "round_robin",
				Upstreams: []string{"localhost:8081", "localhost:8082"},
			},
		},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.LoadBalancing.Enabled {
		t.Error("LoadBalancing.Enabled should round-trip to true")
	}
	if got.Toggles.LoadBalancing.Strategy != "round_robin" {
		t.Errorf("Strategy = %q, want round_robin", got.Toggles.LoadBalancing.Strategy)
	}
	if got.Upstream != "localhost:8080" {
		t.Errorf("Upstream = %q, want localhost:8080", got.Upstream)
	}
	if len(got.Toggles.LoadBalancing.Upstreams) != 2 {
		t.Fatalf("extra upstreams count = %d, want 2", len(got.Toggles.LoadBalancing.Upstreams))
	}
	wantUpstreams := []string{"localhost:8081", "localhost:8082"}
	for i, want := range wantUpstreams {
		if got.Toggles.LoadBalancing.Upstreams[i] != want {
			t.Errorf("Upstreams[%d] = %q, want %q", i, got.Toggles.LoadBalancing.Upstreams[i], want)
		}
	}
}

func TestParseRouteParamsLoadBalancingFirst(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{
			LoadBalancing: LoadBalancing{
				Enabled:   true,
				Strategy:  "first",
				Upstreams: []string{"localhost:8081"},
			},
		},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.LoadBalancing.Enabled {
		t.Error("LoadBalancing.Enabled should round-trip to true")
	}
	if got.Toggles.LoadBalancing.Strategy != "first" {
		t.Errorf("Strategy = %q, want first", got.Toggles.LoadBalancing.Strategy)
	}
	if len(got.Toggles.LoadBalancing.Upstreams) != 1 {
		t.Fatalf("extra upstreams count = %d, want 1", len(got.Toggles.LoadBalancing.Upstreams))
	}
	if got.Toggles.LoadBalancing.Upstreams[0] != "localhost:8081" {
		t.Errorf("extra upstream[0] = %q, want localhost:8081", got.Toggles.LoadBalancing.Upstreams[0])
	}
}

// --- IP filtering ---

func TestBuildRouteIPFilteringBlacklist(t *testing.T) {
	p := RouteParams{
		Domain:     "example.com",
		Upstream:   "localhost:8080",
		IPListIPs:  []string{"10.0.0.1", "192.168.1.0/24"},
		IPListType: "blacklist",
	}
	handlers := buildAndUnmarshalHandlers(t, p)

	// First handler should be the IP filtering subroute
	var sub struct {
		Handler string `json:"handler"`
		Routes  []struct {
			Match []struct {
				RemoteIP struct {
					Ranges []string `json:"ranges"`
				} `json:"remote_ip"`
			} `json:"match"`
			Handle []struct {
				Handler    string `json:"handler"`
				StatusCode string `json:"status_code"`
			} `json:"handle"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(handlers[0], &sub); err != nil {
		t.Fatalf("failed to parse IP filtering handler: %v", err)
	}
	if sub.Handler != "subroute" {
		t.Fatalf("first handler = %q, want subroute", sub.Handler)
	}
	if len(sub.Routes) != 1 {
		t.Fatalf("routes count = %d, want 1", len(sub.Routes))
	}
	if len(sub.Routes[0].Match) != 1 {
		t.Fatalf("match count = %d, want 1", len(sub.Routes[0].Match))
	}
	ranges := sub.Routes[0].Match[0].RemoteIP.Ranges
	if len(ranges) != 2 || ranges[0] != "10.0.0.1" || ranges[1] != "192.168.1.0/24" {
		t.Errorf("ranges = %v, want [10.0.0.1 192.168.1.0/24]", ranges)
	}
	if sub.Routes[0].Handle[0].StatusCode != "403" {
		t.Errorf("status_code = %q, want 403", sub.Routes[0].Handle[0].StatusCode)
	}
}

func TestBuildRouteIPFilteringWhitelist(t *testing.T) {
	p := RouteParams{
		Domain:     "example.com",
		Upstream:   "localhost:8080",
		IPListIPs:  []string{"10.0.0.0/8"},
		IPListType: "whitelist",
	}
	handlers := buildAndUnmarshalHandlers(t, p)

	// Whitelist wraps the remote_ip matcher in a "not" block
	var sub struct {
		Handler string `json:"handler"`
		Routes  []struct {
			Match  []map[string]json.RawMessage `json:"match"`
			Handle []struct {
				StatusCode string `json:"status_code"`
			} `json:"handle"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(handlers[0], &sub); err != nil {
		t.Fatalf("failed to parse IP filtering handler: %v", err)
	}
	if sub.Handler != "subroute" {
		t.Fatalf("first handler = %q, want subroute", sub.Handler)
	}
	match := sub.Routes[0].Match[0]
	notRaw, ok := match["not"]
	if !ok {
		t.Fatal("whitelist match should contain a 'not' key")
	}
	var nots []struct {
		RemoteIP struct {
			Ranges []string `json:"ranges"`
		} `json:"remote_ip"`
	}
	if err := json.Unmarshal(notRaw, &nots); err != nil {
		t.Fatalf("failed to parse not block: %v", err)
	}
	if len(nots) != 1 || len(nots[0].RemoteIP.Ranges) != 1 || nots[0].RemoteIP.Ranges[0] != "10.0.0.0/8" {
		t.Errorf("not ranges = %v, want [[10.0.0.0/8]]", nots)
	}
	if sub.Routes[0].Handle[0].StatusCode != "403" {
		t.Errorf("status_code = %q, want 403", sub.Routes[0].Handle[0].StatusCode)
	}
}

func TestBuildRouteIPFilteringSkippedWhenEmpty(t *testing.T) {
	p := RouteParams{
		Domain:     "example.com",
		Upstream:   "localhost:8080",
		IPListIPs:  []string{},
		IPListType: "blacklist",
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	names := handlerNames(t, handlers)
	if len(names) != 1 || names[0] != "reverse_proxy" {
		t.Errorf("handlers = %v, want [reverse_proxy] when IPListIPs is empty", names)
	}
}

func TestParseRouteParamsIPFilteringBlacklist(t *testing.T) {
	p := RouteParams{
		Domain:     "example.com",
		Upstream:   "localhost:8080",
		IPListIPs:  []string{"10.0.0.1"},
		IPListType: "blacklist",
	}
	raw, err := BuildRoute(p)
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}
	got, err := ParseRouteParams(raw)
	if err != nil {
		t.Fatalf("ParseRouteParams failed: %v", err)
	}
	if !got.Toggles.IPFiltering.Enabled {
		t.Error("IPFiltering.Enabled should be true")
	}
	if got.Toggles.IPFiltering.Type != "blacklist" {
		t.Errorf("IPFiltering.Type = %q, want blacklist", got.Toggles.IPFiltering.Type)
	}
}

func TestParseRouteParamsIPFilteringWhitelist(t *testing.T) {
	p := RouteParams{
		Domain:     "example.com",
		Upstream:   "localhost:8080",
		IPListIPs:  []string{"10.0.0.0/8"},
		IPListType: "whitelist",
	}
	raw, err := BuildRoute(p)
	if err != nil {
		t.Fatalf("BuildRoute failed: %v", err)
	}
	got, err := ParseRouteParams(raw)
	if err != nil {
		t.Fatalf("ParseRouteParams failed: %v", err)
	}
	if !got.Toggles.IPFiltering.Enabled {
		t.Error("IPFiltering.Enabled should be true")
	}
	if got.Toggles.IPFiltering.Type != "whitelist" {
		t.Errorf("IPFiltering.Type = %q, want whitelist", got.Toggles.IPFiltering.Type)
	}
}

// --- Caddyfile-adapted route structure ---

func TestParseRouteParamsCaddyfileWrappedSubroute(t *testing.T) {
	// Caddyfile-adapted routes wrap all handlers inside a single top-level
	// subroute, with each handler in its own nested route.
	caddyfileRoute := json.RawMessage(`{
		"@id": "kaji_example_com",
		"match": [{"host": ["example.com"]}],
		"handle": [{
			"handler": "subroute",
			"routes": [
				{
					"handle": [{
						"handler": "encode",
						"encodings": {"gzip": {}, "zstd": {}},
						"prefer": ["zstd", "gzip"]
					}]
				},
				{
					"handle": [{
						"handler": "reverse_proxy",
						"upstreams": [{"dial": "localhost:9000"}]
					}]
				}
			]
		}],
		"terminal": true
	}`)

	got, err := ParseRouteParams(caddyfileRoute)
	if err != nil {
		t.Fatalf("ParseRouteParams failed: %v", err)
	}
	if got.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", got.Domain)
	}
	if got.Upstream != "localhost:9000" {
		t.Errorf("Upstream = %q, want localhost:9000", got.Upstream)
	}
	if !got.Toggles.Compression {
		t.Error("Compression should be detected from unwrapped subroute")
	}
}

func TestParseRouteParamsCaddyfileForceHTTPSWithHandlers(t *testing.T) {
	// Caddyfile-adapted route where ForceHTTPS subroute also contains
	// additional handlers (like reverse_proxy) in non-redirect routes.
	caddyfileRoute := json.RawMessage(`{
		"@id": "kaji_app_com",
		"match": [{"host": ["app.com"]}],
		"handle": [{
			"handler": "subroute",
			"routes": [
				{
					"match": [{"protocol": "http"}],
					"handle": [{
						"handler": "static_response",
						"status_code": "301",
						"headers": {"Location": ["https://{http.request.host}{http.request.uri}"]}
					}]
				},
				{
					"handle": [{
						"handler": "reverse_proxy",
						"upstreams": [{"dial": "localhost:3000"}]
					}]
				}
			]
		}],
		"terminal": true
	}`)

	got, err := ParseRouteParams(caddyfileRoute)
	if err != nil {
		t.Fatalf("ParseRouteParams failed: %v", err)
	}
	if !got.Toggles.ForceHTTPS {
		t.Error("ForceHTTPS should be detected")
	}
	if got.Upstream != "localhost:3000" {
		t.Errorf("Upstream = %q, want localhost:3000", got.Upstream)
	}
}

// --- Malformed / empty JSON ---

func TestParseRouteParamsEmptyInput(t *testing.T) {
	_, err := ParseRouteParams(json.RawMessage(``))
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseRouteParamsInvalidJSON(t *testing.T) {
	_, err := ParseRouteParams(json.RawMessage(`{not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseRouteParamsEmptyObject(t *testing.T) {
	got, err := ParseRouteParams(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Domain != "" {
		t.Errorf("Domain = %q, want empty", got.Domain)
	}
	if got.Upstream != "" {
		t.Errorf("Upstream = %q, want empty", got.Upstream)
	}
}

func TestParseRouteParamsNoMatchNoHandle(t *testing.T) {
	got, err := ParseRouteParams(json.RawMessage(`{"@id": "test_route"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "test_route" {
		t.Errorf("ID = %q, want test_route", got.ID)
	}
	if got.Domain != "" {
		t.Errorf("Domain = %q, want empty", got.Domain)
	}
}
