package api

import (
	"net/http/httptest"
	"testing"
)

func TestValidateRedirectConfig(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "valid config",
			json:   `{"target_url":"https://example.com","status_code":"301","preserve_path":false}`,
			wantOK: true,
		},
		{
			name:   "valid with preserve path",
			json:   `{"target_url":"https://example.com","status_code":"302","preserve_path":true}`,
			wantOK: true,
		},
		{
			name:   "empty status code allowed",
			json:   `{"target_url":"https://example.com"}`,
			wantOK: true,
		},
		{
			name:    "empty target url",
			json:    `{"target_url":"","status_code":"301"}`,
			wantOK:  false,
			wantMsg: "target URL is required",
		},
		{
			name:    "whitespace only target url",
			json:    `{"target_url":"   ","status_code":"301"}`,
			wantOK:  false,
			wantMsg: "target URL is required",
		},
		{
			name:    "missing target url",
			json:    `{"status_code":"301"}`,
			wantOK:  false,
			wantMsg: "target URL is required",
		},
		{
			name:    "status code out of range high",
			json:    `{"target_url":"https://example.com","status_code":"600"}`,
			wantOK:  false,
			wantMsg: "status code must be between 100 and 599",
		},
		{
			name:    "status code out of range low",
			json:    `{"target_url":"https://example.com","status_code":"99"}`,
			wantOK:  false,
			wantMsg: "status code must be between 100 and 599",
		},
		{
			name:    "status code non-numeric",
			json:    `{"target_url":"https://example.com","status_code":"abc"}`,
			wantOK:  false,
			wantMsg: "status code must be between 100 and 599",
		},
		{
			name:    "invalid json",
			json:    `{bad`,
			wantOK:  false,
			wantMsg: "invalid handler config",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ok := validateRedirectConfig(rec, []byte(tc.json))
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok && tc.wantMsg != "" {
				body := rec.Body.String()
				if body == "" || !contains(body, tc.wantMsg) {
					t.Errorf("body = %q, want to contain %q", body, tc.wantMsg)
				}
			}
		})
	}
}
