package caddy

import (
	"encoding/json"
	"testing"
)

// --- Basic mode response headers ---

func TestBuildResponseHeadersDisabled(t *testing.T) {
	cfg := HeadersConfig{Response: ResponseHeaders{Security: true}}
	handlers := buildResponseHeaders(cfg, false)
	if len(handlers) != 0 {
		t.Errorf("expected no handlers when disabled, got %d", len(handlers))
	}
}

func TestBasicSecurityHeaders(t *testing.T) {
	resp := ResponseHeaders{Security: true}
	handlers := buildBasicResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	for _, key := range []string{
		"Strict-Transport-Security",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
	} {
		if _, ok := set[key]; !ok {
			t.Errorf("missing security header %q", key)
		}
	}
}

func TestBasicCacheControl(t *testing.T) {
	resp := ResponseHeaders{CacheControl: true}
	handlers := buildBasicResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	vals, ok := set["Cache-Control"]
	if !ok || len(vals) == 0 || vals[0] != "no-store" {
		t.Errorf("Cache-Control = %v, want [no-store]", vals)
	}
}

func TestBasicXRobotsTag(t *testing.T) {
	resp := ResponseHeaders{XRobotsTag: true}
	handlers := buildBasicResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	vals, ok := set["X-Robots-Tag"]
	if !ok || len(vals) == 0 || vals[0] != "noindex, nofollow" {
		t.Errorf("X-Robots-Tag = %v, want [noindex, nofollow]", vals)
	}
}

func TestBasicMergesNonCORSIntoSingleHandler(t *testing.T) {
	resp := ResponseHeaders{
		Security:     true,
		CacheControl: true,
		XRobotsTag:   true,
	}
	handlers := buildBasicResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("all non-CORS headers should be in one handler, got %d handlers", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	if _, ok := set["Strict-Transport-Security"]; !ok {
		t.Error("missing security header")
	}
	if _, ok := set["Cache-Control"]; !ok {
		t.Error("missing Cache-Control")
	}
	if _, ok := set["X-Robots-Tag"]; !ok {
		t.Error("missing X-Robots-Tag")
	}
}

func TestBasicCORSSingleOrigin(t *testing.T) {
	resp := ResponseHeaders{
		CORS:        true,
		CORSOrigins: []string{"https://example.com"},
	}
	handlers := buildBasicResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("single-origin CORS should produce 1 handler, got %d", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	if vals := set["Access-Control-Allow-Origin"]; len(vals) == 0 || vals[0] != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %v, want [https://example.com]", vals)
	}
}

func TestBasicCORSWildcard(t *testing.T) {
	resp := ResponseHeaders{
		CORS:        true,
		CORSOrigins: []string{},
	}
	handlers := buildBasicResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("wildcard CORS should produce 1 handler, got %d", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	if vals := set["Access-Control-Allow-Origin"]; len(vals) == 0 || vals[0] != "*" {
		t.Errorf("Access-Control-Allow-Origin = %v, want [*]", vals)
	}
}

func TestBasicCORSMultiOrigin(t *testing.T) {
	resp := ResponseHeaders{
		CORS:        true,
		CORSOrigins: []string{"https://a.com", "https://b.com"},
	}
	handlers := buildBasicResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("multi-origin CORS should produce 1 subroute handler, got %d", len(handlers))
	}

	raw := marshalHandler(t, handlers[0])
	var sub struct {
		Handler string `json:"handler"`
		Routes  []struct {
			Match []struct {
				Header map[string][]string `json:"header"`
			} `json:"match"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(raw, &sub); err != nil {
		t.Fatalf("failed to parse subroute: %v", err)
	}
	if sub.Handler != "subroute" {
		t.Fatalf("handler = %q, want subroute", sub.Handler)
	}
	if len(sub.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(sub.Routes))
	}
	wantOrigins := []string{"https://a.com", "https://b.com"}
	for i, route := range sub.Routes {
		if origins := route.Match[0].Header["Origin"]; len(origins) == 0 || origins[0] != wantOrigins[i] {
			t.Errorf("route[%d] Origin = %v, want [%s]", i, origins, wantOrigins[i])
		}
	}
}

func TestBasicSecurityPlusCORS(t *testing.T) {
	resp := ResponseHeaders{
		Security:    true,
		CORS:        true,
		CORSOrigins: []string{"https://example.com"},
	}
	handlers := buildBasicResponseHeaders(resp)
	// Security in one handler, CORS in another
	if len(handlers) != 2 {
		t.Fatalf("security + CORS should produce 2 handlers, got %d", len(handlers))
	}

	secSet := extractHeaderSet(t, handlers[0])
	if _, ok := secSet["Strict-Transport-Security"]; !ok {
		t.Error("first handler should have security headers")
	}

	corsSet := extractHeaderSet(t, handlers[1])
	if _, ok := corsSet["Access-Control-Allow-Origin"]; !ok {
		t.Error("second handler should have CORS headers")
	}
}

func TestBasicNothingEnabled(t *testing.T) {
	resp := ResponseHeaders{}
	handlers := buildBasicResponseHeaders(resp)
	if len(handlers) != 0 {
		t.Errorf("expected no handlers when nothing enabled, got %d", len(handlers))
	}
}

// --- Advanced mode response headers ---

func TestAdvancedBuiltinHeaders(t *testing.T) {
	resp := ResponseHeaders{
		Builtin: []HeaderEntry{
			{Key: "X-Custom", Value: "yes", Enabled: true},
			{Key: "X-Disabled", Value: "no", Enabled: false},
		},
	}
	handlers := buildAdvancedResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	if vals := set["X-Custom"]; len(vals) == 0 || vals[0] != "yes" {
		t.Errorf("X-Custom = %v, want [yes]", vals)
	}
	if _, ok := set["X-Disabled"]; ok {
		t.Error("disabled header should not appear")
	}
}

func TestAdvancedCustomOverridesBuiltin(t *testing.T) {
	resp := ResponseHeaders{
		Builtin: []HeaderEntry{
			{Key: "X-Frame-Options", Value: "DENY", Enabled: true},
		},
		Custom: []HeaderEntry{
			{Key: "X-Frame-Options", Value: "SAMEORIGIN", Enabled: true},
		},
	}
	handlers := buildAdvancedResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	vals := set["X-Frame-Options"]
	if len(vals) == 0 || vals[0] != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options = %v, want [SAMEORIGIN] (custom should override)", vals)
	}
}

func TestAdvancedCORSSingleOrigin(t *testing.T) {
	resp := ResponseHeaders{
		Builtin: []HeaderEntry{
			{Key: "Access-Control-Allow-Origin", Value: "https://example.com", Enabled: true},
		},
	}
	handlers := buildAdvancedResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("single-origin CORS should produce 1 handler, got %d", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	if vals := set["Access-Control-Allow-Origin"]; len(vals) == 0 || vals[0] != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %v, want [https://example.com]", vals)
	}
}

func TestAdvancedCORSMultiOriginSubroute(t *testing.T) {
	resp := ResponseHeaders{
		Builtin: []HeaderEntry{
			{Key: "Access-Control-Allow-Origin", Value: "https://a.com, https://b.com", Enabled: true},
			{Key: "X-Extra", Value: "kept", Enabled: true},
		},
	}
	handlers := buildAdvancedResponseHeaders(resp)
	// One headers handler for X-Extra, one subroute for CORS
	if len(handlers) != 2 {
		t.Fatalf("expected 2 handlers (headers + subroute), got %d", len(handlers))
	}

	// First should be the non-CORS headers
	set := extractHeaderSet(t, handlers[0])
	if _, ok := set["X-Extra"]; !ok {
		t.Error("non-CORS headers handler should contain X-Extra")
	}

	// Second should be the subroute
	raw := marshalHandler(t, handlers[1])
	var sub struct {
		Handler string `json:"handler"`
		Routes  []struct {
			Match []struct {
				Header map[string][]string `json:"header"`
			} `json:"match"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(raw, &sub); err != nil {
		t.Fatalf("failed to parse subroute: %v", err)
	}
	if sub.Handler != "subroute" {
		t.Errorf("handler = %q, want subroute", sub.Handler)
	}
	if len(sub.Routes) != 2 {
		t.Fatalf("expected 2 CORS routes, got %d", len(sub.Routes))
	}
}

func TestAdvancedCORSWildcard(t *testing.T) {
	resp := ResponseHeaders{
		Builtin: []HeaderEntry{
			{Key: "Access-Control-Allow-Origin", Value: "*", Enabled: true},
		},
	}
	handlers := buildAdvancedResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("wildcard CORS should produce 1 handler, got %d", len(handlers))
	}

	set := extractHeaderSet(t, handlers[0])
	if vals := set["Access-Control-Allow-Origin"]; len(vals) == 0 || vals[0] != "*" {
		t.Errorf("Access-Control-Allow-Origin = %v, want [*]", vals)
	}
}

func TestAdvancedEmptyEntries(t *testing.T) {
	resp := ResponseHeaders{
		Builtin: []HeaderEntry{
			{Key: "", Value: "no-key", Enabled: true},
		},
		Custom: []HeaderEntry{
			{Key: "X-Valid", Value: "", Enabled: true},
		},
	}
	handlers := buildAdvancedResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}
	set := extractHeaderSet(t, handlers[0])
	if _, ok := set[""]; ok {
		t.Error("empty-key entry should be skipped")
	}
	if _, ok := set["X-Valid"]; !ok {
		t.Error("X-Valid should be present even with empty value")
	}
}

// --- Header Up (reverse_proxy request headers) ---

func TestBasicHeaderUpHostOverride(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled:      true,
		HostOverride: true,
		HostValue:    "backend.internal",
	}
	result := BuildHeaderUp(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set, ok := result["set"].(map[string][]string)
	if !ok {
		t.Fatal("expected set field in result")
	}
	if vals := set["Host"]; len(vals) == 0 || vals[0] != "backend.internal" {
		t.Errorf("Host = %v, want [backend.internal]", vals)
	}
}

func TestBasicHeaderUpAuthorization(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled:       true,
		Authorization: true,
		AuthValue:     "Bearer token123",
	}
	result := BuildHeaderUp(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set, ok := result["set"].(map[string][]string)
	if !ok {
		t.Fatal("expected set field in result")
	}
	if vals := set["Authorization"]; len(vals) == 0 || vals[0] != "Bearer token123" {
		t.Errorf("Authorization = %v, want [Bearer token123]", vals)
	}
}

func TestBasicHeaderUpBothHeaders(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled:       true,
		HostOverride:  true,
		HostValue:     "backend.internal",
		Authorization: true,
		AuthValue:     "Bearer abc",
	}
	result := BuildHeaderUp(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set, ok := result["set"].(map[string][]string)
	if !ok {
		t.Fatal("expected set field in result")
	}
	if len(set) != 2 {
		t.Errorf("expected 2 headers, got %d", len(set))
	}
}

func TestBasicHeaderUpDisabledReturnsNil(t *testing.T) {
	cfg := HeaderUpConfig{
		HostOverride: true,
		HostValue:    "backend.internal",
	}
	result := BuildHeaderUp(cfg, false)
	if result != nil {
		t.Errorf("expected nil when disabled, got %v", result)
	}
}

func TestBasicHeaderUpEmptyValueSkipped(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled:      true,
		HostOverride: true,
		HostValue:    "",
	}
	result := BuildHeaderUp(cfg, false)
	if result != nil {
		t.Errorf("expected nil when host value is empty, got %v", result)
	}
}

func TestAdvancedHeaderUp(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled: true,
		Builtin: []HeaderEntry{
			{Key: "Host", Value: "builtin.host", Operation: "set", Enabled: true},
			{Key: "X-Disabled", Value: "no", Operation: "set", Enabled: false},
		},
		Custom: []HeaderEntry{
			{Key: "X-Custom-Req", Value: "custom-val", Operation: "set", Enabled: true},
		},
	}
	result := BuildHeaderUp(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set, ok := result["set"].(map[string][]string)
	if !ok {
		t.Fatal("expected set field in result")
	}
	if vals := set["Host"]; len(vals) == 0 || vals[0] != "builtin.host" {
		t.Errorf("Host = %v, want [builtin.host]", vals)
	}
	if _, ok := set["X-Disabled"]; ok {
		t.Error("disabled entry should not appear")
	}
	if vals := set["X-Custom-Req"]; len(vals) == 0 || vals[0] != "custom-val" {
		t.Errorf("X-Custom-Req = %v, want [custom-val]", vals)
	}
}

func TestAdvancedHeaderUpCustomOverridesBuiltin(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled: true,
		Builtin: []HeaderEntry{
			{Key: "Host", Value: "builtin.host", Operation: "set", Enabled: true},
		},
		Custom: []HeaderEntry{
			{Key: "Host", Value: "custom.host", Operation: "set", Enabled: true},
		},
	}
	result := BuildHeaderUp(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set, ok := result["set"].(map[string][]string)
	if !ok {
		t.Fatal("expected set field in result")
	}
	if vals := set["Host"]; len(vals) == 0 || vals[0] != "custom.host" {
		t.Errorf("Host = %v, want [custom.host] (custom should override)", vals)
	}
}

// --- BuildDomain integration ---

func TestBuildDomainBasicCacheControl(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Response: ResponseHeaders{Enabled: true, CacheControl: true},
		}},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")
	set := extractHeaderSetFromRaw(t, h)
	if vals := set["Cache-Control"]; len(vals) == 0 || vals[0] != "no-store" {
		t.Errorf("Cache-Control = %v, want [no-store]", vals)
	}
}

func TestBuildDomainBasicXRobotsTag(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Response: ResponseHeaders{Enabled: true, XRobotsTag: true},
		}},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")
	set := extractHeaderSetFromRaw(t, h)
	if vals := set["X-Robots-Tag"]; len(vals) == 0 || vals[0] != "noindex, nofollow" {
		t.Errorf("X-Robots-Tag = %v, want [noindex, nofollow]", vals)
	}
}

func TestBuildDomainHeaderUp(t *testing.T) {
	rpCfg, _ := json.Marshal(ReverseProxyConfig{
		HeaderUp: HeaderUpConfig{
			Enabled:       true,
			HostOverride:  true,
			HostValue:     "backend.internal",
			Authorization: true,
			AuthValue:     "Bearer xyz",
		},
	})
	p := DomainParams{
		Domain:        "example.com",
		Upstream:      "localhost:8080",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	rp := findHandler(t, handlers, "reverse_proxy")

	var proxy struct {
		Headers struct {
			Request struct {
				Set map[string][]string `json:"set"`
			} `json:"request"`
		} `json:"headers"`
	}
	if err := json.Unmarshal(rp, &proxy); err != nil {
		t.Fatalf("failed to parse reverse_proxy: %v", err)
	}
	if vals := proxy.Headers.Request.Set["Host"]; len(vals) == 0 || vals[0] != "backend.internal" {
		t.Errorf("Host = %v, want [backend.internal]", vals)
	}
	if vals := proxy.Headers.Request.Set["Authorization"]; len(vals) == 0 || vals[0] != "Bearer xyz" {
		t.Errorf("Authorization = %v, want [Bearer xyz]", vals)
	}
}

func TestBuildDomainAdvancedMode(t *testing.T) {
	rpCfg, _ := json.Marshal(ReverseProxyConfig{
		HeaderUp: HeaderUpConfig{
			Enabled: true,
			Builtin: []HeaderEntry{
				{Key: "Host", Value: "advanced.host", Operation: "set", Enabled: true},
			},
		},
	})
	p := DomainParams{
		Domain:          "example.com",
		Upstream:        "localhost:8080",
		HandlerType:     "reverse_proxy",
		HandlerConfig:   rpCfg,
		AdvancedHeaders: true,
		Toggles: RouteToggles{
			Headers: HeadersConfig{
				Response: ResponseHeaders{
					Enabled: true,
					Builtin: []HeaderEntry{
						{Key: "X-Frame-Options", Value: "DENY", Enabled: true},
					},
					Custom: []HeaderEntry{
						{Key: "X-Custom", Value: "val", Enabled: true},
					},
				},
			},
		},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")
	set := extractHeaderSetFromRaw(t, h)
	if vals := set["X-Frame-Options"]; len(vals) == 0 || vals[0] != "DENY" {
		t.Errorf("X-Frame-Options = %v, want [DENY]", vals)
	}
	if vals := set["X-Custom"]; len(vals) == 0 || vals[0] != "val" {
		t.Errorf("X-Custom = %v, want [val]", vals)
	}

	rp := findHandler(t, handlers, "reverse_proxy")
	var proxy struct {
		Headers struct {
			Request struct {
				Set map[string][]string `json:"set"`
			} `json:"request"`
		} `json:"headers"`
	}
	if err := json.Unmarshal(rp, &proxy); err != nil {
		t.Fatalf("failed to parse reverse_proxy: %v", err)
	}
	if vals := proxy.Headers.Request.Set["Host"]; len(vals) == 0 || vals[0] != "advanced.host" {
		t.Errorf("Host = %v, want [advanced.host]", vals)
	}
}

// --- ParseDomainParams round-trip ---

func TestParseDomainParamsCacheControl(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Response: ResponseHeaders{Enabled: true, CacheControl: true},
		}},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.Headers.Response.Enabled {
		t.Error("Headers.Response.Enabled should round-trip to true")
	}
	if !got.Toggles.Headers.Response.CacheControl {
		t.Error("Headers.Response.CacheControl should round-trip to true")
	}
}

func TestParseDomainParamsXRobotsTag(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Response: ResponseHeaders{Enabled: true, XRobotsTag: true},
		}},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.Headers.Response.Enabled {
		t.Error("Headers.Response.Enabled should round-trip to true")
	}
	if !got.Toggles.Headers.Response.XRobotsTag {
		t.Error("Headers.Response.XRobotsTag should round-trip to true")
	}
}

func TestParseDomainParamsHeaderUp(t *testing.T) {
	rpCfg, _ := json.Marshal(ReverseProxyConfig{
		Upstream: "localhost:8080",
		HeaderUp: HeaderUpConfig{
			Enabled:       true,
			HostOverride:  true,
			HostValue:     "backend.internal",
			Authorization: true,
			AuthValue:     "Bearer tok",
		},
	})
	p := DomainParams{
		Domain:        "example.com",
		Upstream:      "localhost:8080",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}
	got := buildAndParse(t, p)

	var parsedRPCfg ReverseProxyConfig
	if err := json.Unmarshal(got.HandlerConfig, &parsedRPCfg); err != nil {
		t.Fatalf("failed to parse handler config: %v", err)
	}
	if !parsedRPCfg.HeaderUp.Enabled {
		t.Error("HeaderUp.Enabled should round-trip to true")
	}
	if !parsedRPCfg.HeaderUp.HostOverride {
		t.Error("HostOverride should round-trip to true")
	}
	if parsedRPCfg.HeaderUp.HostValue != "backend.internal" {
		t.Errorf("HostValue = %q, want backend.internal", parsedRPCfg.HeaderUp.HostValue)
	}
	if !parsedRPCfg.HeaderUp.Authorization {
		t.Error("Authorization should round-trip to true")
	}
	if parsedRPCfg.HeaderUp.AuthValue != "Bearer tok" {
		t.Errorf("AuthValue = %q, want Bearer tok", parsedRPCfg.HeaderUp.AuthValue)
	}
}

func TestParseDomainParamsAllBasicHeaders(t *testing.T) {
	rpCfg, _ := json.Marshal(ReverseProxyConfig{
		Upstream: "localhost:8080",
		HeaderUp: HeaderUpConfig{
			Enabled:       true,
			HostOverride:  true,
			HostValue:     "internal.host",
			Authorization: true,
			AuthValue:     "Basic creds",
		},
	})
	p := DomainParams{
		Domain:        "example.com",
		Upstream:      "localhost:8080",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
		Toggles: RouteToggles{
			Headers: HeadersConfig{
				Response: ResponseHeaders{
					Enabled:      true,
					Security:     true,
					CORS:         true,
					CORSOrigins:  []string{"https://app.com"},
					CacheControl: true,
					XRobotsTag:   true,
				},
			},
		},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.Headers.Response.Security {
		t.Error("Security should round-trip")
	}
	if !got.Toggles.Headers.Response.CORS {
		t.Error("CORS should round-trip")
	}
	if !got.Toggles.Headers.Response.CacheControl {
		t.Error("CacheControl should round-trip")
	}
	if !got.Toggles.Headers.Response.XRobotsTag {
		t.Error("XRobotsTag should round-trip")
	}

	var parsedRPCfg ReverseProxyConfig
	if err := json.Unmarshal(got.HandlerConfig, &parsedRPCfg); err != nil {
		t.Fatalf("failed to parse handler config: %v", err)
	}
	if !parsedRPCfg.HeaderUp.HostOverride {
		t.Error("HostOverride should round-trip")
	}
	if !parsedRPCfg.HeaderUp.Authorization {
		t.Error("Authorization should round-trip")
	}
}

// --- classifyHeaders ---

func TestClassifyHeadersMixed(t *testing.T) {
	headers := map[string][]string{
		"X-Frame-Options": {"DENY"},
		"Cache-Control":   {"no-store"},
		"X-Custom-Header": {"custom-val"},
		"X-Another":       {"another-val"},
	}
	builtin, custom := classifyHeaders(headers, builtinResponseKeys)

	builtinKeys := map[string]bool{}
	for _, e := range builtin {
		builtinKeys[e.Key] = true
		if !e.Enabled {
			t.Errorf("builtin entry %q should be Enabled", e.Key)
		}
	}
	if !builtinKeys["X-Frame-Options"] {
		t.Error("X-Frame-Options should be classified as builtin")
	}
	if !builtinKeys["Cache-Control"] {
		t.Error("Cache-Control should be classified as builtin")
	}

	customKeys := map[string]bool{}
	for _, e := range custom {
		customKeys[e.Key] = true
		if !e.Enabled {
			t.Errorf("custom entry %q should be Enabled", e.Key)
		}
	}
	if !customKeys["X-Custom-Header"] {
		t.Error("X-Custom-Header should be classified as custom")
	}
	if !customKeys["X-Another"] {
		t.Error("X-Another should be classified as custom")
	}
}

func TestClassifyHeadersAllUnknown(t *testing.T) {
	headers := map[string][]string{
		"X-Foo": {"bar"},
		"X-Baz": {"qux"},
	}
	builtin, custom := classifyHeaders(headers, builtinResponseKeys)
	if len(builtin) != 0 {
		t.Errorf("expected no builtins, got %d", len(builtin))
	}
	if len(custom) != 2 {
		t.Errorf("expected 2 custom entries, got %d", len(custom))
	}
}

func TestClassifyHeadersAllKnown(t *testing.T) {
	headers := map[string][]string{
		"Strict-Transport-Security": {"max-age=31536000"},
		"X-Content-Type-Options":    {"nosniff"},
	}
	builtin, custom := classifyHeaders(headers, builtinResponseKeys)
	if len(builtin) != 2 {
		t.Errorf("expected 2 builtins, got %d", len(builtin))
	}
	if len(custom) != 0 {
		t.Errorf("expected no custom entries, got %d", len(custom))
	}
}

func TestClassifyHeadersEmpty(t *testing.T) {
	builtin, custom := classifyHeaders(map[string][]string{}, builtinResponseKeys)
	if len(builtin) != 0 || len(custom) != 0 {
		t.Errorf("expected empty results for empty input, got %d builtin, %d custom", len(builtin), len(custom))
	}
}

func TestClassifyHeadersHeaderUpKeys(t *testing.T) {
	headers := map[string][]string{
		"Host":          {"backend.internal"},
		"Authorization": {"Bearer tok"},
		"X-Forwarded":   {"custom"},
	}
	builtin, custom := classifyHeaders(headers, builtinHeaderUpKeys)
	if len(builtin) != 2 {
		t.Errorf("expected 2 builtins (Host, Authorization), got %d", len(builtin))
	}
	if len(custom) != 1 {
		t.Errorf("expected 1 custom (X-Forwarded), got %d", len(custom))
	}
}

// --- Round-trip: Builtin/Custom array population ---

func TestRoundTripSecurityPopulatesBuiltin(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Response: ResponseHeaders{Enabled: true, Security: true},
		}},
	}
	got := buildAndParse(t, p)

	builtinKeys := map[string]bool{}
	for _, e := range got.Toggles.Headers.Response.Builtin {
		builtinKeys[e.Key] = true
		if !e.Enabled {
			t.Errorf("parsed builtin %q should be Enabled", e.Key)
		}
	}
	for _, key := range []string{
		"Strict-Transport-Security",
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Permissions-Policy",
	} {
		if !builtinKeys[key] {
			t.Errorf("expected %q in Response.Builtin after round-trip", key)
		}
	}
	if len(got.Toggles.Headers.Response.Custom) != 0 {
		t.Errorf("expected no Custom entries for security headers, got %d", len(got.Toggles.Headers.Response.Custom))
	}
}

func TestRoundTripCacheControlPopulatesBuiltin(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Response: ResponseHeaders{Enabled: true, CacheControl: true},
		}},
	}
	got := buildAndParse(t, p)

	found := false
	for _, e := range got.Toggles.Headers.Response.Builtin {
		if e.Key == "Cache-Control" {
			found = true
			if e.Value != "no-store" {
				t.Errorf("Cache-Control value = %q, want no-store", e.Value)
			}
			if !e.Enabled {
				t.Error("Cache-Control entry should be Enabled")
			}
		}
	}
	if !found {
		t.Error("Cache-Control not found in Response.Builtin after round-trip")
	}
}

func TestRoundTripAdvancedCustomHeadersSurvive(t *testing.T) {
	p := DomainParams{
		Domain:          "example.com",
		Upstream:        "localhost:8080",
		AdvancedHeaders: true,
		Toggles: RouteToggles{Headers: HeadersConfig{
			Response: ResponseHeaders{
				Enabled: true,
				Builtin: []HeaderEntry{
					{Key: "X-Frame-Options", Value: "DENY", Enabled: true},
				},
				Custom: []HeaderEntry{
					{Key: "X-My-Custom", Value: "hello", Enabled: true},
					{Key: "X-Another", Value: "world", Enabled: true},
				},
			},
		}},
	}
	got := buildAndParse(t, p)

	builtinKeys := map[string]bool{}
	for _, e := range got.Toggles.Headers.Response.Builtin {
		builtinKeys[e.Key] = true
	}
	customKeys := map[string]bool{}
	for _, e := range got.Toggles.Headers.Response.Custom {
		customKeys[e.Key] = true
	}

	if !builtinKeys["X-Frame-Options"] {
		t.Error("X-Frame-Options should be in Builtin after round-trip")
	}
	if !customKeys["X-My-Custom"] {
		t.Error("X-My-Custom should be in Custom after round-trip")
	}
	if !customKeys["X-Another"] {
		t.Error("X-Another should be in Custom after round-trip")
	}
}

func TestRoundTripHeaderUpPopulateBuiltin(t *testing.T) {
	rpCfg, _ := json.Marshal(ReverseProxyConfig{
		Upstream: "localhost:8080",
		HeaderUp: HeaderUpConfig{
			Enabled:       true,
			HostOverride:  true,
			HostValue:     "backend.internal",
			Authorization: true,
			AuthValue:     "Bearer tok",
		},
	})
	p := DomainParams{
		Domain:        "example.com",
		Upstream:      "localhost:8080",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}
	got := buildAndParse(t, p)

	var parsedRPCfg ReverseProxyConfig
	if err := json.Unmarshal(got.HandlerConfig, &parsedRPCfg); err != nil {
		t.Fatalf("failed to parse handler config: %v", err)
	}

	builtinKeys := map[string]string{}
	for _, e := range parsedRPCfg.HeaderUp.Builtin {
		builtinKeys[e.Key] = e.Value
	}

	if val, ok := builtinKeys["Host"]; !ok {
		t.Error("Host not found in HeaderUp.Builtin")
	} else if val != "backend.internal" {
		t.Errorf("Host value = %q, want backend.internal", val)
	}

	if val, ok := builtinKeys["Authorization"]; !ok {
		t.Error("Authorization not found in HeaderUp.Builtin")
	} else if val != "Bearer tok" {
		t.Errorf("Authorization value = %q, want Bearer tok", val)
	}
}

func TestRoundTripAdvancedHeaderUpCustomSurvives(t *testing.T) {
	rpCfg, _ := json.Marshal(ReverseProxyConfig{
		Upstream: "localhost:8080",
		HeaderUp: HeaderUpConfig{
			Enabled: true,
			Builtin: []HeaderEntry{
				{Key: "Host", Value: "custom.host", Operation: "set", Enabled: true},
			},
			Custom: []HeaderEntry{
				{Key: "X-Forwarded-Proto", Value: "https", Operation: "set", Enabled: true},
			},
		},
	})
	p := DomainParams{
		Domain:          "example.com",
		Upstream:        "localhost:8080",
		HandlerType:     "reverse_proxy",
		HandlerConfig:   rpCfg,
		AdvancedHeaders: true,
	}
	got := buildAndParse(t, p)

	var parsedRPCfg ReverseProxyConfig
	if err := json.Unmarshal(got.HandlerConfig, &parsedRPCfg); err != nil {
		t.Fatalf("failed to parse handler config: %v", err)
	}

	builtinKeys := map[string]bool{}
	for _, e := range parsedRPCfg.HeaderUp.Builtin {
		builtinKeys[e.Key] = true
	}
	customKeys := map[string]bool{}
	for _, e := range parsedRPCfg.HeaderUp.Custom {
		customKeys[e.Key] = true
	}

	if !builtinKeys["Host"] {
		t.Error("Host should be in HeaderUp.Builtin after round-trip")
	}
	if !customKeys["X-Forwarded-Proto"] {
		t.Error("X-Forwarded-Proto should be in HeaderUp.Custom after round-trip")
	}
}

func TestRoundTripMultiOriginCORSPopulatesBuiltin(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Response: ResponseHeaders{
				Enabled:     true,
				CORS:        true,
				CORSOrigins: []string{"https://a.com", "https://b.com"},
			},
		}},
	}
	got := buildAndParse(t, p)

	builtinByKey := map[string]string{}
	for _, e := range got.Toggles.Headers.Response.Builtin {
		builtinByKey[e.Key] = e.Value
	}

	if _, ok := builtinByKey["Access-Control-Allow-Origin"]; !ok {
		t.Error("Access-Control-Allow-Origin not found in Builtin after CORS round-trip")
	}
	if _, ok := builtinByKey["Access-Control-Allow-Methods"]; !ok {
		t.Error("Access-Control-Allow-Methods not found in Builtin after CORS round-trip")
	}
	if _, ok := builtinByKey["Access-Control-Allow-Headers"]; !ok {
		t.Error("Access-Control-Allow-Headers not found in Builtin after CORS round-trip")
	}

	// Verify no duplicate CORS method/header entries (should come from first subroute route only)
	methodCount := 0
	for _, e := range got.Toggles.Headers.Response.Builtin {
		if e.Key == "Access-Control-Allow-Methods" {
			methodCount++
		}
	}
	if methodCount != 1 {
		t.Errorf("expected 1 Access-Control-Allow-Methods entry, got %d (possible duplicate from multiple subroute routes)", methodCount)
	}
}

func TestRoundTripAdvancedDisabledEntriesNotInOutput(t *testing.T) {
	p := DomainParams{
		Domain:          "example.com",
		Upstream:        "localhost:8080",
		AdvancedHeaders: true,
		Toggles: RouteToggles{Headers: HeadersConfig{
			Response: ResponseHeaders{
				Enabled: true,
				Builtin: []HeaderEntry{
					{Key: "X-Frame-Options", Value: "DENY", Enabled: true},
					{Key: "X-Content-Type-Options", Value: "nosniff", Enabled: false},
				},
				Custom: []HeaderEntry{
					{Key: "X-Active", Value: "yes", Enabled: true},
					{Key: "X-Inactive", Value: "no", Enabled: false},
				},
			},
		}},
	}
	got := buildAndParse(t, p)

	allKeys := map[string]bool{}
	for _, e := range got.Toggles.Headers.Response.Builtin {
		allKeys[e.Key] = true
	}
	for _, e := range got.Toggles.Headers.Response.Custom {
		allKeys[e.Key] = true
	}

	if !allKeys["X-Frame-Options"] {
		t.Error("enabled builtin X-Frame-Options should survive round-trip")
	}
	if !allKeys["X-Active"] {
		t.Error("enabled custom X-Active should survive round-trip")
	}
	if allKeys["X-Content-Type-Options"] {
		t.Error("disabled builtin X-Content-Type-Options should NOT appear after round-trip")
	}
	if allKeys["X-Inactive"] {
		t.Error("disabled custom X-Inactive should NOT appear after round-trip")
	}
}

// --- Header Down (reverse_proxy response headers) ---

func TestBasicHeaderDownStripServer(t *testing.T) {
	cfg := HeaderDownConfig{
		Enabled:     true,
		StripServer: true,
	}
	result := BuildHeaderDown(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	deletes, ok := result["delete"].([]string)
	if !ok {
		t.Fatal("expected delete field in result")
	}
	if len(deletes) != 1 || deletes[0] != "Server" {
		t.Errorf("delete = %v, want [Server]", deletes)
	}
}

func TestBasicHeaderDownStripPoweredBy(t *testing.T) {
	cfg := HeaderDownConfig{
		Enabled:        true,
		StripPoweredBy: true,
	}
	result := BuildHeaderDown(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	deletes, ok := result["delete"].([]string)
	if !ok {
		t.Fatal("expected delete field in result")
	}
	if len(deletes) != 1 || deletes[0] != "X-Powered-By" {
		t.Errorf("delete = %v, want [X-Powered-By]", deletes)
	}
}

func TestBasicHeaderDownBothStrips(t *testing.T) {
	cfg := HeaderDownConfig{
		Enabled:        true,
		StripServer:    true,
		StripPoweredBy: true,
	}
	result := BuildHeaderDown(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	deletes, ok := result["delete"].([]string)
	if !ok {
		t.Fatal("expected delete field in result")
	}
	if len(deletes) != 2 {
		t.Errorf("expected 2 deletes, got %d", len(deletes))
	}
}

func TestBasicHeaderDownDeferred(t *testing.T) {
	cfg := HeaderDownConfig{
		Enabled:     true,
		StripServer: true,
		Deferred:    true,
	}
	result := BuildHeaderDown(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if deferred, ok := result["deferred"].(bool); !ok || !deferred {
		t.Error("expected deferred=true in result")
	}
}

func TestBasicHeaderDownNotDeferred(t *testing.T) {
	cfg := HeaderDownConfig{
		Enabled:     true,
		StripServer: true,
		Deferred:    false,
	}
	result := BuildHeaderDown(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if _, ok := result["deferred"]; ok {
		t.Error("deferred should not be present when false")
	}
}

func TestBasicHeaderDownDisabledReturnsNil(t *testing.T) {
	cfg := HeaderDownConfig{
		StripServer:    true,
		StripPoweredBy: true,
	}
	result := BuildHeaderDown(cfg, false)
	if result != nil {
		t.Errorf("expected nil when disabled, got %v", result)
	}
}

func TestBasicHeaderDownNothingEnabledReturnsNil(t *testing.T) {
	cfg := HeaderDownConfig{Enabled: true}
	result := BuildHeaderDown(cfg, false)
	if result != nil {
		t.Errorf("expected nil when no strips enabled, got %v", result)
	}
}

func TestAdvancedHeaderDown(t *testing.T) {
	cfg := HeaderDownConfig{
		Enabled: true,
		Builtin: []HeaderEntry{
			{Key: "Server", Value: "", Operation: "delete", Enabled: true},
			{Key: "X-Powered-By", Value: "", Operation: "delete", Enabled: false},
		},
		Custom: []HeaderEntry{
			{Key: "X-Custom-Down", Value: "val", Operation: "set", Enabled: true},
		},
	}
	result := BuildHeaderDown(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	deletes, ok := result["delete"].([]string)
	if !ok || len(deletes) != 1 || deletes[0] != "Server" {
		t.Errorf("delete = %v, want [Server]", deletes)
	}
	set, ok := result["set"].(map[string][]string)
	if !ok {
		t.Fatal("expected set field")
	}
	if vals := set["X-Custom-Down"]; len(vals) == 0 || vals[0] != "val" {
		t.Errorf("X-Custom-Down = %v, want [val]", vals)
	}
}

func TestAdvancedHeaderDownDeferred(t *testing.T) {
	cfg := HeaderDownConfig{
		Enabled:  true,
		Deferred: true,
		Builtin: []HeaderEntry{
			{Key: "Server", Value: "", Operation: "delete", Enabled: true},
		},
	}
	result := BuildHeaderDown(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if deferred, ok := result["deferred"].(bool); !ok || !deferred {
		t.Error("expected deferred=true")
	}
}

func TestAdvancedHeaderDownCustomOverridesBuiltin(t *testing.T) {
	cfg := HeaderDownConfig{
		Enabled: true,
		Builtin: []HeaderEntry{
			{Key: "Server", Value: "", Operation: "delete", Enabled: true},
		},
		Custom: []HeaderEntry{
			{Key: "Server", Value: "custom-server", Operation: "set", Enabled: true},
		},
	}
	result := BuildHeaderDown(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set, ok := result["set"].(map[string][]string)
	if !ok {
		t.Fatal("expected set field (custom should override builtin)")
	}
	if vals := set["Server"]; len(vals) == 0 || vals[0] != "custom-server" {
		t.Errorf("Server = %v, want [custom-server]", vals)
	}
	if _, ok := result["delete"]; ok {
		t.Error("delete should not be present after custom override")
	}
}

// --- Domain-level request headers ---

func TestBuildDomainRequestDisabled(t *testing.T) {
	cfg := HeadersConfig{Request: DomainRequestHeaders{XForwardedFor: true}}
	result := buildDomainRequestHeaders(cfg, false)
	if result != nil {
		t.Errorf("expected nil when disabled, got %v", result)
	}
}

func TestBasicDomainRequestXForwardedFor(t *testing.T) {
	req := DomainRequestHeaders{Enabled: true, XForwardedFor: true}
	result := buildBasicDomainRequestHeaders(req)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set, ok := result["set"].(map[string][]string)
	if !ok {
		t.Fatal("expected set field")
	}
	if vals := set["X-Forwarded-For"]; len(vals) == 0 || vals[0] != "{http.request.remote.host}" {
		t.Errorf("X-Forwarded-For = %v, want [{http.request.remote.host}]", vals)
	}
}

func TestBasicDomainRequestXRealIP(t *testing.T) {
	req := DomainRequestHeaders{Enabled: true, XRealIP: true}
	result := buildBasicDomainRequestHeaders(req)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set := result["set"].(map[string][]string)
	if vals := set["X-Real-IP"]; len(vals) == 0 || vals[0] != "{http.request.remote.host}" {
		t.Errorf("X-Real-IP = %v, want [{http.request.remote.host}]", vals)
	}
}

func TestBasicDomainRequestXForwardedProto(t *testing.T) {
	req := DomainRequestHeaders{Enabled: true, XForwardedProto: true}
	result := buildBasicDomainRequestHeaders(req)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set := result["set"].(map[string][]string)
	if vals := set["X-Forwarded-Proto"]; len(vals) == 0 || vals[0] != "{http.request.scheme}" {
		t.Errorf("X-Forwarded-Proto = %v, want [{http.request.scheme}]", vals)
	}
}

func TestBasicDomainRequestXForwardedHost(t *testing.T) {
	req := DomainRequestHeaders{Enabled: true, XForwardedHost: true}
	result := buildBasicDomainRequestHeaders(req)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set := result["set"].(map[string][]string)
	if vals := set["X-Forwarded-Host"]; len(vals) == 0 || vals[0] != "{http.request.host}" {
		t.Errorf("X-Forwarded-Host = %v, want [{http.request.host}]", vals)
	}
}

func TestBasicDomainRequestXRequestID(t *testing.T) {
	req := DomainRequestHeaders{Enabled: true, XRequestID: true}
	result := buildBasicDomainRequestHeaders(req)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set := result["set"].(map[string][]string)
	if vals := set["X-Request-ID"]; len(vals) == 0 || vals[0] != "{http.request.uuid}" {
		t.Errorf("X-Request-ID = %v, want [{http.request.uuid}]", vals)
	}
}

func TestBasicDomainRequestAllToggles(t *testing.T) {
	req := DomainRequestHeaders{
		Enabled:         true,
		XForwardedFor:   true,
		XRealIP:         true,
		XForwardedProto: true,
		XForwardedHost:  true,
		XRequestID:      true,
	}
	result := buildBasicDomainRequestHeaders(req)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set := result["set"].(map[string][]string)
	if len(set) != 5 {
		t.Errorf("expected 5 headers, got %d", len(set))
	}
}

func TestBasicDomainRequestNothingEnabledReturnsNil(t *testing.T) {
	req := DomainRequestHeaders{Enabled: true}
	result := buildBasicDomainRequestHeaders(req)
	if result != nil {
		t.Errorf("expected nil when no toggles enabled, got %v", result)
	}
}

func TestAdvancedDomainRequest(t *testing.T) {
	req := DomainRequestHeaders{
		Enabled: true,
		Builtin: []HeaderEntry{
			{Key: "X-Forwarded-For", Value: "{http.request.remote.host}", Operation: "set", Enabled: true},
			{Key: "X-Real-IP", Value: "{http.request.remote.host}", Operation: "set", Enabled: false},
		},
		Custom: []HeaderEntry{
			{Key: "X-Custom-Req", Value: "custom-val", Operation: "set", Enabled: true},
		},
	}
	result := buildAdvancedDomainRequestHeaders(req)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	set := result["set"].(map[string][]string)
	if vals := set["X-Forwarded-For"]; len(vals) == 0 || vals[0] != "{http.request.remote.host}" {
		t.Errorf("X-Forwarded-For = %v, want [{http.request.remote.host}]", vals)
	}
	if _, ok := set["X-Real-IP"]; ok {
		t.Error("disabled entry should not appear")
	}
	if vals := set["X-Custom-Req"]; len(vals) == 0 || vals[0] != "custom-val" {
		t.Errorf("X-Custom-Req = %v, want [custom-val]", vals)
	}
}

// --- Operations through build pipeline (add, delete, replace) ---

func TestGroupByOperationAllTypes(t *testing.T) {
	entries := []HeaderEntry{
		{Key: "X-Set", Value: "val1", Operation: "set", Enabled: true},
		{Key: "X-Add", Value: "val2", Operation: "add", Enabled: true},
		{Key: "X-Add", Value: "val3", Operation: "add", Enabled: true},
		{Key: "X-Del", Value: "", Operation: "delete", Enabled: true},
		{Key: "X-Rep", Value: "new", Operation: "replace", Search: "old", Enabled: true},
		{Key: "X-Disabled", Value: "no", Operation: "set", Enabled: false},
		{Key: "", Value: "no-key", Operation: "set", Enabled: true},
	}
	g := groupByOperation(entries)

	if vals := g.set["X-Set"]; len(vals) != 1 || vals[0] != "val1" {
		t.Errorf("set[X-Set] = %v, want [val1]", vals)
	}
	if vals := g.add["X-Add"]; len(vals) != 2 {
		t.Errorf("add[X-Add] = %v, want 2 values", vals)
	}
	if len(g.del) != 1 || g.del[0] != "X-Del" {
		t.Errorf("del = %v, want [X-Del]", g.del)
	}
	if reps := g.replace["X-Rep"]; len(reps) != 1 || reps[0]["search"] != "old" || reps[0]["replace"] != "new" {
		t.Errorf("replace[X-Rep] = %v, want [{search:old, replace:new}]", reps)
	}
	if _, ok := g.set["X-Disabled"]; ok {
		t.Error("disabled entry should be skipped")
	}
	if _, ok := g.set[""]; ok {
		t.Error("empty-key entry should be skipped")
	}
}

func TestAdvancedResponseAddOperation(t *testing.T) {
	resp := ResponseHeaders{
		Custom: []HeaderEntry{
			{Key: "X-Multi", Value: "first", Operation: "add", Enabled: true},
			{Key: "X-Extra", Value: "second", Operation: "add", Enabled: true},
		},
	}
	handlers := buildAdvancedResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}
	raw := marshalHandler(t, handlers[0])
	var h struct {
		Response struct {
			Add map[string][]string `json:"add"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if vals := h.Response.Add["X-Multi"]; len(vals) != 1 || vals[0] != "first" {
		t.Errorf("add[X-Multi] = %v, want [first]", vals)
	}
	if vals := h.Response.Add["X-Extra"]; len(vals) != 1 || vals[0] != "second" {
		t.Errorf("add[X-Extra] = %v, want [second]", vals)
	}
}

func TestAdvancedResponseDeleteOperation(t *testing.T) {
	resp := ResponseHeaders{
		Builtin: []HeaderEntry{
			{Key: "X-Remove-Me", Value: "", Operation: "delete", Enabled: true},
		},
	}
	handlers := buildAdvancedResponseHeaders(resp)
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}
	raw := marshalHandler(t, handlers[0])
	var h struct {
		Response struct {
			Delete []string `json:"delete"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(h.Response.Delete) != 1 || h.Response.Delete[0] != "X-Remove-Me" {
		t.Errorf("delete = %v, want [X-Remove-Me]", h.Response.Delete)
	}
}

func TestAdvancedHeaderUpAddOperation(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled: true,
		Custom: []HeaderEntry{
			{Key: "X-Trace", Value: "id1", Operation: "add", Enabled: true},
			{Key: "X-Trace", Value: "id2", Operation: "add", Enabled: true},
		},
	}
	result := BuildHeaderUp(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	add, ok := result["add"].(map[string][]string)
	if !ok {
		t.Fatal("expected add field")
	}
	if vals := add["X-Trace"]; len(vals) != 2 {
		t.Errorf("add[X-Trace] = %v, want 2 values", vals)
	}
}

func TestAdvancedHeaderUpDeleteOperation(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled: true,
		Custom: []HeaderEntry{
			{Key: "Accept-Encoding", Value: "", Operation: "delete", Enabled: true},
		},
	}
	result := BuildHeaderUp(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	del, ok := result["delete"].([]string)
	if !ok {
		t.Fatal("expected delete field")
	}
	if len(del) != 1 || del[0] != "Accept-Encoding" {
		t.Errorf("delete = %v, want [Accept-Encoding]", del)
	}
}

func TestAdvancedHeaderUpReplaceOperation(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled: true,
		Custom: []HeaderEntry{
			{Key: "Authorization", Value: "Bearer new", Operation: "replace", Search: "Bearer old", Enabled: true},
		},
	}
	result := BuildHeaderUp(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	replace, ok := result["replace"].(map[string][]map[string]string)
	if !ok {
		t.Fatal("expected replace field")
	}
	reps := replace["Authorization"]
	if len(reps) != 1 || reps[0]["search"] != "Bearer old" || reps[0]["replace"] != "Bearer new" {
		t.Errorf("replace[Authorization] = %v, want [{search: Bearer old, replace: Bearer new}]", reps)
	}
}

func TestAdvancedHeaderUpMixedOperations(t *testing.T) {
	cfg := HeaderUpConfig{
		Enabled: true,
		Custom: []HeaderEntry{
			{Key: "Host", Value: "backend.internal", Operation: "set", Enabled: true},
			{Key: "X-Extra", Value: "val", Operation: "add", Enabled: true},
			{Key: "Accept-Encoding", Value: "", Operation: "delete", Enabled: true},
			{Key: "Authorization", Value: "new", Operation: "replace", Search: "old", Enabled: true},
		},
	}
	result := BuildHeaderUp(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if _, ok := result["set"]; !ok {
		t.Error("expected set field")
	}
	if _, ok := result["add"]; !ok {
		t.Error("expected add field")
	}
	if _, ok := result["delete"]; !ok {
		t.Error("expected delete field")
	}
	if _, ok := result["replace"]; !ok {
		t.Error("expected replace field")
	}
}

func TestAdvancedDomainRequestReplaceOperation(t *testing.T) {
	req := DomainRequestHeaders{
		Enabled: true,
		Custom: []HeaderEntry{
			{Key: "X-Forwarded-For", Value: "127.0.0.1", Operation: "replace", Search: "10.0.0.1", Enabled: true},
		},
	}
	result := buildAdvancedDomainRequestHeaders(req)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	replace, ok := result["replace"].(map[string][]map[string]string)
	if !ok {
		t.Fatal("expected replace field")
	}
	reps := replace["X-Forwarded-For"]
	if len(reps) != 1 || reps[0]["search"] != "10.0.0.1" || reps[0]["replace"] != "127.0.0.1" {
		t.Errorf("replace = %v", reps)
	}
}

func TestAdvancedHeaderDownDeleteAndAdd(t *testing.T) {
	cfg := HeaderDownConfig{
		Enabled: true,
		Custom: []HeaderEntry{
			{Key: "Server", Value: "", Operation: "delete", Enabled: true},
			{Key: "X-Served-By", Value: "kaji", Operation: "add", Enabled: true},
		},
	}
	result := BuildHeaderDown(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	del, ok := result["delete"].([]string)
	if !ok || len(del) != 1 || del[0] != "Server" {
		t.Errorf("delete = %v, want [Server]", del)
	}
	add, ok := result["add"].(map[string][]string)
	if !ok {
		t.Fatal("expected add field")
	}
	if vals := add["X-Served-By"]; len(vals) != 1 || vals[0] != "kaji" {
		t.Errorf("add[X-Served-By] = %v, want [kaji]", vals)
	}
}

// --- Parse helpers ---

func TestAppendOpsToEntries(t *testing.T) {
	var entries []HeaderEntry
	ops := map[string][]string{
		"X-One": {"val1"},
		"X-Two": {"val2a", "val2b"},
	}
	appendOpsToEntries(&entries, ops, "set")
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Operation != "set" {
			t.Errorf("entry %q operation = %q, want set", e.Key, e.Operation)
		}
		if !e.Enabled {
			t.Errorf("entry %q should be Enabled", e.Key)
		}
	}
}

func TestAppendOpsToEntriesAdd(t *testing.T) {
	var entries []HeaderEntry
	ops := map[string][]string{"X-Multi": {"a", "b"}}
	appendOpsToEntries(&entries, ops, "add")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Operation != "add" {
			t.Errorf("operation = %q, want add", e.Operation)
		}
	}
}

func TestAppendDeleteEntries(t *testing.T) {
	var entries []HeaderEntry
	appendDeleteEntries(&entries, []string{"Server", "X-Powered-By"})
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Operation != "delete" {
			t.Errorf("entry %q operation = %q, want delete", e.Key, e.Operation)
		}
		if e.Value != "" {
			t.Errorf("delete entry %q should have empty value, got %q", e.Key, e.Value)
		}
		if !e.Enabled {
			t.Errorf("entry %q should be Enabled", e.Key)
		}
	}
}

func TestAppendDeleteEntriesEmpty(t *testing.T) {
	var entries []HeaderEntry
	appendDeleteEntries(&entries, nil)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for nil input, got %d", len(entries))
	}
}

func TestAppendReplaceEntries(t *testing.T) {
	var entries []HeaderEntry
	replacements := map[string][]map[string]string{
		"Authorization": {
			{"search": "Bearer old", "replace": "Bearer new"},
		},
		"Host": {
			{"search": "old.host", "replace": "new.host"},
			{"search": "alt.host", "replace": "final.host"},
		},
	}
	appendReplaceEntries(&entries, replacements)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Operation != "replace" {
			t.Errorf("entry %q operation = %q, want replace", e.Key, e.Operation)
		}
		if e.Search == "" {
			t.Errorf("replace entry %q should have non-empty Search", e.Key)
		}
		if !e.Enabled {
			t.Errorf("entry %q should be Enabled", e.Key)
		}
	}
}

func TestAppendReplaceEntriesEmpty(t *testing.T) {
	var entries []HeaderEntry
	appendReplaceEntries(&entries, nil)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for nil input, got %d", len(entries))
	}
}

// --- BuildDomain integration: domain request headers ---

func TestBuildDomainBasicRequestHeaders(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Request: DomainRequestHeaders{
				Enabled:       true,
				XForwardedFor: true,
				XRealIP:       true,
			},
		}},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")

	var parsed struct {
		Request struct {
			Set map[string][]string `json:"set"`
		} `json:"request"`
	}
	if err := json.Unmarshal(h, &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if vals := parsed.Request.Set["X-Forwarded-For"]; len(vals) == 0 {
		t.Error("X-Forwarded-For not found in request headers")
	}
	if vals := parsed.Request.Set["X-Real-IP"]; len(vals) == 0 {
		t.Error("X-Real-IP not found in request headers")
	}
}

func TestBuildDomainCombinedRequestAndResponseHeaders(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Request: DomainRequestHeaders{
				Enabled:       true,
				XForwardedFor: true,
			},
			Response: ResponseHeaders{
				Enabled:      true,
				CacheControl: true,
			},
		}},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")

	var parsed struct {
		Request struct {
			Set map[string][]string `json:"set"`
		} `json:"request"`
		Response struct {
			Set map[string][]string `json:"set"`
		} `json:"response"`
	}
	if err := json.Unmarshal(h, &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if vals := parsed.Request.Set["X-Forwarded-For"]; len(vals) == 0 {
		t.Error("request X-Forwarded-For not found")
	}
	if vals := parsed.Response.Set["Cache-Control"]; len(vals) == 0 || vals[0] != "no-store" {
		t.Error("response Cache-Control not found")
	}
}

func TestBuildDomainHeaderDown(t *testing.T) {
	rpCfg, _ := json.Marshal(ReverseProxyConfig{
		HeaderDown: HeaderDownConfig{
			Enabled:     true,
			StripServer: true,
			Deferred:    true,
		},
	})
	p := DomainParams{
		Domain:        "example.com",
		Upstream:      "localhost:8080",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	rp := findHandler(t, handlers, "reverse_proxy")

	var proxy struct {
		Headers struct {
			Response struct {
				Delete   []string `json:"delete"`
				Deferred bool     `json:"deferred"`
			} `json:"response"`
		} `json:"headers"`
	}
	if err := json.Unmarshal(rp, &proxy); err != nil {
		t.Fatalf("failed to parse reverse_proxy: %v", err)
	}
	if len(proxy.Headers.Response.Delete) != 1 || proxy.Headers.Response.Delete[0] != "Server" {
		t.Errorf("delete = %v, want [Server]", proxy.Headers.Response.Delete)
	}
	if !proxy.Headers.Response.Deferred {
		t.Error("expected deferred=true")
	}
}

// --- Round-trip: Header Down ---

func TestRoundTripHeaderDown(t *testing.T) {
	rpCfg, _ := json.Marshal(ReverseProxyConfig{
		Upstream: "localhost:8080",
		HeaderDown: HeaderDownConfig{
			Enabled:        true,
			StripServer:    true,
			StripPoweredBy: true,
			Deferred:       true,
		},
	})
	p := DomainParams{
		Domain:        "example.com",
		Upstream:      "localhost:8080",
		HandlerType:   "reverse_proxy",
		HandlerConfig: rpCfg,
	}
	got := buildAndParse(t, p)

	var parsedRPCfg ReverseProxyConfig
	if err := json.Unmarshal(got.HandlerConfig, &parsedRPCfg); err != nil {
		t.Fatalf("failed to parse handler config: %v", err)
	}
	if !parsedRPCfg.HeaderDown.Enabled {
		t.Error("HeaderDown.Enabled should round-trip")
	}
	if !parsedRPCfg.HeaderDown.StripServer {
		t.Error("StripServer should round-trip")
	}
	if !parsedRPCfg.HeaderDown.StripPoweredBy {
		t.Error("StripPoweredBy should round-trip")
	}
	if !parsedRPCfg.HeaderDown.Deferred {
		t.Error("Deferred should round-trip")
	}
}

func TestRoundTripAdvancedHeaderDownCustomSurvives(t *testing.T) {
	rpCfg, _ := json.Marshal(ReverseProxyConfig{
		Upstream: "localhost:8080",
		HeaderDown: HeaderDownConfig{
			Enabled: true,
			Builtin: []HeaderEntry{
				{Key: "Server", Value: "", Operation: "delete", Enabled: true},
			},
			Custom: []HeaderEntry{
				{Key: "X-Custom-Down", Value: "val", Operation: "set", Enabled: true},
			},
		},
	})
	p := DomainParams{
		Domain:          "example.com",
		Upstream:        "localhost:8080",
		HandlerType:     "reverse_proxy",
		HandlerConfig:   rpCfg,
		AdvancedHeaders: true,
	}
	got := buildAndParse(t, p)

	var parsedRPCfg ReverseProxyConfig
	if err := json.Unmarshal(got.HandlerConfig, &parsedRPCfg); err != nil {
		t.Fatalf("failed to parse handler config: %v", err)
	}

	builtinKeys := map[string]bool{}
	for _, e := range parsedRPCfg.HeaderDown.Builtin {
		builtinKeys[e.Key] = true
	}
	customKeys := map[string]bool{}
	for _, e := range parsedRPCfg.HeaderDown.Custom {
		customKeys[e.Key] = true
	}
	if !builtinKeys["Server"] {
		t.Error("Server should be in HeaderDown.Builtin after round-trip")
	}
	if !customKeys["X-Custom-Down"] {
		t.Error("X-Custom-Down should be in HeaderDown.Custom after round-trip")
	}
}

// --- Round-trip: Domain request headers ---

func TestRoundTripDomainRequestHeaders(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Request: DomainRequestHeaders{
				Enabled:         true,
				XForwardedFor:   true,
				XRealIP:         true,
				XForwardedProto: true,
				XForwardedHost:  true,
				XRequestID:      true,
			},
		}},
	}
	got := buildAndParse(t, p)
	req := got.Toggles.Headers.Request
	if !req.Enabled {
		t.Error("Request.Enabled should round-trip")
	}
	if !req.XForwardedFor {
		t.Error("XForwardedFor should round-trip")
	}
	if !req.XRealIP {
		t.Error("XRealIP should round-trip")
	}
	if !req.XForwardedProto {
		t.Error("XForwardedProto should round-trip")
	}
	if !req.XForwardedHost {
		t.Error("XForwardedHost should round-trip")
	}
	if !req.XRequestID {
		t.Error("XRequestID should round-trip")
	}
}

// --- Test helpers ---

func extractHeaderSet(t *testing.T, handler any) map[string][]string {
	t.Helper()
	data, err := json.Marshal(handler)
	if err != nil {
		t.Fatalf("failed to marshal handler: %v", err)
	}
	return extractHeaderSetFromRaw(t, data)
}

func extractHeaderSetFromRaw(t *testing.T, raw json.RawMessage) map[string][]string {
	t.Helper()
	var h struct {
		Response struct {
			Set map[string][]string `json:"set"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatalf("failed to parse headers handler: %v", err)
	}
	return h.Response.Set
}

func marshalHandler(t *testing.T, handler any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(handler)
	if err != nil {
		t.Fatalf("failed to marshal handler: %v", err)
	}
	return data
}
