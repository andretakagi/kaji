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

// --- Request headers ---

func TestBasicRequestHostOverride(t *testing.T) {
	req := RequestHeaders{
		Enabled:      true,
		HostOverride: true,
		HostValue:    "backend.internal",
	}
	result := BuildRequestHeaders(req, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if vals := result["Host"]; len(vals) == 0 || vals[0] != "backend.internal" {
		t.Errorf("Host = %v, want [backend.internal]", vals)
	}
}

func TestBasicRequestAuthorization(t *testing.T) {
	req := RequestHeaders{
		Enabled:       true,
		Authorization: true,
		AuthValue:     "Bearer token123",
	}
	result := BuildRequestHeaders(req, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if vals := result["Authorization"]; len(vals) == 0 || vals[0] != "Bearer token123" {
		t.Errorf("Authorization = %v, want [Bearer token123]", vals)
	}
}

func TestBasicRequestBothHeaders(t *testing.T) {
	req := RequestHeaders{
		Enabled:       true,
		HostOverride:  true,
		HostValue:     "backend.internal",
		Authorization: true,
		AuthValue:     "Bearer abc",
	}
	result := BuildRequestHeaders(req, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 2 {
		t.Errorf("expected 2 headers, got %d", len(result))
	}
}

func TestBasicRequestDisabledReturnsNil(t *testing.T) {
	req := RequestHeaders{
		HostOverride: true,
		HostValue:    "backend.internal",
	}
	result := BuildRequestHeaders(req, false)
	if result != nil {
		t.Errorf("expected nil when disabled, got %v", result)
	}
}

func TestBasicRequestEmptyValueSkipped(t *testing.T) {
	req := RequestHeaders{
		Enabled:      true,
		HostOverride: true,
		HostValue:    "",
	}
	result := BuildRequestHeaders(req, false)
	if result != nil {
		t.Errorf("expected nil when host value is empty, got %v", result)
	}
}

func TestAdvancedRequestHeaders(t *testing.T) {
	req := RequestHeaders{
		Enabled: true,
		Builtin: []HeaderEntry{
			{Key: "Host", Value: "builtin.host", Enabled: true},
			{Key: "X-Disabled", Value: "no", Enabled: false},
		},
		Custom: []HeaderEntry{
			{Key: "X-Custom-Req", Value: "custom-val", Enabled: true},
		},
	}
	result := BuildRequestHeaders(req, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if vals := result["Host"]; len(vals) == 0 || vals[0] != "builtin.host" {
		t.Errorf("Host = %v, want [builtin.host]", vals)
	}
	if _, ok := result["X-Disabled"]; ok {
		t.Error("disabled entry should not appear")
	}
	if vals := result["X-Custom-Req"]; len(vals) == 0 || vals[0] != "custom-val" {
		t.Errorf("X-Custom-Req = %v, want [custom-val]", vals)
	}
}

func TestAdvancedRequestCustomOverridesBuiltin(t *testing.T) {
	req := RequestHeaders{
		Enabled: true,
		Builtin: []HeaderEntry{
			{Key: "Host", Value: "builtin.host", Enabled: true},
		},
		Custom: []HeaderEntry{
			{Key: "Host", Value: "custom.host", Enabled: true},
		},
	}
	result := BuildRequestHeaders(req, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if vals := result["Host"]; len(vals) == 0 || vals[0] != "custom.host" {
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

func TestBuildDomainRequestHeaders(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{RequestHeaders: RequestHeaders{
			Enabled:       true,
			HostOverride:  true,
			HostValue:     "backend.internal",
			Authorization: true,
			AuthValue:     "Bearer xyz",
		}},
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
	p := DomainParams{
		Domain:          "example.com",
		Upstream:        "localhost:8080",
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
			RequestHeaders: RequestHeaders{
				Enabled: true,
				Builtin: []HeaderEntry{
					{Key: "Host", Value: "advanced.host", Enabled: true},
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

func TestParseDomainParamsRequestHeaders(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{RequestHeaders: RequestHeaders{
			Enabled:       true,
			HostOverride:  true,
			HostValue:     "backend.internal",
			Authorization: true,
			AuthValue:     "Bearer tok",
		}},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.RequestHeaders.Enabled {
		t.Error("RequestHeaders.Enabled should round-trip to true")
	}
	if !got.Toggles.RequestHeaders.HostOverride {
		t.Error("HostOverride should round-trip to true")
	}
	if got.Toggles.RequestHeaders.HostValue != "backend.internal" {
		t.Errorf("HostValue = %q, want backend.internal", got.Toggles.RequestHeaders.HostValue)
	}
	if !got.Toggles.RequestHeaders.Authorization {
		t.Error("Authorization should round-trip to true")
	}
	if got.Toggles.RequestHeaders.AuthValue != "Bearer tok" {
		t.Errorf("AuthValue = %q, want Bearer tok", got.Toggles.RequestHeaders.AuthValue)
	}
}

func TestParseDomainParamsAllBasicHeaders(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
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
			RequestHeaders: RequestHeaders{
				Enabled:       true,
				HostOverride:  true,
				HostValue:     "internal.host",
				Authorization: true,
				AuthValue:     "Basic creds",
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
	if !got.Toggles.RequestHeaders.HostOverride {
		t.Error("HostOverride should round-trip")
	}
	if !got.Toggles.RequestHeaders.Authorization {
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

func TestClassifyHeadersRequestKeys(t *testing.T) {
	headers := map[string][]string{
		"Host":          {"backend.internal"},
		"Authorization": {"Bearer tok"},
		"X-Forwarded":   {"custom"},
	}
	builtin, custom := classifyHeaders(headers, builtinRequestKeys)
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

func TestRoundTripRequestHeadersPopulateBuiltin(t *testing.T) {
	p := DomainParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{RequestHeaders: RequestHeaders{
			Enabled:       true,
			HostOverride:  true,
			HostValue:     "backend.internal",
			Authorization: true,
			AuthValue:     "Bearer tok",
		}},
	}
	got := buildAndParse(t, p)

	builtinKeys := map[string]string{}
	for _, e := range got.Toggles.RequestHeaders.Builtin {
		builtinKeys[e.Key] = e.Value
	}

	if val, ok := builtinKeys["Host"]; !ok {
		t.Error("Host not found in Request.Builtin")
	} else if val != "backend.internal" {
		t.Errorf("Host value = %q, want backend.internal", val)
	}

	if val, ok := builtinKeys["Authorization"]; !ok {
		t.Error("Authorization not found in Request.Builtin")
	} else if val != "Bearer tok" {
		t.Errorf("Authorization value = %q, want Bearer tok", val)
	}
}

func TestRoundTripAdvancedRequestCustomSurvives(t *testing.T) {
	p := DomainParams{
		Domain:          "example.com",
		Upstream:        "localhost:8080",
		AdvancedHeaders: true,
		Toggles: RouteToggles{RequestHeaders: RequestHeaders{
			Enabled: true,
			Builtin: []HeaderEntry{
				{Key: "Host", Value: "custom.host", Enabled: true},
			},
			Custom: []HeaderEntry{
				{Key: "X-Forwarded-Proto", Value: "https", Enabled: true},
			},
		}},
	}
	got := buildAndParse(t, p)

	builtinKeys := map[string]bool{}
	for _, e := range got.Toggles.RequestHeaders.Builtin {
		builtinKeys[e.Key] = true
	}
	customKeys := map[string]bool{}
	for _, e := range got.Toggles.RequestHeaders.Custom {
		customKeys[e.Key] = true
	}

	if !builtinKeys["Host"] {
		t.Error("Host should be in Request.Builtin after round-trip")
	}
	if !customKeys["X-Forwarded-Proto"] {
		t.Error("X-Forwarded-Proto should be in Request.Custom after round-trip")
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
