package caddy

import (
	"encoding/json"
	"fmt"
)

type RuleBuildParams struct {
	RuleID          string
	MatchType       string // "", "subdomain", "path"
	PathMatch       string // "exact", "prefix", "regex"
	MatchValue      string
	HandlerType     string
	HandlerConfig   json.RawMessage
	AdvancedHeaders bool
}

func BuildRuleRoute(domainName string, rule RuleBuildParams, toggles DomainToggles, ipListIPs []string, ipListType string) (json.RawMessage, error) {
	if domainName == "" {
		return nil, fmt.Errorf("domain name is required")
	}
	if rule.HandlerType != "reverse_proxy" {
		return nil, fmt.Errorf("unsupported handler type: %q", rule.HandlerType)
	}

	var rpCfg ReverseProxyConfig
	if err := json.Unmarshal(rule.HandlerConfig, &rpCfg); err != nil {
		return nil, fmt.Errorf("parsing reverse proxy config: %w", err)
	}
	if rpCfg.Upstream == "" {
		return nil, fmt.Errorf("upstream is required")
	}

	routeID := CaddyRouteID(rule.RuleID)

	// Build match block
	matchBlock := buildMatchBlock(domainName, rule)

	var handlers []any

	// Force HTTPS subroute
	if toggles.ForceHTTPS {
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

	// IP filtering subroute
	if len(ipListIPs) > 0 && ipListType != "" {
		ipMatcher := map[string]any{
			"remote_ip": map[string]any{"ranges": ipListIPs},
		}

		var matchList []map[string]any
		if ipListType == "whitelist" {
			matchList = []map[string]any{
				{"not": []map[string]any{ipMatcher}},
			}
		} else {
			matchList = []map[string]any{ipMatcher}
		}

		handlers = append(handlers, map[string]any{
			"handler": "subroute",
			"routes": []map[string]any{
				{
					"match": matchList,
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

	// Compression
	if toggles.Compression {
		handlers = append(handlers, map[string]any{
			"handler": "encode",
			"encodings": map[string]any{
				"gzip": map[string]any{},
				"zstd": map[string]any{},
			},
			"prefer": []string{"zstd", "gzip"},
		})
	}

	// Response headers
	handlers = append(handlers, buildResponseHeaders(toggles.Headers, rule.AdvancedHeaders)...)

	// Basic auth
	if toggles.BasicAuth.Enabled && toggles.BasicAuth.Username != "" {
		handlers = append(handlers, map[string]any{
			"handler": "authentication",
			"providers": map[string]any{
				"http_basic": map[string]any{
					"accounts": []map[string]string{
						{
							"username": toggles.BasicAuth.Username,
							"password": toggles.BasicAuth.PasswordHash,
						},
					},
					"hash": map[string]any{
						"algorithm": "bcrypt",
					},
				},
			},
		})
	}

	// Reverse proxy (always last)
	upstreams := []map[string]string{{"dial": rpCfg.Upstream}}
	if rpCfg.LoadBalancing.Enabled {
		for _, u := range rpCfg.LoadBalancing.Upstreams {
			upstreams = append(upstreams, map[string]string{"dial": u})
		}
	}
	rp := map[string]any{
		"handler":   "reverse_proxy",
		"upstreams": upstreams,
	}

	if rpCfg.TLSSkipVerify {
		rp["transport"] = map[string]any{
			"protocol": "http",
			"tls":      map[string]any{"insecure_skip_verify": true},
		}
	}

	if rpCfg.WebSocketPassthru {
		rp["flush_interval"] = -1
	}

	if rpCfg.LoadBalancing.Enabled {
		strategy := rpCfg.LoadBalancing.Strategy
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

	if reqHeaders := BuildRequestHeaders(rpCfg.RequestHeaders, rule.AdvancedHeaders); reqHeaders != nil {
		rp["headers"] = map[string]any{
			"request": map[string]any{
				"set": reqHeaders,
			},
		}
	}

	handlers = append(handlers, rp)

	route := map[string]any{
		"@id":      routeID,
		"match":    matchBlock,
		"handle":   handlers,
		"terminal": true,
	}

	data, err := json.Marshal(route)
	if err != nil {
		return nil, fmt.Errorf("marshaling route: %w", err)
	}
	return json.RawMessage(data), nil
}

func buildMatchBlock(domainName string, rule RuleBuildParams) []map[string]any {
	host := domainName
	if rule.MatchType == "subdomain" && rule.MatchValue != "" {
		host = rule.MatchValue + "." + domainName
	}

	match := map[string]any{
		"host": []string{host},
	}

	if rule.MatchType == "path" && rule.MatchValue != "" {
		switch rule.PathMatch {
		case "exact":
			match["path"] = []string{rule.MatchValue}
		case "prefix":
			p := rule.MatchValue
			if len(p) > 0 && p[len(p)-1] != '*' {
				if p[len(p)-1] != '/' {
					p += "/"
				}
				p += "*"
			}
			match["path"] = []string{p}
		case "regex":
			match["path_regexp"] = map[string]string{"pattern": rule.MatchValue}
		}
	}

	return []map[string]any{match}
}
