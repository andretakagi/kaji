package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDataEnvelopeFormat(t *testing.T) {
	envelope := `{
		"kaji_version": "1.5.0",
		"caddy_config": {"apps": {}},
		"app_config": {"auth_enabled": true}
	}`

	d, err := ParseData([]byte(envelope))
	if err != nil {
		t.Fatalf("ParseData: %v", err)
	}
	if d.KajiVersion != "1.5.0" {
		t.Errorf("KajiVersion = %q, want 1.5.0", d.KajiVersion)
	}
	if d.CaddyConfig == nil {
		t.Fatal("CaddyConfig should not be nil")
	}
	if d.AppConfig == nil {
		t.Fatal("AppConfig should not be nil")
	}

	var caddy map[string]any
	json.Unmarshal(d.CaddyConfig, &caddy)
	if _, ok := caddy["apps"]; !ok {
		t.Error("CaddyConfig should contain apps key")
	}

	var app map[string]any
	json.Unmarshal(d.AppConfig, &app)
	if app["auth_enabled"] != true {
		t.Error("AppConfig should contain auth_enabled: true")
	}
}

func TestParseDataLegacyFormat(t *testing.T) {
	legacy := `{"apps":{"http":{"servers":{}}}}`

	d, err := ParseData([]byte(legacy))
	if err != nil {
		t.Fatalf("ParseData: %v", err)
	}
	if d.KajiVersion != "" {
		t.Errorf("KajiVersion = %q, want empty for legacy", d.KajiVersion)
	}
	if d.AppConfig != nil {
		t.Errorf("AppConfig = %s, want nil for legacy", d.AppConfig)
	}
	if d.CaddyConfig == nil {
		t.Fatal("CaddyConfig should not be nil")
	}

	var caddy map[string]any
	json.Unmarshal(d.CaddyConfig, &caddy)
	if _, ok := caddy["apps"]; !ok {
		t.Error("legacy CaddyConfig should contain apps key")
	}
}

func TestParseDataEnvelopeWithoutAppConfig(t *testing.T) {
	envelope := `{"kaji_version": "1.5.0", "caddy_config": {"apps": {}}}`

	d, err := ParseData([]byte(envelope))
	if err != nil {
		t.Fatalf("ParseData: %v", err)
	}
	if d.KajiVersion != "1.5.0" {
		t.Errorf("KajiVersion = %q, want 1.5.0", d.KajiVersion)
	}
	if d.AppConfig != nil {
		t.Errorf("AppConfig should be nil when omitted")
	}
}

func TestParseDataInvalidJSON(t *testing.T) {
	_, err := ParseData([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadDataLegacyFileOnDisk(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Simulate a legacy snapshot by writing raw Caddy JSON directly to disk
	// and manually adding an index entry (bypassing Create which now writes envelopes).
	legacyConfig := []byte(`{"apps":{"http":{"servers":{"srv0":{"routes":[]}}}}}`)
	id := "legacy-snap-001"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), legacyConfig, 0600); err != nil {
		t.Fatal(err)
	}
	s.ReplaceIndex(Index{
		CurrentID: id,
		Snapshots: []Snapshot{
			{ID: id, Name: "legacy", Type: "manual", CreatedAt: "2025-01-01T00:00:00Z"},
		},
	})

	d, err := s.ReadData(id)
	if err != nil {
		t.Fatalf("ReadData: %v", err)
	}
	if d.KajiVersion != "" {
		t.Errorf("KajiVersion = %q, want empty for legacy", d.KajiVersion)
	}
	if d.AppConfig != nil {
		t.Error("AppConfig should be nil for legacy snapshot")
	}

	var caddy map[string]any
	json.Unmarshal(d.CaddyConfig, &caddy)
	if _, ok := caddy["apps"]; !ok {
		t.Error("CaddyConfig should preserve the original Caddy JSON")
	}
}

func TestCreateFullStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	caddyCfg := json.RawMessage(`{"apps":{"http":{}}}`)
	appCfg := json.RawMessage(`{"auth_enabled":true,"caddy_admin_url":"http://localhost:2019"}`)

	data := &Data{
		KajiVersion: "1.5.0",
		CaddyConfig: caddyCfg,
		AppConfig:   appCfg,
	}

	snap, err := s.Create("full-state", "test snapshot", "manual", data)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if snap.KajiVersion != "1.5.0" {
		t.Errorf("snap.KajiVersion = %q, want 1.5.0", snap.KajiVersion)
	}

	got, err := s.ReadData(snap.ID)
	if err != nil {
		t.Fatalf("ReadData: %v", err)
	}
	if got.KajiVersion != "1.5.0" {
		t.Errorf("ReadData KajiVersion = %q, want 1.5.0", got.KajiVersion)
	}

	var gotApp map[string]any
	json.Unmarshal(got.AppConfig, &gotApp)
	if gotApp["auth_enabled"] != true {
		t.Error("AppConfig auth_enabled should be true")
	}
	if gotApp["caddy_admin_url"] != "http://localhost:2019" {
		t.Errorf("caddy_admin_url = %v, want http://localhost:2019", gotApp["caddy_admin_url"])
	}

	var gotCaddy map[string]any
	json.Unmarshal(got.CaddyConfig, &gotCaddy)
	if _, ok := gotCaddy["apps"]; !ok {
		t.Error("CaddyConfig should contain apps key")
	}
}

func TestReadConfigStillReturnsRawBytes(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	data := &Data{
		KajiVersion: "1.5.0",
		CaddyConfig: json.RawMessage(`{"apps":{}}`),
		AppConfig:   json.RawMessage(`{"auth_enabled":false}`),
	}

	snap, err := s.Create("raw-test", "", "manual", data)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	raw, err := s.ReadConfig(snap.ID)
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	// ReadConfig returns the full envelope as raw bytes.
	// Verify it's valid JSON containing the envelope structure.
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("raw bytes are not valid JSON: %v", err)
	}
	if _, ok := envelope["kaji_version"]; !ok {
		t.Error("raw bytes should contain kaji_version key")
	}
	if _, ok := envelope["caddy_config"]; !ok {
		t.Error("raw bytes should contain caddy_config key")
	}
}
