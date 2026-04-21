// HTTP client for Caddy's admin API at localhost:2019.
package caddy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	clientTimeout     = 10 * time.Second
	readyPollInterval = 500 * time.Millisecond
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

// CountDomains returns the total number of domains across all servers in a raw
// Caddy JSON config. Returns 0 if the config can't be parsed.
func CountDomains(raw json.RawMessage) int {
	var cfg caddyConfigPartial
	if json.Unmarshal(raw, &cfg) != nil {
		return 0
	}
	n := 0
	for _, srv := range cfg.Apps.HTTP.Servers {
		n += len(srv.Routes)
	}
	return n
}

type ReviewDomain struct {
	Domain   string `json:"domain"`
	Upstream string `json:"upstream"`
	Enabled  bool   `json:"enabled"`
}

func ExtractReviewDomains(raw json.RawMessage) []ReviewDomain {
	routes := []ReviewDomain{}
	var cfg caddyConfigPartial
	if json.Unmarshal(raw, &cfg) != nil {
		return routes
	}
	for _, srv := range cfg.Apps.HTTP.Servers {
		for _, routeRaw := range srv.Routes {
			params, err := ParseDomainParams(routeRaw)
			if err != nil || params.Domain == "" {
				continue
			}
			routes = append(routes, ReviewDomain{
				Domain:   params.Domain,
				Upstream: params.Upstream,
				Enabled:  true,
			})
		}
	}
	return routes
}

type Client struct {
	baseURL    func() string
	httpClient *http.Client
}

func NewClient(urlFunc func() string) *Client {
	return &Client{
		baseURL:    urlFunc,
		httpClient: &http.Client{Timeout: clientTimeout},
	}
}

func (c *Client) url() string {
	u := c.baseURL()
	if u == "" {
		u = "http://localhost:2019"
	}
	return strings.TrimRight(u, "/")
}

func (c *Client) IsReachable() bool {
	resp, err := c.httpClient.Get(c.url() + "/config/")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}

// WaitReady polls Caddy's admin API until it responds or the timeout is
// reached. Returns nil once Caddy is reachable, or an error after timeout.
func (c *Client) WaitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.IsReachable() {
			return nil
		}
		time.Sleep(readyPollInterval)
	}
	return fmt.Errorf("caddy admin API at %s not reachable after %s", c.url(), timeout)
}

func (c *Client) doGet(rawURL string) ([]byte, error) {
	resp, err := c.httpClient.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed (status %d): %s", resp.StatusCode, body)
	}
	return body, nil
}

func (c *Client) doRequest(method, rawURL, contentType string, body []byte) error {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, rawURL, reqBody)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caddy admin API unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("request failed (status %d, body unreadable)", resp.StatusCode)
		}
		return fmt.Errorf("request failed (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

func (c *Client) getConfigRaw(path string) (json.RawMessage, error) {
	target := c.url() + "/config/"
	if path != "" {
		target += path
	}
	body, err := c.doGet(target)
	if err != nil {
		return nil, err
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
	if err := c.doRequest(http.MethodPost, c.url()+"/load", "application/json", configJSON); err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	return nil
}

var ErrValidationRollbackFailed = errors.New("config validation passed but rollback failed")

// ValidateConfig checks that configJSON is loadable by Caddy without permanently
// applying it. It loads the candidate config, then immediately restores the
// previous config. If Caddy rejects the candidate, the running config is unchanged.
func (c *Client) ValidateConfig(configJSON []byte) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(configJSON, &obj); err != nil {
		return fmt.Errorf("invalid caddy config: not a JSON object")
	}

	current, err := c.GetConfig()
	if err != nil {
		return fmt.Errorf("reading current config for validation: %w", err)
	}

	if err := c.LoadConfig(configJSON); err != nil {
		return fmt.Errorf("caddy rejected config: %w", err)
	}

	if err := c.LoadConfig(current); err != nil {
		return fmt.Errorf("%w: %v", ErrValidationRollbackFailed, err)
	}

	return nil
}

func (c *Client) GetDomainByID(id string) (json.RawMessage, error) {
	body, err := c.doGet(c.url() + "/id/" + url.PathEscape(id))
	if err != nil {
		return nil, fmt.Errorf("domain %q: %w", id, err)
	}
	return json.RawMessage(body), nil
}

func (c *Client) DeleteByID(id string) error {
	if err := c.doRequest(http.MethodDelete, c.url()+"/id/"+url.PathEscape(id), "", nil); err != nil {
		return fmt.Errorf("deleting domain %q: %w", id, err)
	}
	return nil
}

// AddDomain appends a domain to the given server's route list.
// If the server doesn't exist yet, it bootstraps the minimal structure first.
func (c *Client) AddDomain(server string, route json.RawMessage) error {
	routesPath := serverCaddyRoutesPath(server)
	if _, err := c.GetConfigPath(routesPath); err != nil {
		srv := map[string]any{
			"listen": []string{":443"},
			"routes": []json.RawMessage{},
		}
		if err := c.SetConfigCascade(serverPath(server), srv); err != nil {
			return fmt.Errorf("failed to bootstrap http app for server %q: %w", server, err)
		}
	}

	if err := c.doRequest(http.MethodPost, c.url()+"/config/"+routesPath, "application/json", route); err != nil {
		return fmt.Errorf("adding domain to %q: %w", server, err)
	}
	return nil
}

// FindDomainServer searches the full config to find which server a domain with
// the given @id belongs to. Returns the server name.
func (c *Client) FindDomainServer(id string) (string, error) {
	raw, err := c.GetConfig()
	if err != nil {
		return "", fmt.Errorf("fetching config to find domain server: %w", err)
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
	return "", fmt.Errorf("no server found containing domain %q", id)
}

func (c *Client) GetLoggingConfig() (json.RawMessage, error) {
	body, err := c.doGet(c.url() + "/config/logging")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func (c *Client) SetLoggingConfig(loggingJSON []byte) error {
	if err := c.doRequest(http.MethodPost, c.url()+"/config/logging", "application/json", loggingJSON); err != nil {
		return fmt.Errorf("setting logging config: %w", err)
	}
	return nil
}

// ReplaceDomainByID finds a domain by @id in the config, determines its exact
// server and array index, then replaces it in place via PATCH to the direct
// config path. PATCH is required here because Caddy's PUT to an array index
// inserts before that index rather than replacing.
func (c *Client) ReplaceDomainByID(id string, newRoute json.RawMessage) (string, error) {
	raw, err := c.GetConfig()
	if err != nil {
		return "", fmt.Errorf("fetching config to replace domain: %w", err)
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
			path := serverCaddyRoutePath(serverName, i)
			if err := c.PatchConfigPath(path, newRoute); err != nil {
				return "", fmt.Errorf("patching domain %q: %w", id, err)
			}
			return serverName, nil
		}
	}
	return "", fmt.Errorf("domain %q not found", id)
}

func (c *Client) SetDomainAccessLog(server, domain, sinkName string) error {
	if sinkName != "" {
		raw, getErr := c.GetConfigPath(LogSinkPath(sinkName))
		if getErr != nil {
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
			if err := c.SetConfigPath(LogSinkPath(sinkName), data); err != nil {
				return fmt.Errorf("creating %s logger: %w", sinkName, err)
			}
		} else if sinkName == "kaji_access" {
			// Re-enable kaji_access if it was toggled off
			var sink struct {
				Writer struct {
					Output string `json:"output"`
				} `json:"writer"`
			}
			if json.Unmarshal(raw, &sink) == nil && sink.Writer.Output == "discard" {
				var current map[string]any
				if json.Unmarshal(raw, &current) == nil {
					current["writer"] = map[string]string{"output": "stdout"}
					data, err := json.Marshal(current)
					if err != nil {
						return fmt.Errorf("marshaling kaji_access update: %w", err)
					}
					if err := c.SetConfigPath(LogSinkPath("kaji_access"), data); err != nil {
						return fmt.Errorf("re-enabling kaji_access writer: %w", err)
					}
				}
			}
		}

		logNamesPath := serverLoggerNamesPath(server)
		if err := c.EnsureConfigPath(logNamesPath); err != nil {
			return fmt.Errorf("bootstrapping server logs config: %w", err)
		}

		name, err := json.Marshal(sinkName)
		if err != nil {
			return fmt.Errorf("marshaling logger name: %w", err)
		}
		return c.SetConfigPath(
			serverLoggerNamePath(server, domain),
			name,
		)
	}

	if err := c.DeleteConfigPath(
		serverLoggerNamePath(server, domain),
	); err != nil {
		log.Printf("SetDomainAccessLog: removing %s logger_names entry: %v", domain, err)
	}
	return nil
}

func (c *Client) GetAccessLogDomains(server string) (map[string]string, error) {
	raw, err := c.GetConfigPath(serverLoggerNamesPath(server))
	if err != nil {
		return nil, nil
	}
	var names map[string]string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, fmt.Errorf("parsing logger_names: %w", err)
	}
	return names, nil
}

func (c *Client) GetAllAccessLogDomains() (map[string]map[string]string, error) {
	raw, err := c.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("fetching config: %w", err)
	}
	var cfg caddyConfigPartial
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	result := make(map[string]map[string]string)
	for name, srv := range cfg.Apps.HTTP.Servers {
		if srv.Logs == nil {
			continue
		}
		for domain, logger := range srv.Logs.LoggerNames {
			if result[name] == nil {
				result[name] = make(map[string]string)
			}
			result[name][domain] = logger
		}
	}
	return result, nil
}

func (c *Client) ClearDomainsForSink(sinkName string) error {
	domains, err := c.GetAllAccessLogDomains()
	if err != nil {
		return err
	}
	var errs []error
	for server, serverDomains := range domains {
		for domain, logger := range serverDomains {
			if logger == sinkName {
				if err := c.DeleteConfigPath(
					serverLoggerNamePath(server, domain),
				); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (c *Client) IsSinkReferenced(sinkName string) (bool, error) {
	domains, err := c.GetAllAccessLogDomains()
	if err != nil {
		return false, err
	}
	for _, serverDomains := range domains {
		for _, logger := range serverDomains {
			if logger == sinkName {
				return true, nil
			}
		}
	}
	return false, nil
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
	body, err := c.doGet(c.url() + "/reverse_proxy/upstreams")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}
