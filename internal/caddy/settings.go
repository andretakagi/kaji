// Global settings: auto-HTTPS, metrics, ACME email.
package caddy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type GlobalToggles struct {
	AutoHTTPS         string `json:"auto_https"`
	PrometheusMetrics bool   `json:"prometheus_metrics"`
	PerHostMetrics    bool   `json:"per_host_metrics"`
}

func (c *Client) SetConfigPath(path string, data []byte) error {
	if err := c.doRequest(http.MethodPost, c.url()+"/config/"+path, "application/json", data); err != nil {
		return fmt.Errorf("setting config path %q: %w", path, err)
	}
	return nil
}

// PatchConfigPath replaces a JSON value at a Caddy config path via PATCH.
// Unlike POST, PATCH replaces arrays instead of appending to them.
func (c *Client) PatchConfigPath(path string, data []byte) error {
	if err := c.doRequest(http.MethodPatch, c.url()+"/config/"+path, "application/json", data); err != nil {
		return fmt.Errorf("patching config path %q: %w", path, err)
	}
	return nil
}

func (c *Client) DeleteConfigPath(path string) error {
	if err := c.doRequest(http.MethodDelete, c.url()+"/config/"+path, "", nil); err != nil {
		return fmt.Errorf("deleting config path %q: %w", path, err)
	}
	return nil
}

// SetConfigCascade writes a value at the given config path, creating parent
// structure as needed. If setting at the target path fails (because a parent
// doesn't exist yet), it wraps the value in each successive parent key and
// retries one level up until a write succeeds.
func (c *Client) SetConfigCascade(path string, value any) error {
	segments := strings.Split(path, "/")

	var lastErr error
	for i := len(segments); i >= 1; i-- {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal config for %q: %w", path, err)
		}
		p := strings.Join(segments[:i], "/")
		if lastErr = c.SetConfigPath(p, data); lastErr == nil {
			return nil
		}
		value = map[string]any{segments[i-1]: value}
	}

	return fmt.Errorf("failed to set config at any level of %q: %w", path, lastErr)
}

// EnsureConfigPath makes sure a config path exists without overwriting any
// existing value. If the path is missing, it cascades upward creating empty
// objects at each level.
func (c *Client) EnsureConfigPath(path string) error {
	if _, err := c.GetConfigPath(path); err == nil {
		return nil
	}
	return c.SetConfigCascade(path, map[string]any{})
}

func (c *Client) GetGlobalToggles() (*GlobalToggles, error) {
	raw, err := c.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("fetching config for global toggles: %w", err)
	}

	var cfg struct {
		caddyConfigPartial
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse caddy config: %w", err)
	}

	t := &GlobalToggles{
		AutoHTTPS: "on",
	}

	for _, srv := range cfg.Apps.HTTP.Servers {
		if srv.AutoHTTPS != nil {
			if srv.AutoHTTPS.Disable {
				t.AutoHTTPS = "off"
			} else if srv.AutoHTTPS.DisableRedirects {
				t.AutoHTTPS = "disable_redirects"
			}
		}
		if srv.Metrics != nil {
			t.PrometheusMetrics = true
			t.PerHostMetrics = srv.Metrics.PerHost
		}
		break
	}

	return t, nil
}

func (c *Client) SetGlobalToggles(t *GlobalToggles) error {
	raw, err := c.GetConfigPath("apps/http/servers")
	if err != nil {
		raw = nil
	}

	var servers map[string]json.RawMessage
	if raw != nil {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return fmt.Errorf("failed to parse servers config: %w", err)
		}
	}

	for name := range servers {
		base := "apps/http/servers/" + name

		switch t.AutoHTTPS {
		case "off":
			data, err := json.Marshal(map[string]bool{"disable": true})
			if err != nil {
				return fmt.Errorf("failed to marshal auto_https config: %w", err)
			}
			if err := c.SetConfigPath(base+"/automatic_https", data); err != nil {
				return fmt.Errorf("setting auto_https for server %s: %w", name, err)
			}
		case "disable_redirects":
			data, err := json.Marshal(map[string]bool{"disable_redirects": true})
			if err != nil {
				return fmt.Errorf("failed to marshal auto_https config: %w", err)
			}
			if err := c.SetConfigPath(base+"/automatic_https", data); err != nil {
				return fmt.Errorf("setting auto_https for server %s: %w", name, err)
			}
		default:
			_ = c.DeleteConfigPath(base + "/automatic_https")
		}

		if t.PrometheusMetrics {
			metricsObj := map[string]any{}
			if t.PerHostMetrics {
				metricsObj["per_host"] = true
			}
			data, err := json.Marshal(metricsObj)
			if err != nil {
				return fmt.Errorf("failed to marshal metrics config: %w", err)
			}
			if err := c.SetConfigPath(base+"/metrics", data); err != nil {
				return fmt.Errorf("setting metrics for server %s: %w", name, err)
			}
		} else {
			_ = c.DeleteConfigPath(base + "/metrics")
		}
	}

	return nil
}

// tlsPolicy mirrors the subset of a Caddy TLS automation policy we need for
// reading ACME issuer emails.
type tlsPolicy struct {
	Subjects []string    `json:"subjects"`
	Issuers  []tlsIssuer `json:"issuers"`
}

type tlsIssuer struct {
	Module     string         `json:"module"`
	Email      string         `json:"email,omitempty"`
	Challenges *dnsChallenges `json:"challenges,omitempty"`
}

type dnsChallenges struct {
	DNS struct {
		Provider dnsProvider `json:"provider"`
	} `json:"dns"`
}

type dnsProvider struct {
	Name     string `json:"name"`
	APIToken string `json:"api_token"`
}

type DNSProviderResult struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider,omitempty"`
	APIToken string `json:"api_token,omitempty"`
}

func maskToken(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(token)-4) + token[len(token)-4:]
}

// acmeEmailFromPolicies returns the ACME email from a set of TLS automation
// policies. It prefers the catch-all policy (no subjects) since that's what
// SetACMEEmail creates. Falls back to any policy's ACME email, because Caddy's
// Caddyfile adapter puts the email on per-domain policies instead of a catch-all.
func acmeEmailFromPolicies(policies []tlsPolicy) string {
	var fallback string
	for _, p := range policies {
		for _, iss := range p.Issuers {
			if iss.Module == "acme" && iss.Email != "" {
				if len(p.Subjects) == 0 {
					return iss.Email
				}
				if fallback == "" {
					fallback = iss.Email
				}
			}
		}
	}
	return fallback
}

func (c *Client) GetACMEEmail() (string, error) {
	raw, err := c.GetConfigPath("apps/tls/automation/policies")
	if err != nil {
		if strings.Contains(err.Error(), "unreachable") {
			return "", err
		}
		return "", nil
	}

	var policies []tlsPolicy
	if err := json.Unmarshal(raw, &policies); err != nil {
		return "", nil
	}

	return acmeEmailFromPolicies(policies), nil
}

func (c *Client) SetACMEEmail(email string) error {
	raw, err := c.GetConfigPath("apps/tls/automation/policies")
	if err != nil {
		automation := map[string]any{
			"policies": []map[string]any{
				{
					"issuers": []map[string]any{
						{"module": "acme", "email": email},
					},
				},
			},
		}
		if err := c.SetConfigCascade("apps/tls/automation", automation); err != nil {
			return fmt.Errorf("failed to bootstrap TLS config: %w", err)
		}
		return nil
	}

	var policies []json.RawMessage
	if err := json.Unmarshal(raw, &policies); err != nil {
		return fmt.Errorf("failed to parse TLS policies: %w", err)
	}

	// Find the catch-all policy (no subjects)
	for i, rawPolicy := range policies {
		var p struct {
			Subjects []string `json:"subjects"`
		}
		if json.Unmarshal(rawPolicy, &p) != nil {
			continue
		}
		if len(p.Subjects) > 0 {
			continue
		}
		// Found catch-all - update its ACME issuer email
		issuer := map[string]any{"module": "acme", "email": email}
		issuers := []map[string]any{issuer}
		data, err := json.Marshal(issuers)
		if err != nil {
			return fmt.Errorf("failed to marshal ACME issuers: %w", err)
		}
		return c.PatchConfigPath("apps/tls/automation/policies/"+strconv.Itoa(i)+"/issuers", data)
	}

	// No catch-all found - append a new policy
	newPolicy := map[string]any{
		"issuers": []map[string]any{
			{"module": "acme", "email": email},
		},
	}
	data, err := json.Marshal(newPolicy)
	if err != nil {
		return fmt.Errorf("failed to marshal TLS policy: %w", err)
	}
	return c.SetConfigPath("apps/tls/automation/policies/", data)
}

func (c *Client) GetDNSProvider() (*DNSProviderResult, error) {
	raw, err := c.GetConfigPath("apps/tls/automation/policies")
	if err != nil {
		if strings.Contains(err.Error(), "unreachable") {
			return nil, err
		}
		return &DNSProviderResult{Enabled: false}, nil
	}

	var policies []tlsPolicy
	if err := json.Unmarshal(raw, &policies); err != nil {
		return &DNSProviderResult{Enabled: false}, nil
	}

	for _, p := range policies {
		if len(p.Subjects) > 0 {
			continue
		}
		for _, iss := range p.Issuers {
			if iss.Module == "acme" && iss.Challenges != nil && iss.Challenges.DNS.Provider.Name != "" {
				return &DNSProviderResult{
					Enabled:  true,
					Provider: iss.Challenges.DNS.Provider.Name,
					APIToken: maskToken(iss.Challenges.DNS.Provider.APIToken),
				}, nil
			}
		}
	}

	return &DNSProviderResult{Enabled: false}, nil
}

func (c *Client) SetDNSProvider(apiToken string, enabled bool) error {
	raw, err := c.GetConfigPath("apps/tls/automation/policies")
	if err != nil {
		if !enabled {
			return nil
		}
		automation := map[string]any{
			"policies": []map[string]any{
				{
					"issuers": []map[string]any{
						{
							"module": "acme",
							"challenges": map[string]any{
								"dns": map[string]any{
									"provider": map[string]any{
										"name":      "cloudflare",
										"api_token": apiToken,
									},
								},
							},
						},
					},
				},
			},
		}
		return c.SetConfigCascade("apps/tls/automation", automation)
	}

	var policies []json.RawMessage
	if err := json.Unmarshal(raw, &policies); err != nil {
		return fmt.Errorf("failed to parse TLS policies: %w", err)
	}

	for i, rawPolicy := range policies {
		var p struct {
			Subjects []string `json:"subjects"`
		}
		if json.Unmarshal(rawPolicy, &p) != nil {
			continue
		}
		if len(p.Subjects) > 0 {
			continue
		}

		var policy tlsPolicy
		if err := json.Unmarshal(rawPolicy, &policy); err != nil {
			return fmt.Errorf("failed to parse catch-all policy: %w", err)
		}

		acmeIdx := -1
		for j, iss := range policy.Issuers {
			if iss.Module == "acme" {
				acmeIdx = j
				break
			}
		}
		if acmeIdx == -1 {
			policy.Issuers = append(policy.Issuers, tlsIssuer{Module: "acme"})
			acmeIdx = len(policy.Issuers) - 1
		}

		if enabled {
			if apiToken == "" && policy.Issuers[acmeIdx].Challenges != nil {
				apiToken = policy.Issuers[acmeIdx].Challenges.DNS.Provider.APIToken
			}
			if apiToken == "" {
				return fmt.Errorf("API token is required to enable DNS challenge")
			}
			policy.Issuers[acmeIdx].Challenges = &dnsChallenges{}
			policy.Issuers[acmeIdx].Challenges.DNS.Provider = dnsProvider{
				Name:     "cloudflare",
				APIToken: apiToken,
			}
		} else {
			policy.Issuers[acmeIdx].Challenges = nil
		}

		data, err := json.Marshal(policy.Issuers)
		if err != nil {
			return fmt.Errorf("failed to marshal issuers: %w", err)
		}
		return c.PatchConfigPath("apps/tls/automation/policies/"+strconv.Itoa(i)+"/issuers", data)
	}

	if !enabled {
		return nil
	}

	newPolicy := map[string]any{
		"issuers": []map[string]any{
			{
				"module": "acme",
				"challenges": map[string]any{
					"dns": map[string]any{
						"provider": map[string]any{
							"name":      "cloudflare",
							"api_token": apiToken,
						},
					},
				},
			},
		},
	}
	data, err := json.Marshal(newPolicy)
	if err != nil {
		return fmt.Errorf("failed to marshal TLS policy: %w", err)
	}
	return c.SetConfigPath("apps/tls/automation/policies/", data)
}

func (c *Client) EnsureAccessLogger() error {
	if existing, err := c.GetConfigPath("logging/logs/kaji_access"); err == nil && len(existing) > 0 {
		return nil
	}
	if err := c.ensureLoggingLogs(); err != nil {
		return fmt.Errorf("bootstrapping logging for access logger: %w", err)
	}
	accessLog := map[string]any{
		"writer":  map[string]any{"output": "discard"},
		"include": []string{"http.log.access.*"},
	}
	data, err := json.Marshal(accessLog)
	if err != nil {
		return fmt.Errorf("failed to marshal access logger config: %w", err)
	}
	return c.SetConfigPath("logging/logs/kaji_access", data)
}

func (c *Client) EnsureDefaultLogger() error {
	if existing, err := c.GetConfigPath("logging/logs/default"); err == nil && len(existing) > 0 {
		return nil
	}
	if err := c.ensureLoggingLogs(); err != nil {
		return fmt.Errorf("bootstrapping logging for default logger: %w", err)
	}
	defaultLog := map[string]any{
		"level":   "INFO",
		"encoder": map[string]any{"format": "console"},
		"writer":  map[string]any{"output": "discard"},
	}
	data, err := json.Marshal(defaultLog)
	if err != nil {
		return fmt.Errorf("failed to marshal default logger config: %w", err)
	}
	return c.SetConfigPath("logging/logs/default", data)
}

func (c *Client) ensureLoggingLogs() error {
	if _, err := c.GetConfigPath("logging/logs"); err == nil {
		return nil
	}
	// Try setting via the config path API first.
	if err := c.SetConfigCascade("logging/logs", map[string]any{}); err == nil {
		return nil
	}
	// Cascade failed - Caddy can't create top-level keys via the path API
	// when they don't exist yet. Fall back to merging into the full config.
	raw, err := c.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to read config to bootstrap logging: %w", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg == nil {
		cfg = map[string]any{}
	}
	cfg["logging"] = map[string]any{"logs": map[string]any{}}
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config with logging: %w", err)
	}
	return c.LoadConfig(data)
}
