package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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
		DisabledRoutes: []DisabledRoute{
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
	if len(loaded.DisabledRoutes) != 1 {
		t.Fatalf("DisabledRoutes len = %d, want 1", len(loaded.DisabledRoutes))
	}
	if loaded.DisabledRoutes[0].ID != original.DisabledRoutes[0].ID {
		t.Errorf("DisabledRoutes[0].ID = %q, want %q", loaded.DisabledRoutes[0].ID, original.DisabledRoutes[0].ID)
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
	t.Setenv("KAJI_CONFIG_PATH", path)

	initial := DefaultConfig()
	store := NewStore(initial)

	// Pre-populate the file so Update can save to it.
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
			}
		}()
	}

	// Concurrent writers.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.Update(func(current AppConfig) (*AppConfig, error) {
				next := current
				next.LogFile = "/tmp/caddy.log"
				return &next, nil
			})
		}()
	}

	wg.Wait()

	cfg := store.Get()
	if cfg == nil {
		t.Fatal("store.Get() returned nil after concurrent access")
	}
}

func TestConfigStoreGetReturnsCurrentValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("KAJI_CONFIG_PATH", path)

	initial := DefaultConfig()
	store := NewStore(initial)

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
	if got := Path(); got != "/tmp/test-config.json" {
		t.Errorf("Path() = %q, want /tmp/test-config.json", got)
	}
}

func TestPathFallback(t *testing.T) {
	t.Setenv("KAJI_CONFIG_PATH", "")
	if got := Path(); got != fallbackConfigPath {
		t.Errorf("Path() = %q, want %q", got, fallbackConfigPath)
	}
}
