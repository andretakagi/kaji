package caddy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fullConfigWith builds a minimal Caddy config JSON with one server whose
// automatic_https and metrics fields are set as given (nil means omit).
func fullConfigWithServer(autoHTTPS map[string]any, metrics map[string]any) []byte {
	srv := map[string]any{}
	if autoHTTPS != nil {
		srv["automatic_https"] = autoHTTPS
	}
	if metrics != nil {
		srv["metrics"] = metrics
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
	b, _ := json.Marshal(cfg)
	return b
}

func TestGetGlobalTogglesOn(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fullConfigWithServer(nil, nil))
	})
	c := testClient(t, mux)
	got, err := c.GetGlobalToggles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AutoHTTPS != "on" {
		t.Errorf("AutoHTTPS = %q, want on", got.AutoHTTPS)
	}
	if got.PrometheusMetrics {
		t.Error("PrometheusMetrics should be false when metrics is absent")
	}
	if got.PerHostMetrics {
		t.Error("PerHostMetrics should be false when metrics is absent")
	}
}

func TestGetGlobalTogglesOff(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fullConfigWithServer(map[string]any{"disable": true}, nil))
	})
	c := testClient(t, mux)
	got, err := c.GetGlobalToggles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AutoHTTPS != "off" {
		t.Errorf("AutoHTTPS = %q, want off", got.AutoHTTPS)
	}
}

func TestGetGlobalTogglesDisableRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fullConfigWithServer(map[string]any{"disable_redirects": true}, nil))
	})
	c := testClient(t, mux)
	got, err := c.GetGlobalToggles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AutoHTTPS != "disable_redirects" {
		t.Errorf("AutoHTTPS = %q, want disable_redirects", got.AutoHTTPS)
	}
}

func TestGetGlobalTogglesMetrics(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fullConfigWithServer(nil, map[string]any{"per_host": true}))
	})
	c := testClient(t, mux)
	got, err := c.GetGlobalToggles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.PrometheusMetrics {
		t.Error("PrometheusMetrics should be true when metrics object is present")
	}
	if !got.PerHostMetrics {
		t.Error("PerHostMetrics should be true when per_host is true")
	}
}

func TestGetGlobalTogglesMetricsNoPerHost(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fullConfigWithServer(nil, map[string]any{}))
	})
	c := testClient(t, mux)
	got, err := c.GetGlobalToggles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.PrometheusMetrics {
		t.Error("PrometheusMetrics should be true when metrics object is present")
	}
	if got.PerHostMetrics {
		t.Error("PerHostMetrics should be false when per_host is absent")
	}
}

// capturedRequest records the method and path of requests to a given path prefix.
type capturedRequest struct {
	method string
	path   string
	body   []byte
}

func TestSetGlobalTogglesOff(t *testing.T) {
	var captured []capturedRequest

	mux := http.NewServeMux()
	// GET /config/apps/http/servers - return a servers map with one server
	mux.HandleFunc("/config/apps/http/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			servers := map[string]json.RawMessage{
				"srv0": json.RawMessage(`{}`),
			}
			b, _ := json.Marshal(servers)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
		body, _ := io.ReadAll(r.Body)
		captured = append(captured, capturedRequest{method: r.Method, path: r.URL.Path, body: body})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = append(captured, capturedRequest{method: r.Method, path: r.URL.Path, body: body})
		w.WriteHeader(http.StatusOK)
	})

	c := testClient(t, mux)
	err := c.SetGlobalToggles(&GlobalToggles{AutoHTTPS: "off", PrometheusMetrics: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect POST to automatic_https and DELETE to metrics
	var foundAutoHTTPS, foundMetrics bool
	for _, req := range captured {
		if strings.HasSuffix(req.path, "/automatic_https") && req.method == http.MethodPost {
			foundAutoHTTPS = true
			var v map[string]bool
			if err := json.Unmarshal(req.body, &v); err != nil {
				t.Fatalf("automatic_https body invalid JSON: %v", err)
			}
			if !v["disable"] {
				t.Errorf("automatic_https body disable = %v, want true", v["disable"])
			}
		}
		if strings.HasSuffix(req.path, "/metrics") && req.method == http.MethodDelete {
			foundMetrics = true
		}
	}
	if !foundAutoHTTPS {
		t.Error("expected POST to automatic_https for auto_https=off")
	}
	if !foundMetrics {
		t.Error("expected DELETE to metrics when PrometheusMetrics=false")
	}
}

func TestSetGlobalTogglesDisableRedirects(t *testing.T) {
	var captured []capturedRequest

	mux := http.NewServeMux()
	mux.HandleFunc("/config/apps/http/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			servers := map[string]json.RawMessage{"srv0": json.RawMessage(`{}`)}
			b, _ := json.Marshal(servers)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = append(captured, capturedRequest{method: r.Method, path: r.URL.Path, body: body})
		w.WriteHeader(http.StatusOK)
	})

	c := testClient(t, mux)
	err := c.SetGlobalToggles(&GlobalToggles{AutoHTTPS: "disable_redirects"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, req := range captured {
		if strings.HasSuffix(req.path, "/automatic_https") && req.method == http.MethodPost {
			found = true
			var v map[string]bool
			if err := json.Unmarshal(req.body, &v); err != nil {
				t.Fatalf("automatic_https body invalid JSON: %v", err)
			}
			if !v["disable_redirects"] {
				t.Errorf("automatic_https body disable_redirects = %v, want true", v["disable_redirects"])
			}
		}
	}
	if !found {
		t.Error("expected POST to automatic_https for auto_https=disable_redirects")
	}
}

func TestSetGlobalTogglesOnDeletesAutoHTTPS(t *testing.T) {
	var captured []capturedRequest

	mux := http.NewServeMux()
	mux.HandleFunc("/config/apps/http/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			servers := map[string]json.RawMessage{"srv0": json.RawMessage(`{}`)}
			b, _ := json.Marshal(servers)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = append(captured, capturedRequest{method: r.Method, path: r.URL.Path, body: body})
		w.WriteHeader(http.StatusOK)
	})

	c := testClient(t, mux)
	err := c.SetGlobalToggles(&GlobalToggles{AutoHTTPS: "on"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, req := range captured {
		if strings.HasSuffix(req.path, "/automatic_https") && req.method == http.MethodDelete {
			found = true
		}
	}
	if !found {
		t.Error("expected DELETE to automatic_https for auto_https=on")
	}
}

func TestSetGlobalTogglesMetrics(t *testing.T) {
	var captured []capturedRequest

	mux := http.NewServeMux()
	mux.HandleFunc("/config/apps/http/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			servers := map[string]json.RawMessage{"srv0": json.RawMessage(`{}`)}
			b, _ := json.Marshal(servers)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = append(captured, capturedRequest{method: r.Method, path: r.URL.Path, body: body})
		w.WriteHeader(http.StatusOK)
	})

	c := testClient(t, mux)
	err := c.SetGlobalToggles(&GlobalToggles{AutoHTTPS: "on", PrometheusMetrics: true, PerHostMetrics: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, req := range captured {
		if strings.HasSuffix(req.path, "/metrics") && req.method == http.MethodPost {
			found = true
			var v map[string]any
			if err := json.Unmarshal(req.body, &v); err != nil {
				t.Fatalf("metrics body invalid JSON: %v", err)
			}
			if v["per_host"] != true {
				t.Errorf("metrics per_host = %v, want true", v["per_host"])
			}
		}
	}
	if !found {
		t.Error("expected POST to metrics when PrometheusMetrics=true")
	}
}

// TLS policy config helpers

func policiesJSON(policies []map[string]any) []byte {
	b, _ := json.Marshal(policies)
	return b
}

func TestGetACMEEmailCatchAll(t *testing.T) {
	policies := []map[string]any{
		{
			"issuers": []map[string]any{
				{"module": "acme", "email": "admin@example.com"},
			},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/apps/tls/automation/policies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(policiesJSON(policies))
	})
	c := testClient(t, mux)
	email, err := c.GetACMEEmail()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "admin@example.com" {
		t.Errorf("email = %q, want admin@example.com", email)
	}
}

func TestGetACMEEmailPerDomainFallback(t *testing.T) {
	policies := []map[string]any{
		{
			"subjects": []string{"example.com"},
			"issuers": []map[string]any{
				{"module": "acme", "email": "per-domain@example.com"},
			},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/apps/tls/automation/policies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(policiesJSON(policies))
	})
	c := testClient(t, mux)
	email, err := c.GetACMEEmail()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "per-domain@example.com" {
		t.Errorf("email = %q, want per-domain@example.com", email)
	}
}

func TestGetACMEEmailMissing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/apps/tls/automation/policies", func(w http.ResponseWriter, r *http.Request) {
		// Return null to indicate missing path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("null"))
	})
	c := testClient(t, mux)
	email, err := c.GetACMEEmail()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "" {
		t.Errorf("email = %q, want empty string", email)
	}
}

func TestAcmeEmailFromPoliciesCatchAllPreferredOverPerDomain(t *testing.T) {
	policies := []tlsPolicy{
		{
			Subjects: []string{"example.com"},
			Issuers:  []tlsIssuer{{Module: "acme", Email: "per-domain@example.com"}},
		},
		{
			Subjects: nil,
			Issuers:  []tlsIssuer{{Module: "acme", Email: "catchall@example.com"}},
		},
	}
	got := acmeEmailFromPolicies(policies)
	if got != "catchall@example.com" {
		t.Errorf("email = %q, want catchall@example.com", got)
	}
}

func TestAcmeEmailFromPoliciesPerDomainFallback(t *testing.T) {
	policies := []tlsPolicy{
		{
			Subjects: []string{"example.com"},
			Issuers:  []tlsIssuer{{Module: "acme", Email: "per-domain@example.com"}},
		},
	}
	got := acmeEmailFromPolicies(policies)
	if got != "per-domain@example.com" {
		t.Errorf("email = %q, want per-domain@example.com", got)
	}
}

func TestAcmeEmailFromPoliciesEmpty(t *testing.T) {
	got := acmeEmailFromPolicies(nil)
	if got != "" {
		t.Errorf("email = %q, want empty string", got)
	}
}

func TestAcmeEmailFromPoliciesNonACMEModuleIgnored(t *testing.T) {
	policies := []tlsPolicy{
		{
			Subjects: nil,
			Issuers:  []tlsIssuer{{Module: "zerossl", Email: "wrong@example.com"}},
		},
	}
	got := acmeEmailFromPolicies(policies)
	if got != "" {
		t.Errorf("email = %q, want empty string for non-acme module", got)
	}
}

func TestSetACMEEmailCreatesWhenNoPolicies(t *testing.T) {
	var cascadePath string
	var cascadeBody []byte

	mux := http.NewServeMux()
	// policies path returns 404 / null to trigger the create path
	mux.HandleFunc("GET /config/apps/tls/automation/policies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("null"))
	})
	// Catch all POST requests to /config/ for the cascade writes
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			// Record the deepest successful write (first attempt in cascade)
			if cascadePath == "" {
				cascadePath = r.URL.Path
				cascadeBody = body
			}
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})

	c := testClient(t, mux)
	err := c.SetACMEEmail("new@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The cascade should have written something containing the email
	if cascadeBody == nil {
		t.Fatal("expected a POST request to set TLS config")
	}
	if !strings.Contains(string(cascadeBody), "new@example.com") {
		t.Errorf("cascade body does not contain email: %s", cascadeBody)
	}
}

func TestSetACMEEmailUpdatesCatchAll(t *testing.T) {
	policies := []map[string]any{
		{
			"issuers": []map[string]any{
				{"module": "acme", "email": "old@example.com"},
			},
		},
	}

	var patchPath string
	var patchBody []byte

	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/apps/tls/automation/policies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(policiesJSON(policies))
	})
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			body, _ := io.ReadAll(r.Body)
			patchPath = r.URL.Path
			patchBody = body
		}
		w.WriteHeader(http.StatusOK)
	})

	c := testClient(t, mux)
	err := c.SetACMEEmail("new@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(patchPath, "/issuers") {
		t.Errorf("PATCH path = %q, expected it to end with /issuers", patchPath)
	}
	if !strings.Contains(string(patchBody), "new@example.com") {
		t.Errorf("PATCH body does not contain email: %s", patchBody)
	}
}

func TestSetACMEEmailAppendsWhenNoCatchAll(t *testing.T) {
	// Existing policies are per-domain only - no catch-all
	policies := []map[string]any{
		{
			"subjects": []string{"example.com"},
			"issuers": []map[string]any{
				{"module": "acme", "email": "per-domain@example.com"},
			},
		},
	}

	var appendPath string
	var appendBody []byte

	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/apps/tls/automation/policies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(policiesJSON(policies))
	})
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			appendPath = r.URL.Path
			appendBody = body
		}
		w.WriteHeader(http.StatusOK)
	})

	c := testClient(t, mux)
	err := c.SetACMEEmail("new@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should POST to the policies array path (trailing slash = append)
	if !strings.HasSuffix(appendPath, "/policies/") {
		t.Errorf("append path = %q, expected it to end with /policies/", appendPath)
	}
	if !strings.Contains(string(appendBody), "new@example.com") {
		t.Errorf("append body does not contain email: %s", appendBody)
	}
}

func TestSetConfigCascadeSucceedsAtTarget(t *testing.T) {
	var received []capturedRequest

	mux := http.NewServeMux()
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			received = append(received, capturedRequest{method: r.Method, path: r.URL.Path, body: body})
			w.WriteHeader(http.StatusOK)
		}
	})

	c := testClient(t, mux)
	err := c.SetConfigCascade("apps/tls/automation", map[string]any{"policies": []any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should succeed on the first attempt (no cascade needed)
	if len(received) != 1 {
		t.Fatalf("expected 1 request, got %d", len(received))
	}
	if received[0].path != "/config/apps/tls/automation" {
		t.Errorf("path = %q, want /config/apps/tls/automation", received[0].path)
	}
}

func TestSetConfigCascadeCreatesParent(t *testing.T) {
	var received []capturedRequest
	attempts := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			received = append(received, capturedRequest{method: r.Method, path: r.URL.Path, body: body})
			attempts++
			// Fail first two attempts, succeed on third (parent level)
			if attempts < 3 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("path does not exist"))
				return
			}
			w.WriteHeader(http.StatusOK)
		}
	})

	c := testClient(t, mux)
	// path = "a/b/c" - fails at c, fails at b, succeeds at a with wrapped value
	err := c.SetConfigCascade("a/b/c", "value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 requests (cascade), got %d", len(received))
	}

	// First attempt: POST /config/a/b/c with "value"
	if received[0].path != "/config/a/b/c" {
		t.Errorf("first path = %q, want /config/a/b/c", received[0].path)
	}
	// Second attempt: POST /config/a/b with {"c": "value"}
	if received[1].path != "/config/a/b" {
		t.Errorf("second path = %q, want /config/a/b", received[1].path)
	}
	// Third attempt: POST /config/a with {"b": {"c": "value"}}
	if received[2].path != "/config/a" {
		t.Errorf("third path = %q, want /config/a", received[2].path)
	}

	// Verify the third body wraps the value correctly
	var v map[string]any
	if err := json.Unmarshal(received[2].body, &v); err != nil {
		t.Fatalf("third body invalid JSON: %v", err)
	}
	b, ok := v["b"].(map[string]any)
	if !ok {
		t.Fatalf("third body missing key b, got: %v", v)
	}
	if b["c"] != "value" {
		t.Errorf("third body b.c = %v, want value", b["c"])
	}
}

func TestSetConfigCascadeAllFail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	})

	c := testClient(t, mux)
	err := c.SetConfigCascade("x/y", "val")
	if err == nil {
		t.Fatal("expected error when all cascade levels fail")
	}
}
