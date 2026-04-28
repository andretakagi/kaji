package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andretakagi/kaji/internal/config"
)

func TestValidateLogSkipRules(t *testing.T) {
	tests := []struct {
		name    string
		rules   config.LogSkipConfig
		wantErr bool
	}{
		{
			name:    "valid basic empty",
			rules:   config.LogSkipConfig{Mode: "basic", Conditions: []config.SkipCondition{}},
			wantErr: false,
		},
		{
			name: "valid basic with path condition",
			rules: config.LogSkipConfig{
				Mode: "basic",
				Conditions: []config.SkipCondition{
					{Type: "path", Value: "/healthz"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid basic with path_regexp",
			rules: config.LogSkipConfig{
				Mode: "basic",
				Conditions: []config.SkipCondition{
					{Type: "path_regexp", Value: "^/static/"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid basic with remote_ip",
			rules: config.LogSkipConfig{
				Mode: "basic",
				Conditions: []config.SkipCondition{
					{Type: "remote_ip", Value: "10.0.0.0/8"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid basic with header",
			rules: config.LogSkipConfig{
				Mode: "basic",
				Conditions: []config.SkipCondition{
					{Type: "header", Key: "User-Agent", Value: "kube-probe"},
				},
			},
			wantErr: false,
		},
		{
			name:    "valid advanced with json array",
			rules:   config.LogSkipConfig{Mode: "advanced", AdvancedRaw: json.RawMessage(`[{"match_path":"/healthz"}]`)},
			wantErr: false,
		},
		{
			name:    "invalid mode",
			rules:   config.LogSkipConfig{Mode: "fancy"},
			wantErr: true,
		},
		{
			name:    "advanced missing advanced_raw",
			rules:   config.LogSkipConfig{Mode: "advanced"},
			wantErr: true,
		},
		{
			name:    "advanced_raw not a json array",
			rules:   config.LogSkipConfig{Mode: "advanced", AdvancedRaw: json.RawMessage(`{"not":"array"}`)},
			wantErr: true,
		},
		{
			name: "condition unknown type",
			rules: config.LogSkipConfig{
				Mode: "basic",
				Conditions: []config.SkipCondition{
					{Type: "cookie", Value: "session"},
				},
			},
			wantErr: true,
		},
		{
			name: "condition empty value",
			rules: config.LogSkipConfig{
				Mode: "basic",
				Conditions: []config.SkipCondition{
					{Type: "path", Value: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "header condition missing key",
			rules: config.LogSkipConfig{
				Mode: "basic",
				Conditions: []config.SkipCondition{
					{Type: "header", Value: "kube-probe"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := validateLogSkipRules(tt.rules)
			if tt.wantErr && msg == "" {
				t.Error("expected validation error, got none")
			}
			if !tt.wantErr && msg != "" {
				t.Errorf("unexpected validation error: %s", msg)
			}
		})
	}
}

func TestHandleLogSkipRulesGet_DefaultsForUnknownSink(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	req := authedRequest(http.MethodGet, "/api/log-skip-rules/my_sink", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp config.LogSkipConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Mode != "basic" {
		t.Errorf("mode = %q, want basic", resp.Mode)
	}
	if resp.Conditions == nil {
		t.Error("conditions should be an empty slice, not nil")
	}
	if len(resp.Conditions) != 0 {
		t.Errorf("conditions length = %d, want 0", len(resp.Conditions))
	}
}

func TestHandleLogSkipRulesGet_ReturnsStoredRules(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	stored := config.LogSkipConfig{
		Mode: "basic",
		Conditions: []config.SkipCondition{
			{Type: "path", Value: "/healthz"},
		},
	}
	th.store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
		if c.LogSkipRules == nil {
			c.LogSkipRules = make(map[string]config.LogSkipConfig)
		}
		c.LogSkipRules["access"] = stored
		return &c, nil
	})

	req := authedRequest(http.MethodGet, "/api/log-skip-rules/access", "", cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp config.LogSkipConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Mode != "basic" {
		t.Errorf("mode = %q, want basic", resp.Mode)
	}
	if len(resp.Conditions) != 1 || resp.Conditions[0].Value != "/healthz" {
		t.Errorf("conditions = %v, want [{path /healthz}]", resp.Conditions)
	}
}

func TestHandleLogSkipRulesPut_SavesAndReturns(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)
	seedLoggingConfig(t, th, "access")

	body := `{"mode":"basic","conditions":[{"type":"path","value":"/healthz"}],"advanced_raw":null}`
	req := authedRequest(http.MethodPut, "/api/log-skip-rules/access", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp config.LogSkipConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Mode != "basic" {
		t.Errorf("mode = %q, want basic", resp.Mode)
	}
	if len(resp.Conditions) != 1 || resp.Conditions[0].Value != "/healthz" {
		t.Errorf("conditions = %v, want [{path /healthz}]", resp.Conditions)
	}

	cfg := th.store.Get()
	saved, ok := cfg.LogSkipRules["access"]
	if !ok {
		t.Fatal("rules not persisted to store")
	}
	if saved.Mode != "basic" {
		t.Errorf("stored mode = %q, want basic", saved.Mode)
	}
}

func TestHandleLogSkipRulesPut_RejectsDefaultSink(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"mode":"basic","conditions":[]}`
	req := authedRequest(http.MethodPut, "/api/log-skip-rules/default", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
}

func TestHandleLogSkipRulesPut_RejectsNonexistentSink(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)

	body := `{"mode":"basic","conditions":[]}`
	req := authedRequest(http.MethodPut, "/api/log-skip-rules/nonexistent_sink", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogSkipRulesPut_RejectsInvalidConditionType(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)
	seedLoggingConfig(t, th, "access")

	body := `{"mode":"basic","conditions":[{"type":"cookie","value":"session"}]}`
	req := authedRequest(http.MethodPut, "/api/log-skip-rules/access", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
}

func TestHandleLogSkipRulesPut_RejectsEmptyValue(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)
	seedLoggingConfig(t, th, "access")

	body := `{"mode":"basic","conditions":[{"type":"path","value":""}]}`
	req := authedRequest(http.MethodPut, "/api/log-skip-rules/access", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
}

func TestHandleLogSkipRulesPut_RejectsHeaderWithoutKey(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)
	seedLoggingConfig(t, th, "access")

	body := `{"mode":"basic","conditions":[{"type":"header","value":"kube-probe"}]}`
	req := authedRequest(http.MethodPut, "/api/log-skip-rules/access", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
}

func TestHandleLogSkipRulesPut_ValidatesAdvancedRawIsArray(t *testing.T) {
	th := newTestHarness(t)
	setupRec := th.doSetup(t, "testpass")
	cookie := sessionCookie(setupRec)
	seedLoggingConfig(t, th, "access")

	body := `{"mode":"advanced","conditions":[],"advanced_raw":{"not":"array"}}`
	req := authedRequest(http.MethodPut, "/api/log-skip-rules/access", body, cookie)
	rec := httptest.NewRecorder()
	th.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
}

func seedLoggingConfig(t *testing.T, th *testHarness, sinkNames ...string) {
	t.Helper()
	logs := make(map[string]any, len(sinkNames))
	for _, name := range sinkNames {
		logs[name] = map[string]any{}
	}
	payload, _ := json.Marshal(map[string]any{"logs": logs})
	resp, err := http.Post(th.caddySrv.URL+"/config/logging", "application/json", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("seeding logging config: %v", err)
	}
	resp.Body.Close()
}
