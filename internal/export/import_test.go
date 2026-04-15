package export

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
