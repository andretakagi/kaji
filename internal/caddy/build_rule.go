package caddy

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type RuleBuildParams struct {
	RuleID          string
	MatchType       string // "", "path"
	PathMatch       string // "exact", "prefix", "regex"
	MatchValue      string
	HandlerType     string
	HandlerConfig   json.RawMessage
	AdvancedHeaders bool
}

func BuildRuleDomain(domainName string, rule RuleBuildParams, toggles DomainToggles, ipListIPs []string, ipListType string, ipMatcherType string, logSkip bool) (json.RawMessage, error) {
	if domainName == "" {
		return nil, fmt.Errorf("domain name is required")
	}
	routeID := CaddyDomainID(rule.RuleID)

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
		matcherKey := ipMatcherKey(ipMatcherType)
		ipMatcher := map[string]any{
			matcherKey: map[string]any{"ranges": ipListIPs},
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
	if toggles.Auth.Mode == "basic" && toggles.Auth.BasicAuth.Username != "" {
		handlers = append(handlers, map[string]any{
			"handler": "authentication",
			"providers": map[string]any{
				"http_basic": map[string]any{
					"accounts": []map[string]string{
						{
							"username": toggles.Auth.BasicAuth.Username,
							"password": toggles.Auth.BasicAuth.PasswordHash,
						},
					},
					"hash": map[string]any{
						"algorithm": "bcrypt",
					},
				},
			},
		})
	}

	// Path rewriting (reverse proxy only)
	if rule.HandlerType == "reverse_proxy" {
		var rpCfg ReverseProxyConfig
		if err := json.Unmarshal(rule.HandlerConfig, &rpCfg); err == nil {
			if rpCfg.StripPathPrefix != "" {
				handlers = append(handlers, map[string]any{
					"handler":           "rewrite",
					"strip_path_prefix": rpCfg.StripPathPrefix,
				})
			}
			if rpCfg.PrependPathPrefix != "" {
				handlers = append(handlers, map[string]any{
					"handler": "rewrite",
					"uri":     rpCfg.PrependPathPrefix + "{http.request.uri.path}",
				})
			}
		}
	}

	// Terminal handler (always last)
	switch rule.HandlerType {
	case "reverse_proxy":
		rpHandler, err := buildReverseProxyHandler(rule.HandlerConfig, rule.AdvancedHeaders)
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, rpHandler)
	case "static_response":
		srHandler, err := buildStaticResponseHandler(rule.HandlerConfig)
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, srHandler)
	case "redirect":
		rdHandler, err := buildRedirectHandler(rule.HandlerConfig)
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, rdHandler)
	case "file_server":
		fsHandler, err := buildFileServerHandler(rule.HandlerConfig)
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, fsHandler)
	case "error":
		errHandler, err := buildErrorHandler(rule.HandlerConfig)
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, errHandler)
	default:
		return nil, fmt.Errorf("unsupported handler type: %q", rule.HandlerType)
	}

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

func buildReverseProxyHandler(handlerConfig json.RawMessage, advancedHeaders bool) (map[string]any, error) {
	var rpCfg ReverseProxyConfig
	if err := json.Unmarshal(handlerConfig, &rpCfg); err != nil {
		return nil, fmt.Errorf("parsing reverse proxy config: %w", err)
	}
	if rpCfg.Upstream == "" {
		return nil, fmt.Errorf("upstream is required")
	}

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

	var rpHeaders map[string]any
	if up := BuildHeaderUp(rpCfg.HeaderUp, advancedHeaders); up != nil {
		if rpHeaders == nil {
			rpHeaders = make(map[string]any)
		}
		rpHeaders["request"] = up
	}
	if down := BuildHeaderDown(rpCfg.HeaderDown, advancedHeaders); down != nil {
		if rpHeaders == nil {
			rpHeaders = make(map[string]any)
		}
		rpHeaders["response"] = down
	}
	if rpHeaders != nil {
		rp["headers"] = rpHeaders
	}

	return rp, nil
}

func buildStaticResponseHandler(handlerConfig json.RawMessage) (map[string]any, error) {
	var srCfg StaticResponseConfig
	if err := json.Unmarshal(handlerConfig, &srCfg); err != nil {
		return nil, fmt.Errorf("parsing static response config: %w", err)
	}

	sr := map[string]any{
		"handler": "static_response",
	}

	if srCfg.Close {
		sr["close"] = true
		return sr, nil
	}

	if srCfg.StatusCode != "" {
		sr["status_code"] = srCfg.StatusCode
	}
	if srCfg.Body != "" {
		sr["body"] = srCfg.Body
	}
	if len(srCfg.Headers) > 0 {
		sr["headers"] = srCfg.Headers
	}

	return sr, nil
}

func buildRedirectHandler(handlerConfig json.RawMessage) (map[string]any, error) {
	var cfg RedirectConfig
	if err := json.Unmarshal(handlerConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parsing redirect config: %w", err)
	}
	if cfg.TargetURL == "" {
		return nil, fmt.Errorf("target URL is required")
	}

	target := cfg.TargetURL
	if cfg.PreservePath {
		target = strings.TrimRight(target, "/") + "{http.request.uri}"
	}

	return map[string]any{
		"handler":     "static_response",
		"status_code": cfg.StatusCode,
		"headers": map[string][]string{
			"Location": {target},
		},
	}, nil
}

func buildFileServerHandler(handlerConfig json.RawMessage) (map[string]any, error) {
	var cfg FileServerConfig
	if err := json.Unmarshal(handlerConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parsing file server config: %w", err)
	}
	if cfg.Root == "" {
		return nil, fmt.Errorf("root directory is required")
	}

	fs := map[string]any{
		"handler": "file_server",
		"root":    cfg.Root,
	}

	if cfg.Browse {
		fs["browse"] = map[string]any{}
	}
	if len(cfg.IndexNames) > 0 {
		fs["index_names"] = cfg.IndexNames
	}
	if len(cfg.Hide) > 0 {
		fs["hide"] = cfg.Hide
	}

	return fs, nil
}

func buildErrorHandler(handlerConfig json.RawMessage) (map[string]any, error) {
	var cfg ErrorConfig
	if err := json.Unmarshal(handlerConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parsing error handler config: %w", err)
	}

	statusCode, err := strconv.Atoi(cfg.StatusCode)
	if err != nil {
		return nil, fmt.Errorf("invalid status code %q: %w", cfg.StatusCode, err)
	}

	handler := map[string]any{
		"handler":     "error",
		"status_code": statusCode,
	}
	if cfg.Message != "" {
		handler["error"] = cfg.Message
	}

	return handler, nil
}

func buildMatchBlock(domainName string, rule RuleBuildParams) []map[string]any {
	match := map[string]any{
		"host": []string{domainName},
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
