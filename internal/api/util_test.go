package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/andretakagi/kaji/internal/auth"
	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
)

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		name    string
		query   url.Values
		param   string
		min     int
		max     int
		wantVal int
		wantOK  bool
	}{
		{
			name:    "missing",
			query:   url.Values{},
			param:   "limit",
			min:     0,
			max:     1000,
			wantVal: 0,
			wantOK:  true,
		},
		{
			name:    "valid",
			query:   url.Values{"limit": []string{"5"}},
			param:   "limit",
			min:     0,
			max:     1000,
			wantVal: 5,
			wantOK:  true,
		},
		{
			name:    "invalid",
			query:   url.Values{"limit": []string{"abc"}},
			param:   "limit",
			min:     0,
			max:     1000,
			wantVal: 0,
			wantOK:  false,
		},
		{
			name:    "below_min",
			query:   url.Values{"limit": []string{"-1"}},
			param:   "limit",
			min:     0,
			max:     1000,
			wantVal: 0,
			wantOK:  true,
		},
		{
			name:    "above_max",
			query:   url.Values{"limit": []string{"2000"}},
			param:   "limit",
			min:     0,
			max:     1000,
			wantVal: 1000,
			wantOK:  true,
		},
		{
			name:    "negative_max_skips_clamp",
			query:   url.Values{"limit": []string{"9999"}},
			param:   "limit",
			min:     0,
			max:     -1,
			wantVal: 9999,
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			val, ok := parseIntParam(rec, tt.query, tt.param, tt.min, tt.max)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if val != tt.wantVal {
				t.Errorf("val = %d, want %d", val, tt.wantVal)
			}
			if !tt.wantOK && rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestExtractCaddyMessage(t *testing.T) {
	const fallback = "caddy returned an error - check server logs for details"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "embedded_json",
			input: `caddy rejected config (status 400): {"error":"invalid listener address"}`,
			want:  "invalid listener address",
		},
		{
			name:  "loading_prefix",
			input: `caddy rejected (status 400): {"error":"loading new config: bad value"}`,
			want:  "bad value",
		},
		{
			name:  "no_json",
			input: "connection refused",
			want:  fallback,
		},
		{
			name:  "empty_error_field",
			input: `caddy rejected (status 400): {"error":""}`,
			want:  fallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCaddyMessage(tt.input)
			if got != tt.want {
				t.Errorf("extractCaddyMessage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripGoStructs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(string) bool
		desc  string
	}{
		{
			name:  "with_struct",
			input: `open file: &logging.FileWriter{Filename:"/tmp/log"}: permission denied`,
			check: func(s string) bool { return !strings.Contains(s, "&logging") },
			desc:  "should not contain &logging",
		},
		{
			name:  "no_structs",
			input: "simple error message",
			check: func(s string) bool { return s == "simple error message" },
			desc:  "should be unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripGoStructs(tt.input)
			if !tt.check(got) {
				t.Errorf("stripGoStructs(%q) = %q, %s", tt.input, got, tt.desc)
			}
		})
	}
}

func TestValidateLogFilePaths(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "valid_path",
			body:    `{"logs":{"mylog":{"writer":{"output":"file","filename":"/var/log/caddy/access.log"}}}}`,
			wantErr: false,
		},
		{
			name:    "invalid_path",
			body:    `{"logs":{"mylog":{"writer":{"output":"file","filename":"/tmp/evil.log"}}}}`,
			wantErr: true,
		},
		{
			name:    "non_file_output",
			body:    `{"logs":{"mylog":{"writer":{"output":"stdout"}}}}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateLogFilePaths([]byte(tt.body), "/var/log/caddy/")
			if tt.wantErr && got == "" {
				t.Error("expected non-empty error string, got empty")
			}
			if !tt.wantErr && got != "" {
				t.Errorf("expected empty error string, got %q", got)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, map[string]string{"key": "value"})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if got["key"] != "value" {
		t.Errorf("body key = %q, want %q", got["key"], "value")
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, "something broke", http.StatusBadRequest)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if got["error"] != "something broke" {
		t.Errorf("error = %q, want %q", got["error"], "something broke")
	}
}

func TestWriteRawJSON(t *testing.T) {
	raw := []byte(`{"already":"encoded"}`)
	rec := httptest.NewRecorder()
	writeRawJSON(rec, raw)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if rec.Body.String() != string(raw) {
		t.Errorf("body = %q, want %q", rec.Body.String(), string(raw))
	}
}

func TestDecodeBody(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		body := strings.NewReader(`{"name":"test"}`)
		req := httptest.NewRequest(http.MethodPost, "/", body)
		rec := httptest.NewRecorder()

		var got struct{ Name string }
		ok := decodeBody(rec, req, &got)
		if !ok {
			t.Fatal("decodeBody returned false for valid JSON")
		}
		if got.Name != "test" {
			t.Errorf("Name = %q, want %q", got.Name, "test")
		}
	})

	t.Run("invalid", func(t *testing.T) {
		body := strings.NewReader(`not json`)
		req := httptest.NewRequest(http.MethodPost, "/", body)
		rec := httptest.NewRecorder()

		var got struct{ Name string }
		ok := decodeBody(rec, req, &got)
		if ok {
			t.Fatal("decodeBody returned true for invalid JSON")
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
		var errResp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to parse error response: %v", err)
		}
		if errResp["error"] != "invalid request body" {
			t.Errorf("error = %q, want %q", errResp["error"], "invalid request body")
		}
	})
}

func TestCaddyError(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		rec := httptest.NewRecorder()
		caddyError(rec, "test", errors.New("dial tcp: connection unreachable"))

		if rec.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", rec.Code)
		}
		var got map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if !strings.Contains(got["error"], "unreachable") {
			t.Errorf("error = %q, want message about unreachable", got["error"])
		}
	})

	t.Run("caddy_json_error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		caddyError(rec, "test", errors.New(`caddy rejected config (status 400): {"error":"invalid listener address"}`))

		if rec.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", rec.Code)
		}
		var got map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if got["error"] != "invalid listener address" {
			t.Errorf("error = %q, want %q", got["error"], "invalid listener address")
		}
	})
}

func TestSessionMaxAge(t *testing.T) {
	t.Run("custom", func(t *testing.T) {
		cfg := &config.AppConfig{SessionMaxAge: 3600}
		if got := sessionMaxAge(cfg); got != 3600 {
			t.Errorf("sessionMaxAge = %d, want 3600", got)
		}
	})

	t.Run("zero_uses_default", func(t *testing.T) {
		cfg := &config.AppConfig{SessionMaxAge: 0}
		if got := sessionMaxAge(cfg); got != auth.DefaultSessionMaxAge {
			t.Errorf("sessionMaxAge = %d, want %d", got, auth.DefaultSessionMaxAge)
		}
	})

	t.Run("negative_uses_default", func(t *testing.T) {
		cfg := &config.AppConfig{SessionMaxAge: -1}
		if got := sessionMaxAge(cfg); got != auth.DefaultSessionMaxAge {
			t.Errorf("sessionMaxAge = %d, want %d", got, auth.DefaultSessionMaxAge)
		}
	})
}

func TestHashBasicAuthPassword(t *testing.T) {
	t.Run("password_provided", func(t *testing.T) {
		ba := &caddy.BasicAuth{Password: "secret"}
		if err := hashBasicAuthPassword(ba, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ba.PasswordHash == "" {
			t.Error("PasswordHash should be set")
		}
		if ba.Password != "" {
			t.Error("Password should be cleared after hashing")
		}
	})

	t.Run("no_password_with_fallback", func(t *testing.T) {
		ba := &caddy.BasicAuth{}
		if err := hashBasicAuthPassword(ba, "existing-hash"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ba.PasswordHash != "existing-hash" {
			t.Errorf("PasswordHash = %q, want %q", ba.PasswordHash, "existing-hash")
		}
	})

	t.Run("no_password_no_fallback", func(t *testing.T) {
		ba := &caddy.BasicAuth{}
		err := hashBasicAuthPassword(ba, "")
		if err == nil {
			t.Fatal("expected error when no password and no fallback")
		}
		if !strings.Contains(err.Error(), "password is required") {
			t.Errorf("error = %q, want message about password required", err)
		}
	})

	t.Run("existing_hash_preserved", func(t *testing.T) {
		ba := &caddy.BasicAuth{PasswordHash: "already-set"}
		if err := hashBasicAuthPassword(ba, "fallback"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ba.PasswordHash != "already-set" {
			t.Errorf("PasswordHash = %q, want %q (should not be overwritten)", ba.PasswordHash, "already-set")
		}
	})
}
