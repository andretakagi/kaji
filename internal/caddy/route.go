// Route building and parsing. Turns RouteParams into Caddy JSON and back.
package caddy

import (
	"encoding/json"
	"fmt"
	"regexp"
)

type RouteParams struct {
	ID         string       `json:"@id"`
	Domain     string       `json:"domain"`
	Upstream   string       `json:"upstream"`
	Toggles    RouteToggles `json:"toggles"`
	IPListIPs  []string     `json:"-"`
	IPListType string       `json:"-"`
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
	SecurityHeaders   bool            `json:"security_headers"`
	CORS              CORSOpts        `json:"cors"`
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

type CORSOpts struct {
	Enabled        bool     `json:"enabled"`
	AllowedOrigins []string `json:"allowed_origins"`
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

func BuildRoute(p RouteParams) (json.RawMessage, error) {
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

	// Security headers
	if p.Toggles.SecurityHeaders {
		handlers = append(handlers, map[string]any{
			"handler": "headers",
			"response": map[string]any{
				"set": map[string][]string{
					"Strict-Transport-Security": {"max-age=31536000; includeSubDomains; preload"},
					"X-Content-Type-Options":    {"nosniff"},
					"X-Frame-Options":           {"DENY"},
					"Referrer-Policy":           {"strict-origin-when-cross-origin"},
					"Permissions-Policy":        {"accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()"},
				},
			},
		})
	}

	// CORS headers
	if p.Toggles.CORS.Enabled {
		corsHeaders := map[string][]string{
			"Access-Control-Allow-Methods": {"GET, POST, PUT, DELETE, OPTIONS"},
			"Access-Control-Allow-Headers": {"Content-Type, Authorization"},
		}

		if len(p.Toggles.CORS.AllowedOrigins) <= 1 {
			origin := "*"
			if len(p.Toggles.CORS.AllowedOrigins) == 1 {
				origin = p.Toggles.CORS.AllowedOrigins[0]
			}
			corsHeaders["Access-Control-Allow-Origin"] = []string{origin}
			handlers = append(handlers, map[string]any{
				"handler":  "headers",
				"response": map[string]any{"set": corsHeaders},
			})
		} else {
			// Multiple origins need conditional matching since
			// Access-Control-Allow-Origin only accepts one value.
			// Each origin gets its own route that checks the request
			// Origin header and responds with that specific origin.
			var routes []map[string]any
			for _, o := range p.Toggles.CORS.AllowedOrigins {
				h := map[string][]string{
					"Access-Control-Allow-Origin": {o},
					"Vary":                        {"Origin"},
				}
				for k, v := range corsHeaders {
					h[k] = v
				}
				routes = append(routes, map[string]any{
					"match": []map[string]any{
						{"header": map[string][]string{"Origin": {o}}},
					},
					"handle": []map[string]any{
						{"handler": "headers", "response": map[string]any{"set": h}},
					},
				})
			}
			handlers = append(handlers, map[string]any{
				"handler": "subroute",
				"routes":  routes,
			})
		}
	}

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

// ParseRouteParams extracts RouteParams from an existing Caddy route JSON.
// Used to populate the toggle panel from a live route.
//
// Handles two route structures:
//   - Admin API routes (Kaji-created): handlers are a flat array at the top level
//   - Caddyfile-adapted routes: all handlers are wrapped in a single top-level
//     subroute, with each handler in its own nested route
func ParseRouteParams(raw json.RawMessage) (RouteParams, error) {
	var route struct {
		ID    string `json:"@id"`
		Match []struct {
			Host []string `json:"host"`
		} `json:"match"`
		Handle []json.RawMessage `json:"handle"`
	}
	if err := json.Unmarshal(raw, &route); err != nil {
		return RouteParams{}, err
	}

	p := RouteParams{ID: route.ID}
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

func parseHandlers(handlers []json.RawMessage, p *RouteParams) {
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

		case "encode":
			p.Toggles.Compression = true

		case "headers":
			if handler.Response != nil {
				var resp struct {
					Set map[string][]string `json:"set"`
				}
				if json.Unmarshal(handler.Response, &resp) == nil {
					if _, ok := resp.Set["X-Content-Type-Options"]; ok {
						p.Toggles.SecurityHeaders = true
					}
					if origins, ok := resp.Set["Access-Control-Allow-Origin"]; ok {
						p.Toggles.CORS.Enabled = true
						if len(origins) > 0 && origins[0] != "*" {
							p.Toggles.CORS.AllowedOrigins = []string{origins[0]}
						}
					}
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
				p.Toggles.CORS.Enabled = true
				for _, r := range sub.Routes {
					for _, m := range r.Match {
						var match struct {
							Header map[string][]string `json:"header"`
						}
						if json.Unmarshal(m, &match) == nil {
							if origins, ok := match.Header["Origin"]; ok && len(origins) > 0 {
								p.Toggles.CORS.AllowedOrigins = append(p.Toggles.CORS.AllowedOrigins, origins[0])
							}
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
		}
	}
}
