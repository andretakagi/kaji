package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestExistsTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KAJI_CONFIG_PATH", path)

	if !Exists() {
		t.Error("Exists() = false, want true when config file exists")
	}
}

func TestExistsFalse(t *testing.T) {
	t.Setenv("KAJI_CONFIG_PATH", "/nonexistent/path/config.json")

	if Exists() {
		t.Error("Exists() = true, want false when config file does not exist")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.CaddyAdminURL != "http://localhost:2019" {
		t.Errorf("CaddyAdminURL = %q, want %q", cfg.CaddyAdminURL, "http://localhost:2019")
	}
	if cfg.CaddyConfigPath != "/etc/caddy/caddy.json" {
		t.Errorf("CaddyConfigPath = %q, want %q", cfg.CaddyConfigPath, "/etc/caddy/caddy.json")
	}
	if cfg.LogFile != "/var/log/caddy/access.log" {
		t.Errorf("LogFile = %q, want %q", cfg.LogFile, "/var/log/caddy/access.log")
	}
	if cfg.Loki.BatchSize != 1048576 {
		t.Errorf("Loki.BatchSize = %d, want 1048576", cfg.Loki.BatchSize)
	}
	if cfg.Loki.FlushIntervalSeconds != 5 {
		t.Errorf("Loki.FlushIntervalSeconds = %d, want 5", cfg.Loki.FlushIntervalSeconds)
	}
	if cfg.AuthEnabled {
		t.Error("AuthEnabled should default to false")
	}
	if cfg.Loki.Enabled {
		t.Error("Loki.Enabled should default to false")
	}
}

func TestLoadFromSaveToRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := &AppConfig{
		AuthEnabled:     true,
		PasswordHash:    "somehash",
		SessionSecret:   "supersecret",
		SessionMaxAge:   3600,
		APIKeyHash:      "apihash",
		CaddyAdminURL:   "http://localhost:2019",
		CaddyConfigPath: "/etc/caddy/caddy.json",
		SecureCookies:   "auto",
		LogFile:         "/var/log/caddy/access.log",
		Loki: LokiConfig{
			Enabled:              true,
			Endpoint:             "http://loki:3100",
			Labels:               map[string]string{"app": "kaji"},
			BatchSize:            512000,
			FlushIntervalSeconds: 10,
		},
		DisabledDomains: []DisabledDomain{
			{
				ID:         "route-1",
				Server:     "srv0",
				DisabledAt: "2024-01-01T00:00:00Z",
				Route:      json.RawMessage(`{"handle":[]}`),
			},
		},
	}

	if err := SaveTo(original, path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if loaded.AuthEnabled != original.AuthEnabled {
		t.Errorf("AuthEnabled = %v, want %v", loaded.AuthEnabled, original.AuthEnabled)
	}
	if loaded.PasswordHash != original.PasswordHash {
		t.Errorf("PasswordHash = %q, want %q", loaded.PasswordHash, original.PasswordHash)
	}
	if loaded.SessionSecret != original.SessionSecret {
		t.Errorf("SessionSecret = %q, want %q", loaded.SessionSecret, original.SessionSecret)
	}
	if loaded.SessionMaxAge != original.SessionMaxAge {
		t.Errorf("SessionMaxAge = %d, want %d", loaded.SessionMaxAge, original.SessionMaxAge)
	}
	if loaded.APIKeyHash != original.APIKeyHash {
		t.Errorf("APIKeyHash = %q, want %q", loaded.APIKeyHash, original.APIKeyHash)
	}
	if loaded.CaddyAdminURL != original.CaddyAdminURL {
		t.Errorf("CaddyAdminURL = %q, want %q", loaded.CaddyAdminURL, original.CaddyAdminURL)
	}
	if loaded.CaddyConfigPath != original.CaddyConfigPath {
		t.Errorf("CaddyConfigPath = %q, want %q", loaded.CaddyConfigPath, original.CaddyConfigPath)
	}
	if loaded.SecureCookies != original.SecureCookies {
		t.Errorf("SecureCookies = %q, want %q", loaded.SecureCookies, original.SecureCookies)
	}
	if loaded.LogFile != original.LogFile {
		t.Errorf("LogFile = %q, want %q", loaded.LogFile, original.LogFile)
	}
	if loaded.Loki.Enabled != original.Loki.Enabled {
		t.Errorf("Loki.Enabled = %v, want %v", loaded.Loki.Enabled, original.Loki.Enabled)
	}
	if loaded.Loki.Endpoint != original.Loki.Endpoint {
		t.Errorf("Loki.Endpoint = %q, want %q", loaded.Loki.Endpoint, original.Loki.Endpoint)
	}
	if loaded.Loki.BatchSize != original.Loki.BatchSize {
		t.Errorf("Loki.BatchSize = %d, want %d", loaded.Loki.BatchSize, original.Loki.BatchSize)
	}
	if loaded.Loki.FlushIntervalSeconds != original.Loki.FlushIntervalSeconds {
		t.Errorf("Loki.FlushIntervalSeconds = %d, want %d", loaded.Loki.FlushIntervalSeconds, original.Loki.FlushIntervalSeconds)
	}
	if loaded.Loki.Labels["app"] != original.Loki.Labels["app"] {
		t.Errorf("Loki.Labels[app] = %q, want %q", loaded.Loki.Labels["app"], original.Loki.Labels["app"])
	}
	if len(loaded.DisabledDomains) != 1 {
		t.Fatalf("DisabledDomains len = %d, want 1", len(loaded.DisabledDomains))
	}
	if loaded.DisabledDomains[0].ID != original.DisabledDomains[0].ID {
		t.Errorf("DisabledDomains[0].ID = %q, want %q", loaded.DisabledDomains[0].ID, original.DisabledDomains[0].ID)
	}
}

func TestValidateEmptyCaddyAdminURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CaddyAdminURL = ""

	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for empty CaddyAdminURL, got nil")
	}
	if !strings.Contains(err.Error(), "caddy_admin_url") {
		t.Errorf("error %q should mention caddy_admin_url", err.Error())
	}
}

func TestValidateAuthEnabledWithoutHash(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthEnabled = true
	cfg.PasswordHash = ""
	cfg.SessionSecret = "secret"

	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for auth enabled without password hash, got nil")
	}
	if !strings.Contains(err.Error(), "password_hash") {
		t.Errorf("error %q should mention password_hash", err.Error())
	}
}

func TestValidateAuthEnabledWithoutSecret(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthEnabled = true
	cfg.PasswordHash = "hash"
	cfg.SessionSecret = ""

	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for auth enabled without session secret, got nil")
	}
	if !strings.Contains(err.Error(), "session_secret") {
		t.Errorf("error %q should mention session_secret", err.Error())
	}
}

func TestValidateNegativeSessionMaxAge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxAge = -1

	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for negative session_max_age, got nil")
	}
	if !strings.Contains(err.Error(), "session_max_age") {
		t.Errorf("error %q should mention session_max_age", err.Error())
	}
}

func TestValidateLokiEnabledWithoutEndpoint(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Loki.Enabled = true
	cfg.Loki.Endpoint = ""

	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for loki enabled without endpoint, got nil")
	}
	if !strings.Contains(err.Error(), "loki endpoint") {
		t.Errorf("error %q should mention loki endpoint", err.Error())
	}
}

func TestValidateLokiBadBatchSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Loki.BatchSize = 0

	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for loki batch_size <= 0, got nil")
	}
	if !strings.Contains(err.Error(), "batch_size") {
		t.Errorf("error %q should mention batch_size", err.Error())
	}
}

func TestValidateLokiBadFlushInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Loki.FlushIntervalSeconds = 0

	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for loki flush_interval_seconds <= 0, got nil")
	}
	if !strings.Contains(err.Error(), "flush_interval_seconds") {
		t.Errorf("error %q should mention flush_interval_seconds", err.Error())
	}
}

func TestValidateValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.validate(); err != nil {
		t.Errorf("valid default config failed validation: %v", err)
	}

	full := &AppConfig{
		AuthEnabled:     true,
		PasswordHash:    "hash",
		SessionSecret:   "secret",
		SessionMaxAge:   86400,
		CaddyAdminURL:   "http://localhost:2019",
		CaddyConfigPath: "/etc/caddy/caddy.json",
		Loki: LokiConfig{
			Enabled:              true,
			Endpoint:             "http://loki:3100",
			BatchSize:            1024,
			FlushIntervalSeconds: 1,
		},
	}
	if err := full.validate(); err != nil {
		t.Errorf("valid full config failed validation: %v", err)
	}
}

func TestLoadFromMissingFile(t *testing.T) {
	_, err := LoadFrom("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadFromInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoadFromInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	bad := `{"caddy_admin_url": "", "loki": {"batch_size": 100, "flush_interval_seconds": 5}}`
	if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for config that fails validation, got nil")
	}
	if !strings.Contains(err.Error(), "config validation") {
		t.Errorf("error %q should mention config validation", err.Error())
	}
}

func TestConfigStoreConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initial := DefaultConfig()
	store := NewStoreWithPath(initial, path)

	if err := SaveTo(initial, path); err != nil {
		t.Fatalf("seeding config file: %v", err)
	}

	const goroutines = 20
	var wg sync.WaitGroup

	// Concurrent readers.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := store.Get()
			if cfg == nil {
				t.Errorf("Get returned nil")
				return
			}
			if cfg.CaddyAdminURL != "http://localhost:2019" {
				t.Errorf("Get returned unexpected CaddyAdminURL = %q", cfg.CaddyAdminURL)
			}
		}()
	}

	// Concurrent writers.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := store.Update(func(current AppConfig) (*AppConfig, error) {
				next := current
				next.LogFile = "/tmp/caddy.log"
				return &next, nil
			})
			if err != nil {
				t.Errorf("Update: %v", err)
			}
		}()
	}

	wg.Wait()

	cfg := store.Get()
	if cfg == nil {
		t.Fatal("store.Get() returned nil after concurrent access")
	}
	if cfg.LogFile != "/tmp/caddy.log" {
		t.Errorf("LogFile = %q after concurrent writes, want /tmp/caddy.log", cfg.LogFile)
	}
}

func TestConfigStoreGetReturnsCurrentValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initial := DefaultConfig()
	store := NewStoreWithPath(initial, path)

	if err := SaveTo(initial, path); err != nil {
		t.Fatalf("seeding config file: %v", err)
	}

	err := store.Update(func(current AppConfig) (*AppConfig, error) {
		next := current
		next.LogFile = "/updated/log"
		return &next, nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	cfg := store.Get()
	if cfg.LogFile != "/updated/log" {
		t.Errorf("LogFile = %q after update, want /updated/log", cfg.LogFile)
	}
}

func TestPathEnvVar(t *testing.T) {
	t.Setenv("KAJI_CONFIG_PATH", "/tmp/test-config.json")
	if got := ConfigPath(); got != "/tmp/test-config.json" {
		t.Errorf("ConfigPath() = %q, want /tmp/test-config.json", got)
	}
}

func TestPathFallback(t *testing.T) {
	t.Setenv("KAJI_CONFIG_PATH", "")
	if got := ConfigPath(); got != fallbackConfigPath {
		t.Errorf("ConfigPath() = %q, want %q", got, fallbackConfigPath)
	}
}

func TestUpdateFnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initial := DefaultConfig()
	if err := SaveTo(initial, path); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	store := NewStoreWithPath(initial, path)

	wantErr := "something broke"
	err := store.Update(func(current AppConfig) (*AppConfig, error) {
		return nil, fmt.Errorf("%s", wantErr)
	})
	if err == nil {
		t.Fatal("expected error from Update, got nil")
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("error %q should contain %q", err.Error(), wantErr)
	}

	cfg := store.Get()
	if cfg.LogFile != initial.LogFile {
		t.Errorf("config mutated after failed Update: LogFile = %q, want %q", cfg.LogFile, initial.LogFile)
	}
}

func TestUpdateInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initial := DefaultConfig()
	if err := SaveTo(initial, path); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	store := NewStoreWithPath(initial, path)

	err := store.Update(func(current AppConfig) (*AppConfig, error) {
		next := current
		next.CaddyAdminURL = ""
		return &next, nil
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "config validation") {
		t.Errorf("error %q should mention config validation", err.Error())
	}

	cfg := store.Get()
	if cfg.CaddyAdminURL != initial.CaddyAdminURL {
		t.Errorf("config mutated after invalid Update: CaddyAdminURL = %q, want %q", cfg.CaddyAdminURL, initial.CaddyAdminURL)
	}
}

func TestUpdateDiskPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initial := DefaultConfig()
	if err := SaveTo(initial, path); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	store := NewStoreWithPath(initial, path)

	err := store.Update(func(current AppConfig) (*AppConfig, error) {
		next := current
		next.LogFile = "/updated/access.log"
		return &next, nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom after Update: %v", err)
	}
	if loaded.LogFile != "/updated/access.log" {
		t.Errorf("persisted LogFile = %q, want /updated/access.log", loaded.LogFile)
	}
}

func TestLoadFromMergesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	partial := `{"caddy_admin_url": "http://localhost:2019"}`
	if err := os.WriteFile(path, []byte(partial), 0600); err != nil {
		t.Fatalf("writing partial config: %v", err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	defaults := DefaultConfig()
	if cfg.LogFile != defaults.LogFile {
		t.Errorf("LogFile = %q, want default %q", cfg.LogFile, defaults.LogFile)
	}
	if cfg.CaddyConfigPath != defaults.CaddyConfigPath {
		t.Errorf("CaddyConfigPath = %q, want default %q", cfg.CaddyConfigPath, defaults.CaddyConfigPath)
	}
	if cfg.Loki.BatchSize != defaults.Loki.BatchSize {
		t.Errorf("Loki.BatchSize = %d, want default %d", cfg.Loki.BatchSize, defaults.Loki.BatchSize)
	}
	if cfg.Loki.FlushIntervalSeconds != defaults.Loki.FlushIntervalSeconds {
		t.Errorf("Loki.FlushIntervalSeconds = %d, want default %d", cfg.Loki.FlushIntervalSeconds, defaults.Loki.FlushIntervalSeconds)
	}
}

func TestDefaultConfigCaddyLogDir(t *testing.T) {
	t.Setenv("CADDY_LOG_DIR", "/custom/logs")
	cfg := DefaultConfig()
	if cfg.LogDir != "/custom/logs/" {
		t.Errorf("LogDir = %q, want /custom/logs/ (trailing slash added)", cfg.LogDir)
	}

	t.Setenv("CADDY_LOG_DIR", "/already/trailing/")
	cfg = DefaultConfig()
	if cfg.LogDir != "/already/trailing/" {
		t.Errorf("LogDir = %q, want /already/trailing/", cfg.LogDir)
	}

	if cfg.LogFile != "/var/log/caddy/access.log" {
		t.Errorf("LogFile = %q, want /var/log/caddy/access.log (env should not affect LogFile)", cfg.LogFile)
	}
}

func TestStripCredentials(t *testing.T) {
	cfg := &AppConfig{
		PasswordHash:  "hash123",
		SessionSecret: "secret456",
		SessionMaxAge: 3600,
		SecureCookies: "always",
		APIKeyHash:    "apikey789",
		CaddyAdminURL: "http://localhost:2019",
		LogFile:       "/var/log/caddy/access.log",
	}

	cfg.StripCredentials()

	if cfg.PasswordHash != "" {
		t.Errorf("PasswordHash = %q, want empty", cfg.PasswordHash)
	}
	if cfg.SessionSecret != "" {
		t.Errorf("SessionSecret = %q, want empty", cfg.SessionSecret)
	}
	if cfg.SessionMaxAge != 0 {
		t.Errorf("SessionMaxAge = %d, want 0", cfg.SessionMaxAge)
	}
	if cfg.SecureCookies != "" {
		t.Errorf("SecureCookies = %q, want empty", cfg.SecureCookies)
	}
	if cfg.APIKeyHash != "" {
		t.Errorf("APIKeyHash = %q, want empty", cfg.APIKeyHash)
	}
	if cfg.CaddyAdminURL != "http://localhost:2019" {
		t.Error("StripCredentials should not modify non-credential fields")
	}
	if cfg.LogFile != "/var/log/caddy/access.log" {
		t.Error("StripCredentials should not modify non-credential fields")
	}
}

func TestPreserveCredentials(t *testing.T) {
	imported := &AppConfig{
		CaddyAdminURL: "http://localhost:2019",
		PasswordHash:  "",
		SessionSecret: "",
		SessionMaxAge: 0,
		SecureCookies: "",
		APIKeyHash:    "",
		LogFile:       "/imported/log",
	}

	current := &AppConfig{
		PasswordHash:  "current-hash",
		SessionSecret: "current-secret",
		SessionMaxAge: 7200,
		SecureCookies: "never",
		APIKeyHash:    "current-apikey",
		LogFile:       "/current/log",
	}

	imported.PreserveCredentials(current)

	if imported.PasswordHash != "current-hash" {
		t.Errorf("PasswordHash = %q, want %q", imported.PasswordHash, "current-hash")
	}
	if imported.SessionSecret != "current-secret" {
		t.Errorf("SessionSecret = %q, want %q", imported.SessionSecret, "current-secret")
	}
	if imported.SessionMaxAge != 7200 {
		t.Errorf("SessionMaxAge = %d, want 7200", imported.SessionMaxAge)
	}
	if imported.SecureCookies != "never" {
		t.Errorf("SecureCookies = %q, want %q", imported.SecureCookies, "never")
	}
	if imported.APIKeyHash != "current-apikey" {
		t.Errorf("APIKeyHash = %q, want %q", imported.APIKeyHash, "current-apikey")
	}
	if imported.LogFile != "/imported/log" {
		t.Error("PreserveCredentials should not modify non-credential fields")
	}
}

func TestNewStoreNoDiskWrite(t *testing.T) {
	dir := t.TempDir()
	phantom := filepath.Join(dir, "should-not-exist.json")

	initial := DefaultConfig()
	store := NewStore(initial)

	err := store.Update(func(current AppConfig) (*AppConfig, error) {
		next := current
		next.LogFile = "/changed"
		return &next, nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if store.Get().LogFile != "/changed" {
		t.Errorf("in-memory LogFile = %q, want /changed", store.Get().LogFile)
	}

	if _, err := os.Stat(phantom); err == nil {
		t.Error("NewStore should not write to disk, but file exists")
	}
}
