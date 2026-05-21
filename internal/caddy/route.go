// Domain building and parsing. Turns DomainParams into Caddy JSON and back.
package caddy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type DomainParams struct {
	ID                string          `json:"@id"`
	Domain            string          `json:"domain"`
	Upstream          string          `json:"upstream"`
	HandlerType       string          `json:"handler_type"`
	HandlerConfig     json.RawMessage `json:"handler_config,omitempty"`
	Toggles           RouteToggles    `json:"toggles"`
	AdvancedHeaders   bool            `json:"-"`
	StripPathPrefix   string          `json:"-"`
	PrependPathPrefix string          `json:"-"`
	IPListIPs         []string        `json:"-"`
	IPListType        string          `json:"-"`
}

type IPFilteringOpts struct {
	Enabled bool   `json:"enabled"`
	ListID  string `json:"list_id"`
	Type    string `json:"type"`
	Matcher string `json:"matcher"`
}

func ipMatcherKey(matcher string) string {
	if matcher == "client_ip" {
		return "client_ip"
	}
	return "remote_ip"
}

type RouteToggles struct {
	Enabled           bool            `json:"enabled"`
	ForceHTTPS        bool            `json:"force_https"`
	Compression       bool            `json:"compression"`
	Headers           HeadersConfig   `json:"headers"`
	TLSSkipVerify     bool            `json:"tls_skip_verify"`
	Auth              AuthToggle      `json:"auth"`
	AccessLog         string          `json:"access_log"`
	WebSocketPassthru bool            `json:"websocket_passthrough"`
	LoadBalancing     LoadBalancing   `json:"load_balancing"`
	IPFiltering       IPFilteringOpts `json:"ip_filtering"`
}

type LoadBalancing struct {
	Enabled   bool     `json:"enabled"`
	Strategy  string   `json:"strategy"`
	Upstreams []string `json:"upstreams,omitempty"`
}

type HeadersConfig struct {
	Request  DomainRequestHeaders `json:"request"`
	Response ResponseHeaders      `json:"response"`
}

type DomainRequestHeaders struct {
	Enabled         bool          `json:"enabled"`
	XForwardedFor   bool          `json:"x_forwarded_for"`
	XRealIP         bool          `json:"x_real_ip"`
	XForwardedProto bool          `json:"x_forwarded_proto"`
	XForwardedHost  bool          `json:"x_forwarded_host"`
	XRequestID      bool          `json:"x_request_id"`
	Builtin         []HeaderEntry `json:"builtin"`
	Custom          []HeaderEntry `json:"custom"`
}

type ResponseHeaders struct {
	Enabled      bool          `json:"enabled"`
	Security     bool          `json:"security"`
	CORS         bool          `json:"cors"`
	CORSOrigins  []string      `json:"cors_origins"`
	CacheControl bool          `json:"cache_control"`
	XRobotsTag   bool          `json:"x_robots_tag"`
	Deferred     bool          `json:"deferred"`
	Builtin      []HeaderEntry `json:"builtin"`
	Custom       []HeaderEntry `json:"custom"`
}

type HeaderUpConfig struct {
	Enabled       bool          `json:"enabled"`
	HostOverride  bool          `json:"host_override"`
	HostValue     string        `json:"host_value"`
	Authorization bool          `json:"authorization"`
	AuthValue     string        `json:"auth_value"`
	Builtin       []HeaderEntry `json:"builtin"`
	Custom        []HeaderEntry `json:"custom"`
}

type HeaderDownConfig struct {
	Enabled        bool          `json:"enabled"`
	StripServer    bool          `json:"strip_server"`
	StripPoweredBy bool          `json:"strip_powered_by"`
	Deferred       bool          `json:"deferred"`
	Builtin        []HeaderEntry `json:"builtin"`
	Custom         []HeaderEntry `json:"custom"`
}

type HeaderEntry struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Operation string `json:"operation"`
	Search    string `json:"search,omitempty"`
	Enabled   bool   `json:"enabled"`
}

type BasicAuth struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Password     string `json:"password,omitempty"`
}

type AuthToggle struct {
	Mode      string    `json:"mode"`
	BasicAuth BasicAuth `json:"basic_auth"`
}

type ForwardAuthConfig struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	URL      string `json:"url"`
}

var idSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func GenerateRouteID(domain string) string {
	safe := idSanitizer.ReplaceAllString(domain, "_")
	return "kaji_" + safe
}

func BuildDomain(p DomainParams) (json.RawMessage, error) {
	if p.Domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if p.Upstream == "" {
		return nil, fmt.Errorf("upstream is required")
	}
	if p.ID == "" {
		p.ID = GenerateRouteID(p.Domain)
	}

	var handlers []any

	if p.Toggles.ForceHTTPS {
		handlers = append(handlers, map[string]any{
			"handler": "subroute",
			"routes": []map[string]any{
				{
					"match": []map[string]any{
						{"protocol": "http"},
					},
					"handle": []map[string]any{
						{
							"handler":     "static_response",
							"status_code": "301",
							"headers": map[string][]string{
								"Location": {"https://{http.request.host}{http.request.uri}"},
							},
						},
					},
				},
			},
		})
	}

	if len(p.IPListIPs) > 0 && p.IPListType != "" {
		matcherKey := ipMatcherKey(p.Toggles.IPFiltering.Matcher)
		ipMatcher := map[string]any{
			matcherKey: map[string]any{"ranges": p.IPListIPs},
		}

		var matchBlock []map[string]any
		if p.IPListType == "whitelist" {
			matchBlock = []map[string]any{
				{"not": []map[string]any{ipMatcher}},
			}
		} else {
			matchBlock = []map[string]any{ipMatcher}
		}

		handlers = append(handlers, map[string]any{
			"handler": "subroute",
			"routes": []map[string]any{
				{
					"match": matchBlock,
					"handle": []map[string]any{
						{
							"handler":     "static_response",
							"status_code": "403",
							"body":        "Forbidden",
						},
					},
				},
			},
		})
	}

	// Compression (gzip + zstd via encode handler)
	if p.Toggles.Compression {
		handlers = append(handlers, map[string]any{
			"handler": "encode",
			"encodings": map[string]any{
				"gzip": map[string]any{},
				"zstd": map[string]any{},
			},
			"prefer": []string{"zstd", "gzip"},
		})
	}

	// Domain-level headers (request + response combined into one handler when possible)
	reqBlock := buildDomainRequestHeaders(p.Toggles.Headers, p.AdvancedHeaders)
	respHandlers := buildResponseHeaders(p.Toggles.Headers, p.AdvancedHeaders)

	if reqBlock != nil {
		merged := false
		if len(respHandlers) > 0 {
			if first, ok := respHandlers[0].(map[string]any); ok && first["handler"] == "headers" {
				first["request"] = reqBlock
				merged = true
			}
		}
		if !merged {
			handlers = append(handlers, map[string]any{
				"handler": "headers",
				"request": reqBlock,
			})
		}
	}
	handlers = append(handlers, respHandlers...)

	// Basic auth
	if p.Toggles.Auth.Mode == "basic" && p.Toggles.Auth.BasicAuth.Username != "" {
		handlers = append(handlers, map[string]any{
			"handler": "authentication",
			"providers": map[string]any{
				"http_basic": map[string]any{
					"accounts": []map[string]string{
						{
							"username": p.Toggles.Auth.BasicAuth.Username,
							"password": p.Toggles.Auth.BasicAuth.PasswordHash,
						},
					},
					"hash": map[string]any{
						"algorithm": "bcrypt",
					},
				},
			},
		})
	}

	// Reverse proxy (always last in handler chain)
	upstreams := []map[string]string{{"dial": p.Upstream}}
	if p.Toggles.LoadBalancing.Enabled {
		for _, u := range p.Toggles.LoadBalancing.Upstreams {
			upstreams = append(upstreams, map[string]string{"dial": u})
		}
	}
	rp := map[string]any{
		"handler":   "reverse_proxy",
		"upstreams": upstreams,
	}

	if p.Toggles.TLSSkipVerify {
		rp["transport"] = map[string]any{
			"protocol": "http",
			"tls":      map[string]any{"insecure_skip_verify": true},
		}
	}

	if p.Toggles.WebSocketPassthru {
		// flush_interval -1 enables streaming/websocket passthrough
		rp["flush_interval"] = -1
	}

	if p.Toggles.LoadBalancing.Enabled {
		strategy := p.Toggles.LoadBalancing.Strategy
		if strategy == "" {
			strategy = "round_robin"
		}
		rp["load_balancing"] = map[string]any{
			"selection_policy": map[string]any{
				"policy": strategy,
			},
		}
	}

	if p.HandlerType == "reverse_proxy" && len(p.HandlerConfig) > 0 {
		var rpCfg ReverseProxyConfig
		if json.Unmarshal(p.HandlerConfig, &rpCfg) == nil {
			var rpHeaders map[string]any
			if up := BuildHeaderUp(rpCfg.HeaderUp, p.AdvancedHeaders); up != nil {
				if rpHeaders == nil {
					rpHeaders = make(map[string]any)
				}
				rpHeaders["request"] = up
			}
			if down := BuildHeaderDown(rpCfg.HeaderDown, p.AdvancedHeaders); down != nil {
				if rpHeaders == nil {
					rpHeaders = make(map[string]any)
				}
				rpHeaders["response"] = down
			}
			if rpHeaders != nil {
				rp["headers"] = rpHeaders
			}

			if rpCfg.HealthChecks.Enabled {
				hc := make(map[string]any)
				if rpCfg.HealthChecks.Active.Enabled {
					active := make(map[string]any)
					if rpCfg.HealthChecks.Active.URI != "" {
						active["uri"] = rpCfg.HealthChecks.Active.URI
					}
					if rpCfg.HealthChecks.Active.Interval != "" {
						active["interval"] = rpCfg.HealthChecks.Active.Interval
					}
					if rpCfg.HealthChecks.Active.Timeout != "" {
						active["timeout"] = rpCfg.HealthChecks.Active.Timeout
					}
					if rpCfg.HealthChecks.Active.Port != 0 {
						active["port"] = rpCfg.HealthChecks.Active.Port
					}
					if rpCfg.HealthChecks.Active.ExpectStatus != 0 {
						active["expect_status"] = rpCfg.HealthChecks.Active.ExpectStatus
					}
					if rpCfg.HealthChecks.Active.ExpectBody != "" {
						active["expect_body"] = rpCfg.HealthChecks.Active.ExpectBody
					}
					hc["active"] = active
				}
				if rpCfg.HealthChecks.Passive.Enabled {
					passive := make(map[string]any)
					if rpCfg.HealthChecks.Passive.FailDuration != "" {
						passive["fail_duration"] = rpCfg.HealthChecks.Passive.FailDuration
					}
					if rpCfg.HealthChecks.Passive.MaxFails != 0 {
						passive["max_fails"] = rpCfg.HealthChecks.Passive.MaxFails
					}
					if len(rpCfg.HealthChecks.Passive.UnhealthyStatus) > 0 {
						passive["unhealthy_status"] = rpCfg.HealthChecks.Passive.UnhealthyStatus
					}
					if rpCfg.HealthChecks.Passive.UnhealthyLatency != "" {
						passive["unhealthy_latency"] = rpCfg.HealthChecks.Passive.UnhealthyLatency
					}
					if rpCfg.HealthChecks.Passive.UnhealthyRequestCount != 0 {
						passive["unhealthy_request_count"] = rpCfg.HealthChecks.Passive.UnhealthyRequestCount
					}
					hc["passive"] = passive
				}
				if len(hc) > 0 {
					rp["health_checks"] = hc
				}
			}
		}
	}

	handlers = append(handlers, rp)

	route := map[string]any{
		"@id":      p.ID,
		"match":    []map[string]any{{"host": []string{p.Domain}}},
		"handle":   handlers,
		"terminal": true,
	}

	data, err := json.Marshal(route)
	if err != nil {
		return nil, fmt.Errorf("marshaling route: %w", err)
	}
	return json.RawMessage(data), nil
}

// ParseDomainParams extracts DomainParams from an existing Caddy route JSON.
// Used to populate the toggle panel from a live domain.
//
// Handles two route structures:
//   - Admin API routes (Kaji-created): handlers are a flat array at the top level
//   - Caddyfile-adapted routes: all handlers are wrapped in a single top-level
//     subroute, with each handler in its own nested route
func ParseDomainParams(raw json.RawMessage) (DomainParams, error) {
	var route struct {
		ID    string `json:"@id"`
		Match []struct {
			Host []string `json:"host"`
		} `json:"match"`
		Handle []json.RawMessage `json:"handle"`
	}
	if err := json.Unmarshal(raw, &route); err != nil {
		return DomainParams{}, err
	}

	p := DomainParams{ID: route.ID}
	p.Toggles.Enabled = true
	if len(route.Match) > 0 && len(route.Match[0].Host) > 0 {
		p.Domain = route.Match[0].Host[0]
	}

	handlers := flattenHandlers(route.Handle, &p.Toggles)
	parseHandlers(handlers, &p)

	if p.HandlerType == "reverse_proxy" && (p.StripPathPrefix != "" || p.PrependPathPrefix != "") {
		var rpCfg ReverseProxyConfig
		if err := json.Unmarshal(p.HandlerConfig, &rpCfg); err == nil {
			rpCfg.StripPathPrefix = p.StripPathPrefix
			rpCfg.PrependPathPrefix = p.PrependPathPrefix
			if updated, err := json.Marshal(rpCfg); err == nil {
				p.HandlerConfig = updated
			}
		}
	}

	return p, nil
}

// flattenHandlers collects all handler JSON blobs into a flat list.
// Caddyfile-adapted routes wrap everything in a top-level subroute - this
// unwraps that layer. Kaji's ForceHTTPS subroute (identifiable by a nested
// "protocol":"http" match) is detected separately and not unwrapped.
func flattenHandlers(topLevel []json.RawMessage, toggles *RouteToggles) []json.RawMessage {
	var result []json.RawMessage

	for _, h := range topLevel {
		var peek struct {
			Handler string `json:"handler"`
			Routes  []struct {
				Match  []json.RawMessage `json:"match"`
				Handle []json.RawMessage `json:"handle"`
			} `json:"routes"`
		}
		if json.Unmarshal(h, &peek) != nil || peek.Handler != "subroute" {
			result = append(result, h)
			continue
		}

		if isForceHTTPSSubroute(peek.Routes) {
			toggles.ForceHTTPS = true
			// Caddyfile-adapted routes may wrap all handlers (including
			// reverse_proxy) in the same subroute. Extract handlers from
			// nested routes that aren't the HTTP redirect.
			for _, nestedRoute := range peek.Routes {
				isRedirect := false
				for _, m := range nestedRoute.Match {
					var match struct {
						Protocol string `json:"protocol"`
					}
					if json.Unmarshal(m, &match) == nil && match.Protocol == "http" {
						isRedirect = true
						break
					}
				}
				if !isRedirect {
					result = append(result, nestedRoute.Handle...)
				}
			}
			continue
		}

		if isIPFilteringSubroute(peek.Routes) {
			parseIPFilteringSubroute(peek.Routes, toggles)
			continue
		}

		if isCORSSubroute(peek.Routes) {
			result = append(result, h)
			continue
		}

		// Caddyfile wrapper subroute - extract handlers from nested routes
		for _, nestedRoute := range peek.Routes {
			result = append(result, nestedRoute.Handle...)
		}
	}

	return result
}

// isForceHTTPSSubroute checks whether a subroute's nested routes contain a
// protocol:http match, which is how Kaji's ForceHTTPS redirect is structured.
func isForceHTTPSSubroute(routes []struct {
	Match  []json.RawMessage `json:"match"`
	Handle []json.RawMessage `json:"handle"`
}) bool {
	for _, r := range routes {
		for _, m := range r.Match {
			var match struct {
				Protocol string `json:"protocol"`
			}
			if json.Unmarshal(m, &match) == nil && match.Protocol == "http" {
				return true
			}
		}
	}
	return false
}

// isCORSSubroute checks whether a subroute matches request Origin headers
// and sets Access-Control-Allow-Origin conditionally per origin.
func isCORSSubroute(routes []struct {
	Match  []json.RawMessage `json:"match"`
	Handle []json.RawMessage `json:"handle"`
}) bool {
	if len(routes) == 0 {
		return false
	}
	for _, r := range routes {
		for _, m := range r.Match {
			var match struct {
				Header map[string][]string `json:"header"`
			}
			if json.Unmarshal(m, &match) == nil {
				if _, ok := match.Header["Origin"]; ok {
					return true
				}
			}
		}
	}
	return false
}

func isIPFilteringSubroute(routes []struct {
	Match  []json.RawMessage `json:"match"`
	Handle []json.RawMessage `json:"handle"`
}) bool {
	if len(routes) != 1 {
		return false
	}
	for _, m := range routes[0].Match {
		var match map[string]json.RawMessage
		if json.Unmarshal(m, &match) != nil {
			continue
		}
		if _, ok := match["remote_ip"]; ok {
			return true
		}
		if _, ok := match["client_ip"]; ok {
			return true
		}
		if notRaw, ok := match["not"]; ok {
			var nots []map[string]json.RawMessage
			if json.Unmarshal(notRaw, &nots) == nil {
				for _, n := range nots {
					if _, ok := n["remote_ip"]; ok {
						return true
					}
					if _, ok := n["client_ip"]; ok {
						return true
					}
				}
			}
		}
	}
	return false
}

func parseIPFilteringSubroute(routes []struct {
	Match  []json.RawMessage `json:"match"`
	Handle []json.RawMessage `json:"handle"`
}, toggles *RouteToggles) {
	if len(routes) == 0 {
		return
	}
	toggles.IPFiltering.Enabled = true
	for _, m := range routes[0].Match {
		var match map[string]json.RawMessage
		if json.Unmarshal(m, &match) != nil {
			continue
		}
		if _, ok := match["remote_ip"]; ok {
			toggles.IPFiltering.Type = "blacklist"
			toggles.IPFiltering.Matcher = "remote_ip"
			return
		}
		if _, ok := match["client_ip"]; ok {
			toggles.IPFiltering.Type = "blacklist"
			toggles.IPFiltering.Matcher = "client_ip"
			return
		}
		if notRaw, ok := match["not"]; ok {
			toggles.IPFiltering.Type = "whitelist"
			var nots []map[string]json.RawMessage
			if json.Unmarshal(notRaw, &nots) == nil {
				for _, n := range nots {
					if _, ok := n["client_ip"]; ok {
						toggles.IPFiltering.Matcher = "client_ip"
						return
					}
				}
			}
			toggles.IPFiltering.Matcher = "remote_ip"
			return
		}
	}
}

func parseHandlers(handlers []json.RawMessage, p *DomainParams) {
	for _, h := range handlers {
		var handler struct {
			Handler       string          `json:"handler"`
			FlushInterval *json.Number    `json:"flush_interval,omitempty"`
			Transport     json.RawMessage `json:"transport,omitempty"`
			Upstreams     []struct {
				Dial string `json:"dial"`
			} `json:"upstreams,omitempty"`
			Response        json.RawMessage `json:"response,omitempty"`
			Encodings       json.RawMessage `json:"encodings,omitempty"`
			Providers       json.RawMessage `json:"providers,omitempty"`
			LB              json.RawMessage `json:"load_balancing,omitempty"`
			StripPathPrefix string          `json:"strip_path_prefix,omitempty"`
			URI             string          `json:"uri,omitempty"`
		}
		if err := json.Unmarshal(h, &handler); err != nil {
			continue
		}

		switch handler.Handler {
		case "reverse_proxy":
			p.HandlerType = "reverse_proxy"
			if len(handler.Upstreams) > 0 {
				p.Upstream = handler.Upstreams[0].Dial
				for _, u := range handler.Upstreams[1:] {
					p.Toggles.LoadBalancing.Upstreams = append(p.Toggles.LoadBalancing.Upstreams, u.Dial)
				}
			}
			if handler.Transport != nil {
				var t struct {
					TLS struct {
						InsecureSkipVerify bool `json:"insecure_skip_verify"`
					} `json:"tls"`
				}
				if json.Unmarshal(handler.Transport, &t) == nil {
					p.Toggles.TLSSkipVerify = t.TLS.InsecureSkipVerify
				}
			}
			if handler.FlushInterval != nil {
				if v, err := handler.FlushInterval.Int64(); err == nil && v == -1 {
					p.Toggles.WebSocketPassthru = true
				}
			}
			if handler.LB != nil {
				var lb struct {
					SelectionPolicy struct {
						Policy string `json:"policy"`
					} `json:"selection_policy"`
				}
				if json.Unmarshal(handler.LB, &lb) == nil && lb.SelectionPolicy.Policy != "" {
					p.Toggles.LoadBalancing.Enabled = true
					p.Toggles.LoadBalancing.Strategy = lb.SelectionPolicy.Policy
				}
			}
			var hcRaw struct {
				HealthChecks struct {
					Active struct {
						URI          string `json:"uri"`
						Interval     string `json:"interval"`
						Timeout      string `json:"timeout"`
						Port         int    `json:"port"`
						ExpectStatus int    `json:"expect_status"`
						ExpectBody   string `json:"expect_body"`
					} `json:"active"`
					Passive struct {
						FailDuration          string `json:"fail_duration"`
						MaxFails              int    `json:"max_fails"`
						UnhealthyStatus       []int  `json:"unhealthy_status"`
						UnhealthyLatency      string `json:"unhealthy_latency"`
						UnhealthyRequestCount int    `json:"unhealthy_request_count"`
					} `json:"passive"`
				} `json:"health_checks"`
			}
			if json.Unmarshal(h, &hcRaw) == nil {
				hc := hcRaw.HealthChecks
				hasActive := hc.Active.URI != "" || hc.Active.Interval != "" || hc.Active.Timeout != "" || hc.Active.Port != 0 || hc.Active.ExpectStatus != 0 || hc.Active.ExpectBody != ""
				hasPassive := hc.Passive.FailDuration != "" || hc.Passive.MaxFails != 0 || len(hc.Passive.UnhealthyStatus) > 0 || hc.Passive.UnhealthyLatency != "" || hc.Passive.UnhealthyRequestCount != 0
				if hasActive || hasPassive {
					var rpCfg ReverseProxyConfig
					if len(p.HandlerConfig) > 0 {
						json.Unmarshal(p.HandlerConfig, &rpCfg)
					}
					rpCfg.HealthChecks.Enabled = true
					if hasActive {
						rpCfg.HealthChecks.Active = ActiveHealthCheckConfig{
							Enabled:      true,
							URI:          hc.Active.URI,
							Interval:     hc.Active.Interval,
							Timeout:      hc.Active.Timeout,
							Port:         hc.Active.Port,
							ExpectStatus: hc.Active.ExpectStatus,
							ExpectBody:   hc.Active.ExpectBody,
						}
					}
					if hasPassive {
						rpCfg.HealthChecks.Passive = PassiveHealthCheckConfig{
							Enabled:               true,
							FailDuration:          hc.Passive.FailDuration,
							MaxFails:              hc.Passive.MaxFails,
							UnhealthyStatus:       hc.Passive.UnhealthyStatus,
							UnhealthyLatency:      hc.Passive.UnhealthyLatency,
							UnhealthyRequestCount: hc.Passive.UnhealthyRequestCount,
						}
					}
					if data, err := json.Marshal(rpCfg); err == nil {
						p.HandlerConfig = data
					}
				}
			}
			parseReverseProxyHeaders(h, p)

		case "encode":
			p.Toggles.Compression = true

		case "headers":
			var full struct {
				Request  json.RawMessage `json:"request"`
				Response json.RawMessage `json:"response"`
			}
			if json.Unmarshal(h, &full) != nil {
				continue
			}

			if full.Request != nil {
				parseHeadersRequestBlock(full.Request, p)
			}

			if full.Response != nil {
				parseHeadersResponseBlock(full.Response, p)
			}

		case "subroute":
			var sub struct {
				Routes []struct {
					Match  []json.RawMessage `json:"match"`
					Handle []json.RawMessage `json:"handle"`
				} `json:"routes"`
			}
			if json.Unmarshal(h, &sub) == nil && isCORSSubroute(sub.Routes) {
				p.Toggles.Headers.Response.Enabled = true
				p.Toggles.Headers.Response.CORS = true
				for _, r := range sub.Routes {
					for _, m := range r.Match {
						var match struct {
							Header map[string][]string `json:"header"`
						}
						if json.Unmarshal(m, &match) == nil {
							if origins, ok := match.Header["Origin"]; ok && len(origins) > 0 {
								p.Toggles.Headers.Response.CORSOrigins = append(p.Toggles.Headers.Response.CORSOrigins, origins[0])
							}
						}
					}
				}
				// Extract CORS headers from the first nested route only
				// (all routes have the same headers except per-origin Allow-Origin).
				// Replace the per-origin Allow-Origin with the joined origins list.
				if len(sub.Routes) > 0 {
					for _, nh := range sub.Routes[0].Handle {
						var nested struct {
							Handler  string `json:"handler"`
							Response struct {
								Set map[string][]string `json:"set"`
							} `json:"response"`
						}
						if json.Unmarshal(nh, &nested) == nil && nested.Handler == "headers" {
							// Replace per-origin value with joined origins
							if _, ok := nested.Response.Set["Access-Control-Allow-Origin"]; ok {
								nested.Response.Set["Access-Control-Allow-Origin"] = []string{strings.Join(p.Toggles.Headers.Response.CORSOrigins, ", ")}
							}
							builtin, custom := classifyHeaders(nested.Response.Set, builtinResponseKeys)
							p.Toggles.Headers.Response.Builtin = append(p.Toggles.Headers.Response.Builtin, builtin...)
							p.Toggles.Headers.Response.Custom = append(p.Toggles.Headers.Response.Custom, custom...)
						}
					}
				}
			} else {
				p.Toggles.ForceHTTPS = true
			}

		case "authentication":
			p.Toggles.Auth.Mode = "basic"
			if handler.Providers != nil {
				var providers struct {
					HTTPBasic struct {
						Accounts []struct {
							Username string `json:"username"`
							Password string `json:"password"`
						} `json:"accounts"`
					} `json:"http_basic"`
				}
				if json.Unmarshal(handler.Providers, &providers) == nil && len(providers.HTTPBasic.Accounts) > 0 {
					p.Toggles.Auth.BasicAuth.Username = providers.HTTPBasic.Accounts[0].Username
					p.Toggles.Auth.BasicAuth.PasswordHash = providers.HTTPBasic.Accounts[0].Password
				}
			}

		case "static_response":
			parseStaticResponseHandler(h, p)

		case "file_server":
			parseFileServerHandler(h, p)

		case "error":
			parseErrorHandler(h, p)

		case "rewrite":
			if handler.StripPathPrefix != "" {
				p.StripPathPrefix = handler.StripPathPrefix
			}
			if handler.URI != "" {
				suffix := "{http.request.uri.path}"
				if strings.HasSuffix(handler.URI, suffix) {
					prefix := strings.TrimSuffix(handler.URI, suffix)
					if prefix != "" {
						p.PrependPathPrefix = prefix
					}
				}
			}
		}
	}
}

func parseHeadersRequestBlock(raw json.RawMessage, p *DomainParams) {
	var block struct {
		Set     map[string][]string            `json:"set"`
		Add     map[string][]string            `json:"add"`
		Delete  []string                       `json:"delete"`
		Replace map[string][]map[string]string `json:"replace"`
	}
	if json.Unmarshal(raw, &block) != nil {
		return
	}

	hasContent := len(block.Set) > 0 || len(block.Add) > 0 || len(block.Delete) > 0 || len(block.Replace) > 0
	if !hasContent {
		return
	}

	p.Toggles.Headers.Request.Enabled = true

	if _, ok := block.Set["X-Forwarded-For"]; ok {
		p.Toggles.Headers.Request.XForwardedFor = true
	}
	if _, ok := block.Set["X-Real-IP"]; ok {
		p.Toggles.Headers.Request.XRealIP = true
	}
	if _, ok := block.Set["X-Forwarded-Proto"]; ok {
		p.Toggles.Headers.Request.XForwardedProto = true
	}
	if _, ok := block.Set["X-Forwarded-Host"]; ok {
		p.Toggles.Headers.Request.XForwardedHost = true
	}
	if _, ok := block.Set["X-Request-ID"]; ok {
		p.Toggles.Headers.Request.XRequestID = true
	}

	var entries []HeaderEntry
	appendOpsToEntries(&entries, block.Set, "set")
	appendOpsToEntries(&entries, block.Add, "add")
	appendDeleteEntries(&entries, block.Delete)
	appendReplaceEntries(&entries, block.Replace)

	for _, e := range entries {
		if builtinDomainRequestKeys[e.Key] {
			p.Toggles.Headers.Request.Builtin = append(p.Toggles.Headers.Request.Builtin, e)
		} else {
			p.Toggles.Headers.Request.Custom = append(p.Toggles.Headers.Request.Custom, e)
		}
	}
}

func parseHeadersResponseBlock(raw json.RawMessage, p *DomainParams) {
	var resp struct {
		Set      map[string][]string            `json:"set"`
		Add      map[string][]string            `json:"add"`
		Delete   []string                       `json:"delete"`
		Replace  map[string][]map[string]string `json:"replace"`
		Deferred bool                           `json:"deferred"`
	}
	if json.Unmarshal(raw, &resp) != nil {
		return
	}

	if len(resp.Set) == 0 && len(resp.Add) == 0 && len(resp.Delete) == 0 && len(resp.Replace) == 0 {
		return
	}

	p.Toggles.Headers.Response.Enabled = true
	p.Toggles.Headers.Response.Deferred = resp.Deferred

	if _, ok := resp.Set["X-Content-Type-Options"]; ok {
		p.Toggles.Headers.Response.Security = true
	}
	if origins, ok := resp.Set["Access-Control-Allow-Origin"]; ok {
		p.Toggles.Headers.Response.CORS = true
		if len(origins) > 0 && origins[0] != "*" {
			p.Toggles.Headers.Response.CORSOrigins = []string{origins[0]}
		}
	}
	if _, ok := resp.Set["Cache-Control"]; ok {
		p.Toggles.Headers.Response.CacheControl = true
	}
	if _, ok := resp.Set["X-Robots-Tag"]; ok {
		p.Toggles.Headers.Response.XRobotsTag = true
	}

	builtin, custom := classifyHeaders(resp.Set, builtinResponseKeys)
	p.Toggles.Headers.Response.Builtin = append(p.Toggles.Headers.Response.Builtin, builtin...)
	p.Toggles.Headers.Response.Custom = append(p.Toggles.Headers.Response.Custom, custom...)
}

func parseReverseProxyHeaders(raw json.RawMessage, p *DomainParams) {
	var rp struct {
		Headers struct {
			Request  json.RawMessage `json:"request"`
			Response json.RawMessage `json:"response"`
		} `json:"headers"`
	}
	if json.Unmarshal(raw, &rp) != nil {
		return
	}

	var headerUp HeaderUpConfig
	if len(rp.Headers.Request) > 0 {
		headerUp = parseHeaderUpBlock(rp.Headers.Request)
	}

	var headerDown HeaderDownConfig
	if len(rp.Headers.Response) > 0 {
		headerDown = parseHeaderDownBlock(rp.Headers.Response)
	}

	if !headerUp.Enabled && !headerDown.Enabled {
		return
	}

	// Merge parsed headers into the handler config
	var rpCfg ReverseProxyConfig
	if len(p.HandlerConfig) > 0 {
		json.Unmarshal(p.HandlerConfig, &rpCfg)
	}
	if headerUp.Enabled {
		rpCfg.HeaderUp = headerUp
	}
	if headerDown.Enabled {
		rpCfg.HeaderDown = headerDown
	}
	if data, err := json.Marshal(rpCfg); err == nil {
		p.HandlerConfig = data
	}
}

func parseHeaderUpBlock(raw json.RawMessage) HeaderUpConfig {
	var block struct {
		Set     map[string][]string            `json:"set"`
		Add     map[string][]string            `json:"add"`
		Delete  []string                       `json:"delete"`
		Replace map[string][]map[string]string `json:"replace"`
	}
	if json.Unmarshal(raw, &block) != nil {
		return HeaderUpConfig{}
	}

	hasContent := len(block.Set) > 0 || len(block.Add) > 0 || len(block.Delete) > 0 || len(block.Replace) > 0
	if !hasContent {
		return HeaderUpConfig{}
	}

	cfg := HeaderUpConfig{Enabled: true}

	if vals, ok := block.Set["Host"]; ok && len(vals) > 0 {
		cfg.HostOverride = true
		cfg.HostValue = vals[0]
	}
	if vals, ok := block.Set["Authorization"]; ok && len(vals) > 0 {
		cfg.Authorization = true
		cfg.AuthValue = vals[0]
	}

	var entries []HeaderEntry
	appendOpsToEntries(&entries, block.Set, "set")
	appendOpsToEntries(&entries, block.Add, "add")
	appendDeleteEntries(&entries, block.Delete)
	appendReplaceEntries(&entries, block.Replace)

	for _, e := range entries {
		if builtinHeaderUpKeys[e.Key] {
			cfg.Builtin = append(cfg.Builtin, e)
		} else {
			cfg.Custom = append(cfg.Custom, e)
		}
	}

	return cfg
}

func parseHeaderDownBlock(raw json.RawMessage) HeaderDownConfig {
	var block struct {
		Set      map[string][]string            `json:"set"`
		Add      map[string][]string            `json:"add"`
		Delete   []string                       `json:"delete"`
		Replace  map[string][]map[string]string `json:"replace"`
		Deferred bool                           `json:"deferred"`
	}
	if json.Unmarshal(raw, &block) != nil {
		return HeaderDownConfig{}
	}

	hasContent := len(block.Set) > 0 || len(block.Add) > 0 || len(block.Delete) > 0 || len(block.Replace) > 0
	if !hasContent {
		return HeaderDownConfig{}
	}

	cfg := HeaderDownConfig{Enabled: true, Deferred: block.Deferred}

	for _, key := range block.Delete {
		if key == "Server" {
			cfg.StripServer = true
		}
		if key == "X-Powered-By" {
			cfg.StripPoweredBy = true
		}
	}

	var entries []HeaderEntry
	appendOpsToEntries(&entries, block.Set, "set")
	appendOpsToEntries(&entries, block.Add, "add")
	appendDeleteEntries(&entries, block.Delete)
	appendReplaceEntries(&entries, block.Replace)

	for _, e := range entries {
		if builtinHeaderDownKeys[e.Key] {
			cfg.Builtin = append(cfg.Builtin, e)
		} else {
			cfg.Custom = append(cfg.Custom, e)
		}
	}

	return cfg
}

const requestURIPlaceholder = "{http.request.uri}"

func parseFileServerHandler(raw json.RawMessage, p *DomainParams) {
	var fs struct {
		Root       string   `json:"root"`
		Browse     any      `json:"browse"`
		IndexNames []string `json:"index_names"`
		Hide       []string `json:"hide"`
	}
	if json.Unmarshal(raw, &fs) != nil {
		return
	}

	cfg := FileServerConfig{
		Root:       fs.Root,
		Browse:     fs.Browse != nil,
		IndexNames: fs.IndexNames,
		Hide:       fs.Hide,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	p.HandlerType = "file_server"
	p.HandlerConfig = data
}

func parseStaticResponseHandler(raw json.RawMessage, p *DomainParams) {
	var sr struct {
		StatusCode string              `json:"status_code"`
		Body       string              `json:"body"`
		Headers    map[string][]string `json:"headers"`
		Close      bool                `json:"close"`
	}
	if json.Unmarshal(raw, &sr) != nil {
		return
	}

	locations, hasLocation := sr.Headers["Location"]
	if hasLocation && len(locations) > 0 {
		target := locations[0]
		preservePath := false
		if strings.HasSuffix(target, requestURIPlaceholder) {
			preservePath = true
			target = strings.TrimSuffix(target, requestURIPlaceholder)
			target = strings.TrimRight(target, "/")
		}
		cfg := RedirectConfig{
			TargetURL:    target,
			StatusCode:   sr.StatusCode,
			PreservePath: preservePath,
		}
		data, err := json.Marshal(cfg)
		if err != nil {
			return
		}
		p.HandlerType = "redirect"
		p.HandlerConfig = data
		return
	}

	cfg := StaticResponseConfig{
		StatusCode: sr.StatusCode,
		Body:       sr.Body,
		Headers:    sr.Headers,
		Close:      sr.Close,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	p.HandlerType = "static_response"
	p.HandlerConfig = data
}

func parseErrorHandler(raw json.RawMessage, p *DomainParams) {
	var eh struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"error"`
	}
	if json.Unmarshal(raw, &eh) != nil {
		return
	}

	cfg := ErrorConfig{
		StatusCode: strconv.Itoa(eh.StatusCode),
		Message:    eh.Message,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	p.HandlerType = "error"
	p.HandlerConfig = data
}
