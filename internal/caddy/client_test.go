package caddy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(func() string { return srv.URL })
}

func TestClientGetConfig(t *testing.T) {
	want := `{"apps":{"http":{"servers":{}}}}`
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/config/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(want))
	}))

	got, err := c.GetConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != want {
		t.Errorf("GetConfig = %s, want %s", got, want)
	}
}

func TestClientGetConfigNullIsValid(t *testing.T) {
	// GetConfig accepts null (fresh Caddy instance has no config)
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("null"))
	}))

	got, err := c.GetConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "null" {
		t.Errorf("GetConfig = %s, want null", got)
	}
}

func TestClientGetConfigPathFound(t *testing.T) {
	want := `{"routes":[]}`
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config/apps/http/servers/srv0/routes" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(want))
	}))

	got, err := c.GetConfigPath("apps/http/servers/srv0/routes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != want {
		t.Errorf("GetConfigPath = %s, want %s", got, want)
	}
}

func TestClientGetConfigPathNullError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("null"))
	}))

	_, err := c.GetConfigPath("apps/http/servers/missing/routes")
	if err == nil {
		t.Fatal("expected error for null response")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want 'does not exist'", err)
	}
}

func TestClientGetConfigPathMissingError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	_, err := c.GetConfigPath("apps/http/servers/missing/routes")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestClientLoadConfig(t *testing.T) {
	var receivedBody []byte
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/load" {
			http.NotFound(w, r)
			return
		}
		var err error
		receivedBody = make([]byte, r.ContentLength)
		_, err = r.Body.Read(receivedBody)
		_ = err
		w.WriteHeader(http.StatusOK)
	}))

	payload := []byte(`{"apps":{}}`)
	if err := c.LoadConfig(payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientLoadConfigNon200(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad config", http.StatusBadRequest)
	}))

	err := c.LoadConfig([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "rejected") {
		t.Errorf("error = %q, want 'rejected'", err)
	}
}

func TestClientDeleteByID(t *testing.T) {
	var deletedPath string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		deletedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	if err := c.DeleteByID("kaji_example_com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedPath != "/id/kaji_example_com" {
		t.Errorf("DELETE path = %q, want /id/kaji_example_com", deletedPath)
	}
}

func TestClientDeleteByIDNon200(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	err := c.DeleteByID("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestClientAddRouteServerExists(t *testing.T) {
	var postedPath string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/config/apps/http/servers/srv0/routes":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/config/apps/http/servers/srv0/routes":
			postedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))

	route := json.RawMessage(`{"@id":"kaji_example_com"}`)
	if err := c.AddRoute("srv0", route); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postedPath != "/config/apps/http/servers/srv0/routes" {
		t.Errorf("POST path = %q, want /config/apps/http/servers/srv0/routes", postedPath)
	}
}

func TestClientAddRouteBootstrapsServer(t *testing.T) {
	// First GET returns null (server missing), then POST to bootstrap, then POST route
	var calls []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/config/apps/http/servers/newserver/routes":
			// Simulate missing path - return null
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("null"))
		case r.Method == http.MethodPost:
			// Accept any POST (bootstrap or route append)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))

	route := json.RawMessage(`{"@id":"kaji_test_com"}`)
	if err := c.AddRoute("newserver", route); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must have at least a GET (checking routes), a POST (bootstrap), and a POST (add route)
	var gets, posts int
	for _, call := range calls {
		if strings.HasPrefix(call, "GET") {
			gets++
		}
		if strings.HasPrefix(call, "POST") {
			posts++
		}
	}
	if gets == 0 {
		t.Error("expected at least one GET to check routes existence")
	}
	if posts < 2 {
		t.Errorf("expected at least 2 POSTs (bootstrap + append), got %d", posts)
	}
}

func TestClientReplaceRouteByID(t *testing.T) {
	configWithRoute := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": map[string]any{
						"routes": []any{
							map[string]any{"@id": "kaji_old_com", "match": []any{}},
						},
					},
				},
			},
		},
	}
	configJSON, _ := json.Marshal(configWithRoute)

	var patchedPath string
	var patchedBody []byte
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/config/":
			w.Header().Set("Content-Type", "application/json")
			w.Write(configJSON)
		case r.Method == http.MethodPatch:
			patchedPath = r.URL.Path
			patchedBody = make([]byte, r.ContentLength)
			r.Body.Read(patchedBody)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))

	newRoute := json.RawMessage(`{"@id":"kaji_old_com","match":[{"host":["new.example.com"]}]}`)
	serverName, err := c.ReplaceRouteByID("kaji_old_com", newRoute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if serverName != "srv0" {
		t.Errorf("serverName = %q, want srv0", serverName)
	}
	if patchedPath != "/config/apps/http/servers/srv0/routes/0" {
		t.Errorf("PATCH path = %q, want /config/apps/http/servers/srv0/routes/0", patchedPath)
	}
	if string(patchedBody) != string(newRoute) {
		t.Errorf("PATCH body = %s, want %s", patchedBody, newRoute)
	}
}

func TestClientReplaceRouteByIDNotFound(t *testing.T) {
	configWithRoute := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": map[string]any{
						"routes": []any{
							map[string]any{"@id": "kaji_other_com"},
						},
					},
				},
			},
		},
	}
	configJSON, _ := json.Marshal(configWithRoute)

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/config/" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(configJSON)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := c.ReplaceRouteByID("kaji_nonexistent", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing route")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err)
	}
}

func TestClientFindRouteServer(t *testing.T) {
	configWithRoutes := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": map[string]any{
						"routes": []any{
							map[string]any{"@id": "kaji_example_com"},
						},
					},
					"srv1": map[string]any{
						"routes": []any{
							map[string]any{"@id": "kaji_other_com"},
						},
					},
				},
			},
		},
	}
	configJSON, _ := json.Marshal(configWithRoutes)

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/config/" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(configJSON)
			return
		}
		http.NotFound(w, r)
	}))

	name, err := c.FindRouteServer("kaji_example_com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "srv0" {
		t.Errorf("FindRouteServer = %q, want srv0", name)
	}
}

func TestClientFindRouteServerNotFound(t *testing.T) {
	configJSON, _ := json.Marshal(map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{},
			},
		},
	})

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(configJSON)
	}))

	_, err := c.FindRouteServer("kaji_missing")
	if err == nil {
		t.Fatal("expected error for missing route")
	}
	if !strings.Contains(err.Error(), "no server found") {
		t.Errorf("error = %q, want 'no server found'", err)
	}
}

func TestClientAdaptCaddyfile(t *testing.T) {
	adaptResult := json.RawMessage(`{"apps":{"http":{}}}`)
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/adapt" {
			http.NotFound(w, r)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "text/caddyfile" {
			http.Error(w, "wrong content type", http.StatusBadRequest)
			return
		}
		resp, _ := json.Marshal(map[string]any{"result": adaptResult})
		w.Header().Set("Content-Type", "application/json")
		w.Write(resp)
	}))

	got, err := c.AdaptCaddyfile("example.com { reverse_proxy localhost:8080 }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(adaptResult) {
		t.Errorf("AdaptCaddyfile = %s, want %s", got, adaptResult)
	}
}

func TestClientAdaptCaddyfileNon200(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid caddyfile", http.StatusUnprocessableEntity)
	}))

	_, err := c.AdaptCaddyfile("invalid caddyfile text")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "could not parse") {
		t.Errorf("error = %q, want 'could not parse'", err)
	}
}

func TestClientUnreachableServer(t *testing.T) {
	// Point client at a port where nothing is listening
	c := NewClient(func() string { return "http://127.0.0.1:1" })

	_, err := c.GetConfig()
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("error = %q, want 'unreachable'", err)
	}
}

func TestClientLoadConfigUnreachable(t *testing.T) {
	c := NewClient(func() string { return "http://127.0.0.1:1" })

	err := c.LoadConfig([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("error = %q, want 'unreachable'", err)
	}
}

func TestClientDeleteByIDUnreachable(t *testing.T) {
	c := NewClient(func() string { return "http://127.0.0.1:1" })

	err := c.DeleteByID("kaji_example_com")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("error = %q, want 'unreachable'", err)
	}
}
