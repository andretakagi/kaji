// App config: a flat JSON file at /etc/caddy-gui/config.json.
// ConfigStore wraps it for concurrent access.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

var ErrSetupDone = errors.New("setup already completed")

const fallbackConfigPath = "/etc/caddy-gui/config.json"

func Path() string {
	if p := os.Getenv("KAJI_CONFIG_PATH"); p != "" {
		return p
	}
	return fallbackConfigPath
}

type LokiConfig struct {
	Enabled              bool              `json:"enabled"`
	Endpoint             string            `json:"endpoint"`
	Labels               map[string]string `json:"labels"`
	BatchSize            int               `json:"batch_size"`
	FlushIntervalSeconds int               `json:"flush_interval_seconds"`
}

type DisabledRoute struct {
	ID         string          `json:"id"`
	Server     string          `json:"server"`
	DisabledAt string          `json:"disabled_at"`
	Route      json.RawMessage `json:"route"`
}

type AppConfig struct {
	AuthEnabled     bool            `json:"auth_enabled"`
	PasswordHash    string          `json:"password_hash"`
	SessionSecret   string          `json:"session_secret"`
	SessionMaxAge   int             `json:"session_max_age"`
	APIKeyHash      string          `json:"api_key_hash"`
	CaddyAdminURL   string          `json:"caddy_admin_url"`
	CaddyConfigPath string          `json:"caddy_config_path"`
	SecureCookies   string          `json:"secure_cookies"`
	LogFile         string          `json:"log_file"`
	Loki            LokiConfig      `json:"loki"`
	DisabledRoutes  []DisabledRoute `json:"disabled_routes"`
}

func DefaultConfig() *AppConfig {
	return &AppConfig{
		CaddyAdminURL:   "http://localhost:2019",
		CaddyConfigPath: "/etc/caddy/caddy.json",
		LogFile:         "/var/log/caddy/access.log",
		Loki: LokiConfig{
			BatchSize:            1048576,
			FlushIntervalSeconds: 5,
		},
	}
}

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

func Load() (*AppConfig, error) {
	return LoadFrom(Path())
}

func LoadFrom(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	return cfg, nil
}

func (c *AppConfig) validate() error {
	if c.CaddyAdminURL == "" {
		return fmt.Errorf("caddy_admin_url must not be empty")
	}
	if c.AuthEnabled && c.PasswordHash == "" {
		return fmt.Errorf("password_hash is required when auth is enabled")
	}
	if c.AuthEnabled && c.SessionSecret == "" {
		return fmt.Errorf("session_secret is required when auth is enabled")
	}
	if c.SessionMaxAge < 0 {
		return fmt.Errorf("session_max_age must not be negative")
	}
	if c.Loki.Enabled && c.Loki.Endpoint == "" {
		return fmt.Errorf("loki endpoint is required when loki is enabled")
	}
	if c.Loki.BatchSize <= 0 {
		return fmt.Errorf("loki batch_size must be positive")
	}
	if c.Loki.FlushIntervalSeconds <= 0 {
		return fmt.Errorf("loki flush_interval_seconds must be positive")
	}
	return nil
}

func Save(cfg *AppConfig) error {
	return SaveTo(cfg, Path())
}

func SaveTo(cfg *AppConfig, path string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// ConfigStore provides concurrent-safe access to AppConfig. Reads are lock-free
// via an atomic pointer. Writes are serialized with a mutex so that disk and
// in-memory state always stay in sync.
type ConfigStore struct {
	p  atomic.Pointer[AppConfig]
	mu sync.Mutex
}

func NewStore(initial *AppConfig) *ConfigStore {
	s := &ConfigStore{}
	s.p.Store(initial)
	return s
}

func (s *ConfigStore) Get() *AppConfig {
	return s.p.Load()
}

// Update applies fn to a value copy of the current config, saves the result
// to disk, then swaps the in-memory pointer. The mutex ensures only one
// writer runs at a time, so disk and memory can never desync.
func (s *ConfigStore) Update(fn func(current AppConfig) (*AppConfig, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.p.Load()
	next, err := fn(*old)
	if err != nil {
		return fmt.Errorf("applying config update: %w", err)
	}
	if err := Save(next); err != nil {
		return fmt.Errorf("saving updated config: %w", err)
	}
	s.p.Store(next)
	return nil
}
