package export

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"kaji-export/manifest.json", "manifest.json"},
		{"kaji-export/snapshots/abc.json", "snapshots/abc.json"},
		{"kaji-export/", ""},
		{"noprefix", ""},
		{"a/b/c", "b/c"},
	}
	for _, tt := range tests {
		got := stripPrefix(tt.input)
		if got != tt.want {
			t.Errorf("stripPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func buildTestZIP(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("creating %s in zip: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buf.Bytes()
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func validManifest(version string) Manifest {
	return Manifest{
		Version:     1,
		ExportedAt:  "2025-01-01T00:00:00Z",
		KajiVersion: version,
	}
}

func validCaddyConfig() json.RawMessage {
	return json.RawMessage(`{"apps":{"http":{"servers":{}}}}`)
}

func validAppConfig() config.AppConfig {
	return config.AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		Loki: config.LokiConfig{
			BatchSize:            1048576,
			FlushIntervalSeconds: 5,
		},
	}
}

func TestParseZIPValid(t *testing.T) {
	manifest := validManifest("1.0.0")
	appCfg := validAppConfig()

	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json": mustJSON(t, manifest),
		"kaji-export/caddy.json":    validCaddyConfig(),
		"kaji-export/config.json":   mustJSON(t, appCfg),
	})

	backup, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err != nil {
		t.Fatalf("ParseZIP: %v", err)
	}

	if backup.Manifest.Version != 1 {
		t.Errorf("manifest version = %d, want 1", backup.Manifest.Version)
	}
	if backup.Manifest.KajiVersion != "1.0.0" {
		t.Errorf("kaji_version = %q, want 1.0.0", backup.Manifest.KajiVersion)
	}
	if backup.AppConfig.CaddyAdminURL != "http://localhost:2019" {
		t.Errorf("caddy_admin_url = %q, want http://localhost:2019", backup.AppConfig.CaddyAdminURL)
	}
	if backup.Snapshots != nil {
		t.Error("snapshots should be nil when no snapshot files present")
	}
}

func TestParseZIPWithSnapshots(t *testing.T) {
	snapIndex := snapshot.Index{
		Snapshots: []snapshot.Snapshot{
			{ID: "snap1", Name: "first", Type: "manual"},
		},
	}

	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json":        mustJSON(t, validManifest("1.0.0")),
		"kaji-export/caddy.json":           validCaddyConfig(),
		"kaji-export/config.json":          mustJSON(t, validAppConfig()),
		"kaji-export/snapshots/index.json": mustJSON(t, snapIndex),
		"kaji-export/snapshots/snap1.json": []byte(`{"routes":[]}`),
	})

	backup, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err != nil {
		t.Fatalf("ParseZIP: %v", err)
	}

	if backup.Snapshots == nil {
		t.Fatal("expected snapshots to be present")
	}
	if len(backup.Snapshots.Index.Snapshots) != 1 {
		t.Fatalf("snapshot count = %d, want 1", len(backup.Snapshots.Index.Snapshots))
	}
	if _, ok := backup.Snapshots.Files["snap1"]; !ok {
		t.Error("snap1 file data missing")
	}
}

func TestParseZIPMissingManifest(t *testing.T) {
	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/caddy.json":  validCaddyConfig(),
		"kaji-export/config.json": mustJSON(t, validAppConfig()),
	})

	_, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestParseZIPMissingCaddyJSON(t *testing.T) {
	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json": mustJSON(t, validManifest("1.0.0")),
		"kaji-export/config.json":   mustJSON(t, validAppConfig()),
	})

	_, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err == nil {
		t.Fatal("expected error for missing caddy.json")
	}
}

func TestParseZIPMissingConfig(t *testing.T) {
	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json": mustJSON(t, validManifest("1.0.0")),
		"kaji-export/caddy.json":    validCaddyConfig(),
	})

	_, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err == nil {
		t.Fatal("expected error for missing config.json")
	}
}

func TestParseZIPRejectsNewerVersion(t *testing.T) {
	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json": mustJSON(t, validManifest("2.0.0")),
		"kaji-export/caddy.json":    validCaddyConfig(),
		"kaji-export/config.json":   mustJSON(t, validAppConfig()),
	})

	_, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err == nil {
		t.Fatal("expected error for newer backup version")
	}
}

func TestParseZIPRejectsBadManifestVersion(t *testing.T) {
	manifest := Manifest{
		Version:     0,
		ExportedAt:  "2025-01-01T00:00:00Z",
		KajiVersion: "1.0.0",
	}
	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json": mustJSON(t, manifest),
		"kaji-export/caddy.json":    validCaddyConfig(),
		"kaji-export/config.json":   mustJSON(t, validAppConfig()),
	})

	_, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err == nil {
		t.Fatal("expected error for manifest version 0")
	}
}

func TestParseZIPRunsMigrations(t *testing.T) {
	origMigrations := migrations
	defer func() { migrations = origMigrations }()

	migrations = []Migration{
		{
			Before:  "1.1.0",
			Summary: "add new_field",
			Fn: func(m map[string]any) []string {
				return []string{setDefault(m, "new_field", true)}
			},
		},
	}

	appCfg := validAppConfig()
	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json": mustJSON(t, validManifest("1.0.0")),
		"kaji-export/caddy.json":    validCaddyConfig(),
		"kaji-export/config.json":   mustJSON(t, appCfg),
	})

	backup, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.1.0")
	if err != nil {
		t.Fatalf("ParseZIP: %v", err)
	}
	if len(backup.MigrationLog) != 1 {
		t.Errorf("migration log length = %d, want 1", len(backup.MigrationLog))
	}
}

func TestParseZIPSizeTooLarge(t *testing.T) {
	_, err := ParseZIP(bytes.NewReader(nil), MaxZIPSize+1, "1.0.0")
	if err == nil {
		t.Fatal("expected error for oversized zip")
	}
}

func TestRestoreSnapshots(t *testing.T) {
	dir := t.TempDir()
	ss := snapshot.NewStore(dir)

	existing, err := ss.Create("existing", "pre-existing", "manual", &snapshot.Data{CaddyConfig: json.RawMessage(`{"old":true}`)})
	if err != nil {
		t.Fatalf("Create existing: %v", err)
	}

	importData := &SnapshotData{
		Index: snapshot.Index{
			Snapshots: []snapshot.Snapshot{
				{ID: "imported1", Name: "imported", Type: "manual", CreatedAt: "2025-01-01T00:00:00Z"},
			},
		},
		Files: map[string]json.RawMessage{
			"imported1": json.RawMessage(`{"new":true}`),
		},
	}

	if err := restoreSnapshots(ss, importData); err != nil {
		t.Fatalf("restoreSnapshots: %v", err)
	}

	idx := ss.GetIndex()
	if len(idx.Snapshots) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(idx.Snapshots))
	}

	foundExisting, foundImported := false, false
	for _, s := range idx.Snapshots {
		if s.ID == existing.ID {
			foundExisting = true
		}
		if s.ID == "imported1" {
			foundImported = true
		}
	}
	if !foundExisting {
		t.Error("existing snapshot should still be present")
	}
	if !foundImported {
		t.Error("imported snapshot should be present")
	}

	data, err := os.ReadFile(filepath.Join(dir, "imported1.json"))
	if err != nil {
		t.Fatalf("reading imported snapshot file: %v", err)
	}
	if string(data) != `{"new":true}` {
		t.Errorf("imported snapshot data = %s, want {\"new\":true}", data)
	}
}

func TestRestoreSnapshotsDeduplicates(t *testing.T) {
	dir := t.TempDir()
	ss := snapshot.NewStore(dir)

	existing, err := ss.Create("dupe", "original", "manual", &snapshot.Data{CaddyConfig: json.RawMessage(`{"v":1}`)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	importData := &SnapshotData{
		Index: snapshot.Index{
			Snapshots: []snapshot.Snapshot{
				{ID: existing.ID, Name: "dupe", Type: "manual"},
			},
		},
		Files: map[string]json.RawMessage{
			existing.ID: json.RawMessage(`{"v":2}`),
		},
	}

	if err := restoreSnapshots(ss, importData); err != nil {
		t.Fatalf("restoreSnapshots: %v", err)
	}

	idx := ss.GetIndex()
	count := 0
	for _, s := range idx.Snapshots {
		if s.ID == existing.ID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("duplicate snapshot appeared %d times, want 1", count)
	}
}

func TestReconcilePathsFallsBackToCurrentConfig(t *testing.T) {
	imported := &config.AppConfig{
		CaddyConfigPath: "/nonexistent/path/caddy.json",
		LogFile:         "/nonexistent/log/access.log",
		LogDir:          "/nonexistent/logdir",
		CaddyDataDir:    "/nonexistent/data",
	}
	current := &config.AppConfig{
		CaddyConfigPath: "/tmp/caddy.json",
		LogFile:         "/tmp/access.log",
		LogDir:          t.TempDir(),
		CaddyDataDir:    t.TempDir(),
	}

	// Create the directories that current config points to so they pass existence checks
	os.MkdirAll(filepath.Dir(current.CaddyConfigPath), 0755)
	os.MkdirAll(filepath.Dir(current.LogFile), 0755)

	warnings := reconcilePaths(imported, current)

	if len(warnings) == 0 {
		t.Fatal("expected warnings for adjusted paths")
	}
	if imported.CaddyDataDir != current.CaddyDataDir {
		t.Errorf("CaddyDataDir = %q, want %q", imported.CaddyDataDir, current.CaddyDataDir)
	}
	if imported.LogDir != current.LogDir {
		t.Errorf("LogDir = %q, want %q", imported.LogDir, current.LogDir)
	}
}

func TestReconcilePathsNoWarningsWhenValid(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	logDir := filepath.Join(dir, "log")
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(logDir, 0755)
	os.MkdirAll(dataDir, 0755)

	imported := &config.AppConfig{
		CaddyConfigPath: filepath.Join(configDir, "caddy.json"),
		LogFile:         filepath.Join(logDir, "access.log"),
		LogDir:          logDir,
		CaddyDataDir:    dataDir,
	}
	current := &config.AppConfig{}

	warnings := reconcilePaths(imported, current)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestReconcilePathsEmptyFieldsSkipped(t *testing.T) {
	imported := &config.AppConfig{}
	current := &config.AppConfig{}

	warnings := reconcilePaths(imported, current)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for empty paths, got %v", warnings)
	}
}

func TestReconcilePathsFallsBackToDefaults(t *testing.T) {
	imported := &config.AppConfig{
		CaddyConfigPath: "/nonexistent-a/caddy.json",
		LogFile:         "/nonexistent-a/access.log",
		LogDir:          "/nonexistent-a/logdir",
		CaddyDataDir:    "/nonexistent-a/data",
	}
	current := &config.AppConfig{
		CaddyConfigPath: "/nonexistent-b/caddy.json",
		LogFile:         "/nonexistent-b/access.log",
		LogDir:          "/nonexistent-b/logdir",
		CaddyDataDir:    "/nonexistent-b/data",
	}

	warnings := reconcilePaths(imported, current)

	defaults := config.DefaultConfig()
	if imported.CaddyConfigPath != defaults.CaddyConfigPath {
		t.Errorf("CaddyConfigPath = %q, want default %q", imported.CaddyConfigPath, defaults.CaddyConfigPath)
	}
	if imported.LogFile != defaults.LogFile {
		t.Errorf("LogFile = %q, want default %q", imported.LogFile, defaults.LogFile)
	}
	if imported.LogDir != defaults.LogDir {
		t.Errorf("LogDir = %q, want default %q", imported.LogDir, defaults.LogDir)
	}
	if imported.CaddyDataDir != "" {
		t.Errorf("CaddyDataDir = %q, want empty (auto-detect)", imported.CaddyDataDir)
	}
	if len(warnings) != 4 {
		t.Errorf("warning count = %d, want 4", len(warnings))
	}
}

func TestRestoreSnapshotsPreservesAutoSettings(t *testing.T) {
	dir := t.TempDir()
	ss := snapshot.NewStore(dir)
	ss.UpdateSettings(true, 25)

	importData := &SnapshotData{
		Index: snapshot.Index{
			AutoSnapshotEnabled: false,
			AutoSnapshotLimit:   100,
			Snapshots: []snapshot.Snapshot{
				{ID: "s1", Name: "imported", Type: "manual", CreatedAt: "2025-01-01T00:00:00Z"},
			},
		},
		Files: map[string]json.RawMessage{
			"s1": json.RawMessage(`{"data":true}`),
		},
	}

	if err := restoreSnapshots(ss, importData); err != nil {
		t.Fatalf("restoreSnapshots: %v", err)
	}

	idx := ss.GetIndex()
	if !idx.AutoSnapshotEnabled {
		t.Error("AutoSnapshotEnabled should be preserved as true from current index")
	}
	if idx.AutoSnapshotLimit != 25 {
		t.Errorf("AutoSnapshotLimit = %d, want 25 (preserved from current index)", idx.AutoSnapshotLimit)
	}
}

func TestParseZIPSecondSizeGuard(t *testing.T) {
	oversized := make([]byte, MaxZIPSize+1)
	_, err := ParseZIP(bytes.NewReader(oversized), MaxZIPSize, "1.0.0")
	if err == nil {
		t.Fatal("expected error when actual data exceeds MaxZIPSize despite declared size within limit")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %q, want substring %q", err.Error(), "too large")
	}
}

func TestParseZIPMalformedConfigJSON(t *testing.T) {
	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json": mustJSON(t, validManifest("1.0.0")),
		"kaji-export/caddy.json":    validCaddyConfig(),
		"kaji-export/config.json":   []byte(`{not valid`),
	})

	_, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err == nil {
		t.Fatal("expected error for malformed config.json")
	}
	if !strings.Contains(err.Error(), "parsing config.json") {
		t.Errorf("error = %q, want substring %q", err.Error(), "parsing config.json")
	}
}

func TestParseZIPMalformedCaddyJSONPassesThrough(t *testing.T) {
	data := buildTestZIP(t, map[string][]byte{
		"kaji-export/manifest.json": mustJSON(t, validManifest("1.0.0")),
		"kaji-export/caddy.json":    []byte(`{not valid`),
		"kaji-export/config.json":   mustJSON(t, validAppConfig()),
	})

	backup, err := ParseZIP(bytes.NewReader(data), int64(len(data)), "1.0.0")
	if err != nil {
		t.Fatalf("ParseZIP should succeed with malformed caddy.json: %v", err)
	}
	if string(backup.CaddyConfig) != `{not valid` {
		t.Errorf("CaddyConfig = %s, want malformed content preserved as-is", backup.CaddyConfig)
	}
}

// mockCaddyServer creates an httptest server that simulates Caddy's admin API
// for Restore testing. It tracks /load calls and can be configured to reject them.
type mockCaddyServer struct {
	mu            sync.Mutex
	currentConfig string
	loadCalls     [][]byte
	loadFailAt    int // if >= 0, the Nth /load call returns 400
	srv           *httptest.Server
}

func newMockCaddy(t *testing.T, initialConfig string) (*mockCaddyServer, *caddy.Client) {
	t.Helper()
	m := &mockCaddyServer{
		currentConfig: initialConfig,
		loadFailAt:    -1,
	}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/config/":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(m.currentConfig))
		case r.Method == http.MethodPost && r.URL.Path == "/load":
			body, _ := io.ReadAll(r.Body)
			callIdx := len(m.loadCalls)
			m.loadCalls = append(m.loadCalls, body)
			if m.loadFailAt == callIdx {
				http.Error(w, "bad config", http.StatusBadRequest)
				return
			}
			m.currentConfig = string(body)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(m.srv.Close)
	cc := caddy.NewClient(func() string { return m.srv.URL })
	return m, cc
}

func TestRestoreHappyPath(t *testing.T) {
	initialCaddyConfig := `{"apps":{"http":{"servers":{}}}}`
	m, cc := newMockCaddy(t, initialCaddyConfig)

	store := config.NewStore(&config.AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		PasswordHash:  "original-hash",
		SessionSecret: "original-secret",
		SessionMaxAge: 3600,
		SecureCookies: "always",
		APIKeyHash:    "original-apikey",
		Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
	})

	snapDir := t.TempDir()
	ss := snapshot.NewStore(snapDir)

	backup := &Backup{
		Manifest:    validManifest("1.0.0"),
		CaddyConfig: json.RawMessage(`{"apps":{"http":{"servers":{"srv0":{"routes":[]}}}}}`),
		AppConfig: config.AppConfig{
			CaddyAdminURL: "http://localhost:2019",
			Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
		},
	}

	warnings, err := Restore(backup, cc, store, ss, false, "1.0.0")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Caddy should have received one /load call with the backup config
	m.mu.Lock()
	if len(m.loadCalls) != 1 {
		t.Errorf("expected 1 /load call, got %d", len(m.loadCalls))
	}
	m.mu.Unlock()

	// Warnings may or may not exist depending on path existence, just verify no error
	_ = warnings

	// Credentials should be preserved from the original config
	cfg := store.Get()
	if cfg.PasswordHash != "original-hash" {
		t.Errorf("PasswordHash = %q, want original-hash", cfg.PasswordHash)
	}
	if cfg.SessionSecret != "original-secret" {
		t.Errorf("SessionSecret = %q, want original-secret", cfg.SessionSecret)
	}
	if cfg.APIKeyHash != "original-apikey" {
		t.Errorf("APIKeyHash = %q, want original-apikey (PreserveCredentials)", cfg.APIKeyHash)
	}
}

func TestRestoreCaddyLoadFailure(t *testing.T) {
	m, cc := newMockCaddy(t, `{"apps":{}}`)
	m.loadFailAt = 0

	store := config.NewStore(&config.AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
	})

	snapDir := t.TempDir()
	ss := snapshot.NewStore(snapDir)

	backup := &Backup{
		Manifest:    validManifest("1.0.0"),
		CaddyConfig: json.RawMessage(`{"bad":"config"}`),
		AppConfig: config.AppConfig{
			CaddyAdminURL: "http://localhost:2019",
			Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
		},
	}

	_, err := Restore(backup, cc, store, ss, false, "1.0.0")
	if err == nil {
		t.Fatal("expected error when caddy rejects config")
	}
	if !strings.Contains(err.Error(), "loading caddy config") {
		t.Errorf("error = %q, want 'loading caddy config'", err)
	}
}

func TestRestoreConfigUpdateFailureRollsCaddyBack(t *testing.T) {
	initialCaddyConfig := `{"apps":{"http":{"servers":{}}}}`
	m, cc := newMockCaddy(t, initialCaddyConfig)

	store := config.NewStore(&config.AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
	})

	snapDir := t.TempDir()
	ss := snapshot.NewStore(snapDir)

	// Import a config with empty CaddyAdminURL to trigger validation failure
	backup := &Backup{
		Manifest:    validManifest("1.0.0"),
		CaddyConfig: json.RawMessage(`{"apps":{"http":{"servers":{"srv0":{"routes":[]}}}}}`),
		AppConfig: config.AppConfig{
			CaddyAdminURL: "",
			Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
		},
	}

	_, err := Restore(backup, cc, store, ss, false, "1.0.0")
	if err == nil {
		t.Fatal("expected error when config validation fails")
	}
	if !strings.Contains(err.Error(), "updating app config") {
		t.Errorf("error = %q, want 'updating app config'", err)
	}

	// Caddy should have received 2 /load calls: backup config, then rollback
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.loadCalls) != 2 {
		t.Fatalf("expected 2 /load calls (apply + rollback), got %d", len(m.loadCalls))
	}
	if string(m.loadCalls[1]) != initialCaddyConfig {
		t.Errorf("rollback config = %s, want original config", m.loadCalls[1])
	}
}

func TestRestoreWithAutoSnapshot(t *testing.T) {
	_, cc := newMockCaddy(t, `{"apps":{}}`)

	store := config.NewStore(&config.AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		PasswordHash:  "hash",
		SessionSecret: "secret",
		Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
	})

	snapDir := t.TempDir()
	ss := snapshot.NewStore(snapDir)

	backup := &Backup{
		Manifest:    validManifest("1.0.0"),
		CaddyConfig: json.RawMessage(`{"apps":{}}`),
		AppConfig: config.AppConfig{
			CaddyAdminURL: "http://localhost:2019",
			Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
		},
	}

	_, err := Restore(backup, cc, store, ss, true, "1.0.0")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	idx := ss.GetIndex()
	if len(idx.Snapshots) == 0 {
		t.Fatal("expected auto-snapshot to be created, got none")
	}
	found := false
	for _, s := range idx.Snapshots {
		if strings.HasPrefix(s.Name, "pre-import-") && s.Type == "auto" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a pre-import auto snapshot")
	}
}

func TestRestoreNoAutoSnapshot(t *testing.T) {
	_, cc := newMockCaddy(t, `{"apps":{}}`)

	store := config.NewStore(&config.AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
	})

	snapDir := t.TempDir()
	ss := snapshot.NewStore(snapDir)

	backup := &Backup{
		Manifest:    validManifest("1.0.0"),
		CaddyConfig: json.RawMessage(`{"apps":{}}`),
		AppConfig: config.AppConfig{
			CaddyAdminURL: "http://localhost:2019",
			Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
		},
	}

	_, err := Restore(backup, cc, store, ss, false, "1.0.0")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	idx := ss.GetIndex()
	if len(idx.Snapshots) != 0 {
		t.Errorf("expected no snapshots when autoSnapshot=false, got %d", len(idx.Snapshots))
	}
}

func TestRestoreWithBackupSnapshots(t *testing.T) {
	_, cc := newMockCaddy(t, `{"apps":{}}`)

	store := config.NewStore(&config.AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
	})

	snapDir := t.TempDir()
	ss := snapshot.NewStore(snapDir)

	backup := &Backup{
		Manifest:    validManifest("1.0.0"),
		CaddyConfig: json.RawMessage(`{"apps":{}}`),
		AppConfig: config.AppConfig{
			CaddyAdminURL: "http://localhost:2019",
			Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
		},
		Snapshots: &SnapshotData{
			Index: snapshot.Index{
				Snapshots: []snapshot.Snapshot{
					{ID: "imported-snap", Name: "backup snapshot", Type: "manual", CreatedAt: "2025-01-01T00:00:00Z"},
				},
			},
			Files: map[string]json.RawMessage{
				"imported-snap": json.RawMessage(`{"caddy_config":{"apps":{}}}`),
			},
		},
	}

	_, err := Restore(backup, cc, store, ss, false, "1.0.0")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	idx := ss.GetIndex()
	found := false
	for _, s := range idx.Snapshots {
		if s.ID == "imported-snap" {
			found = true
			break
		}
	}
	if !found {
		t.Error("imported snapshot should be present after restore")
	}

	data, err := os.ReadFile(filepath.Join(snapDir, "imported-snap.json"))
	if err != nil {
		t.Fatalf("reading imported snapshot file: %v", err)
	}
	if string(data) != `{"caddy_config":{"apps":{}}}` {
		t.Errorf("snapshot data = %s, want preserved content", data)
	}
}

func TestRestoreSnapshotFailureRollsBack(t *testing.T) {
	initialCaddyConfig := `{"apps":{}}`
	m, cc := newMockCaddy(t, initialCaddyConfig)

	originalAppConfig := &config.AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		LogFile:       "/original/log",
		Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
	}
	store := config.NewStore(originalAppConfig)

	// Use a path nested under a file (not a directory) so os.MkdirAll fails
	blocker := filepath.Join(t.TempDir(), "blocker")
	os.WriteFile(blocker, []byte("not a directory"), 0600)
	brokenSnapDir := filepath.Join(blocker, "snapshots", "deep")
	ss := snapshot.NewStore(brokenSnapDir)

	backup := &Backup{
		Manifest:    validManifest("1.0.0"),
		CaddyConfig: json.RawMessage(`{"apps":{"new":true}}`),
		AppConfig: config.AppConfig{
			CaddyAdminURL: "http://localhost:2019",
			Loki:          config.LokiConfig{BatchSize: 1048576, FlushIntervalSeconds: 5},
		},
		Snapshots: &SnapshotData{
			Index: snapshot.Index{
				Snapshots: []snapshot.Snapshot{
					{ID: "fail-snap", Name: "will fail", Type: "manual"},
				},
			},
			Files: map[string]json.RawMessage{
				"fail-snap": json.RawMessage(`{}`),
			},
		},
	}

	_, err := Restore(backup, cc, store, ss, false, "1.0.0")
	if err == nil {
		t.Fatal("expected error when snapshot restore fails")
	}
	if !strings.Contains(err.Error(), "restoring snapshots") {
		t.Errorf("error = %q, want 'restoring snapshots'", err)
	}

	// Caddy should be rolled back (2 loads: apply + rollback)
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.loadCalls) < 2 {
		t.Fatalf("expected at least 2 /load calls, got %d", len(m.loadCalls))
	}
	lastLoad := string(m.loadCalls[len(m.loadCalls)-1])
	if lastLoad != initialCaddyConfig {
		t.Errorf("rollback config = %s, want original", lastLoad)
	}
}

func TestParseZIPErrorMessages(t *testing.T) {
	validMfst := mustJSON(t, validManifest("1.0.0"))
	validCaddy := validCaddyConfig()
	validCfg := mustJSON(t, validAppConfig())

	tests := []struct {
		name    string
		files   map[string][]byte
		version string
		wantSub string
	}{
		{
			name: "missing manifest",
			files: map[string][]byte{
				"kaji-export/caddy.json":  validCaddy,
				"kaji-export/config.json": validCfg,
			},
			version: "1.0.0",
			wantSub: "missing manifest.json",
		},
		{
			name: "missing caddy.json",
			files: map[string][]byte{
				"kaji-export/manifest.json": validMfst,
				"kaji-export/config.json":   validCfg,
			},
			version: "1.0.0",
			wantSub: "missing caddy.json",
		},
		{
			name: "missing config.json",
			files: map[string][]byte{
				"kaji-export/manifest.json": validMfst,
				"kaji-export/caddy.json":    validCaddy,
			},
			version: "1.0.0",
			wantSub: "missing config.json",
		},
		{
			name: "unsupported manifest version",
			files: map[string][]byte{
				"kaji-export/manifest.json": mustJSON(t, Manifest{Version: 0, ExportedAt: "x", KajiVersion: "1.0.0"}),
				"kaji-export/caddy.json":    validCaddy,
				"kaji-export/config.json":   validCfg,
			},
			version: "1.0.0",
			wantSub: "unsupported manifest version",
		},
		{
			name: "malformed manifest JSON",
			files: map[string][]byte{
				"kaji-export/manifest.json": []byte(`{bad`),
				"kaji-export/caddy.json":    validCaddy,
				"kaji-export/config.json":   validCfg,
			},
			version: "1.0.0",
			wantSub: "parsing manifest.json",
		},
		{
			name: "newer backup version",
			files: map[string][]byte{
				"kaji-export/manifest.json": mustJSON(t, validManifest("2.0.0")),
				"kaji-export/caddy.json":    validCaddy,
				"kaji-export/config.json":   validCfg,
			},
			version: "1.0.0",
			wantSub: "upgrade before importing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := buildTestZIP(t, tt.files)
			_, err := ParseZIP(bytes.NewReader(data), int64(len(data)), tt.version)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func TestToSyncDomains(t *testing.T) {
	domains := []config.Domain{
		{
			ID:      "dom_1",
			Name:    "example.com",
			Enabled: true,
			Toggles: caddy.DomainToggles{
				ForceHTTPS:  true,
				Compression: true,
			},
			Rules: []config.Rule{
				{
					ID:            "rule_1",
					Label:         "api",
					Enabled:       true,
					MatchType:     "path",
					PathMatch:     "/api",
					MatchValue:    "",
					HandlerType:   "reverse_proxy",
					HandlerConfig: json.RawMessage(`{"upstream":"localhost:8080","tls_skip_verify":false}`),
					ToggleOverrides: &caddy.DomainToggles{
						ForceHTTPS: false,
					},
					AdvancedHeaders: false,
				},
				{
					ID:              "rule_2",
					Label:           "static",
					Enabled:         false,
					MatchType:       "path",
					PathMatch:       "/static",
					MatchValue:      "",
					HandlerType:     "file_server",
					HandlerConfig:   json.RawMessage(`{"root":"/var/www","browse":true}`),
					ToggleOverrides: nil,
					AdvancedHeaders: true,
				},
			},
		},
		{
			ID:      "dom_2",
			Name:    "test.local",
			Enabled: false,
			Toggles: caddy.DomainToggles{
				ForceHTTPS: false,
			},
			Rules: []config.Rule{},
		},
	}

	result := ToSyncDomains(domains)

	if len(result) != 2 {
		t.Fatalf("result length = %d, want 2", len(result))
	}

	// Check first domain
	dom0 := result[0]
	if dom0.Name != "example.com" {
		t.Errorf("domain[0].Name = %q, want example.com", dom0.Name)
	}
	if !dom0.Enabled {
		t.Error("domain[0].Enabled = false, want true")
	}
	if !dom0.Toggles.ForceHTTPS {
		t.Error("domain[0].Toggles.ForceHTTPS = false, want true")
	}
	if !dom0.Toggles.Compression {
		t.Error("domain[0].Toggles.Compression = false, want true")
	}

	if len(dom0.Rules) != 2 {
		t.Fatalf("domain[0].Rules length = %d, want 2", len(dom0.Rules))
	}

	// Check first rule
	rule0 := dom0.Rules[0]
	if rule0.RuleID != "rule_1" {
		t.Errorf("rule[0].RuleID = %q, want rule_1", rule0.RuleID)
	}
	if rule0.MatchType != "path" {
		t.Errorf("rule[0].MatchType = %q, want path", rule0.MatchType)
	}
	if rule0.PathMatch != "/api" {
		t.Errorf("rule[0].PathMatch = %q, want /api", rule0.PathMatch)
	}
	if rule0.HandlerType != "reverse_proxy" {
		t.Errorf("rule[0].HandlerType = %q, want reverse_proxy", rule0.HandlerType)
	}
	if !rule0.Enabled {
		t.Error("rule[0].Enabled = false, want true")
	}
	if rule0.ToggleOverrides == nil {
		t.Fatal("rule[0].ToggleOverrides should not be nil")
	}
	if rule0.ToggleOverrides.ForceHTTPS {
		t.Error("rule[0].ToggleOverrides.ForceHTTPS = true, want false")
	}

	// Check second rule
	rule1 := dom0.Rules[1]
	if rule1.RuleID != "rule_2" {
		t.Errorf("rule[1].RuleID = %q, want rule_2", rule1.RuleID)
	}
	if rule1.Enabled {
		t.Error("rule[1].Enabled = true, want false")
	}
	if !rule1.AdvancedHeaders {
		t.Error("rule[1].AdvancedHeaders = false, want true")
	}

	// Check second domain (disabled)
	dom1 := result[1]
	if dom1.Name != "test.local" {
		t.Errorf("domain[1].Name = %q, want test.local", dom1.Name)
	}
	if dom1.Enabled {
		t.Error("domain[1].Enabled = true, want false")
	}
	if len(dom1.Rules) != 0 {
		t.Errorf("domain[1].Rules length = %d, want 0", len(dom1.Rules))
	}
}
