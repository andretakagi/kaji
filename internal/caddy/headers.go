package caddy

import (
	"strings"
)

var securityHeaders = map[string][]string{
	"Strict-Transport-Security": {"max-age=31536000; includeSubDomains; preload"},
	"X-Content-Type-Options":    {"nosniff"},
	"X-Frame-Options":           {"DENY"},
	"Referrer-Policy":           {"strict-origin-when-cross-origin"},
	"Permissions-Policy":        {"accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()"},
}

var baseCORSHeaders = map[string][]string{
	"Access-Control-Allow-Methods": {"GET, POST, PUT, DELETE, OPTIONS"},
	"Access-Control-Allow-Headers": {"Content-Type, Authorization"},
}

func buildResponseHeaders(cfg HeadersConfig, advancedMode bool) []any {
	if !cfg.Enabled {
		return nil
	}
	if advancedMode {
		return buildAdvancedResponseHeaders(cfg.Response)
	}
	return buildBasicResponseHeaders(cfg.Response)
}

func buildBasicResponseHeaders(resp ResponseHeaders) []any {
	headerSet := make(map[string][]string)

	if resp.Security {
		for k, v := range securityHeaders {
			headerSet[k] = v
		}
	}
	if resp.CacheControl {
		headerSet["Cache-Control"] = []string{"no-store"}
	}
	if resp.XRobotsTag {
		headerSet["X-Robots-Tag"] = []string{"noindex, nofollow"}
	}

	var handlers []any

	if len(headerSet) > 0 {
		handlers = append(handlers, map[string]any{
			"handler":  "headers",
			"response": map[string]any{"set": headerSet},
		})
	}

	if resp.CORS {
		handlers = append(handlers, buildCORSHandlers(resp.CORSOrigins)...)
	}

	return handlers
}

func buildAdvancedResponseHeaders(resp ResponseHeaders) []any {
	merged := make(map[string][]string)

	for _, entry := range resp.Builtin {
		if entry.Enabled && entry.Key != "" {
			merged[entry.Key] = []string{entry.Value}
		}
	}
	// Custom entries override built-in on key conflict
	for _, entry := range resp.Custom {
		if entry.Enabled && entry.Key != "" {
			merged[entry.Key] = []string{entry.Value}
		}
	}

	// Separate CORS origin header for special multi-origin handling
	var corsOrigins []string
	hasCORS := false
	if vals, ok := merged["Access-Control-Allow-Origin"]; ok {
		hasCORS = true
		raw := vals[0]
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				corsOrigins = append(corsOrigins, trimmed)
			}
		}
		delete(merged, "Access-Control-Allow-Origin")
	}

	var handlers []any

	if len(merged) > 0 {
		handlers = append(handlers, map[string]any{
			"handler":  "headers",
			"response": map[string]any{"set": merged},
		})
	}

	if hasCORS {
		handlers = append(handlers, buildCORSHandlers(corsOrigins)...)
	}

	return handlers
}

func buildCORSHandlers(origins []string) []any {
	corsHeaders := make(map[string][]string)
	for k, v := range baseCORSHeaders {
		corsHeaders[k] = v
	}

	if len(origins) <= 1 {
		origin := "*"
		if len(origins) == 1 {
			origin = origins[0]
		}
		corsHeaders["Access-Control-Allow-Origin"] = []string{origin}
		return []any{
			map[string]any{
				"handler":  "headers",
				"response": map[string]any{"set": corsHeaders},
			},
		}
	}

	// Multiple origins need conditional matching since
	// Access-Control-Allow-Origin only accepts one value.
	var routes []map[string]any
	for _, o := range origins {
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
	return []any{
		map[string]any{
			"handler": "subroute",
			"routes":  routes,
		},
	}
}

func buildRequestHeaderSet(cfg HeadersConfig, advancedMode bool) map[string][]string {
	if !cfg.Enabled {
		return nil
	}
	if advancedMode {
		return buildAdvancedRequestHeaders(cfg.Request)
	}
	return buildBasicRequestHeaders(cfg.Request)
}

func buildBasicRequestHeaders(req RequestHeaders) map[string][]string {
	result := make(map[string][]string)

	if req.HostOverride && req.HostValue != "" {
		result["Host"] = []string{req.HostValue}
	}
	if req.Authorization && req.AuthValue != "" {
		result["Authorization"] = []string{req.AuthValue}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func buildAdvancedRequestHeaders(req RequestHeaders) map[string][]string {
	merged := make(map[string][]string)

	for _, entry := range req.Builtin {
		if entry.Enabled && entry.Key != "" {
			merged[entry.Key] = []string{entry.Value}
		}
	}
	for _, entry := range req.Custom {
		if entry.Enabled && entry.Key != "" {
			merged[entry.Key] = []string{entry.Value}
		}
	}

	if len(merged) == 0 {
		return nil
	}
	return merged
}
