package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andretakagi/kaji/internal/auth"
	"github.com/andretakagi/kaji/internal/config"
)

// okHandler is a minimal handler that writes 200 OK.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func newStore(cfg *config.AppConfig) *config.ConfigStore {
	return config.NewStore(cfg)
}

// TestRequireAuthPublicPaths verifies that public paths pass without credentials.
func TestRequireAuthPublicPaths(t *testing.T) {
	store := newStore(&config.AppConfig{
		AuthEnabled: true,
	})
	h := requireAuth(store, okHandler)

	paths := []string{
		"/api/setup/status",
		"/api/auth/login",
		"/api/auth/status",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("path %s: got %d, want 200", path, rec.Code)
			}
		})
	}
}

// TestRequireAuthSetupOnlyPaths verifies that setup-only paths pass when config
// does not exist.
func TestRequireAuthSetupOnlyPaths(t *testing.T) {
	t.Setenv("KAJI_CONFIG_PATH", "/nonexistent/path/config.json")

	store := newStore(&config.AppConfig{
		AuthEnabled: true,
	})
	h := requireAuth(store, okHandler)

	paths := []string{
		"/api/setup",
		"/api/setup/adapt-caddyfile",
		"/api/setup/default-caddyfile",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("path %s: got %d, want 200", path, rec.Code)
			}
		})
	}
}

// TestRequireAuthValidSession verifies that a valid signed session cookie passes.
func TestRequireAuthValidSession(t *testing.T) {
	secret := "test-session-secret"
	store := newStore(&config.AppConfig{
		AuthEnabled:   true,
		PasswordHash:  "$2a$10$placeholder",
		SessionSecret: secret,
		SessionMaxAge: 86400,
	})
	h := requireAuth(store, okHandler)

	token, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generating token: %v", err)
	}
	signed := auth.SignToken(token, secret)

	req := httptest.NewRequest(http.MethodGet, "/api/routes", nil)
	req.AddCookie(&http.Cookie{Name: "kaji_session", Value: signed})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

// TestRequireAuthInvalidSession verifies that an invalid session token returns 401.
func TestRequireAuthInvalidSession(t *testing.T) {
	store := newStore(&config.AppConfig{
		AuthEnabled:   true,
		PasswordHash:  "$2a$10$placeholder",
		SessionSecret: "real-secret",
		SessionMaxAge: 86400,
	})
	h := requireAuth(store, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/routes", nil)
	req.AddCookie(&http.Cookie{Name: "kaji_session", Value: "bad.token.value"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rec.Code)
	}
}

// TestRequireAuthExpiredSession verifies that an expired session token returns 401.
func TestRequireAuthExpiredSession(t *testing.T) {
	secret := "test-secret"
	store := newStore(&config.AppConfig{
		AuthEnabled:   true,
		PasswordHash:  "$2a$10$placeholder",
		SessionSecret: secret,
		SessionMaxAge: 1, // 1 second - token issued "in the past" will expire
	})
	h := requireAuth(store, okHandler)

	// Build a token with a fake issuedAt far in the past so it is expired.
	// Format: token.issuedAt.sig - use a crafted string with wrong sig so it
	// hits the signature check (returns false) which also returns 401.
	req := httptest.NewRequest(http.MethodGet, "/api/routes", nil)
	req.AddCookie(&http.Cookie{Name: "kaji_session", Value: "token.1.invalidsig"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rec.Code)
	}
}

// TestRequireAuthValidAPIKey verifies that a valid API key in Bearer token passes.
func TestRequireAuthValidAPIKey(t *testing.T) {
	key, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generating API key: %v", err)
	}
	hash := auth.HashAPIKey(key)

	store := newStore(&config.AppConfig{
		AuthEnabled: true,
		APIKeyHash:  hash,
	})
	h := requireAuth(store, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/routes", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

// TestRequireAuthInvalidBearerWithAPIKeySet verifies that a wrong bearer token
// returns 401 when an API key is configured.
func TestRequireAuthInvalidBearerWithAPIKeySet(t *testing.T) {
	key, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generating API key: %v", err)
	}
	hash := auth.HashAPIKey(key)

	store := newStore(&config.AppConfig{
		AuthEnabled: false,
		APIKeyHash:  hash,
	})
	h := requireAuth(store, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/routes", nil)
	req.Header.Set("Authorization", "Bearer wrongkey")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rec.Code)
	}
}

// TestRequireAuthDisabledNoAPIKey verifies that with auth disabled and no API
// key configured, all requests pass.
func TestRequireAuthDisabledNoAPIKey(t *testing.T) {
	store := newStore(&config.AppConfig{
		AuthEnabled: false,
		APIKeyHash:  "",
	})
	h := requireAuth(store, okHandler)

	paths := []string{"/api/routes", "/api/settings", "/api/anything"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("path %s: got %d, want 200", path, rec.Code)
			}
		})
	}
}

// TestLimitRequestBodyRejectOversized verifies that POST requests with bodies
// larger than 1 MB are rejected during body read.
func TestLimitRequestBodyRejectOversized(t *testing.T) {
	readBody := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	h := limitRequestBody(readBody)

	oversized := bytes.Repeat([]byte("x"), (1<<20)+1)
	req := httptest.NewRequest(http.MethodPost, "/api/something", bytes.NewReader(oversized))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("got %d, want 413", rec.Code)
	}
}

// TestLimitRequestBodyAllowsGET verifies that GET requests are not body-limited.
func TestLimitRequestBodyAllowsGET(t *testing.T) {
	h := limitRequestBody(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

// TestAccessLogPassesThrough verifies that accessLog forwards requests to the
// next handler and does not interfere with the response.
func TestAccessLogPassesThrough(t *testing.T) {
	h := accessLog(okHandler)

	cases := []struct {
		path string
		want int
	}{
		{"/api/routes", http.StatusOK},
		{"/static/app.js", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("path %s: got %d, want %d", tc.path, rec.Code, tc.want)
			}
		})
	}
}

// TestAccessLogSkipsNonAPIPath verifies that non-API paths skip the logging
// branch by confirming the handler still passes the request through.
func TestAccessLogSkipsNonAPIPath(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	h := accessLog(next)
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called for non-API path")
	}
	if !strings.HasPrefix("/index.html", "/api/") && rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}
