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
	caddyConfig := json.RawMessage(`{"apps":{"http":{"servers":{"srv0":{"routes":[{"match":[{"host":["example.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"localhost:3000"}]}]}]}}}}}`)
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
		IPLists: []config.IPList{
			{ID: "list1", Name: "blocklist", Type: "deny", IPs: []string{"10.0.0.1"}},
		},
		DisabledDomains: []config.DisabledDomain{
			{ID: "route-abc", Server: "srv0", DisabledAt: "2025-01-01T00:00:00Z"},
		},
		DomainIPLists: map[string]string{"route-abc": "list1"},
	}
	store := config.NewStore(appCfg)

	ssDir := t.TempDir()
	ss := snapshot.NewStore(ssDir)
	ss.Create("snap-a", "first snapshot", "manual", &snapshot.Data{CaddyConfig: json.RawMessage(`{"snap":"a"}`)})

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

	caddyData, ok := files["kaji-export/caddy.json"]
	if !ok {
		t.Fatal("caddy.json missing from zip")
	}
	var gotCaddy json.RawMessage
	if err := json.Unmarshal(caddyData, &gotCaddy); err != nil {
		t.Fatalf("caddy.json is not valid JSON: %v", err)
	}
	// Re-marshal both to compare without whitespace differences.
	wantCompact, _ := json.Marshal(caddyConfig)
	gotCompact, _ := json.Marshal(gotCaddy)
	if string(gotCompact) != string(wantCompact) {
		t.Errorf("caddy.json content = %s, want %s", gotCompact, wantCompact)
	}

	caddyfileData, ok := files["kaji-export/Caddyfile"]
	if !ok {
		t.Fatal("Caddyfile missing from zip")
	}
	if !bytes.Contains(caddyfileData, []byte("example.com")) {
		t.Errorf("Caddyfile should contain route host; got %s", caddyfileData)
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
	if exportedCfg.Loki.BatchSize != 1048576 {
		t.Errorf("Loki.BatchSize = %d, want 1048576", exportedCfg.Loki.BatchSize)
	}
	if exportedCfg.Loki.FlushIntervalSeconds != 5 {
		t.Errorf("Loki.FlushIntervalSeconds = %d, want 5", exportedCfg.Loki.FlushIntervalSeconds)
	}
	if len(exportedCfg.IPLists) != 1 || exportedCfg.IPLists[0].ID != "list1" {
		t.Errorf("IPLists not preserved: got %+v", exportedCfg.IPLists)
	}
	if len(exportedCfg.DisabledDomains) != 1 || exportedCfg.DisabledDomains[0].ID != "route-abc" {
		t.Errorf("DisabledDomains not preserved: got %+v", exportedCfg.DisabledDomains)
	}
	if exportedCfg.DomainIPLists["route-abc"] != "list1" {
		t.Errorf("DomainIPLists not preserved: got %+v", exportedCfg.DomainIPLists)
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
	ss.Create("roundtrip", "test", "manual", &snapshot.Data{CaddyConfig: json.RawMessage(`{"rt":true}`)})

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

func TestFullStateSnapshotRoundTrip(t *testing.T) {
	caddyConfig := json.RawMessage(`{"apps":{"http":{"servers":{"srv0":{"routes":[]}}}}}`)
	cc := fakeCaddyServer(t, caddyConfig)

	appCfg := &config.AppConfig{
		AuthEnabled:   true,
		PasswordHash:  "secret",
		CaddyAdminURL: "http://localhost:2019",
		Loki: config.LokiConfig{
			BatchSize:            1048576,
			FlushIntervalSeconds: 5,
		},
		IPLists: []config.IPList{
			{ID: "list1", Name: "blocklist", Type: "deny", IPs: []string{"10.0.0.1"}},
		},
	}
	store := config.NewStore(appCfg)

	ssDir := t.TempDir()
	ss := snapshot.NewStore(ssDir)

	// Create a full-state snapshot (new envelope format with app config and version).
	snapAppCfg := json.RawMessage(`{"auth_enabled":true,"caddy_admin_url":"http://localhost:2019"}`)
	ss.Create("full-state", "with app config", "manual", &snapshot.Data{
		KajiVersion: "1.5.0",
		CaddyConfig: json.RawMessage(`{"apps":{"http":{}}}`),
		AppConfig:   snapAppCfg,
	})

	var buf bytes.Buffer
	if err := BuildZIP(&buf, cc, store, ss, "1.5.0"); err != nil {
		t.Fatalf("BuildZIP: %v", err)
	}

	// Import the ZIP on a fresh snapshot store.
	data := buf.Bytes()
	backup, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.5.0")
	if err != nil {
		t.Fatalf("ParseZIP: %v", err)
	}

	if backup.Snapshots == nil {
		t.Fatal("snapshots should be present")
	}

	importDir := t.TempDir()
	importSS := snapshot.NewStore(importDir)
	if err := restoreSnapshots(importSS, backup.Snapshots); err != nil {
		t.Fatalf("restoreSnapshots: %v", err)
	}

	// Read the imported snapshot and verify the envelope survived.
	idx := importSS.GetIndex()
	if len(idx.Snapshots) != 1 {
		t.Fatalf("snapshot count = %d, want 1", len(idx.Snapshots))
	}

	snap := idx.Snapshots[0]
	if snap.KajiVersion != "1.5.0" {
		t.Errorf("imported snapshot KajiVersion = %q, want 1.5.0", snap.KajiVersion)
	}

	d, err := importSS.ReadData(snap.ID)
	if err != nil {
		t.Fatalf("ReadData: %v", err)
	}
	if d.KajiVersion != "1.5.0" {
		t.Errorf("envelope KajiVersion = %q, want 1.5.0", d.KajiVersion)
	}
	if d.AppConfig == nil {
		t.Fatal("envelope AppConfig should not be nil")
	}

	var gotApp map[string]any
	json.Unmarshal(d.AppConfig, &gotApp)
	if gotApp["auth_enabled"] != true {
		t.Error("AppConfig auth_enabled should survive export/import round-trip")
	}
}

func TestLegacySnapshotSurvivesExportImport(t *testing.T) {
	caddyConfig := json.RawMessage(`{"apps":{"http":{"servers":{"srv0":{"routes":[]}}}}}`)

	snapIndex := snapshot.Index{
		CurrentID: "legacy1",
		Snapshots: []snapshot.Snapshot{
			{ID: "legacy1", Name: "old-snap", Type: "manual", CreatedAt: "2025-01-01T00:00:00Z"},
		},
	}

	zipData := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json":          mustJSON(t, validManifest("1.0.0")),
		"kaji-export/caddy.json":             caddyConfig,
		"kaji-export/config.json":            mustJSON(t, validAppConfig()),
		"kaji-export/snapshots/index.json":   mustJSON(t, snapIndex),
		"kaji-export/snapshots/legacy1.json": []byte(`{"apps":{"http":{"servers":{}}}}`),
	})

	backup, err := ParseZIP(bytes.NewReader(zipData), int64(len(zipData)), "1.5.0")
	if err != nil {
		t.Fatalf("ParseZIP: %v", err)
	}

	importDir := t.TempDir()
	importSS := snapshot.NewStore(importDir)
	if err := restoreSnapshots(importSS, backup.Snapshots); err != nil {
		t.Fatalf("restoreSnapshots: %v", err)
	}

	// The legacy snapshot should be readable via ReadData and detected as legacy.
	d, err := importSS.ReadData("legacy1")
	if err != nil {
		t.Fatalf("ReadData: %v", err)
	}
	if d.KajiVersion != "" {
		t.Errorf("legacy snapshot KajiVersion = %q, want empty", d.KajiVersion)
	}
	if d.AppConfig != nil {
		t.Error("legacy snapshot should have nil AppConfig")
	}
	if d.CaddyConfig == nil {
		t.Fatal("legacy snapshot should still have CaddyConfig")
	}
}
