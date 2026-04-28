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
	ID              string          `json:"@id"`
	Domain          string          `json:"domain"`
	Upstream        string          `json:"upstream"`
	HandlerType     string          `json:"handler_type"`
	HandlerConfig   json.RawMessage `json:"handler_config,omitempty"`
	Toggles         RouteToggles    `json:"toggles"`
	AdvancedHeaders bool            `json:"-"`
	IPListIPs       []string        `json:"-"`
	IPListType      string          `json:"-"`
}

type IPFilteringOpts struct {
	Enabled bool   `json:"enabled"`
	ListID  string `json:"list_id"`
	Type    string `json:"type"`
}

type RouteToggles struct {
	Enabled           bool            `json:"enabled"`
	ForceHTTPS        bool            `json:"force_https"`
	Compression       bool            `json:"compression"`
	Headers           HeadersConfig   `json:"headers"`
	RequestHeaders    RequestHeaders  `json:"request_headers"`
	TLSSkipVerify     bool            `json:"tls_skip_verify"`
	BasicAuth         BasicAuth       `json:"basic_auth"`
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
	Response ResponseHeaders `json:"response"`
}

type ResponseHeaders struct {
	Enabled      bool          `json:"enabled"`
	Security     bool          `json:"security"`
	CORS         bool          `json:"cors"`
	CORSOrigins  []string      `json:"cors_origins"`
	CacheControl bool          `json:"cache_control"`
	XRobotsTag   bool          `json:"x_robots_tag"`
	Builtin      []HeaderEntry `json:"builtin"`
	Custom       []HeaderEntry `json:"custom"`
}

type RequestHeaders struct {
	Enabled       bool          `json:"enabled"`
	HostOverride  bool          `json:"host_override"`
	HostValue     string        `json:"host_value"`
	Authorization bool          `json:"authorization"`
	AuthValue     string        `json:"auth_value"`
	Builtin       []HeaderEntry `json:"builtin"`
	Custom        []HeaderEntry `json:"custom"`
}

type HeaderEntry struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type BasicAuth struct {
	Enabled      bool   `json:"enabled"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Password     string `json:"password,omitempty"`
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
		ipMatcher := map[string]any{
			"remote_ip": map[string]any{"ranges": p.IPListIPs},
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

	// Response headers (security, CORS, cache-control, x-robots-tag, advanced)
	handlers = append(handlers, buildResponseHeaders(p.Toggles.Headers, p.AdvancedHeaders)...)

	// Basic auth
	if p.Toggles.BasicAuth.Enabled && p.Toggles.BasicAuth.Username != "" {
		handlers = append(handlers, map[string]any{
			"handler": "authentication",
			"providers": map[string]any{
				"http_basic": map[string]any{
					"accounts": []map[string]string{
						{
							"username": p.Toggles.BasicAuth.Username,
							"password": p.Toggles.BasicAuth.PasswordHash,
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
		if strategy == "first" {
			rp["health_checks"] = map[string]any{
				"passive": map[string]any{
					"fail_duration": "30s",
					"max_fails":     3,
				},
			}
		}
	}

	if reqHeaders := BuildRequestHeaders(p.Toggles.RequestHeaders, p.AdvancedHeaders); reqHeaders != nil {
		rp["headers"] = map[string]any{
			"request": map[string]any{
				"set": reqHeaders,
			},
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
		if notRaw, ok := match["not"]; ok {
			var nots []map[string]json.RawMessage
			if json.Unmarshal(notRaw, &nots) == nil {
				for _, n := range nots {
					if _, ok := n["remote_ip"]; ok {
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
			return
		}
		if _, ok := match["not"]; ok {
			toggles.IPFiltering.Type = "whitelist"
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
			Response  json.RawMessage `json:"response,omitempty"`
			Encodings json.RawMessage `json:"encodings,omitempty"`
			Providers json.RawMessage `json:"providers,omitempty"`
			LB        json.RawMessage `json:"load_balancing,omitempty"`
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
			parseReverseProxyRequestHeaders(h, p)

		case "encode":
			p.Toggles.Compression = true

		case "headers":
			if handler.Response != nil {
				var resp struct {
					Set map[string][]string `json:"set"`
				}
				if json.Unmarshal(handler.Response, &resp) == nil && len(resp.Set) > 0 {
					p.Toggles.Headers.Response.Enabled = true
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
			p.Toggles.BasicAuth.Enabled = true
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
					p.Toggles.BasicAuth.Username = providers.HTTPBasic.Accounts[0].Username
					p.Toggles.BasicAuth.PasswordHash = providers.HTTPBasic.Accounts[0].Password
				}
			}

		case "static_response":
			parseStaticResponseHandler(h, p)

		case "file_server":
			parseFileServerHandler(h, p)

		case "error":
			parseErrorHandler(h, p)
		}
	}
}

func parseReverseProxyRequestHeaders(raw json.RawMessage, p *DomainParams) {
	var rp struct {
		Headers struct {
			Request struct {
				Set map[string][]string `json:"set"`
			} `json:"request"`
		} `json:"headers"`
	}
	if json.Unmarshal(raw, &rp) != nil {
		return
	}
	reqSet := rp.Headers.Request.Set
	if len(reqSet) == 0 {
		return
	}

	p.Toggles.RequestHeaders.Enabled = true

	if vals, ok := reqSet["Host"]; ok && len(vals) > 0 {
		p.Toggles.RequestHeaders.HostOverride = true
		p.Toggles.RequestHeaders.HostValue = vals[0]
	}
	if vals, ok := reqSet["Authorization"]; ok && len(vals) > 0 {
		p.Toggles.RequestHeaders.Authorization = true
		p.Toggles.RequestHeaders.AuthValue = vals[0]
	}

	builtin, custom := classifyHeaders(reqSet, builtinRequestKeys)
	p.Toggles.RequestHeaders.Builtin = append(p.Toggles.RequestHeaders.Builtin, builtin...)
	p.Toggles.RequestHeaders.Custom = append(p.Toggles.RequestHeaders.Custom, custom...)
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
		Message    string `json:"message"`
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
