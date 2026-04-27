package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andretakagi/kaji/internal/config"
)

const baseDomainBody = `{"name":"example.com","toggles":{},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`

// mustCreateSubdomain posts to /api/domains/{id}/subdomains and fails the test
// if the response status is not 200. Returns the parsed JSON.
func mustCreateSubdomain(t *testing.T, th *testHarness, domainID, body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domainID+"/subdomains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create subdomain: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse create subdomain response: %v", err)
	}
	return resp
}

// findStoreSubdomain returns the subdomain with subID inside the domain with
// domainID, reading directly from the store. Fails the test if not found.
func findStoreSubdomain(t *testing.T, th *testHarness, domainID, subID string) *config.Subdomain {
	t.Helper()
	cfg := th.store.Get()
	for i := range cfg.Domains {
		if cfg.Domains[i].ID != domainID {
			continue
		}
		for j := range cfg.Domains[i].Subdomains {
			if cfg.Domains[i].Subdomains[j].ID == subID {
				return &cfg.Domains[i].Subdomains[j]
			}
		}
	}
	t.Fatalf("subdomain %s not found under domain %s", subID, domainID)
	return nil
}

// --- handleCreateSubdomain ---

func TestHandleCreateSubdomainBasic(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{"name":"api","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}}`
	resp := mustCreateSubdomain(t, th, domID, body)

	subs := resp["subdomains"].([]any)
	if len(subs) != 1 {
		t.Fatalf("expected 1 subdomain on parent, got %d", len(subs))
	}
	sub := subs[0].(map[string]any)
	if sub["name"] != "api" {
		t.Errorf("subdomain name = %v, want api", sub["name"])
	}
	if sub["enabled"] != true {
		t.Errorf("subdomain enabled = %v, want true", sub["enabled"])
	}
	if sub["id"] == "" {
		t.Error("subdomain id should be generated")
	}
}

func TestHandleCreateSubdomainInheritsParentToggles(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, `{"name":"example.com","toggles":{"force_https":true},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:8080"}}}`)
	domID := dom["id"].(string)

	body := `{"name":"api","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}}`
	mustCreateSubdomain(t, th, domID, body)

	cfg := th.store.Get()
	sub := cfg.Domains[0].Subdomains[0]
	if !sub.Toggles.ForceHTTPS {
		t.Error("subdomain should inherit parent's force_https toggle")
	}
}

func TestHandleCreateSubdomainCustomToggles(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{"name":"api","toggles":{"compression":true},"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}}`
	mustCreateSubdomain(t, th, domID, body)

	cfg := th.store.Get()
	sub := cfg.Domains[0].Subdomains[0]
	if !sub.Toggles.Compression {
		t.Error("subdomain should have compression toggle")
	}
}

func TestHandleCreateSubdomainAcceptsRuleNone(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{"name":"api","rule":{"handler_type":"none","handler_config":{}}}`
	resp := mustCreateSubdomain(t, th, domID, body)

	subs := resp["subdomains"].([]any)
	sub := subs[0].(map[string]any)
	rule := sub["rule"].(map[string]any)
	if rule["handler_type"] != "none" {
		t.Errorf("subdomain rule handler_type = %v, want none", rule["handler_type"])
	}
}

func TestHandleCreateSubdomainWithPaths(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{
		"name":"api",
		"rule":{"handler_type":"none","handler_config":{}},
		"paths":[{
			"path_match":"prefix",
			"match_value":"/v1",
			"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}
		}]
	}`
	mustCreateSubdomain(t, th, domID, body)

	cfg := th.store.Get()
	sub := cfg.Domains[0].Subdomains[0]
	if len(sub.Paths) != 1 {
		t.Fatalf("expected 1 path on subdomain, got %d", len(sub.Paths))
	}
	if sub.Paths[0].MatchValue != "/v1" {
		t.Errorf("path match_value = %s, want /v1", sub.Paths[0].MatchValue)
	}
	if sub.Paths[0].ID == "" {
		t.Error("path ID should be generated")
	}
}

func TestHandleCreateSubdomainPathRejectsNoneRule(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{
		"name":"api",
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}},
		"paths":[{
			"path_match":"prefix",
			"match_value":"/v1",
			"rule":{"handler_type":"none","handler_config":{}}
		}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "handler_type cannot be none for a path") {
		t.Errorf("expected error about path rule rejection; got %s", rec.Body.String())
	}
}

func TestHandleCreateSubdomainDuplicateName(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{"name":"api","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}}`
	mustCreateSubdomain(t, th, domID, body)

	dupBody := `{"name":"API","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9001"}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains", strings.NewReader(dupBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("got %d, want 409", rec.Code)
	}
}

func TestHandleCreateSubdomainInvalidName(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	cases := []struct {
		name string
		body string
	}{
		{"empty", `{"name":"","rule":{"handler_type":"none","handler_config":{}}}`},
		{"illegal_chars", `{"name":"a.b","rule":{"handler_type":"none","handler_config":{}}}`},
		{"leading_hyphen", `{"name":"-api","rule":{"handler_type":"none","handler_config":{}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			th.handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("got %d, want 400; body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleCreateSubdomainDomainNotFound(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"api","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/domains/nope/subdomains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestHandleCreateSubdomainBasicAuthHashed(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{
		"name":"api",
		"toggles":{"basic_auth":{"enabled":true,"username":"admin","password":"secret"}},
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}
	}`
	mustCreateSubdomain(t, th, domID, body)

	cfg := th.store.Get()
	sub := cfg.Domains[0].Subdomains[0]
	if sub.Toggles.BasicAuth.PasswordHash == "" {
		t.Error("expected hashed password on subdomain")
	}
	if sub.Toggles.BasicAuth.Password != "" {
		t.Error("plaintext password should not be stored")
	}
}

func TestHandleCreateSubdomainBasicAuthMissingUsername(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{
		"name":"api",
		"toggles":{"basic_auth":{"enabled":true,"password":"secret"}},
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

// --- handleUpdateSubdomain ---

func TestHandleUpdateSubdomain(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	sub := mustCreateSubdomain(t, th,
		domID,
		`{"name":"api","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}}`,
	)
	subs := sub["subdomains"].([]any)
	subID := subs[0].(map[string]any)["id"].(string)

	updateBody := `{"name":"v2","toggles":{"compression":true}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/"+subID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	stored := findStoreSubdomain(t, th, domID, subID)
	if stored.Name != "v2" {
		t.Errorf("subdomain name = %s, want v2", stored.Name)
	}
	if !stored.Toggles.Compression {
		t.Error("compression should be true after update")
	}
}

func TestHandleUpdateSubdomainDuplicateName(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	mustCreateSubdomain(t, th, domID, `{"name":"api","rule":{"handler_type":"none","handler_config":{}}}`)
	resp := mustCreateSubdomain(t, th, domID, `{"name":"v2","rule":{"handler_type":"none","handler_config":{}}}`)

	var v2ID string
	for _, s := range resp["subdomains"].([]any) {
		m := s.(map[string]any)
		if m["name"] == "v2" {
			v2ID = m["id"].(string)
		}
	}

	dupBody := `{"name":"API","toggles":{}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/"+v2ID, strings.NewReader(dupBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("got %d, want 409", rec.Code)
	}
}

func TestHandleUpdateSubdomainPreservesBasicAuthHash(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	sub := mustCreateSubdomain(t, th, domID, `{
		"name":"api",
		"toggles":{"basic_auth":{"enabled":true,"username":"admin","password":"secret"}},
		"rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}
	}`)
	subID := sub["subdomains"].([]any)[0].(map[string]any)["id"].(string)

	originalHash := findStoreSubdomain(t, th, domID, subID).Toggles.BasicAuth.PasswordHash
	if originalHash == "" {
		t.Fatal("expected password hash to be set after create")
	}

	// Update without resending the password
	updateBody := `{"name":"api","toggles":{"basic_auth":{"enabled":true,"username":"admin"}}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/"+subID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	updatedHash := findStoreSubdomain(t, th, domID, subID).Toggles.BasicAuth.PasswordHash
	if updatedHash != originalHash {
		t.Errorf("hash changed: %q -> %q", originalHash, updatedHash)
	}
}

func TestHandleUpdateSubdomainEnabledFlag(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	sub := mustCreateSubdomain(t, th,
		domID,
		`{"name":"api","rule":{"handler_type":"none","handler_config":{}}}`,
	)
	subID := sub["subdomains"].([]any)[0].(map[string]any)["id"].(string)

	updateBody := `{"name":"api","enabled":false,"toggles":{}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/"+subID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if findStoreSubdomain(t, th, domID, subID).Enabled {
		t.Error("subdomain should be disabled after update")
	}
}

func TestHandleUpdateSubdomainNotFound(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{"name":"api","toggles":{}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/missing", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestHandleUpdateSubdomainDomainNotFound(t *testing.T) {
	th := newTestHarness(t)

	body := `{"name":"api","toggles":{}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/nope/subdomains/none", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

// --- handleDeleteSubdomain ---

func TestHandleDeleteSubdomain(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	sub := mustCreateSubdomain(t, th,
		domID,
		`{"name":"api","rule":{"handler_type":"none","handler_config":{}}}`,
	)
	subID := sub["subdomains"].([]any)[0].(map[string]any)["id"].(string)

	req := httptest.NewRequest(http.MethodDelete, "/api/domains/"+domID+"/subdomains/"+subID, nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cfg := th.store.Get()
	if len(cfg.Domains[0].Subdomains) != 0 {
		t.Errorf("expected 0 subdomains, got %d", len(cfg.Domains[0].Subdomains))
	}
}

func TestHandleDeleteSubdomainNotFound(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	req := httptest.NewRequest(http.MethodDelete, "/api/domains/"+domID+"/subdomains/missing", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

// --- handleEnableSubdomain / handleDisableSubdomain ---

func TestHandleEnableDisableSubdomain(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	sub := mustCreateSubdomain(t, th,
		domID,
		`{"name":"api","rule":{"handler_type":"none","handler_config":{}}}`,
	)
	subID := sub["subdomains"].([]any)[0].(map[string]any)["id"].(string)

	req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains/"+subID+"/disable", nil)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if findStoreSubdomain(t, th, domID, subID).Enabled {
		t.Error("subdomain should be disabled")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains/"+subID+"/enable", nil)
	rec = httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !findStoreSubdomain(t, th, domID, subID).Enabled {
		t.Error("subdomain should be enabled")
	}
}

func TestHandleToggleSubdomainNotFound(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	for _, action := range []string{"enable", "disable"} {
		t.Run(action, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/domains/"+domID+"/subdomains/missing/"+action, nil)
			rec := httptest.NewRecorder()
			th.handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("got %d, want 404", rec.Code)
			}
		})
	}
}

// --- handleUpdateSubdomainRule ---

func TestHandleUpdateSubdomainRule(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	sub := mustCreateSubdomain(t, th,
		domID,
		`{"name":"api","rule":{"handler_type":"none","handler_config":{}}}`,
	)
	subID := sub["subdomains"].([]any)[0].(map[string]any)["id"].(string)

	body := `{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9100"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/"+subID+"/rule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	stored := findStoreSubdomain(t, th, domID, subID)
	if stored.Rule.HandlerType != "reverse_proxy" {
		t.Errorf("handler type = %s, want reverse_proxy", stored.Rule.HandlerType)
	}
}

func TestHandleUpdateSubdomainRuleAcceptsNone(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)
	sub := mustCreateSubdomain(t, th,
		domID,
		`{"name":"api","rule":{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9000"}}}`,
	)
	subID := sub["subdomains"].([]any)[0].(map[string]any)["id"].(string)

	body := `{"handler_type":"none","handler_config":{}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/"+subID+"/rule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	stored := findStoreSubdomain(t, th, domID, subID)
	if stored.Rule.HandlerType != "none" {
		t.Errorf("handler type = %s, want none", stored.Rule.HandlerType)
	}
}

func TestHandleUpdateSubdomainRuleNotFound(t *testing.T) {
	th := newTestHarness(t)
	dom := mustCreateDomain(t, th, baseDomainBody)
	domID := dom["id"].(string)

	body := `{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9100"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/"+domID+"/subdomains/missing/rule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestHandleUpdateSubdomainRuleDomainNotFound(t *testing.T) {
	th := newTestHarness(t)

	body := `{"handler_type":"reverse_proxy","handler_config":{"upstream":"127.0.0.1:9100"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/domains/nope/subdomains/missing/rule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}
