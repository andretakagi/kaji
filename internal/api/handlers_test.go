package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/logging"
	"github.com/andretakagi/kaji/internal/snapshot"
	"github.com/andretakagi/kaji/internal/system"
)

// testManager is a minimal CaddyManager that always reports running.
type testManager struct{}

func (m *testManager) Start() error          { return nil }
func (m *testManager) Stop() error           { return nil }
func (m *testManager) Restart() error        { return nil }
func (m *testManager) Status() (bool, error) { return true, nil }

var _ system.CaddyManager = (*testManager)(nil)

// fakeCaddy is an in-memory Caddy admin API that stores config as nested maps.
// It handles the paths the handlers actually hit during tests.
type fakeCaddy struct {
	mu     sync.Mutex
	config map[string]any
	routes map[string]json.RawMessage // keyed by @id
}

func newFakeCaddy() *fakeCaddy {
	return &fakeCaddy{
		config: map[string]any{"apps": map[string]any{}},
		routes: make(map[string]json.RawMessage),
	}
}

func (fc *fakeCaddy) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fc.mu.Lock()
		defer fc.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/load" && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var cfg map[string]any
			if json.Unmarshal(body, &cfg) == nil {
				fc.config = cfg
			}
			w.Write([]byte(`{}`))

		case r.URL.Path == "/adapt" && r.Method == http.MethodPost:
			w.Write([]byte(`{"result":{}}`))

		case r.URL.Path == "/reverse_proxy/upstreams":
			w.Write([]byte(`[]`))

		case strings.HasPrefix(r.URL.Path, "/id/"):
			id := strings.TrimPrefix(r.URL.Path, "/id/")
			switch r.Method {
			case http.MethodGet:
				if route, ok := fc.routes[id]; ok {
					w.Write(route)
				} else {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte(`{"error":"not found"}`))
				}
			case http.MethodDelete:
				delete(fc.routes, id)
				// Also remove from the config routes array
				fc.removeRouteFromConfig(id)
				w.Write([]byte(`{}`))
			}

		case r.URL.Path == "/config/" && r.Method == http.MethodGet:
			data, _ := json.Marshal(fc.config)
			w.Write(data)

		case strings.HasPrefix(r.URL.Path, "/config/"):
			path := strings.TrimPrefix(r.URL.Path, "/config/")
			path = strings.TrimRight(path, "/")

			switch r.Method {
			case http.MethodGet:
				val := fc.getPath(path)
				if val == nil {
					w.Write([]byte(`null`))
				} else {
					data, _ := json.Marshal(val)
					w.Write(data)
				}
			case http.MethodPost:
				body, _ := io.ReadAll(r.Body)
				var val any
				json.Unmarshal(body, &val)
				fc.setPath(path, val)
				// Track routes by @id
				fc.trackRoute(body)
				w.Write([]byte(`{}`))
			case http.MethodPatch:
				body, _ := io.ReadAll(r.Body)
				var val any
				json.Unmarshal(body, &val)
				fc.setPath(path, val)
				fc.trackRoute(body)
				w.Write([]byte(`{}`))
			case http.MethodDelete:
				fc.deletePath(path)
				w.Write([]byte(`{}`))
			}

		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`))
		}
	})
}

func (fc *fakeCaddy) trackRoute(body []byte) {
	var route struct {
		ID string `json:"@id"`
	}
	if json.Unmarshal(body, &route) == nil && route.ID != "" {
		fc.routes[route.ID] = json.RawMessage(body)
	}
}

func (fc *fakeCaddy) removeRouteFromConfig(id string) {
	apps, ok := fc.config["apps"].(map[string]any)
	if !ok {
		return
	}
	httpApp, ok := apps["http"].(map[string]any)
	if !ok {
		return
	}
	servers, ok := httpApp["servers"].(map[string]any)
	if !ok {
		return
	}
	for srvName, srvVal := range servers {
		srv, ok := srvVal.(map[string]any)
		if !ok {
			continue
		}
		routes, ok := srv["routes"].([]any)
		if !ok {
			continue
		}
		filtered := make([]any, 0, len(routes))
		for _, r := range routes {
			if m, ok := r.(map[string]any); ok {
				if m["@id"] == id {
					continue
				}
			}
			filtered = append(filtered, r)
		}
		srv["routes"] = filtered
		servers[srvName] = srv
	}
}

func (fc *fakeCaddy) getPath(path string) any {
	parts := strings.Split(path, "/")
	var current any = fc.config
	for _, p := range parts {
		if p == "" {
			continue
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[p]
		if !ok {
			return nil
		}
	}
	return current
}

func (fc *fakeCaddy) setPath(path string, val any) {
	parts := strings.Split(path, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "" {
			parts = append(parts[:i], parts[i+1:]...)
		}
	}
	if len(parts) == 0 {
		return
	}

	var current any = fc.config
	for _, p := range parts[:len(parts)-1] {
		m, ok := current.(map[string]any)
		if !ok {
			return
		}
		next, exists := m[p]
		if !exists {
			next = map[string]any{}
			m[p] = next
		}
		current = next
	}

	last := parts[len(parts)-1]
	m, ok := current.(map[string]any)
	if !ok {
		return
	}

	// If target is a slice and val is not a slice, append
	if existing, ok := m[last]; ok {
		if arr, isArr := existing.([]any); isArr {
			if _, valIsArr := val.([]any); !valIsArr {
				m[last] = append(arr, val)
				return
			}
		}
	}
	m[last] = val
}

func (fc *fakeCaddy) deletePath(path string) {
	parts := strings.Split(path, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "" {
			parts = append(parts[:i], parts[i+1:]...)
		}
	}
	if len(parts) == 0 {
		return
	}

	var current any = fc.config
	for _, p := range parts[:len(parts)-1] {
		m, ok := current.(map[string]any)
		if !ok {
			return
		}
		current = m[p]
	}
	if m, ok := current.(map[string]any); ok {
		delete(m, parts[len(parts)-1])
	}
}

// testHarness bundles everything needed for handler integration tests.
type testHarness struct {
	handler  http.Handler
	store    *config.ConfigStore
	ss       *snapshot.Store
	caddySrv *httptest.Server
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("KAJI_CONFIG_PATH", configPath)

	store := config.NewStoreWithPath(config.DefaultConfig(), configPath)

	fc := newFakeCaddy()
	caddySrv := httptest.NewServer(fc.handler())
	t.Cleanup(caddySrv.Close)

	cc := caddy.NewClient(func() string { return caddySrv.URL })
	snapDir := filepath.Join(tmpDir, "snapshots")
	ss := snapshot.NewStore(snapDir)

	mux := http.NewServeMux()
	pipeline := logging.NewLokiPipeline(store, filepath.Join(tmpDir, "positions.json"), func() map[string]string { return nil })
	h := RegisterRoutes(mux, store, &testManager{}, cc, ss, pipeline, "test")

	return &testHarness{
		handler:  h,
		store:    store,
		ss:       ss,
		caddySrv: caddySrv,
	}
}

// doSetup sends a POST /api/setup and returns the recorder.
func (th *testHarness) doSetup(t *testing.T, password string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"password":"` + password + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	return rec
}

// sessionCookie extracts the kaji_session cookie from a recorder.
func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "kaji_session" {
			return c
		}
	}
	return nil
}

// authedRequest builds a request with the session cookie attached.
func authedRequest(method, path string, body string, cookie *http.Cookie) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return req
}

// --- Setup tests ---

func TestHandleSetup(t *testing.T) {
	th := newTestHarness(t)

	rec := th.doSetup(t, "testpass")
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: got status %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("setup response status = %v, want ok", resp["status"])
	}

	cookie := sessionCookie(rec)
	if cookie == nil {
		t.Error("setup did not return a session cookie")
	}

	if !config.Exists() {
		t.Error("config file was not created after setup")
	}
}

func TestHandleSetupDuplicateReturnsConflict(t *testing.T) {
	th := newTestHarness(t)

	rec := th.doSetup(t, "testpass")
	if rec.Code != http.StatusOK {
		t.Fatalf("first setup: got %d, want 200", rec.Code)
	}
	cookie := sessionCookie(rec)

	// After setup, /api/setup requires auth since config now exists.
	// Use the session cookie so the request reaches the handler.
	body := `{"password":"testpass"}`
	req := authedRequest(http.MethodPost, "/api/setup", body, cookie)
	rec2 := httptest.NewRecorder()
	th.handler.ServeHTTP(rec2, req)

	if rec2.Code != http.StatusConflict {
		t.Errorf("second setup: got %d, want 409", rec2.Code)
	}
}

// --- Auth tests ---

func TestHandleLoginCorrectPassword(t *testing.T) {
	th := newTestHarness(t)
	th.doSetup(t, "testpass")

	body := `{"password":"testpass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cookie := sessionCookie(rec)
	if cookie == nil {
		t.Error("login did not return a session cookie")
	}
}

func TestHandleLoginWrongPassword(t *testing.T) {
	th := newTestHarness(t)
	th.doSetup(t, "testpass")

	body := `{"password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("login with wrong password: got %d, want 401", rec.Code)
	}
}

func TestHandleLogout(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/auth/logout", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("logout: got %d, want 200", rec.Code)
	}
}

// --- Route CRUD tests ---

func TestHandleRouteCreateAndDelete(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create route
	createBody := `{"domain":"example.com","upstream":"127.0.0.1:8080"}`
	req := authedRequest(http.MethodPost, "/api/routes", createBody, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create route: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	routeID, ok := createResp["@id"].(string)
	if !ok || routeID == "" {
		t.Fatal("create route did not return @id")
	}

	// Delete route
	req = authedRequest(http.MethodDelete, "/api/routes/"+routeID, "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("delete route: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRouteUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	createBody := `{"domain":"update-test.com","upstream":"127.0.0.1:3000"}`
	req := authedRequest(http.MethodPost, "/api/routes", createBody, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create route: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var createResp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	routeID := createResp["@id"].(string)

	updateBody := `{"domain":"update-test.com","upstream":"127.0.0.1:4000"}`
	req = authedRequest(http.MethodPut, "/api/routes/"+routeID, updateBody, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("update route: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRouteCreateDuplicate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	createBody := `{"domain":"dup.com","upstream":"127.0.0.1:8080"}`
	req := authedRequest(http.MethodPost, "/api/routes", createBody, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("first create: got %d, want 200", rec.Code)
	}

	req = authedRequest(http.MethodPost, "/api/routes", createBody, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate create: got %d, want 409", rec.Code)
	}
}

func TestHandleRouteRequiresAuth(t *testing.T) {
	th := newTestHarness(t)
	th.doSetup(t, "testpass")

	req := httptest.NewRequest(http.MethodPost, "/api/routes", strings.NewReader(`{"domain":"x.com","upstream":"1.2.3.4:80"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("route without auth: got %d, want 401", rec.Code)
	}
}

// --- Settings tests ---

func TestHandleSettingsGlobalToggles(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/settings/global-toggles", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("global toggles: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var toggles map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &toggles); err != nil {
		t.Fatalf("failed to parse toggles response: %v", err)
	}
	if _, ok := toggles["auto_https"]; !ok {
		t.Error("global toggles response missing auto_https field")
	}
}

func TestHandleSettingsACMEEmail(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/settings/acme-email", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("acme email get: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse acme email response: %v", err)
	}
	if _, ok := resp["email"]; !ok {
		t.Error("acme email response missing email field")
	}
}

func TestHandleSettingsAuthToggle(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Disable auth
	body := `{"auth_enabled":false}`
	req := authedRequest(http.MethodPut, "/api/settings/auth", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("auth toggle disable: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if cfg.AuthEnabled {
		t.Error("auth should be disabled after toggle")
	}

	// Re-enable auth (password hash was cleared on disable, so provide one)
	body = `{"auth_enabled":true,"password":"newpass"}`
	req = httptest.NewRequest(http.MethodPut, "/api/settings/auth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("auth toggle enable: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

// --- API key tests ---

func TestHandleAPIKeyLifecycle(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Generate key
	req := authedRequest(http.MethodPost, "/api/settings/api-key", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("api key generate: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var genResp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &genResp)
	if genResp["api_key"] == "" {
		t.Fatal("api key generate did not return a key")
	}

	// Check status
	req = authedRequest(http.MethodGet, "/api/settings/api-key", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("api key status: got %d, want 200", rec.Code)
	}

	var statusResp map[string]bool
	json.Unmarshal(rec.Body.Bytes(), &statusResp)
	if !statusResp["has_api_key"] {
		t.Error("api key status: expected has_api_key=true after generate")
	}

	// Revoke
	req = authedRequest(http.MethodDelete, "/api/settings/api-key", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("api key revoke: got %d, want 200", rec.Code)
	}

	// Verify revoked
	req = authedRequest(http.MethodGet, "/api/settings/api-key", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	json.Unmarshal(rec.Body.Bytes(), &statusResp)
	if statusResp["has_api_key"] {
		t.Error("api key status: expected has_api_key=false after revoke")
	}
}

func TestHandleAPIKeyAuth(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Generate key
	req := authedRequest(http.MethodPost, "/api/settings/api-key", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	var genResp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &genResp)
	apiKey := genResp["api_key"]

	// Use the API key to make an authenticated request
	req = httptest.NewRequest(http.MethodGet, "/api/settings/api-key", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("api key auth: got %d, want 200", rec.Code)
	}
}

// --- Snapshot tests ---

func TestHandleSnapshotCreateAndList(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create snapshot
	body := `{"name":"test-snapshot","description":"test description"}`
	req := authedRequest(http.MethodPost, "/api/snapshots", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create snapshot: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var snapResp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &snapResp)
	if snapResp["id"] == nil || snapResp["id"] == "" {
		t.Error("create snapshot did not return an id")
	}
	if snapResp["name"] != "test-snapshot" {
		t.Errorf("snapshot name = %v, want test-snapshot", snapResp["name"])
	}

	// List snapshots
	req = authedRequest(http.MethodGet, "/api/snapshots", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list snapshots: got %d, want 200", rec.Code)
	}

	var listResp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &listResp)
	snapshots, ok := listResp["snapshots"].([]any)
	if !ok || len(snapshots) == 0 {
		t.Error("list snapshots should contain at least one entry")
	}
}

func TestHandleSnapshotUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create
	req := authedRequest(http.MethodPost, "/api/snapshots", `{"name":"orig"}`, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create snapshot: got %d", rec.Code)
	}

	var snap map[string]any
	json.Unmarshal(rec.Body.Bytes(), &snap)
	snapID := snap["id"].(string)

	// Update
	updateBody := `{"name":"renamed","description":"updated desc"}`
	req = authedRequest(http.MethodPut, "/api/snapshots/"+snapID, updateBody, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("update snapshot: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSnapshotDelete(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create two snapshots so we can delete one (can't delete current)
	req := authedRequest(http.MethodPost, "/api/snapshots", `{"name":"first"}`, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create first snapshot: got %d", rec.Code)
	}

	var firstSnap map[string]any
	json.Unmarshal(rec.Body.Bytes(), &firstSnap)
	firstID := firstSnap["id"].(string)

	// Create second (becomes current)
	req = authedRequest(http.MethodPost, "/api/snapshots", `{"name":"second"}`, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create second snapshot: got %d", rec.Code)
	}

	// Delete first (not current)
	req = authedRequest(http.MethodDelete, "/api/snapshots/"+firstID, "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("delete snapshot: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSnapshotSettings(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"auto_snapshot_enabled":true,"auto_snapshot_limit":10}`
	req := authedRequest(http.MethodPut, "/api/snapshots/settings", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("snapshot settings: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	idx := th.ss.GetIndex()
	if !idx.AutoSnapshotEnabled {
		t.Error("auto snapshot should be enabled")
	}
	if idx.AutoSnapshotLimit != 10 {
		t.Errorf("auto snapshot limit = %d, want 10", idx.AutoSnapshotLimit)
	}
}

// --- Version endpoint test ---

func TestHandleVersion(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/version", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("version: got %d, want 200", rec.Code)
	}

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["version"] != "test" {
		t.Errorf("version = %q, want %q", resp["version"], "test")
	}
}

// --- Caddy status endpoint test ---

func TestHandleCaddyStatus(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/caddy/status", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("caddy status: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

// --- Setup status test ---

func TestHandleSetupStatus(t *testing.T) {
	th := newTestHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("setup status before setup: got %d, want 200", rec.Code)
	}

	var resp map[string]bool
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp["is_first_run"] {
		t.Error("expected is_first_run=true before setup")
	}

	th.doSetup(t, "testpass")

	req = httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["is_first_run"] {
		t.Error("expected is_first_run=false after setup")
	}
}

// --- Password change test ---

func TestHandlePasswordChange(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"new_password":"newpass"}`
	req := authedRequest(http.MethodPut, "/api/auth/password", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("password change: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	newCookie := sessionCookie(rec)
	if newCookie == nil {
		t.Error("password change should return a new session cookie")
	}

	// Old password should fail
	loginBody := `{"password":"testpass"}`
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("old password after change: got %d, want 401", rec.Code)
	}

	// New password should work
	loginBody = `{"password":"newpass"}`
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("new password after change: got %d, want 200", rec.Code)
	}
}

// --- Disabled routes test ---

func TestHandleDisabledRoutes(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/routes/disabled", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("disabled routes: got %d, want 200", rec.Code)
	}

	var routes []any
	json.Unmarshal(rec.Body.Bytes(), &routes)
	if len(routes) != 0 {
		t.Errorf("expected 0 disabled routes, got %d", len(routes))
	}
}

// --- Caddy service control tests ---

func TestHandleCaddyStart(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/caddy/start", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("caddy start: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("caddy start status = %q, want ok", resp["status"])
	}
}

func TestHandleCaddyStop(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/caddy/stop", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("caddy stop: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("caddy stop status = %q, want ok", resp["status"])
	}
}

func TestHandleCaddyRestart(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/caddy/restart", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("caddy restart: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("caddy restart status = %q, want ok", resp["status"])
	}
}

// --- Route disable/enable tests ---

func TestHandleRouteDisableAndEnable(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	createBody := `{"domain":"disable-test.com","upstream":"127.0.0.1:8080"}`
	req := authedRequest(http.MethodPost, "/api/routes", createBody, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create route: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	routeID := caddy.GenerateRouteID("disable-test.com")

	// Disable the route
	disableBody := `{"@id":"` + routeID + `"}`
	req = authedRequest(http.MethodPost, "/api/routes/disable", disableBody, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("disable route: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Verify it appears in the disabled list
	req = authedRequest(http.MethodGet, "/api/routes/disabled", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("disabled routes after disable: got %d, want 200", rec.Code)
	}

	var disabled []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &disabled)
	if len(disabled) != 1 {
		t.Fatalf("expected 1 disabled route, got %d", len(disabled))
	}

	// Enable the route
	enableBody := `{"@id":"` + routeID + `"}`
	req = authedRequest(http.MethodPost, "/api/routes/enable", enableBody, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("enable route: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Verify disabled list is empty again
	req = authedRequest(http.MethodGet, "/api/routes/disabled", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	var disabledAfter []any
	json.Unmarshal(rec.Body.Bytes(), &disabledAfter)
	if len(disabledAfter) != 0 {
		t.Errorf("expected 0 disabled routes after enable, got %d", len(disabledAfter))
	}
}

func TestHandleEnableRouteNotFound(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"@id":"kaji_nonexistent_com"}`
	req := authedRequest(http.MethodPost, "/api/routes/enable", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("enable nonexistent route: got %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

// --- Settings update tests ---

func TestHandleGlobalTogglesUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create a route first so the fake Caddy has a server in the config.
	// SetGlobalToggles iterates over existing servers, so without one
	// it's a no-op and the GET returns defaults.
	createBody := `{"domain":"toggles-test.com","upstream":"127.0.0.1:8080"}`
	req := authedRequest(http.MethodPost, "/api/routes", createBody, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create route: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	body := `{"auto_https":"off","prometheus_metrics":true,"per_host_metrics":false}`
	req = authedRequest(http.MethodPut, "/api/settings/global-toggles", body, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("global toggles update: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	req = authedRequest(http.MethodGet, "/api/settings/global-toggles", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("global toggles get after update: got %d, want 200", rec.Code)
	}

	var toggles map[string]any
	json.Unmarshal(rec.Body.Bytes(), &toggles)
	if toggles["auto_https"] != "off" {
		t.Errorf("auto_https = %v, want off", toggles["auto_https"])
	}
}

func TestHandleGlobalTogglesUpdateInvalidAutoHTTPS(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"auto_https":"bogus"}`
	req := authedRequest(http.MethodPut, "/api/settings/global-toggles", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid auto_https: got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleACMEEmailUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"email":"test@example.com"}`
	req := authedRequest(http.MethodPut, "/api/settings/acme-email", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("acme email update: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleACMEEmailUpdateInvalid(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"email":"not-an-email"}`
	req := authedRequest(http.MethodPut, "/api/settings/acme-email", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid email: got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAdvancedGetAndUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/settings/advanced", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("advanced get: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var getResp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &getResp)
	if _, ok := getResp["caddy_admin_url"]; !ok {
		t.Fatal("advanced get response missing caddy_admin_url")
	}

	body := `{"caddy_admin_url":"http://localhost:2020"}`
	req = authedRequest(http.MethodPut, "/api/settings/advanced", body, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("advanced update: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	req = authedRequest(http.MethodGet, "/api/settings/advanced", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	json.Unmarshal(rec.Body.Bytes(), &getResp)
	if getResp["caddy_admin_url"] != "http://localhost:2020" {
		t.Errorf("caddy_admin_url = %q, want http://localhost:2020", getResp["caddy_admin_url"])
	}
}

func TestHandleAdvancedUpdateInvalidURL(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"caddy_admin_url":"not a url"}`
	req := authedRequest(http.MethodPut, "/api/settings/advanced", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid URL: got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDNSProviderGetAndUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"enabled":true,"api_token":"testtoken123"}`
	req := authedRequest(http.MethodPut, "/api/settings/dns-provider", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("dns provider update: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	req = authedRequest(http.MethodGet, "/api/settings/dns-provider", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("dns provider get: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["enabled"] != true {
		t.Errorf("dns provider enabled = %v, want true", resp["enabled"])
	}
}

// --- Config proxy and load tests ---

func TestHandleConfigProxy(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/caddy/config", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("config proxy: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	if !json.Valid(rec.Body.Bytes()) {
		t.Error("config proxy response is not valid JSON")
	}
}

func TestHandleConfigLoad(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"apps":{}}`
	req := authedRequest(http.MethodPost, "/api/caddy/load", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("config load: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleConfigLoadInvalidJSON(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/caddy/load", "not json", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON load: got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpstreams(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/caddy/upstreams", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("upstreams: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	if !json.Valid(rec.Body.Bytes()) {
		t.Error("upstreams response is not valid JSON")
	}

	var arr []any
	if err := json.Unmarshal(rec.Body.Bytes(), &arr); err != nil {
		t.Errorf("upstreams response is not a JSON array: %v", err)
	}
}

func TestHandleCaddyfileExport(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/export/caddyfile", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("caddyfile export: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse caddyfile response: %v", err)
	}
	if _, ok := resp["content"]; !ok {
		t.Error("caddyfile response missing content field")
	}
}

// --- Log tests ---

func TestHandleLogsNoFile(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/logs", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("logs no file: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogConfigGet(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/logs/config", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("log config get: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse log config response: %v", err)
	}
	if _, ok := resp["logs"]; !ok {
		t.Error("log config response missing logs field")
	}
}

func TestHandleAccessDomains(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/logs/access-domains", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("access domains: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

// --- Snapshot restore test ---

func TestHandleSnapshotRestore(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/snapshots", `{"name":"restore-test"}`, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create snapshot: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var snap map[string]any
	json.Unmarshal(rec.Body.Bytes(), &snap)
	snapID := snap["id"].(string)

	req = authedRequest(http.MethodPost, "/api/snapshots/"+snapID+"/restore", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot restore: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("snapshot restore status = %q, want ok", resp["status"])
	}
}
