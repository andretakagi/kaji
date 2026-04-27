package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andretakagi/kaji/internal/config"
)

const reverseProxyPathBody = `{
	"path_match":"prefix",
	"match_value":"/api",
	"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}
}`

func postPath(t *testing.T, th *testHarness, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	return rec
}

func mustCreateDomainPath(t *testing.T, th *testHarness, domainID, body string) string {
	t.Helper()
	rec := postPath(t, th, "/api/domains/"+domainID+"/paths", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create domain path: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	paths := resp["paths"].([]any)
	if len(paths) == 0 {
		t.Fatalf("expected paths in response, got none: %s", rec.Body.String())
	}
	return paths[len(paths)-1].(map[string]any)["id"].(string)
}

func mustCreateSubdomainPath(t *testing.T, th *testHarness, domainID, subID, body string) string {
	t.Helper()
	rec := postPath(t, th, "/api/domains/"+domainID+"/subdomains/"+subID+"/paths", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create subdomain path: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, s := range resp["subdomains"].([]any) {
		sub := s.(map[string]any)
		if sub["id"] == subID {
			paths := sub["paths"].([]any)
			if len(paths) == 0 {
				t.Fatalf("expected at least one path on subdomain")
			}
			return paths[len(paths)-1].(map[string]any)["id"].(string)
		}
	}
	t.Fatalf("subdomain %s not found in response", subID)
	return ""
}

func storeDomainPath(t *testing.T, th *testHarness, domainID, pathID string) *config.Path {
	t.Helper()
	cfg := th.store.Get()
	for i := range cfg.Domains {
		if cfg.Domains[i].ID != domainID {
			continue
		}
		for j := range cfg.Domains[i].Paths {
			if cfg.Domains[i].Paths[j].ID == pathID {
				return &cfg.Domains[i].Paths[j]
			}
		}
	}
	t.Fatalf("path %s not found in domain %s", pathID, domainID)
	return nil
}

func storeSubdomainPath(t *testing.T, th *testHarness, domainID, subID, pathID string) *config.Path {
	t.Helper()
	cfg := th.store.Get()
	for i := range cfg.Domains {
		if cfg.Domains[i].ID != domainID {
			continue
		}
		for j := range cfg.Domains[i].Subdomains {
			if cfg.Domains[i].Subdomains[j].ID != subID {
				continue
			}
			for k := range cfg.Domains[i].Subdomains[j].Paths {
				if cfg.Domains[i].Subdomains[j].Paths[k].ID == pathID {
					return &cfg.Domains[i].Subdomains[j].Paths[k]
				}
			}
		}
	}
	t.Fatalf("path %s not found in subdomain %s", pathID, subID)
	return nil
}

// --- handleCreateDomainPath ---

func TestHandleCreateDomainPath(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	pathID := mustCreateDomainPath(t, th, domID, reverseProxyPathBody)
	stored := storeDomainPath(t, th, domID, pathID)

	if !stored.Enabled {
		t.Error("new path should be enabled")
	}
	if stored.MatchValue != "/api" {
		t.Errorf("match_value = %s, want /api", stored.MatchValue)
	}
	if stored.Rule.HandlerType != "reverse_proxy" {
		t.Errorf("handler type = %s, want reverse_proxy", stored.Rule.HandlerType)
	}
}

func TestHandleCreateDomainPathDomainNotFound(t *testing.T) {
	th := newTestHarness(t)
	rec := postPath(t, th, "/api/domains/nope/paths", reverseProxyPathBody)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestHandleCreateDomainPathRejectsRuleNone(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{"path_match":"prefix","match_value":"/api","rule":{"handler_type":"none","handler_config":{}}}`
	rec := postPath(t, th, "/api/domains/"+domID+"/paths", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateDomainPathRejectsBadPathMatch(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{"path_match":"bogus","match_value":"/api","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}}`
	rec := postPath(t, th, "/api/domains/"+domID+"/paths", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainPathRejectsEmptyMatchValue(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{"path_match":"prefix","match_value":"   ","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}}`
	rec := postPath(t, th, "/api/domains/"+domID+"/paths", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestHandleCreateDomainPathHashesBasicAuthOverride(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{
		"path_match":"prefix",
		"match_value":"/api",
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}},
		"toggle_overrides":{"basic_auth":{"enabled":true,"username":"user","password":"pass"}}
	}`
	pathID := mustCreateDomainPath(t, th, domID, body)
	stored := storeDomainPath(t, th, domID, pathID)
	if stored.ToggleOverrides == nil {
		t.Fatal("expected toggle_overrides to be set")
	}
	if stored.ToggleOverrides.BasicAuth.PasswordHash == "" {
		t.Error("expected hashed password on path override")
	}
	if stored.ToggleOverrides.BasicAuth.Password != "" {
		t.Error("plaintext password should not be stored")
	}
}

func TestHandleCreateDomainPathBasicAuthOverrideMissingUsername(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{
		"path_match":"prefix",
		"match_value":"/api",
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}},
		"toggle_overrides":{"basic_auth":{"enabled":true,"password":"pass"}}
	}`
	rec := postPath(t, th, "/api/domains/"+domID+"/paths", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

// --- handleUpdateDomainPath ---

func TestHandleUpdateDomainPath(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	pathID := mustCreateDomainPath(t, th, domID, reverseProxyPathBody)

	updateBody := `{
		"label":"v2",
		"path_match":"prefix",
		"match_value":"/v2",
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9100"}}
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/paths/"+pathID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	stored := storeDomainPath(t, th, domID, pathID)
	if stored.Label != "v2" {
		t.Errorf("label = %s, want v2", stored.Label)
	}
	if stored.MatchValue != "/v2" {
		t.Errorf("match_value = %s, want /v2", stored.MatchValue)
	}
}

func TestHandleUpdateDomainPathPreservesBasicAuthHash(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	createBody := `{
		"path_match":"prefix",
		"match_value":"/api",
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}},
		"toggle_overrides":{"basic_auth":{"enabled":true,"username":"user","password":"original"}}
	}`
	pathID := mustCreateDomainPath(t, th, domID, createBody)
	originalHash := storeDomainPath(t, th, domID, pathID).ToggleOverrides.BasicAuth.PasswordHash
	if originalHash == "" {
		t.Fatal("expected hash after create")
	}

	updateBody := `{
		"path_match":"prefix",
		"match_value":"/api",
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}},
		"toggle_overrides":{"basic_auth":{"enabled":true,"username":"user"}}
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/paths/"+pathID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	updatedHash := storeDomainPath(t, th, domID, pathID).ToggleOverrides.BasicAuth.PasswordHash
	if updatedHash != originalHash {
		t.Errorf("hash changed: %q -> %q (should be preserved)", originalHash, updatedHash)
	}
}

func TestHandleUpdateDomainPathNotFound(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/paths/missing", strings.NewReader(reverseProxyPathBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

// --- handleDeleteDomainPath ---

func TestHandleDeleteDomainPath(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	pathID := mustCreateDomainPath(t, th, domID, reverseProxyPathBody)

	req := httptest.NewRequest(http.MethodDelete, "/api/domains/"+domID+"/paths/"+pathID, nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if len(cfg.Domains[0].Paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(cfg.Domains[0].Paths))
	}
}

func TestHandleDeleteDomainPathNotFound(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	req := httptest.NewRequest(http.MethodDelete, "/api/domains/"+domID+"/paths/missing", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

// --- handleEnableDomainPath / handleDisableDomainPath ---

func TestHandleEnableDisableDomainPath(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	pathID := mustCreateDomainPath(t, th, domID, reverseProxyPathBody)

	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/paths/"+pathID+"/disable", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if storeDomainPath(t, th, domID, pathID).Enabled {
		t.Error("path should be disabled")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/paths/"+pathID+"/enable", nil)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !storeDomainPath(t, th, domID, pathID).Enabled {
		t.Error("path should be enabled")
	}
}

func TestHandleToggleDomainPathNotFound(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	for _, action := range []string{"enable", "disable"} {
		t.Run(action, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/paths/missing/"+action, nil)
			rec := httptest.NewRecorder()
			th.handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("got %d, want 404", rec.Code)
			}
		})
	}
}

// --- Subdomain paths ---

// setupSubdomain creates a domain + subdomain and returns both IDs.
func setupSubdomain(t *testing.T, th *testHarness) (domID, subID string) {
	t.Helper()
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID = dom["id"].(string)
	sub := mustCreateSubdomain(t, th, domID, `{"name":"api","rule":{"handler_type":"none","handler_config":{}}}`)
	subID = sub["subdomains"].([]any)[0].(map[string]any)["id"].(string)
	return domID, subID
}

func TestHandleCreateSubdomainPath(t *testing.T) {
	th := newTestHarness(t)
	domID, subID := setupSubdomain(t, th)

	pathID := mustCreateSubdomainPath(t, th, domID, subID, reverseProxyPathBody)
	stored := storeSubdomainPath(t, th, domID, subID, pathID)
	if stored.MatchValue != "/api" {
		t.Errorf("match_value = %s, want /api", stored.MatchValue)
	}
}

func TestHandleCreateSubdomainPathDomainNotFound(t *testing.T) {
	th := newTestHarness(t)
	rec := postPath(t, th, "/api/domains/nope/subdomains/missing/paths", reverseProxyPathBody)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestHandleCreateSubdomainPathSubdomainNotFound(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	rec := postPath(t, th, "/api/domains/"+domID+"/subdomains/missing/paths", reverseProxyPathBody)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestHandleCreateSubdomainPathRejectsRuleNone(t *testing.T) {
	th := newTestHarness(t)
	domID, subID := setupSubdomain(t, th)

	body := `{"path_match":"prefix","match_value":"/api","rule":{"handler_type":"none","handler_config":{}}}`
	rec := postPath(t, th, "/api/domains/"+domID+"/subdomains/"+subID+"/paths", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateSubdomainPath(t *testing.T) {
	th := newTestHarness(t)
	domID, subID := setupSubdomain(t, th)
	pathID := mustCreateSubdomainPath(t, th, domID, subID, reverseProxyPathBody)

	updateBody := `{
		"path_match":"prefix",
		"match_value":"/v2",
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9100"}}
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/"+subID+"/paths/"+pathID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	stored := storeSubdomainPath(t, th, domID, subID, pathID)
	if stored.MatchValue != "/v2" {
		t.Errorf("match_value = %s, want /v2", stored.MatchValue)
	}
}

func TestHandleUpdateSubdomainPathPreservesBasicAuthHash(t *testing.T) {
	th := newTestHarness(t)
	domID, subID := setupSubdomain(t, th)
	createBody := `{
		"path_match":"prefix",
		"match_value":"/api",
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}},
		"toggle_overrides":{"basic_auth":{"enabled":true,"username":"user","password":"original"}}
	}`
	pathID := mustCreateSubdomainPath(t, th, domID, subID, createBody)
	originalHash := storeSubdomainPath(t, th, domID, subID, pathID).ToggleOverrides.BasicAuth.PasswordHash
	if originalHash == "" {
		t.Fatal("expected hash after create")
	}

	updateBody := `{
		"path_match":"prefix",
		"match_value":"/api",
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}},
		"toggle_overrides":{"basic_auth":{"enabled":true,"username":"user"}}
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/"+subID+"/paths/"+pathID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	updatedHash := storeSubdomainPath(t, th, domID, subID, pathID).ToggleOverrides.BasicAuth.PasswordHash
	if updatedHash != originalHash {
		t.Errorf("hash changed: %q -> %q", originalHash, updatedHash)
	}
}

func TestHandleUpdateSubdomainPathNotFound(t *testing.T) {
	th := newTestHarness(t)
	domID, subID := setupSubdomain(t, th)

	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/"+subID+"/paths/missing", strings.NewReader(reverseProxyPathBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestHandleDeleteSubdomainPath(t *testing.T) {
	th := newTestHarness(t)
	domID, subID := setupSubdomain(t, th)
	pathID := mustCreateSubdomainPath(t, th, domID, subID, reverseProxyPathBody)

	req := httptest.NewRequest(http.MethodDelete, "/api/domains/"+domID+"/subdomains/"+subID+"/paths/"+pathID, nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if len(cfg.Domains[0].Subdomains[0].Paths) != 0 {
		t.Errorf("expected 0 paths after delete, got %d", len(cfg.Domains[0].Subdomains[0].Paths))
	}
}

func TestHandleDeleteSubdomainPathNotFound(t *testing.T) {
	th := newTestHarness(t)
	domID, subID := setupSubdomain(t, th)

	req := httptest.NewRequest(http.MethodDelete, "/api/domains/"+domID+"/subdomains/"+subID+"/paths/missing", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestHandleEnableDisableSubdomainPath(t *testing.T) {
	th := newTestHarness(t)
	domID, subID := setupSubdomain(t, th)
	pathID := mustCreateSubdomainPath(t, th, domID, subID, reverseProxyPathBody)

	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains/"+subID+"/paths/"+pathID+"/disable", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if storeSubdomainPath(t, th, domID, subID, pathID).Enabled {
		t.Error("path should be disabled")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains/"+subID+"/paths/"+pathID+"/enable", nil)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !storeSubdomainPath(t, th, domID, subID, pathID).Enabled {
		t.Error("path should be enabled")
	}
}

func TestHandleToggleSubdomainPathNotFound(t *testing.T) {
	th := newTestHarness(t)
	domID, subID := setupSubdomain(t, th)

	for _, action := range []string{"enable", "disable"} {
		t.Run(action, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains/"+subID+"/paths/missing/"+action, nil)
			rec := httptest.NewRecorder()
			th.handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("got %d, want 404", rec.Code)
			}
		})
	}
}
