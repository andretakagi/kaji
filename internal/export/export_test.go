package export

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func fakeCaddyServer(t *testing.T, caddyConfig json.RawMessage) *caddy.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/config/" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(caddyConfig)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/load" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return caddy.NewClient(func() string { return srv.URL })
}

func TestBuildZIPRoundTrip(t *testing.T) {
	caddyConfig := json.RawMessage(`{"apps":{"http":{"servers":{"srv0":{"routes":[]}}}}}`)
	cc := fakeCaddyServer(t, caddyConfig)

	appCfg := &config.AppConfig{
		AuthEnabled:     true,
		PasswordHash:    "secret_hash",
		SessionSecret:   "secret_session",
		SessionMaxAge:   3600,
		SecureCookies:   "auto",
		CaddyAdminURL:   "http://localhost:2019",
		CaddyConfigPath: "/etc/caddy/caddy.json",
		LogFile:         "/var/log/caddy/access.log",
		Loki: config.LokiConfig{
			BatchSize:            1048576,
			FlushIntervalSeconds: 5,
		},
	}
	store := config.NewStore(appCfg)

	ssDir := t.TempDir()
	ss := snapshot.NewStore(ssDir)
	ss.Create("snap-a", "first snapshot", "manual", json.RawMessage(`{"snap":"a"}`))

	var buf bytes.Buffer
	if err := BuildZIP(&buf, cc, store, ss, "1.2.3"); err != nil {
		t.Fatalf("BuildZIP: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("opening zip: %v", err)
	}

	files := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = data
	}

	manifestData, ok := files["kaji-export/manifest.json"]
	if !ok {
		t.Fatal("manifest.json missing from zip")
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("parsing manifest: %v", err)
	}
	if manifest.Version != 1 {
		t.Errorf("manifest version = %d, want 1", manifest.Version)
	}
	if manifest.KajiVersion != "1.2.3" {
		t.Errorf("kaji_version = %q, want 1.2.3", manifest.KajiVersion)
	}

	if _, ok := files["kaji-export/caddy.json"]; !ok {
		t.Error("caddy.json missing from zip")
	}

	if _, ok := files["kaji-export/Caddyfile"]; !ok {
		t.Error("Caddyfile missing from zip")
	}

	configData, ok := files["kaji-export/config.json"]
	if !ok {
		t.Fatal("config.json missing from zip")
	}
	var exportedCfg config.AppConfig
	if err := json.Unmarshal(configData, &exportedCfg); err != nil {
		t.Fatalf("parsing config: %v", err)
	}
	if exportedCfg.PasswordHash != "" {
		t.Error("PasswordHash should be stripped from export")
	}
	if exportedCfg.SessionSecret != "" {
		t.Error("SessionSecret should be stripped from export")
	}
	if exportedCfg.SessionMaxAge != 0 {
		t.Error("SessionMaxAge should be stripped from export")
	}
	if exportedCfg.SecureCookies != "" {
		t.Error("SecureCookies should be stripped from export")
	}
	if !exportedCfg.AuthEnabled {
		t.Error("AuthEnabled should be preserved in export")
	}
	if exportedCfg.CaddyAdminURL != "http://localhost:2019" {
		t.Errorf("CaddyAdminURL = %q, want http://localhost:2019", exportedCfg.CaddyAdminURL)
	}

	if _, ok := files["kaji-export/snapshots/index.json"]; !ok {
		t.Error("snapshots/index.json missing from zip")
	}
	snapFileFound := false
	for name := range files {
		if name != "kaji-export/snapshots/index.json" && len(name) > len("kaji-export/snapshots/") {
			snapFileFound = true
		}
	}
	if !snapFileFound {
		t.Error("expected at least one snapshot data file in zip")
	}
}

func TestBuildZIPNoSnapshots(t *testing.T) {
	caddyConfig := json.RawMessage(`{"apps":{"http":{"servers":{"srv0":{"routes":[]}}}}}`)
	cc := fakeCaddyServer(t, caddyConfig)

	appCfg := &config.AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		Loki: config.LokiConfig{
			BatchSize:            1048576,
			FlushIntervalSeconds: 5,
		},
	}
	store := config.NewStore(appCfg)

	ssDir := t.TempDir()
	ss := snapshot.NewStore(ssDir)

	var buf bytes.Buffer
	if err := BuildZIP(&buf, cc, store, ss, "1.0.0"); err != nil {
		t.Fatalf("BuildZIP: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("opening zip: %v", err)
	}

	for _, f := range zr.File {
		if f.Name == "kaji-export/snapshots/index.json" {
			t.Error("snapshots/index.json should not be present when there are no snapshots")
		}
	}
}

func TestBuildAndParseRoundTrip(t *testing.T) {
	caddyConfig := json.RawMessage(`{"apps":{"http":{"servers":{"srv0":{"routes":[]}}}}}`)
	cc := fakeCaddyServer(t, caddyConfig)

	appCfg := &config.AppConfig{
		AuthEnabled:     true,
		PasswordHash:    "stripped",
		SessionSecret:   "stripped",
		CaddyAdminURL:   "http://localhost:2019",
		CaddyConfigPath: "/etc/caddy/caddy.json",
		Loki: config.LokiConfig{
			BatchSize:            1048576,
			FlushIntervalSeconds: 5,
		},
	}
	store := config.NewStore(appCfg)

	ssDir := t.TempDir()
	ss := snapshot.NewStore(ssDir)
	ss.Create("roundtrip", "test", "manual", json.RawMessage(`{"rt":true}`))

	var buf bytes.Buffer
	if err := BuildZIP(&buf, cc, store, ss, "1.0.0"); err != nil {
		t.Fatalf("BuildZIP: %v", err)
	}

	data := buf.Bytes()
	backup, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err != nil {
		t.Fatalf("ParseZIP: %v", err)
	}

	if backup.Manifest.KajiVersion != "1.0.0" {
		t.Errorf("round-trip version = %q, want 1.0.0", backup.Manifest.KajiVersion)
	}
	if backup.AppConfig.PasswordHash != "" {
		t.Error("credentials should not survive round-trip")
	}
	if !backup.AppConfig.AuthEnabled {
		t.Error("AuthEnabled should survive round-trip")
	}
	if backup.Snapshots == nil {
		t.Fatal("snapshots should survive round-trip")
	}
	if len(backup.Snapshots.Index.Snapshots) != 1 {
		t.Errorf("snapshot count = %d, want 1", len(backup.Snapshots.Index.Snapshots))
	}
}
