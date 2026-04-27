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

	"github.com/andretakagi/kaji/internal/caddy"
)

var ErrSetupDone = errors.New("setup already completed")

const fallbackConfigPath = "/etc/caddy-gui/config.json"

func ConfigPath() string {
	if p := os.Getenv("KAJI_CONFIG_PATH"); p != "" {
		return p
	}
	return fallbackConfigPath
}

type LokiConfig struct {
	Enabled              bool              `json:"enabled"`
	Endpoint             string            `json:"endpoint"`
	BearerToken          string            `json:"bearer_token"`
	TenantID             string            `json:"tenant_id"`
	Labels               map[string]string `json:"labels"`
	BatchSize            int               `json:"batch_size"`
	FlushIntervalSeconds int               `json:"flush_interval_seconds"`
	Sinks                []string          `json:"sinks"`
}

type IPList struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	IPs         []string `json:"ips"`
	Children    []string `json:"children"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type Rule struct {
	HandlerType     string          `json:"handler_type"`
	HandlerConfig   json.RawMessage `json:"handler_config"`
	AdvancedHeaders bool            `json:"advanced_headers,omitempty"`
}

type Path struct {
	ID              string               `json:"id"`
	Label           string               `json:"label,omitempty"`
	Enabled         bool                 `json:"enabled"`
	PathMatch       string               `json:"path_match"`
	MatchValue      string               `json:"match_value"`
	Rule            Rule                 `json:"rule"`
	ToggleOverrides *caddy.DomainToggles `json:"toggle_overrides,omitempty"`
}

type Subdomain struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Enabled bool                `json:"enabled"`
	Toggles caddy.DomainToggles `json:"toggles"`
	Rule    Rule                `json:"rule"`
	Paths   []Path              `json:"paths"`
}

type Domain struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Enabled    bool                `json:"enabled"`
	Toggles    caddy.DomainToggles `json:"toggles"`
	Rule       Rule                `json:"rule"`
	Subdomains []Subdomain         `json:"subdomains"`
	Paths      []Path              `json:"paths"`
}

func IPListsToEntries(lists []IPList) []caddy.IPListEntry {
	entries := make([]caddy.IPListEntry, len(lists))
	for i, l := range lists {
		entries[i] = caddy.IPListEntry{
			ID:       l.ID,
			IPs:      l.IPs,
			Children: l.Children,
		}
	}
	return entries
}

type AppConfig struct {
	AuthEnabled     bool              `json:"auth_enabled"`
	PasswordHash    string            `json:"password_hash"`
	SessionSecret   string            `json:"session_secret"`
	SessionMaxAge   int               `json:"session_max_age"`
	APIKeyHash      string            `json:"api_key_hash"`
	CaddyAdminURL   string            `json:"caddy_admin_url"`
	CaddyConfigPath string            `json:"caddy_config_path"`
	CaddyDataDir    string            `json:"caddy_data_dir"`
	SecureCookies   string            `json:"secure_cookies"`
	LogFile         string            `json:"log_file"`
	LogDir          string            `json:"log_dir"`
	Loki            LokiConfig        `json:"loki"`
	Domains         []Domain          `json:"domains"`
	KajiVersion     string            `json:"kaji_version,omitempty"`
	IPLists         []IPList          `json:"ip_lists"`
	DomainIPLists   map[string]string `json:"domain_ip_lists"`
}

func (c *AppConfig) StripCredentials() {
	c.PasswordHash = ""
	c.SessionSecret = ""
	c.SessionMaxAge = 0
	c.SecureCookies = ""
	c.APIKeyHash = ""
}

func (c *AppConfig) PreserveCredentials(from *AppConfig) {
	c.PasswordHash = from.PasswordHash
	c.SessionSecret = from.SessionSecret
	c.SessionMaxAge = from.SessionMaxAge
	c.SecureCookies = from.SecureCookies
	c.APIKeyHash = from.APIKeyHash
}

func DefaultConfig() *AppConfig {
	logDir := "/var/log/caddy/"
	if v := os.Getenv("CADDY_LOG_DIR"); v != "" {
		logDir = v
		if logDir[len(logDir)-1] != '/' {
			logDir += "/"
		}
	}
	return &AppConfig{
		CaddyAdminURL:   "http://localhost:2019",
		CaddyConfigPath: "/etc/caddy/caddy.json",
		LogFile:         "/var/log/caddy/access.log",
		LogDir:          logDir,
		Loki: LokiConfig{
			Endpoint:             "http://loki:3100",
			BatchSize:            1048576,
			FlushIntervalSeconds: 5,
			Labels:               map[string]string{"job": "kaji"},
		},
	}
}

func Exists() bool {
	_, err := os.Stat(ConfigPath())
	return err == nil
}

func Load() (*AppConfig, error) {
	return LoadFrom(ConfigPath())
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
	if v := os.Getenv("CADDY_LOG_DIR"); v != "" {
		if v[len(v)-1] != '/' {
			v += "/"
		}
		cfg.LogDir = v
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	cfg.NormalizeSlices()
	return cfg, nil
}

func (c *AppConfig) NormalizeSlices() {
	for i := range c.Domains {
		if c.Domains[i].Subdomains == nil {
			c.Domains[i].Subdomains = []Subdomain{}
		}
		if c.Domains[i].Paths == nil {
			c.Domains[i].Paths = []Path{}
		}
		for j := range c.Domains[i].Subdomains {
			if c.Domains[i].Subdomains[j].Paths == nil {
				c.Domains[i].Subdomains[j].Paths = []Path{}
			}
		}
	}
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
	return SaveTo(cfg, ConfigPath())
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
	p    atomic.Pointer[AppConfig]
	mu   sync.Mutex
	path string
}

// NewStore creates an in-memory config store with no disk persistence.
// Useful in tests where you don't want disk writes.
func NewStore(initial *AppConfig) *ConfigStore {
	s := &ConfigStore{}
	s.p.Store(initial)
	return s
}

// NewStoreWithPath creates a config store that persists changes to the given path.
func NewStoreWithPath(initial *AppConfig, path string) *ConfigStore {
	s := &ConfigStore{path: path}
	s.p.Store(initial)
	return s
}

func (s *ConfigStore) Get() *AppConfig {
	return s.p.Load()
}

// Update applies fn to a value copy of the current config, validates the result,
// persists to disk (if the store has a path), then swaps the in-memory pointer.
// The mutex ensures only one writer runs at a time.
func (s *ConfigStore) Update(fn func(current AppConfig) (*AppConfig, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.p.Load()
	next, err := fn(*old)
	if err != nil {
		return fmt.Errorf("applying config update: %w", err)
	}
	if err := next.validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}
	next.NormalizeSlices()
	if s.path != "" {
		if err := SaveTo(next, s.path); err != nil {
			return fmt.Errorf("saving updated config: %w", err)
		}
	}
	s.p.Store(next)
	return nil
}
