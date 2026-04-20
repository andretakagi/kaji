package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFileServerConfig(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "notadir.txt")
	if err := os.WriteFile(tmpFile, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		json    string
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "valid config",
			json:   `{"root":"` + tmpDir + `","browse":false,"index_names":["index.html"],"hide":[".*"]}`,
			wantOK: true,
		},
		{
			name:   "valid minimal",
			json:   `{"root":"` + tmpDir + `"}`,
			wantOK: true,
		},
		{
			name:    "empty root",
			json:    `{"root":""}`,
			wantOK:  false,
			wantMsg: "root directory is required",
		},
		{
			name:    "missing root",
			json:    `{}`,
			wantOK:  false,
			wantMsg: "root directory is required",
		},
		{
			name:    "root does not exist",
			json:    `{"root":"/nonexistent/path/that/does/not/exist"}`,
			wantOK:  false,
			wantMsg: "root directory does not exist",
		},
		{
			name:    "root is a file",
			json:    `{"root":"` + tmpFile + `"}`,
			wantOK:  false,
			wantMsg: "root path is not a directory",
		},
		{
			name:    "empty string in index_names",
			json:    `{"root":"` + tmpDir + `","index_names":["index.html",""]}`,
			wantOK:  false,
			wantMsg: "index_names entries must not be empty",
		},
		{
			name:    "empty string in hide",
			json:    `{"root":"` + tmpDir + `","hide":[".*",""]}`,
			wantOK:  false,
			wantMsg: "hide entries must not be empty",
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
			ok := validateFileServerConfig(rec, []byte(tc.json))
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
