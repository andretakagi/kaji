// HTTP client for Caddy's admin API at localhost:2019.
package caddy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// caddyServer holds the subset of a Caddy server's config that Kaji reads.
// JSON unmarshaling silently ignores fields not listed here, so callers only
// use the fields they need.
type caddyServer struct {
	Routes    []json.RawMessage `json:"routes"`
	AutoHTTPS *struct {
		Disable          bool `json:"disable"`
		DisableRedirects bool `json:"disable_redirects"`
	} `json:"automatic_https"`
	Metrics *struct {
		PerHost bool `json:"per_host"`
	} `json:"metrics"`
	Logs *struct {
		LoggerNames map[string]string `json:"logger_names"`
	} `json:"logs"`
}

// caddyConfigPartial is the minimal top-level Caddy config structure needed
// to reach the HTTP servers map.
type caddyConfigPartial struct {
	Apps struct {
		HTTP struct {
			Servers map[string]caddyServer `json:"servers"`
		} `json:"http"`
	} `json:"apps"`
}

type Client struct {
	baseURL    func() string
	httpClient *http.Client
}

func NewClient(urlFunc func() string) *Client {
	return &Client{
		baseURL:    urlFunc,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) url() string {
	u := c.baseURL()
	if u == "" {
		u = "http://localhost:2019"
	}
	return strings.TrimRight(u, "/")
}

func (c *Client) getConfigRaw(path string) (json.RawMessage, error) {
	target := c.url() + "/config/"
	if path != "" {
		target += path
	}
	resp, err := c.httpClient.Get(target)
	if err != nil {
		return nil, fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading config response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get config (status %d): %s", resp.StatusCode, body)
	}
	return json.RawMessage(body), nil
}

// GetConfig returns the full Caddy config. A null response is valid here
// since a fresh Caddy instance has no config loaded.
func (c *Client) GetConfig() (json.RawMessage, error) {
	return c.getConfigRaw("")
}

// GetConfigPath returns the value at a specific config path. Caddy returns
// null with 200 for missing keys inside existing objects, so this treats
// null as "not found" to avoid silent failures.
func (c *Client) GetConfigPath(path string) (json.RawMessage, error) {
	body, err := c.getConfigRaw(path)
	if err != nil {
		return nil, err
	}
	if trimmed := bytes.TrimSpace(body); len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, fmt.Errorf("config path %q does not exist", path)
	}
	return body, nil
}

func (c *Client) LoadConfig(configJSON []byte) error {
	resp, err := c.httpClient.Post(
		c.url()+"/load",
		"application/json",
		bytes.NewReader(configJSON),
	)
	if err != nil {
		return fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("caddy rejected config (status %d, body unreadable)", resp.StatusCode)
		}
		return fmt.Errorf("caddy rejected config (status %d): %s", resp.StatusCode, body)
	}
	return nil
}

func (c *Client) GetRouteByID(id string) (json.RawMessage, error) {
	resp, err := c.httpClient.Get(c.url() + "/id/" + url.PathEscape(id))
	if err != nil {
		return nil, fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading route response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("route %q not found (status %d): %s", id, resp.StatusCode, body)
	}
	return json.RawMessage(body), nil
}

func (c *Client) DeleteByID(id string) error {
	req, err := http.NewRequest(http.MethodDelete, c.url()+"/id/"+url.PathEscape(id), nil)
	if err != nil {
		return fmt.Errorf("building delete request for route %q: %w", id, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to delete %q (status %d, body unreadable)", id, resp.StatusCode)
		}
		return fmt.Errorf("failed to delete %q (status %d): %s", id, resp.StatusCode, body)
	}
	return nil
}

// AddRoute appends a route to the given server's route list.
// If the server doesn't exist yet, it bootstraps the minimal structure first.
func (c *Client) AddRoute(server string, route json.RawMessage) error {
	routesPath := "apps/http/servers/" + server + "/routes"
	if _, err := c.GetConfigPath(routesPath); err != nil {
		srv := map[string]any{
			"listen": []string{":443"},
			"routes": []json.RawMessage{},
		}
		if err := c.SetConfigCascade("apps/http/servers/"+server, srv); err != nil {
			return fmt.Errorf("failed to bootstrap http app for server %q: %w", server, err)
		}
	}

	target := c.url() + "/config/" + routesPath
	resp, err := c.httpClient.Post(target, "application/json", bytes.NewReader(route))
	if err != nil {
		return fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to add route (status %d, body unreadable)", resp.StatusCode)
		}
		return fmt.Errorf("failed to add route (status %d): %s", resp.StatusCode, body)
	}
	return nil
}

// FindRouteServer searches the full config to find which server a route with
// the given @id belongs to. Returns the server name.
func (c *Client) FindRouteServer(id string) (string, error) {
	raw, err := c.GetConfig()
	if err != nil {
		return "", fmt.Errorf("fetching config to find route server: %w", err)
	}

	var cfg caddyConfigPartial
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse caddy config: %w", err)
	}

	for name, srv := range cfg.Apps.HTTP.Servers {
		for _, route := range srv.Routes {
			var r struct {
				ID string `json:"@id"`
			}
			if json.Unmarshal(route, &r) == nil && r.ID == id {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("no server found containing route %q", id)
}

func (c *Client) GetLoggingConfig() (json.RawMessage, error) {
	resp, err := c.httpClient.Get(c.url() + "/config/logging")
	if err != nil {
		return nil, fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading logging config response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get logging config (status %d): %s", resp.StatusCode, body)
	}
	return json.RawMessage(body), nil
}

func (c *Client) SetLoggingConfig(loggingJSON []byte) error {
	req, err := http.NewRequest(http.MethodPost, c.url()+"/config/logging", bytes.NewReader(loggingJSON))
	if err != nil {
		return fmt.Errorf("building logging config request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("caddy rejected logging config (status %d, body unreadable)", resp.StatusCode)
		}
		return fmt.Errorf("caddy rejected logging config (status %d): %s", resp.StatusCode, body)
	}
	return nil
}

// ReplaceRouteByID finds a route by @id in the config, determines its exact
// server and array index, then replaces it in place via PATCH to the direct
// config path. PATCH is required here because Caddy's PUT to an array index
// inserts before that index rather than replacing.
func (c *Client) ReplaceRouteByID(id string, newRoute json.RawMessage) (string, error) {
	raw, err := c.GetConfig()
	if err != nil {
		return "", fmt.Errorf("fetching config to replace route: %w", err)
	}

	var cfg caddyConfigPartial
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse caddy config: %w", err)
	}

	for serverName, srv := range cfg.Apps.HTTP.Servers {
		for i, route := range srv.Routes {
			var r struct {
				ID string `json:"@id"`
			}
			if json.Unmarshal(route, &r) != nil || r.ID != id {
				continue
			}
			path := fmt.Sprintf("apps/http/servers/%s/routes/%d", serverName, i)
			if err := c.PatchConfigPath(path, newRoute); err != nil {
				return "", fmt.Errorf("patching route %q: %w", id, err)
			}
			return serverName, nil
		}
	}
	return "", fmt.Errorf("route %q not found", id)
}

// SetRouteAccessLog adds or removes a domain from the server's access log
// configuration. When enabled, the domain is mapped to the "kaji_access"
// logger in the server's logs.logger_names. When disabled, the mapping is
// removed.
func (c *Client) SetRouteAccessLog(server, domain string, enabled bool) error {
	if enabled {
		// Create the kaji_access logger only if it doesn't already exist.
		if _, err := c.GetConfigPath("logging/logs/kaji_access"); err != nil {
			if err := c.ensureLoggingLogs(); err != nil {
				return fmt.Errorf("bootstrapping logging config: %w", err)
			}
			logger := map[string]any{
				"writer":  map[string]string{"output": "stdout"},
				"include": []string{"http.log.access.*"},
			}
			data, err := json.Marshal(logger)
			if err != nil {
				return fmt.Errorf("marshaling logger config: %w", err)
			}
			if err := c.SetConfigPath("logging/logs/kaji_access", data); err != nil {
				return fmt.Errorf("creating kaji_access logger: %w", err)
			}
		}

		// On a fresh server this structure won't exist yet.
		logNamesPath := "apps/http/servers/" + server + "/logs/logger_names"
		if err := c.EnsureConfigPath(logNamesPath); err != nil {
			return fmt.Errorf("bootstrapping server logs config: %w", err)
		}

		// Map this domain to the logger
		name, err := json.Marshal("kaji_access")
		if err != nil {
			return fmt.Errorf("marshaling logger name: %w", err)
		}
		return c.SetConfigPath(
			"apps/http/servers/"+server+"/logs/logger_names/"+domain,
			name,
		)
	}

	// Disabled: remove the domain mapping (ignore errors if it doesn't exist)
	_ = c.DeleteConfigPath(
		"apps/http/servers/" + server + "/logs/logger_names/" + domain,
	)
	return nil
}

// GetAccessLogDomains returns the set of domains that have per-route access
// logging enabled on the given server.
func (c *Client) GetAccessLogDomains(server string) (map[string]bool, error) {
	raw, err := c.GetConfigPath("apps/http/servers/" + server + "/logs/logger_names")
	if err != nil {
		// No logger_names configured yet
		return nil, nil
	}
	var names map[string]string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, fmt.Errorf("parsing logger_names: %w", err)
	}
	result := make(map[string]bool, len(names))
	for domain := range names {
		result[domain] = true
	}
	return result, nil
}

// GetAllAccessLogDomains returns domains with access logging enabled across
// all servers, keyed by server name.
func (c *Client) GetAllAccessLogDomains() (map[string][]string, error) {
	raw, err := c.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("fetching config: %w", err)
	}
	var cfg caddyConfigPartial
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	result := make(map[string][]string)
	for name, srv := range cfg.Apps.HTTP.Servers {
		if srv.Logs == nil {
			continue
		}
		for domain, logger := range srv.Logs.LoggerNames {
			if logger == "kaji_access" {
				result[name] = append(result[name], domain)
			}
		}
	}
	return result, nil
}

// ClearAllAccessLogDomains removes all domain mappings to the kaji_access
// logger across every server. Called when the kaji_access sink is deleted.
func (c *Client) ClearAllAccessLogDomains() error {
	domains, err := c.GetAllAccessLogDomains()
	if err != nil {
		return err
	}
	for server, serverDomains := range domains {
		for _, domain := range serverDomains {
			_ = c.DeleteConfigPath(
				"apps/http/servers/" + server + "/logs/logger_names/" + domain,
			)
		}
	}
	return nil
}

// AdaptCaddyfile sends Caddyfile text to Caddy's /adapt endpoint and returns
// the equivalent JSON config.
func (c *Client) AdaptCaddyfile(caddyfileText string) (json.RawMessage, error) {
	resp, err := c.httpClient.Post(
		c.url()+"/adapt",
		"text/caddyfile",
		strings.NewReader(caddyfileText),
	)
	if err != nil {
		return nil, fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading adapt response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("caddy could not parse Caddyfile (status %d): %s", resp.StatusCode, body)
	}

	var result struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing adapt response: %w", err)
	}
	return result.Result, nil
}

func (c *Client) GetUpstreams() (json.RawMessage, error) {
	resp, err := c.httpClient.Get(c.url() + "/reverse_proxy/upstreams")
	if err != nil {
		return nil, fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading upstreams response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get upstreams (status %d): %s", resp.StatusCode, body)
	}
	return json.RawMessage(body), nil
}
