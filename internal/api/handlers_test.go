package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

// errManager is a CaddyManager where each method returns a configurable error.
type errManager struct {
	startErr   error
	stopErr    error
	restartErr error
	statusErr  error
}

func (m *errManager) Start() error          { return m.startErr }
func (m *errManager) Stop() error           { return m.stopErr }
func (m *errManager) Restart() error        { return m.restartErr }
func (m *errManager) Status() (bool, error) { return false, m.statusErr }

var _ system.CaddyManager = (*errManager)(nil)

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
			caddyfileBytes, _ := io.ReadAll(r.Body)
			result := fc.buildAdaptResult(string(caddyfileBytes))
			w.Write(result)

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

// buildAdaptResult builds a minimal adapted JSON response from Caddyfile text.
// This approximates what Caddy's real /adapt endpoint returns.
func (fc *fakeCaddy) buildAdaptResult(caddyfileText string) []byte {
	result := map[string]any{}

	adminAddr := caddy.ParseCaddyfileAdminAddr(caddyfileText)
	if adminAddr != "" {
		result["admin"] = map[string]any{"listen": adminAddr}
	}

	data, _ := json.Marshal(map[string]any{"result": result})
	return data
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
	return newTestHarnessWithManager(t, &testManager{}, "")
}

func newTestHarnessWithVersion(t *testing.T, version string) *testHarness {
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
	h := RegisterRoutes(mux, store, &testManager{}, cc, ss, pipeline, version)

	return &testHarness{
		handler:  h,
		store:    store,
		ss:       ss,
		caddySrv: caddySrv,
	}
}

// newTestHarnessWithManager builds a test harness with a custom CaddyManager.
// If caddyURL is non-empty, the Caddy client points there instead of a live
// fake server (useful for testing unreachable-API branches).
func newTestHarnessWithManager(t *testing.T, mgr system.CaddyManager, caddyURL string) *testHarness {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("KAJI_CONFIG_PATH", configPath)

	store := config.NewStoreWithPath(config.DefaultConfig(), configPath)

	var caddySrv *httptest.Server
	if caddyURL == "" {
		fc := newFakeCaddy()
		caddySrv = httptest.NewServer(fc.handler())
		t.Cleanup(caddySrv.Close)
		caddyURL = caddySrv.URL
	}

	cc := caddy.NewClient(func() string { return caddyURL })
	snapDir := filepath.Join(tmpDir, "snapshots")
	ss := snapshot.NewStore(snapDir)

	mux := http.NewServeMux()
	pipeline := logging.NewLokiPipeline(store, filepath.Join(tmpDir, "positions.json"), func() map[string]string { return nil })
	h := RegisterRoutes(mux, store, mgr, cc, ss, pipeline, "test")

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
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse setup response: %v", err)
	}
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
	if resp["email"] != "" {
		t.Errorf("email = %v, want empty string (no email configured)", resp["email"])
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
	if err := json.Unmarshal(rec.Body.Bytes(), &genResp); err != nil {
		t.Fatalf("failed to parse api key generate response: %v", err)
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("failed to parse api key status response: %v", err)
	}
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

	if err := json.Unmarshal(rec.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("failed to parse api key status response: %v", err)
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &genResp); err != nil {
		t.Fatalf("failed to parse api key generate response: %v", err)
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &snapResp); err != nil {
		t.Fatalf("failed to parse create snapshot response: %v", err)
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse list snapshots response: %v", err)
	}
	snapshots, ok := listResp["snapshots"].([]any)
	if !ok || len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	entry, ok := snapshots[0].(map[string]any)
	if !ok {
		t.Fatal("snapshot entry is not an object")
	}
	if entry["name"] != "test-snapshot" {
		t.Errorf("listed snapshot name = %v, want test-snapshot", entry["name"])
	}
	if entry["description"] != "test description" {
		t.Errorf("listed snapshot description = %v, want test description", entry["description"])
	}
	if entry["id"] != snapResp["id"] {
		t.Errorf("listed snapshot id = %v, want %v", entry["id"], snapResp["id"])
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
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("failed to parse create snapshot response: %v", err)
	}
	snapID := snap["id"].(string)

	// Update
	updateBody := `{"name":"renamed","description":"updated desc"}`
	req = authedRequest(http.MethodPut, "/api/snapshots/"+snapID, updateBody, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("update snapshot: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Verify the update took effect by listing
	req = authedRequest(http.MethodGet, "/api/snapshots", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	var listResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse list response: %v", err)
	}
	snapshots := listResp["snapshots"].([]any)
	found := false
	for _, s := range snapshots {
		entry := s.(map[string]any)
		if entry["id"] == snapID {
			if entry["name"] != "renamed" {
				t.Errorf("updated snapshot name = %v, want renamed", entry["name"])
			}
			if entry["description"] != "updated desc" {
				t.Errorf("updated snapshot description = %v, want updated desc", entry["description"])
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("snapshot %s not found in list after update", snapID)
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
	if err := json.Unmarshal(rec.Body.Bytes(), &firstSnap); err != nil {
		t.Fatalf("failed to parse create snapshot response: %v", err)
	}
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
		t.Fatalf("delete snapshot: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Verify the deleted snapshot is gone
	req = authedRequest(http.MethodGet, "/api/snapshots", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	var listResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse list response: %v", err)
	}
	for _, s := range listResp["snapshots"].([]any) {
		entry := s.(map[string]any)
		if entry["id"] == firstID {
			t.Errorf("snapshot %s should have been deleted but still appears in list", firstID)
		}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse version response: %v", err)
	}
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

	var resp map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse caddy status response: %v", err)
	}
	if !resp["running"] {
		t.Error("caddy status running = false, want true")
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
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse setup status response: %v", err)
	}
	if !resp["is_first_run"] {
		t.Error("expected is_first_run=true before setup")
	}

	th.doSetup(t, "testpass")

	req = httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse setup status response: %v", err)
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse caddy start response: %v", err)
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse caddy stop response: %v", err)
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse caddy restart response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("caddy restart status = %q, want ok", resp["status"])
	}
}

// --- Caddy service control error tests ---

func TestHandleCaddyStatusError(t *testing.T) {
	mgr := &errManager{statusErr: errors.New("dbus timeout")}
	th := newTestHarnessWithManager(t, mgr, "")
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/caddy/status", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("caddy status error: got %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCaddyStartError(t *testing.T) {
	mgr := &errManager{startErr: errors.New("systemctl start failed")}
	th := newTestHarnessWithManager(t, mgr, "")
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/caddy/start", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("caddy start error: got %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCaddyStartWaitReadyError(t *testing.T) {
	orig := caddyReadyTimeout
	caddyReadyTimeout = 50 * time.Millisecond
	t.Cleanup(func() { caddyReadyTimeout = orig })

	// Use a live fake Caddy for setup, then kill it so WaitReady fails.
	th := newTestHarnessWithManager(t, &testManager{}, "")
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)
	th.caddySrv.Close()

	req := authedRequest(http.MethodPost, "/api/caddy/start", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("caddy start wait-ready: got %d, want 502; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCaddyStopError(t *testing.T) {
	mgr := &errManager{stopErr: errors.New("systemctl stop failed")}
	th := newTestHarnessWithManager(t, mgr, "")
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/caddy/stop", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("caddy stop error: got %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCaddyRestartError(t *testing.T) {
	mgr := &errManager{restartErr: errors.New("systemctl restart failed")}
	th := newTestHarnessWithManager(t, mgr, "")
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/caddy/restart", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("caddy restart error: got %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCaddyRestartWaitReadyError(t *testing.T) {
	orig := caddyReadyTimeout
	caddyReadyTimeout = 50 * time.Millisecond
	t.Cleanup(func() { caddyReadyTimeout = orig })

	th := newTestHarnessWithManager(t, &testManager{}, "")
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)
	th.caddySrv.Close()

	req := authedRequest(http.MethodPost, "/api/caddy/restart", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("caddy restart wait-ready: got %d, want 502; body: %s", rec.Code, rec.Body.String())
	}
}

// --- Settings update tests ---

func TestHandleGlobalTogglesUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create a domain first so the fake Caddy has a server in the config.
	// SetGlobalToggles iterates over existing servers, so without one
	// it's a no-op and the GET returns defaults.
	createBody := `{"name":"toggles-test.com","first_rule":{"match_type":"","handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	req := authedRequest(http.MethodPost, "/api/domains", createBody, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create domain: got %d, want 200; body: %s", rec.Code, rec.Body.String())
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
	if err := json.Unmarshal(rec.Body.Bytes(), &toggles); err != nil {
		t.Fatalf("failed to parse global toggles response: %v", err)
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to parse advanced settings response: %v", err)
	}
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

	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to parse advanced settings response: %v", err)
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse dns provider response: %v", err)
	}
	if resp["enabled"] != true {
		t.Errorf("dns provider enabled = %v, want true", resp["enabled"])
	}
	token, _ := resp["api_token"].(string)
	if token == "testtoken123" {
		t.Error("GET response should mask api_token, but returned raw value")
	}
	if token == "" {
		t.Error("GET response missing api_token field")
	}
	// Token "testtoken123" (12 chars) should be masked as "********n123"
	if token != "********n123" {
		t.Errorf("masked api_token = %q, want ********n123", token)
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
		t.Fatal("config proxy response is not valid JSON")
	}

	var cfg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("failed to parse config proxy response: %v", err)
	}
	if _, ok := cfg["apps"]; !ok {
		t.Error("config proxy response missing apps key")
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

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse logs response: %v", err)
	}
	entries, ok := resp["entries"].([]any)
	if !ok {
		t.Fatal("logs response missing entries array")
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 log entries, got %d", len(entries))
	}
	if resp["has_more"] != false {
		t.Errorf("has_more = %v, want false", resp["has_more"])
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
	logs, ok := resp["logs"]
	if !ok {
		t.Fatal("log config response missing logs field")
	}
	if _, ok := logs.(map[string]any); !ok {
		t.Fatalf("logs field is %T, want map", logs)
	}
	if _, ok := resp["log_dir"]; !ok {
		t.Error("log config response missing log_dir field")
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

	var domains map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &domains); err != nil {
		t.Fatalf("failed to parse access domains response: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected empty access domains map, got %d servers", len(domains))
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
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("failed to parse create snapshot response: %v", err)
	}
	snapID := snap["id"].(string)

	req = authedRequest(http.MethodPost, "/api/snapshots/"+snapID+"/restore", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot restore: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse snapshot restore response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("snapshot restore status = %q, want ok", resp["status"])
	}
}

// TestCaddyfileImportParity verifies that importing the same Caddyfile through
// the setup flow and the settings flow produces the same app config. This
// catches cases where a Caddyfile-derived setting (like admin URL) is handled
// in one path but forgotten in the other.
func TestCaddyfileImportParity(t *testing.T) {
	caddyfile := "{\n\tadmin 10.0.0.1:2019\n}\n"

	jsonBody := func(v any) string {
		b, _ := json.Marshal(v)
		return string(b)
	}

	// --- Setup flow: preview then submit ---
	setupHarness := newTestHarness(t)

	previewReq := httptest.NewRequest(http.MethodPost, "/api/setup/import/caddyfile",
		strings.NewReader(jsonBody(map[string]string{"caddyfile": caddyfile})))
	previewReq.Header.Set("Content-Type", "application/json")
	previewRec := httptest.NewRecorder()
	setupHarness.handler.ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("setup preview: got %d; body: %s", previewRec.Code, previewRec.Body.String())
	}

	var preview map[string]any
	if err := json.Unmarshal(previewRec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("failed to parse setup preview response: %v", err)
	}

	// Submit setup with the extracted settings (mimicking the frontend).
	setupBody := map[string]any{
		"password": "testpass",
	}
	if adminListen, ok := preview["admin_listen"].(string); ok && adminListen != "" {
		setupBody["caddy_admin_url"] = "http://" + adminListen
	}
	if adaptedCfg, ok := preview["adapted_config"]; ok {
		setupBody["caddyfile_json"] = adaptedCfg
	}

	setupReq := httptest.NewRequest(http.MethodPost, "/api/setup",
		strings.NewReader(jsonBody(setupBody)))
	setupReq.Header.Set("Content-Type", "application/json")
	setupRec := httptest.NewRecorder()
	setupHarness.handler.ServeHTTP(setupRec, setupReq)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup submit: got %d; body: %s", setupRec.Code, setupRec.Body.String())
	}

	setupCfg := setupHarness.store.Get()

	// --- Settings flow: direct import ---
	settingsHarness := newTestHarness(t)
	cookie := sessionCookie(settingsHarness.doSetup(t, "testpass"))

	importReq := authedRequest(http.MethodPost, "/api/import/caddyfile",
		jsonBody(map[string]string{"caddyfile": caddyfile}), cookie)
	importRec := httptest.NewRecorder()
	settingsHarness.handler.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("settings import: got %d; body: %s", importRec.Code, importRec.Body.String())
	}

	settingsCfg := settingsHarness.store.Get()

	// --- Compare Caddyfile-derived app config fields ---
	// When a new field is derived from imported Caddyfiles, add it here.
	if setupCfg.CaddyAdminURL != settingsCfg.CaddyAdminURL {
		t.Errorf("CaddyAdminURL mismatch: setup=%q, settings=%q",
			setupCfg.CaddyAdminURL, settingsCfg.CaddyAdminURL)
	}
}

// --- IP list tests ---

func TestHandleIPListCreateAndList(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// List should be empty initially
	req := authedRequest(http.MethodGet, "/api/ip-lists", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list ip lists (empty): got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var emptyList []any
	if err := json.Unmarshal(rec.Body.Bytes(), &emptyList); err != nil {
		t.Fatalf("failed to parse empty ip lists response: %v", err)
	}
	if len(emptyList) != 0 {
		t.Errorf("expected 0 ip lists, got %d", len(emptyList))
	}

	// Create an IP list
	createBody := `{"name":"test-blocklist","description":"blocks bad IPs","type":"blacklist","ips":["10.0.0.1","192.168.1.0/24"]}`
	req = authedRequest(http.MethodPost, "/api/ip-lists", createBody, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create ip list: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse create ip list response: %v", err)
	}
	if created["id"] == nil || created["id"] == "" {
		t.Fatal("create ip list did not return an id")
	}
	if created["name"] != "test-blocklist" {
		t.Errorf("name = %v, want test-blocklist", created["name"])
	}
	if created["description"] != "blocks bad IPs" {
		t.Errorf("description = %v, want blocks bad IPs", created["description"])
	}
	if created["type"] != "blacklist" {
		t.Errorf("type = %v, want blacklist", created["type"])
	}
	ips, ok := created["ips"].([]any)
	if !ok || len(ips) != 2 {
		t.Fatalf("expected 2 ips, got %v", created["ips"])
	}
	if ips[0] != "10.0.0.1" || ips[1] != "192.168.1.0/24" {
		t.Errorf("ips = %v, want [10.0.0.1, 192.168.1.0/24]", ips)
	}

	// List should now have one entry
	req = authedRequest(http.MethodGet, "/api/ip-lists", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list ip lists: got %d, want 200", rec.Code)
	}

	var lists []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &lists); err != nil {
		t.Fatalf("failed to parse ip lists response: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("expected 1 ip list, got %d", len(lists))
	}
	if lists[0]["name"] != "test-blocklist" {
		t.Errorf("listed name = %v, want test-blocklist", lists[0]["name"])
	}
	resolvedCount, _ := lists[0]["resolved_count"].(float64)
	if resolvedCount != 2 {
		t.Errorf("resolved_count = %v, want 2", resolvedCount)
	}
}

func TestHandleIPListCreateValidation(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Missing name
	req := authedRequest(http.MethodPost, "/api/ip-lists", `{"name":"","type":"whitelist"}`, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty name: got %d, want 400", rec.Code)
	}

	// Invalid type
	req = authedRequest(http.MethodPost, "/api/ip-lists", `{"name":"test","type":"invalid"}`, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid type: got %d, want 400", rec.Code)
	}

	// Invalid IP
	req = authedRequest(http.MethodPost, "/api/ip-lists", `{"name":"test","type":"whitelist","ips":["not-an-ip"]}`, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid IP: got %d, want 400", rec.Code)
	}
}

func TestHandleIPListUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create
	createBody := `{"name":"orig-list","type":"whitelist","ips":["10.0.0.1"]}`
	req := authedRequest(http.MethodPost, "/api/ip-lists", createBody, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse create response: %v", err)
	}
	listID := created["id"].(string)

	// Update
	updateBody := `{"name":"renamed-list","ips":["10.0.0.2","10.0.0.3"]}`
	req = authedRequest(http.MethodPut, "/api/ip-lists/"+listID, updateBody, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("update: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var updated map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("failed to parse update response: %v", err)
	}
	if updated["name"] != "renamed-list" {
		t.Errorf("updated name = %v, want renamed-list", updated["name"])
	}
	updatedIPs, _ := updated["ips"].([]any)
	if len(updatedIPs) != 2 {
		t.Errorf("updated ips count = %d, want 2", len(updatedIPs))
	}
}

func TestHandleIPListUpdateNotFound(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPut, "/api/ip-lists/nonexistent", `{"name":"test"}`, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("update nonexistent: got %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleIPListDelete(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create
	createBody := `{"name":"delete-me","type":"blacklist","ips":["10.0.0.1"]}`
	req := authedRequest(http.MethodPost, "/api/ip-lists", createBody, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: got %d, want 200", rec.Code)
	}

	var created map[string]any
	json.Unmarshal(rec.Body.Bytes(), &created)
	listID := created["id"].(string)

	// Delete
	req = authedRequest(http.MethodDelete, "/api/ip-lists/"+listID, "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse delete response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("delete status = %q, want ok", resp["status"])
	}

	// Verify gone
	req = authedRequest(http.MethodGet, "/api/ip-lists", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	var lists []any
	json.Unmarshal(rec.Body.Bytes(), &lists)
	if len(lists) != 0 {
		t.Errorf("expected 0 ip lists after delete, got %d", len(lists))
	}
}

func TestHandleIPListDeleteNotFound(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodDelete, "/api/ip-lists/nonexistent", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("delete nonexistent: got %d, want 404", rec.Code)
	}
}

func TestHandleIPListUsage(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create a list
	createBody := `{"name":"usage-list","type":"whitelist","ips":["10.0.0.1"]}`
	req := authedRequest(http.MethodPost, "/api/ip-lists", createBody, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: got %d, want 200", rec.Code)
	}

	var created map[string]any
	json.Unmarshal(rec.Body.Bytes(), &created)
	listID := created["id"].(string)

	// Check usage (no routes bound)
	req = authedRequest(http.MethodGet, "/api/ip-lists/"+listID+"/usage", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("usage: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var usage map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &usage); err != nil {
		t.Fatalf("failed to parse usage response: %v", err)
	}
	routes, ok := usage["routes"].([]any)
	if !ok {
		t.Fatal("usage response missing routes array")
	}
	if len(routes) != 0 {
		t.Errorf("expected 0 routes using list, got %d", len(routes))
	}
	composites, ok := usage["composite_lists"].([]any)
	if !ok {
		t.Fatal("usage response missing composite_lists array")
	}
	if len(composites) != 0 {
		t.Errorf("expected 0 composite lists, got %d", len(composites))
	}
}

func TestHandleRouteIPListBindings(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/ip-lists/bindings", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("bindings: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var bindings map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &bindings); err != nil {
		t.Fatalf("failed to parse bindings response: %v", err)
	}
	if len(bindings) != 0 {
		t.Errorf("expected empty bindings map, got %d entries", len(bindings))
	}
}

// --- Certificate tests ---

func TestHandleCertificatesList(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Point data dir to an empty temp dir so no certs are found
	tmpCertDir := t.TempDir()
	th.store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
		c.CaddyDataDir = tmpCertDir
		return &c, nil
	})

	req := authedRequest(http.MethodGet, "/api/certificates", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("certificates list: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var certs []any
	if err := json.Unmarshal(rec.Body.Bytes(), &certs); err != nil {
		t.Fatalf("failed to parse certificates response: %v", err)
	}
}

func TestHandleCertificateRenewNotFound(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	tmpCertDir := t.TempDir()
	th.store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
		c.CaddyDataDir = tmpCertDir
		return &c, nil
	})

	body := `{"issuer_key":"acme-v02","domain":"nonexistent.com"}`
	req := authedRequest(http.MethodPost, "/api/certificates/renew", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("renew nonexistent cert: got %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCertificateRenewValidation(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Missing fields
	body := `{"issuer_key":"","domain":""}`
	req := authedRequest(http.MethodPost, "/api/certificates/renew", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("renew empty fields: got %d, want 400", rec.Code)
	}

	// Path traversal
	body = `{"issuer_key":"../etc","domain":"test.com"}`
	req = authedRequest(http.MethodPost, "/api/certificates/renew", body, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("renew path traversal: got %d, want 400", rec.Code)
	}
}

func TestHandleCertificateDeleteValidation(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Backslash in issuer
	req := authedRequest(http.MethodDelete, "/api/certificates/bad\\issuer/test.com", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("delete backslash issuer: got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCertificateDownloadNotFound(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	tmpCertDir := t.TempDir()
	th.store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
		c.CaddyDataDir = tmpCertDir
		return &c, nil
	})

	req := authedRequest(http.MethodGet, "/api/certificates/acme-v02/nonexistent.com/download", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("download nonexistent cert: got %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCertificateDownloadSuccess(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Create a fake cert file in the expected directory structure.
	// CertFilePath builds: dataDir/certificates/issuerKey/domain/domain.crt
	tmpCertDir := t.TempDir()
	certDir := filepath.Join(tmpCertDir, "certificates", "acme-v02", "example.com")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	certContent := "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n"
	if err := os.WriteFile(filepath.Join(certDir, "example.com.crt"), []byte(certContent), 0o644); err != nil {
		t.Fatal(err)
	}

	th.store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
		c.CaddyDataDir = tmpCertDir
		return &c, nil
	})

	req := authedRequest(http.MethodGet, "/api/certificates/acme-v02/example.com/download", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("download cert: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-pem-file" {
		t.Errorf("Content-Type = %q, want application/x-pem-file", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "example.com.crt") {
		t.Errorf("Content-Disposition missing filename: %q", rec.Header().Get("Content-Disposition"))
	}
	if rec.Body.String() != certContent {
		t.Errorf("body = %q, want cert content", rec.Body.String())
	}
}

// --- Loki tests ---

func TestHandleLokiStatus(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/loki/status", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("loki status: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse loki status response: %v", err)
	}
	if _, ok := resp["running"]; !ok {
		t.Error("loki status missing running field")
	}
	if _, ok := resp["sinks"]; !ok {
		t.Error("loki status missing sinks field")
	}
}

func TestHandleLokiConfigGetAndUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Get default config
	req := authedRequest(http.MethodGet, "/api/loki/config", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("loki config get: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var lokiCfg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &lokiCfg); err != nil {
		t.Fatalf("failed to parse loki config response: %v", err)
	}
	if lokiCfg["enabled"] != false {
		t.Errorf("loki enabled = %v, want false", lokiCfg["enabled"])
	}

	// Update config
	updateBody := `{"enabled":true,"endpoint":"http://loki:3100/loki/api/v1/push","bearer_token":"","tenant_id":"","labels":{},"batch_size":100,"flush_interval_seconds":5,"sinks":[]}`
	req = authedRequest(http.MethodPut, "/api/loki/config", updateBody, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("loki config update: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var updateResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("failed to parse loki config update response: %v", err)
	}
	if updateResp["status"] != "ok" {
		t.Errorf("loki config update status = %q, want ok", updateResp["status"])
	}

	// Verify persisted
	cfg := th.store.Get()
	if !cfg.Loki.Enabled {
		t.Error("loki should be enabled after update")
	}
	if cfg.Loki.Endpoint != "http://loki:3100/loki/api/v1/push" {
		t.Errorf("loki endpoint = %q, want http://loki:3100/loki/api/v1/push", cfg.Loki.Endpoint)
	}
}

func TestHandleLokiTestNoEndpoint(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Default config has a Loki endpoint - clear it so we test the empty path
	th.store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
		c.Loki.Endpoint = ""
		return &c, nil
	})

	req := authedRequest(http.MethodPost, "/api/loki/test", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("loki test: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse loki test response: %v", err)
	}
	if resp["success"] != false {
		t.Errorf("loki test success = %v, want false (no endpoint configured)", resp["success"])
	}
	msg, _ := resp["message"].(string)
	if !strings.Contains(msg, "No Loki endpoint") {
		t.Errorf("loki test message = %q, want message about no endpoint", msg)
	}
}

// --- Export/Import tests ---

func TestHandleExportFull(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/export/full", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("export full: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "kaji-export-") {
		t.Errorf("Content-Disposition missing expected filename: %q", rec.Header().Get("Content-Disposition"))
	}

	// Verify it's a valid ZIP
	_, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("response is not a valid ZIP: %v", err)
	}
}

// buildTestZIP creates a minimal valid export ZIP for import testing.
func buildTestZIP(t *testing.T, kajiVersion string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	writeZIPFile := func(name string, data []byte) {
		t.Helper()
		f, err := zw.Create("kaji-export/" + name)
		if err != nil {
			t.Fatalf("create %s in zip: %v", name, err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatalf("write %s in zip: %v", name, err)
		}
	}

	manifest, _ := json.Marshal(map[string]any{
		"version":      1,
		"exported_at":  time.Now().UTC().Format(time.RFC3339),
		"kaji_version": kajiVersion,
	})
	writeZIPFile("manifest.json", manifest)

	caddyConfig, _ := json.Marshal(map[string]any{"apps": map[string]any{}})
	writeZIPFile("caddy.json", caddyConfig)

	appConfig, _ := json.Marshal(config.DefaultConfig())
	writeZIPFile("config.json", appConfig)

	zw.Close()
	return &buf
}

func TestHandleImportFull(t *testing.T) {
	th := newTestHarnessWithVersion(t, "1.0.0")
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	zipBuf := buildTestZIP(t, "1.0.0")
	req := authedRequest(http.MethodPost, "/api/import/full", "", cookie)
	req.Body = io.NopCloser(zipBuf)
	req.ContentLength = int64(zipBuf.Len())
	req.Header.Set("Content-Type", "application/zip")

	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("import full: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse import full response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("import status = %v, want ok", resp["status"])
	}
	if _, ok := resp["route_count"]; !ok {
		t.Error("import response missing route_count")
	}
	if _, ok := resp["snapshot_count"]; !ok {
		t.Error("import response missing snapshot_count")
	}
}

func TestHandleImportFullInvalidZIP(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPost, "/api/import/full", "", cookie)
	req.Body = io.NopCloser(strings.NewReader("not a zip"))
	req.ContentLength = 9
	req.Header.Set("Content-Type", "application/zip")

	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("import invalid zip: got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSetupImportFull(t *testing.T) {
	th := newTestHarnessWithVersion(t, "1.0.0")

	// SetupImportFull doesn't require auth (it's part of the setup flow)
	zipBuf := buildTestZIP(t, "1.0.0")
	req := httptest.NewRequest(http.MethodPost, "/api/setup/import/full", zipBuf)
	req.ContentLength = int64(zipBuf.Len())
	req.Header.Set("Content-Type", "application/zip")

	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("setup import full: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse setup import full response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	if _, ok := resp["backup_data"]; !ok {
		t.Error("response missing backup_data")
	}
	summary, ok := resp["summary"].(map[string]any)
	if !ok {
		t.Fatal("response missing summary object")
	}
	if _, ok := summary["snapshot_count"]; !ok {
		t.Error("summary missing snapshot_count")
	}
}

// --- Log config update test ---

func TestHandleLogConfigUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Get the current log dir so we can build a valid file path
	cfg := th.store.Get()
	logDir := cfg.LogDir
	if logDir == "" {
		logDir = "/var/log/kaji"
	}

	body := `{"logs":{"default":{"writer":{"output":"discard"}}}}`
	req := authedRequest(http.MethodPut, "/api/logs/config", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("log config update: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse log config update response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("log config update status = %q, want ok", resp["status"])
	}
}

func TestHandleLogConfigUpdateInvalidJSON(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodPut, "/api/logs/config", "not json", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON: got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

// --- Caddy data dir tests ---

func TestHandleCaddyDataDirGet(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/settings/caddy-data-dir", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("caddy data dir get: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse caddy data dir response: %v", err)
	}
	if resp["caddy_data_dir"] == "" {
		t.Error("caddy_data_dir is empty, expected a resolved path")
	}
	if resp["is_override"] != "false" {
		t.Errorf("is_override = %q, want false (no override set)", resp["is_override"])
	}
}

func TestHandleCaddyDataDirUpdate(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	tmpDir := t.TempDir()
	body := `{"caddy_data_dir":"` + tmpDir + `"}`
	req := authedRequest(http.MethodPut, "/api/settings/caddy-data-dir", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("caddy data dir update: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse update response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("update status = %q, want ok", resp["status"])
	}

	// Verify persisted and now marked as override
	req = authedRequest(http.MethodGet, "/api/settings/caddy-data-dir", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse get response: %v", err)
	}
	if resp["caddy_data_dir"] != tmpDir {
		t.Errorf("caddy_data_dir = %q, want %q", resp["caddy_data_dir"], tmpDir)
	}
	if resp["is_override"] != "true" {
		t.Errorf("is_override = %q, want true", resp["is_override"])
	}
}

func TestHandleCaddyDataDirUpdateInvalidPath(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"caddy_data_dir":"/nonexistent/path/that/does/not/exist"}`
	req := authedRequest(http.MethodPut, "/api/settings/caddy-data-dir", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid path: got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCaddyDataDirUpdateClear(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	// Set an override first
	tmpDir := t.TempDir()
	body := `{"caddy_data_dir":"` + tmpDir + `"}`
	req := authedRequest(http.MethodPut, "/api/settings/caddy-data-dir", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set override: got %d, want 200", rec.Code)
	}

	// Clear the override by setting empty string
	body = `{"caddy_data_dir":""}`
	req = authedRequest(http.MethodPut, "/api/settings/caddy-data-dir", body, cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("clear override: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Verify cleared
	req = authedRequest(http.MethodGet, "/api/settings/caddy-data-dir", "", cookie)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["is_override"] != "false" {
		t.Errorf("is_override = %q after clear, want false", resp["is_override"])
	}
}
