package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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
			got := validateLogFilePaths([]byte(tt.body))
			if tt.wantErr && got == "" {
				t.Error("expected non-empty error string, got empty")
			}
			if !tt.wantErr && got != "" {
				t.Errorf("expected empty error string, got %q", got)
			}
		})
	}
}
