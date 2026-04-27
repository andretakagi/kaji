package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andretakagi/kaji/internal/config"
)

// createDomain posts to /api/domains/full and returns the recorder.
func createDomain(t *testing.T, th *testHarness, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/domains/full", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	return rec
}

// mustCreateDomain creates a domain and fails the test if the status is not 200.
// Returns the parsed domain response.
func mustCreateDomain(t *testing.T, th *testHarness, body string) map[string]any {
	t.Helper()
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create domain: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse create domain response: %v", err)
	}
	return resp
}

// --- handleListDomains ---

func TestHandleListDomainsEmpty(t *testing.T) {
	th := newTestHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/domains", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list domains: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var domains []any
	if err := json.Unmarshal(rec.Body.Bytes(), &domains); err != nil {
		t.Fatalf("parse list domains response: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected empty list, got %d domains", len(domains))
	}
}

func TestHandleListDomainsAfterCreation(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	mustCreateDomain(t, th, body)

	req := httptest.NewRequest(http.MethodGet, "/api/domains", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list domains: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var domains []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &domains); err != nil {
		t.Fatalf("parse list domains response: %v", err)
	}
	if len(domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(domains))
	}
	if domains[0]["name"] != "example.com" {
		t.Errorf("domain name = %v, want example.com", domains[0]["name"])
	}
}

// --- handleGetDomain ---

func TestHandleGetDomainNotFound(t *testing.T) {
	th := newTestHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/domains/nonexistent", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("get nonexistent domain: got %d, want 404", rec.Code)
	}
}

func TestHandleGetDomainByID(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	created := mustCreateDomain(t, th, body)
	id := created["id"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/domains/"+id, nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get domain: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var dom map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &dom); err != nil {
		t.Fatalf("parse get domain response: %v", err)
	}
	if dom["id"] != id {
		t.Errorf("domain id = %v, want %s", dom["id"], id)
	}
	if dom["name"] != "example.com" {
		t.Errorf("domain name = %v, want example.com", dom["name"])
	}
}

// --- handleCreateDomainFull ---

func TestHandleCreateDomainFullBasic(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	rec := createDomain(t, th, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("create domain: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var dom map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &dom); err != nil {
		t.Fatalf("parse create domain response: %v", err)
	}
	if dom["id"] == nil || dom["id"] == "" {
		t.Error("create domain did not return an id")
	}
	if dom["name"] != "example.com" {
		t.Errorf("domain name = %v, want example.com", dom["name"])
	}
	if dom["enabled"] != true {
		t.Errorf("domain enabled = %v, want true", dom["enabled"])
	}

	cfg := th.store.Get()
	if len(cfg.Domains) != 1 {
		t.Fatalf("expected 1 domain in store, got %d", len(cfg.Domains))
	}
}

func TestHandleCreateDomainFullDuplicateName(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	mustCreateDomain(t, th, body)

	rec := createDomain(t, th, body)
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate domain: got %d, want 409", rec.Code)
	}
}

func TestHandleCreateDomainFullCaseInsensitiveDuplicate(t *testing.T) {
	th := newTestHarness(t)

	body1 := `{"name":"Example.Com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	mustCreateDomain(t, th, body1)

	body2 := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	rec := createDomain(t, th, body2)
	if rec.Code != http.StatusConflict {
		t.Errorf("case-insensitive duplicate domain: got %d, want 409", rec.Code)
	}
}

func TestHandleCreateDomainFullEmptyName(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"","toggles":{},"rule":{"handler_type":"none","handler_config":{}}}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty domain name: got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainFullInvalidName(t *testing.T) {
	th := newTestHarness(t)

	// Single label, no dot - should fail
	body := `{"name":"nodot","toggles":{},"rule":{"handler_type":"none","handler_config":{}}}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid domain name (no dot): got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainFullValidatesReverseProxy(t *testing.T) {
	th := newTestHarness(t)

	// Missing upstream
	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{}}}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("reverse_proxy missing upstream: got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainFullValidatesStaticResponse(t *testing.T) {
	th := newTestHarness(t)

	// Invalid status code
	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"static_response","handler_config":{"status_code":"999"}}}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("static_response invalid status code: got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainFullValidatesRedirect(t *testing.T) {
	th := newTestHarness(t)

	// Missing target URL
	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"redirect","handler_config":{}}}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("redirect missing target_url: got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainFullValidatesPathMatch(t *testing.T) {
	th := newTestHarness(t)

	// Path with invalid path_match value
	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"none","handler_config":{}},"paths":[{"path_match":"invalid","match_value":"/api","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}]}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid path_match: got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainFullHashesBasicAuthPassword(t *testing.T) {
	th := newTestHarness(t)

	body := `{
		"name":"example.com",
		"toggles":{"basic_auth":{"enabled":true,"username":"admin","password":"secret"}},
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}
	}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create domain with basic auth: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if len(cfg.Domains) != 1 {
		t.Fatalf("expected 1 domain in store")
	}
	hash := cfg.Domains[0].Toggles.BasicAuth.PasswordHash
	if hash == "" {
		t.Error("expected basic auth password to be hashed, got empty hash")
	}
	if hash == "secret" {
		t.Error("password was stored as plaintext instead of being hashed")
	}
}

func TestHandleCreateDomainFullHashesBasicAuthInPathToggleOverrides(t *testing.T) {
	th := newTestHarness(t)

	body := `{
		"name":"example.com",
		"toggles":{},
		"rule":{"handler_type":"none","handler_config":{}},
		"paths":[{
			"path_match":"prefix",
			"match_value":"/api",
			"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}},
			"toggle_overrides":{"basic_auth":{"enabled":true,"username":"user","password":"pass"}}
		}]
	}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create domain with path basic auth: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if len(cfg.Domains) != 1 || len(cfg.Domains[0].Paths) != 1 {
		t.Fatal("expected 1 domain with 1 path")
	}
	overrides := cfg.Domains[0].Paths[0].ToggleOverrides
	if overrides == nil {
		t.Fatal("expected toggle_overrides on path")
	}
	hash := overrides.BasicAuth.PasswordHash
	if hash == "" {
		t.Error("expected path basic auth password to be hashed, got empty hash")
	}
	if hash == "pass" {
		t.Error("path password was stored as plaintext instead of being hashed")
	}
}

func TestHandleCreateDomainFullBasicAuthWithoutUsername(t *testing.T) {
	th := newTestHarness(t)

	body := `{
		"name":"example.com",
		"toggles":{"basic_auth":{"enabled":true,"password":"secret"}},
		"rule":{"handler_type":"none","handler_config":{}}
	}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("basic auth without username: got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainFullPathBasicAuthWithoutUsername(t *testing.T) {
	th := newTestHarness(t)

	body := `{
		"name":"example.com",
		"toggles":{},
		"rule":{"handler_type":"none","handler_config":{}},
		"paths":[{
			"path_match":"prefix",
			"match_value":"/api",
			"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}},
			"toggle_overrides":{"basic_auth":{"enabled":true,"password":"pass"}}
		}]
	}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("path basic auth without username: got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainFullStaticResponse(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"static_response","handler_config":{"status_code":"200","body":"OK"}}}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create domain with static_response: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if len(cfg.Domains) != 1 {
		t.Fatal("expected 1 domain in store")
	}
	if cfg.Domains[0].Rule.HandlerType != "static_response" {
		t.Errorf("handler type = %s, want static_response", cfg.Domains[0].Rule.HandlerType)
	}
}

func TestHandleCreateDomainFullRedirect(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"redirect","handler_config":{"target_url":"https://other.example.com","status_code":"301"}}}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create domain with redirect: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if len(cfg.Domains) != 1 {
		t.Fatal("expected 1 domain in store")
	}
	if cfg.Domains[0].Rule.HandlerType != "redirect" {
		t.Errorf("handler type = %s, want redirect", cfg.Domains[0].Rule.HandlerType)
	}
}

func TestHandleCreateDomainFullMixedPaths(t *testing.T) {
	th := newTestHarness(t)

	body := `{
		"name":"example.com",
		"toggles":{},
		"rule":{"handler_type":"redirect","handler_config":{"target_url":"https://other.example.com"}},
		"paths":[
			{"path_match":"prefix","match_value":"/api","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}},
			{"path_match":"prefix","match_value":"/static","rule":{"handler_type":"static_response","handler_config":{"status_code":"200","body":"ok"}}}
		]
	}`
	rec := createDomain(t, th, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create domain with mixed paths: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if len(cfg.Domains) != 1 {
		t.Fatal("expected 1 domain")
	}
	if len(cfg.Domains[0].Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(cfg.Domains[0].Paths))
	}
	if cfg.Domains[0].Rule.HandlerType != "redirect" {
		t.Errorf("domain rule handler = %s, want redirect", cfg.Domains[0].Rule.HandlerType)
	}
}

// --- handleUpdateDomain ---

func TestHandleUpdateDomain(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	created := mustCreateDomain(t, th, body)
	id := created["id"].(string)

	updateBody := `{"name":"updated.com","toggles":{"force_https":true}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+id, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("update domain: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	var dom *config.Domain
	for i := range cfg.Domains {
		if cfg.Domains[i].ID == id {
			dom = &cfg.Domains[i]
			break
		}
	}
	if dom == nil {
		t.Fatal("domain not found in store after update")
	}
	if dom.Name != "updated.com" {
		t.Errorf("domain name = %s, want updated.com", dom.Name)
	}
	if !dom.Toggles.ForceHTTPS {
		t.Error("force_https should be true after update")
	}
}

func TestHandleUpdateDomainNotFound(t *testing.T) {
	th := newTestHarness(t)

	updateBody := `{"name":"updated.com","toggles":{}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/nonexistent", strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("update nonexistent domain: got %d, want 404", rec.Code)
	}
}

// --- handleDeleteDomain ---

func TestHandleDeleteDomain(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	created := mustCreateDomain(t, th, body)
	id := created["id"].(string)

	req := httptest.NewRequest(http.MethodDelete, "/api/domains/"+id, nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete domain: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if len(cfg.Domains) != 0 {
		t.Errorf("expected 0 domains after delete, got %d", len(cfg.Domains))
	}
}

func TestHandleDeleteDomainNotFound(t *testing.T) {
	th := newTestHarness(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/domains/nonexistent", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("delete nonexistent domain: got %d, want 404", rec.Code)
	}
}

// --- handleEnableDomain / handleDisableDomain ---

func TestHandleEnableDomain(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	created := mustCreateDomain(t, th, body)
	id := created["id"].(string)

	// First disable it so enable has something to do
	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+id+"/disable", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable domain before enable test: got %d", rec.Code)
	}

	// Now enable
	req = httptest.NewRequest(http.MethodPost, "/api/domains/"+id+"/enable", nil)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("enable domain: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var dom map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &dom); err != nil {
		t.Fatalf("parse enable domain response: %v", err)
	}
	if dom["enabled"] != true {
		t.Errorf("domain enabled = %v, want true", dom["enabled"])
	}

	cfg := th.store.Get()
	for _, d := range cfg.Domains {
		if d.ID == id {
			if !d.Enabled {
				t.Error("domain should be enabled in store")
			}
			return
		}
	}
	t.Error("domain not found in store after enable")
}

func TestHandleDisableDomain(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	created := mustCreateDomain(t, th, body)
	id := created["id"].(string)

	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+id+"/disable", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("disable domain: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var dom map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &dom); err != nil {
		t.Fatalf("parse disable domain response: %v", err)
	}
	if dom["enabled"] != false {
		t.Errorf("domain enabled = %v, want false", dom["enabled"])
	}

	cfg := th.store.Get()
	for _, d := range cfg.Domains {
		if d.ID == id {
			if d.Enabled {
				t.Error("domain should be disabled in store")
			}
			return
		}
	}
	t.Error("domain not found in store after disable")
}

func TestHandleEnableDomainNotFound(t *testing.T) {
	th := newTestHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/api/domains/nonexistent/enable", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("enable nonexistent domain: got %d, want 404", rec.Code)
	}
}

func TestHandleDisableDomainNotFound(t *testing.T) {
	th := newTestHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/api/domains/nonexistent/disable", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("disable nonexistent domain: got %d, want 404", rec.Code)
	}
}

// --- handleEnableDomainRule / handleDisableDomainRule ---

func TestHandleEnableDisableDomainRule(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`
	created := mustCreateDomain(t, th, body)
	id := created["id"].(string)

	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+id+"/rule/disable", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable domain rule: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	for _, d := range cfg.Domains {
		if d.ID == id {
			if d.Rule.Enabled {
				t.Error("domain rule should be disabled in store")
			}
			break
		}
	}

	req = httptest.NewRequest(http.MethodPost, "/api/domains/"+id+"/rule/enable", nil)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable domain rule: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg = th.store.Get()
	for _, d := range cfg.Domains {
		if d.ID == id {
			if !d.Rule.Enabled {
				t.Error("domain rule should be enabled in store")
			}
			return
		}
	}
	t.Error("domain not found in store after rule toggle")
}

func TestHandleEnableDomainRuleNotFound(t *testing.T) {
	th := newTestHarness(t)

	for _, action := range []string{"enable", "disable"} {
		t.Run(action, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/domains/nonexistent/rule/"+action, nil)
			rec := httptest.NewRecorder()
			th.handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("got %d, want 404", rec.Code)
			}
		})
	}
}
