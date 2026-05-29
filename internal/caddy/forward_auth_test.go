package caddy

import (
	"encoding/json"
	"strings"
	"testing"
)

// jsonNormalize round-trips a map through JSON so all nested types are
// consistent ([]any, map[string]any, float64, string).
func jsonNormalize(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// checkForwardAuthBase verifies the structure shared by all valid results:
// handler type, upstreams dial, rewrite, X-Forwarded headers, and fail response.
// Returns the normalized map for case-specific checks.
func checkForwardAuthBase(t *testing.T, h map[string]any, wantDial, wantURI string) map[string]any {
	t.Helper()
	m := jsonNormalize(t, h)

	if m["handler"] != "reverse_proxy" {
		t.Errorf("handler = %v, want reverse_proxy", m["handler"])
	}

	upstreams := m["upstreams"].([]any)
	if len(upstreams) != 1 {
		t.Fatalf("upstreams count = %d, want 1", len(upstreams))
	}
	up := upstreams[0].(map[string]any)
	if up["dial"] != wantDial {
		t.Errorf("dial = %v, want %q", up["dial"], wantDial)
	}

	rewrite := m["rewrite"].(map[string]any)
	if rewrite["method"] != "{http.request.method}" {
		t.Errorf("rewrite method = %v, want {http.request.method}", rewrite["method"])
	}
	if rewrite["uri"] != wantURI {
		t.Errorf("rewrite uri = %v, want %q", rewrite["uri"], wantURI)
	}

	headers := m["headers"].(map[string]any)
	req := headers["request"].(map[string]any)
	set := req["set"].(map[string]any)
	if _, ok := set["X-Forwarded-Method"]; !ok {
		t.Error("missing X-Forwarded-Method header")
	}
	if _, ok := set["X-Forwarded-URI"]; !ok {
		t.Error("missing X-Forwarded-URI header")
	}

	handleResp := m["handle_response"].([]any)
	if len(handleResp) != 2 {
		t.Fatalf("handle_response count = %d, want 2", len(handleResp))
	}

	success := handleResp[0].(map[string]any)
	match := success["match"].(map[string]any)
	codes := match["status_code"].([]any)
	if len(codes) != 1 || codes[0].(float64) != 2 {
		t.Errorf("success match status_code = %v, want [2]", codes)
	}

	fail := handleResp[1].(map[string]any)
	failRoutes := fail["routes"].([]any)
	failRoute := failRoutes[0].(map[string]any)
	failHandlers := failRoute["handle"].([]any)
	if len(failHandlers) != 2 {
		t.Fatalf("fail handler count = %d, want 2", len(failHandlers))
	}
	fh0 := failHandlers[0].(map[string]any)
	if fh0["handler"] != "copy_response" {
		t.Errorf("fail handler[0] = %v, want copy_response", fh0["handler"])
	}
	if fh0["status_code"] != "{http.reverse_proxy.status_code}" {
		t.Errorf("fail status_code = %v, want template", fh0["status_code"])
	}
	fh1 := failHandlers[1].(map[string]any)
	if fh1["handler"] != "copy_response_headers" {
		t.Errorf("fail handler[1] = %v, want copy_response_headers", fh1["handler"])
	}

	return m
}

// checkIdentityHeaders verifies the success response contains the expected
// provider identity headers with correct reverse_proxy template values.
func checkIdentityHeaders(t *testing.T, m map[string]any, wantHeaders []string) {
	t.Helper()
	handleResp := m["handle_response"].([]any)
	success := handleResp[0].(map[string]any)
	routes := success["routes"].([]any)
	route := routes[0].(map[string]any)
	handle := route["handle"].([]any)
	hh := handle[0].(map[string]any)

	if hh["handler"] != "headers" {
		t.Errorf("identity handler = %v, want headers", hh["handler"])
	}

	req := hh["request"].(map[string]any)
	set := req["set"].(map[string]any)

	if len(set) != len(wantHeaders) {
		t.Errorf("identity header count = %d, want %d", len(set), len(wantHeaders))
	}
	for _, hdr := range wantHeaders {
		vals, ok := set[hdr].([]any)
		if !ok || len(vals) != 1 {
			t.Errorf("missing or invalid identity header %q", hdr)
			continue
		}
		want := "{http.reverse_proxy.header." + hdr + "}"
		if vals[0] != want {
			t.Errorf("header %q = %v, want %q", hdr, vals[0], want)
		}
	}
}

func TestBuildForwardAuthHandler(t *testing.T) {
	t.Run("authelia_https", func(t *testing.T) {
		h, err := buildForwardAuthHandler(ForwardAuthConfig{
			Enabled:  true,
			Provider: "authelia",
			URL:      "https://auth.example.com/api/verify",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		m := checkForwardAuthBase(t, h, "auth.example.com:443", "/api/verify")

		transport := m["transport"].(map[string]any)
		if transport["protocol"] != "http" {
			t.Errorf("transport protocol = %v, want http", transport["protocol"])
		}
		if _, ok := transport["tls"]; !ok {
			t.Error("missing tls config in transport")
		}

		checkIdentityHeaders(t, m, []string{
			"Remote-User", "Remote-Groups", "Remote-Email", "Remote-Name",
		})
	})

	t.Run("authentik_http_custom_port", func(t *testing.T) {
		h, err := buildForwardAuthHandler(ForwardAuthConfig{
			Enabled:  true,
			Provider: "authentik",
			URL:      "http://auth.local:9000/outpost.goauthentik.io/auth/caddy",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		m := checkForwardAuthBase(t, h, "auth.local:9000", "/outpost.goauthentik.io/auth/caddy")

		if _, ok := m["transport"]; ok {
			t.Error("transport should not be present for HTTP")
		}

		checkIdentityHeaders(t, m, []string{
			"X-authentik-username", "X-authentik-groups",
			"X-authentik-email", "X-authentik-name", "X-authentik-uid",
		})
	})

	t.Run("custom_no_identity_headers", func(t *testing.T) {
		h, err := buildForwardAuthHandler(ForwardAuthConfig{
			Enabled:  true,
			Provider: "custom",
			URL:      "http://auth.local/check",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		m := checkForwardAuthBase(t, h, "auth.local:80", "/check")

		if _, ok := m["transport"]; ok {
			t.Error("transport should not be present for HTTP")
		}

		handleResp := m["handle_response"].([]any)
		success := handleResp[0].(map[string]any)
		if _, ok := success["routes"]; ok {
			t.Error("custom provider should not have routes in success response")
		}
	})

	t.Run("url_with_query", func(t *testing.T) {
		h, err := buildForwardAuthHandler(ForwardAuthConfig{
			Enabled:  true,
			Provider: "authelia",
			URL:      "http://auth.local/api/verify?rd=https://app.example.com",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		checkForwardAuthBase(t, h, "auth.local:80", "/api/verify?rd=https://app.example.com")
	})

	t.Run("invalid_url", func(t *testing.T) {
		_, err := buildForwardAuthHandler(ForwardAuthConfig{
			URL: "://bad",
		})
		if err == nil {
			t.Fatal("expected error for invalid URL")
		}
		if !strings.Contains(err.Error(), "parsing forward auth URL") {
			t.Errorf("error = %q, want message about parsing URL", err)
		}
	})
}
