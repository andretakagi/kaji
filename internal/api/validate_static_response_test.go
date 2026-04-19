package api

import (
	"net/http/httptest"
	"testing"
)

func TestValidateHandlerType(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"reverse_proxy", "reverse_proxy", ""},
		{"static_response", "static_response", ""},
		{"redirect not yet supported", "redirect", `handler type "redirect" is not yet supported`},
		{"file_server not yet supported", "file_server", `handler type "file_server" is not yet supported`},
		{"unknown type", "websocket", "unknown handler type: websocket"},
		{"empty", "", "unknown handler type: "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateHandlerType(tc.input)
			if got != tc.want {
				t.Errorf("validateHandlerType(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateStaticResponseConfig(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "close true",
			json:   `{"close":true}`,
			wantOK: true,
		},
		{
			name:   "valid status code",
			json:   `{"status_code":"200","body":"OK"}`,
			wantOK: true,
		},
		{
			name:   "empty status code allowed",
			json:   `{"body":"hello"}`,
			wantOK: true,
		},
		{
			name:   "status 100",
			json:   `{"status_code":"100"}`,
			wantOK: true,
		},
		{
			name:   "status 599",
			json:   `{"status_code":"599"}`,
			wantOK: true,
		},
		{
			name:    "status below 100",
			json:    `{"status_code":"99"}`,
			wantOK:  false,
			wantMsg: "status code must be between 100 and 599",
		},
		{
			name:    "status above 599",
			json:    `{"status_code":"600"}`,
			wantOK:  false,
			wantMsg: "status code must be between 100 and 599",
		},
		{
			name:    "status non-numeric",
			json:    `{"status_code":"abc"}`,
			wantOK:  false,
			wantMsg: "status code must be between 100 and 599",
		},
		{
			name:    "invalid json",
			json:    `{bad`,
			wantOK:  false,
			wantMsg: "invalid handler config",
		},
		{
			name:   "close true ignores invalid status",
			json:   `{"close":true,"status_code":"999"}`,
			wantOK: true,
		},
		{
			name:   "with headers",
			json:   `{"status_code":"200","headers":{"Content-Type":["text/plain"]}}`,
			wantOK: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ok := validateStaticResponseConfig(rec, []byte(tc.json))
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
