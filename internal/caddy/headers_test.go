package caddy

import (
	"encoding/json"
	"testing"
)

// --- Basic mode response headers ---

func TestBuildResponseHeadersDisabled(t *testing.T) {
	cfg := HeadersConfig{Enabled: false, Response: ResponseHeaders{Security: true}}
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
	cfg := HeadersConfig{
		Enabled: true,
		Request: RequestHeaders{
			HostOverride: true,
			HostValue:    "backend.internal",
		},
	}
	result := buildRequestHeaderSet(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if vals := result["Host"]; len(vals) == 0 || vals[0] != "backend.internal" {
		t.Errorf("Host = %v, want [backend.internal]", vals)
	}
}

func TestBasicRequestAuthorization(t *testing.T) {
	cfg := HeadersConfig{
		Enabled: true,
		Request: RequestHeaders{
			Authorization: true,
			AuthValue:     "Bearer token123",
		},
	}
	result := buildRequestHeaderSet(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if vals := result["Authorization"]; len(vals) == 0 || vals[0] != "Bearer token123" {
		t.Errorf("Authorization = %v, want [Bearer token123]", vals)
	}
}

func TestBasicRequestBothHeaders(t *testing.T) {
	cfg := HeadersConfig{
		Enabled: true,
		Request: RequestHeaders{
			HostOverride:  true,
			HostValue:     "backend.internal",
			Authorization: true,
			AuthValue:     "Bearer abc",
		},
	}
	result := buildRequestHeaderSet(cfg, false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 2 {
		t.Errorf("expected 2 headers, got %d", len(result))
	}
}

func TestBasicRequestDisabledReturnsNil(t *testing.T) {
	cfg := HeadersConfig{
		Enabled: false,
		Request: RequestHeaders{
			HostOverride: true,
			HostValue:    "backend.internal",
		},
	}
	result := buildRequestHeaderSet(cfg, false)
	if result != nil {
		t.Errorf("expected nil when disabled, got %v", result)
	}
}

func TestBasicRequestEmptyValueSkipped(t *testing.T) {
	cfg := HeadersConfig{
		Enabled: true,
		Request: RequestHeaders{
			HostOverride: true,
			HostValue:    "",
		},
	}
	result := buildRequestHeaderSet(cfg, false)
	if result != nil {
		t.Errorf("expected nil when host value is empty, got %v", result)
	}
}

func TestAdvancedRequestHeaders(t *testing.T) {
	cfg := HeadersConfig{
		Enabled: true,
		Request: RequestHeaders{
			Builtin: []HeaderEntry{
				{Key: "Host", Value: "builtin.host", Enabled: true},
				{Key: "X-Disabled", Value: "no", Enabled: false},
			},
			Custom: []HeaderEntry{
				{Key: "X-Custom-Req", Value: "custom-val", Enabled: true},
			},
		},
	}
	result := buildRequestHeaderSet(cfg, true)
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
	cfg := HeadersConfig{
		Enabled: true,
		Request: RequestHeaders{
			Builtin: []HeaderEntry{
				{Key: "Host", Value: "builtin.host", Enabled: true},
			},
			Custom: []HeaderEntry{
				{Key: "Host", Value: "custom.host", Enabled: true},
			},
		},
	}
	result := buildRequestHeaderSet(cfg, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if vals := result["Host"]; len(vals) == 0 || vals[0] != "custom.host" {
		t.Errorf("Host = %v, want [custom.host] (custom should override)", vals)
	}
}

// --- BuildRoute integration ---

func TestBuildRouteBasicCacheControl(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Enabled:  true,
			Response: ResponseHeaders{CacheControl: true},
		}},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")
	set := extractHeaderSetFromRaw(t, h)
	if vals := set["Cache-Control"]; len(vals) == 0 || vals[0] != "no-store" {
		t.Errorf("Cache-Control = %v, want [no-store]", vals)
	}
}

func TestBuildRouteBasicXRobotsTag(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Enabled:  true,
			Response: ResponseHeaders{XRobotsTag: true},
		}},
	}
	handlers := buildAndUnmarshalHandlers(t, p)
	h := findHandler(t, handlers, "headers")
	set := extractHeaderSetFromRaw(t, h)
	if vals := set["X-Robots-Tag"]; len(vals) == 0 || vals[0] != "noindex, nofollow" {
		t.Errorf("X-Robots-Tag = %v, want [noindex, nofollow]", vals)
	}
}

func TestBuildRouteRequestHeaders(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Enabled: true,
			Request: RequestHeaders{
				HostOverride:  true,
				HostValue:     "backend.internal",
				Authorization: true,
				AuthValue:     "Bearer xyz",
			},
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

func TestBuildRouteAdvancedMode(t *testing.T) {
	p := RouteParams{
		Domain:          "example.com",
		Upstream:        "localhost:8080",
		AdvancedHeaders: true,
		Toggles: RouteToggles{Headers: HeadersConfig{
			Enabled: true,
			Response: ResponseHeaders{
				Builtin: []HeaderEntry{
					{Key: "X-Frame-Options", Value: "DENY", Enabled: true},
				},
				Custom: []HeaderEntry{
					{Key: "X-Custom", Value: "val", Enabled: true},
				},
			},
			Request: RequestHeaders{
				Builtin: []HeaderEntry{
					{Key: "Host", Value: "advanced.host", Enabled: true},
				},
			},
		}},
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

// --- ParseRouteParams round-trip ---

func TestParseRouteParamsCacheControl(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Enabled:  true,
			Response: ResponseHeaders{CacheControl: true},
		}},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.Headers.Enabled {
		t.Error("Headers.Enabled should round-trip to true")
	}
	if !got.Toggles.Headers.Response.CacheControl {
		t.Error("Headers.Response.CacheControl should round-trip to true")
	}
}

func TestParseRouteParamsXRobotsTag(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Enabled:  true,
			Response: ResponseHeaders{XRobotsTag: true},
		}},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.Headers.Enabled {
		t.Error("Headers.Enabled should round-trip to true")
	}
	if !got.Toggles.Headers.Response.XRobotsTag {
		t.Error("Headers.Response.XRobotsTag should round-trip to true")
	}
}

func TestParseRouteParamsRequestHeaders(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Enabled: true,
			Request: RequestHeaders{
				HostOverride:  true,
				HostValue:     "backend.internal",
				Authorization: true,
				AuthValue:     "Bearer tok",
			},
		}},
	}
	got := buildAndParse(t, p)
	if !got.Toggles.Headers.Enabled {
		t.Error("Headers.Enabled should round-trip to true")
	}
	if !got.Toggles.Headers.Request.HostOverride {
		t.Error("HostOverride should round-trip to true")
	}
	if got.Toggles.Headers.Request.HostValue != "backend.internal" {
		t.Errorf("HostValue = %q, want backend.internal", got.Toggles.Headers.Request.HostValue)
	}
	if !got.Toggles.Headers.Request.Authorization {
		t.Error("Authorization should round-trip to true")
	}
	if got.Toggles.Headers.Request.AuthValue != "Bearer tok" {
		t.Errorf("AuthValue = %q, want Bearer tok", got.Toggles.Headers.Request.AuthValue)
	}
}

func TestParseRouteParamsAllBasicHeaders(t *testing.T) {
	p := RouteParams{
		Domain:   "example.com",
		Upstream: "localhost:8080",
		Toggles: RouteToggles{Headers: HeadersConfig{
			Enabled: true,
			Response: ResponseHeaders{
				Security:     true,
				CORS:         true,
				CORSOrigins:  []string{"https://app.com"},
				CacheControl: true,
				XRobotsTag:   true,
			},
			Request: RequestHeaders{
				HostOverride:  true,
				HostValue:     "internal.host",
				Authorization: true,
				AuthValue:     "Basic creds",
			},
		}},
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
	if !got.Toggles.Headers.Request.HostOverride {
		t.Error("HostOverride should round-trip")
	}
	if !got.Toggles.Headers.Request.Authorization {
		t.Error("Authorization should round-trip")
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
