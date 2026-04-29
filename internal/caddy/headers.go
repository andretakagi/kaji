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
	"Access-Control-Allow-Methods":     {"GET, POST, PUT, DELETE, OPTIONS"},
	"Access-Control-Allow-Headers":     {"Content-Type, Authorization"},
	"Access-Control-Allow-Credentials": {"true"},
}

var builtinResponseKeys = map[string]bool{
	"Strict-Transport-Security":        true,
	"X-Content-Type-Options":           true,
	"X-Frame-Options":                  true,
	"Referrer-Policy":                  true,
	"Permissions-Policy":               true,
	"Cache-Control":                    true,
	"X-Robots-Tag":                     true,
	"Access-Control-Allow-Origin":      true,
	"Access-Control-Allow-Methods":     true,
	"Access-Control-Allow-Headers":     true,
	"Access-Control-Allow-Credentials": true,
	"Content-Security-Policy":          true,
}

var builtinDomainRequestKeys = map[string]bool{
	"X-Forwarded-For":   true,
	"X-Real-IP":         true,
	"X-Forwarded-Proto": true,
	"X-Forwarded-Host":  true,
	"X-Request-ID":      true,
}

var builtinHeaderUpKeys = map[string]bool{
	"Host":          true,
	"Authorization": true,
}

var builtinHeaderDownKeys = map[string]bool{
	"Server":       true,
	"X-Powered-By": true,
}

// groupedOps collects header entries by Caddy operation type.
type groupedOps struct {
	set     map[string][]string
	add     map[string][]string
	del     []string
	replace map[string][]map[string]string
}

func groupByOperation(entries []HeaderEntry) groupedOps {
	g := groupedOps{
		set:     make(map[string][]string),
		add:     make(map[string][]string),
		replace: make(map[string][]map[string]string),
	}
	for _, e := range entries {
		if !e.Enabled || e.Key == "" {
			continue
		}
		switch e.Operation {
		case "add":
			g.add[e.Key] = append(g.add[e.Key], e.Value)
		case "delete":
			g.del = append(g.del, e.Key)
		case "replace":
			g.replace[e.Key] = append(g.replace[e.Key], map[string]string{
				"search":  e.Search,
				"replace": e.Value,
			})
		default:
			g.set[e.Key] = []string{e.Value}
		}
	}
	return g
}

func (g groupedOps) isEmpty() bool {
	return len(g.set) == 0 && len(g.add) == 0 && len(g.del) == 0 && len(g.replace) == 0
}

func (g groupedOps) toMap() map[string]any {
	m := make(map[string]any)
	if len(g.set) > 0 {
		m["set"] = g.set
	}
	if len(g.add) > 0 {
		m["add"] = g.add
	}
	if len(g.del) > 0 {
		m["delete"] = g.del
	}
	if len(g.replace) > 0 {
		m["replace"] = g.replace
	}
	return m
}

func mergeEntries(builtin, custom []HeaderEntry) []HeaderEntry {
	seen := make(map[string]int)
	merged := make([]HeaderEntry, 0, len(builtin)+len(custom))
	for _, e := range builtin {
		seen[e.Key] = len(merged)
		merged = append(merged, e)
	}
	for _, e := range custom {
		if idx, ok := seen[e.Key]; ok {
			merged[idx] = e
		} else {
			merged = append(merged, e)
		}
	}
	return merged
}

func classifyHeaders(headerSet map[string][]string, knownKeys map[string]bool) (builtin, custom []HeaderEntry) {
	for key, vals := range headerSet {
		value := ""
		if len(vals) > 0 {
			value = vals[0]
		}
		entry := HeaderEntry{Key: key, Value: value, Operation: "set", Enabled: true}
		if knownKeys[key] {
			builtin = append(builtin, entry)
		} else {
			custom = append(custom, entry)
		}
	}
	return builtin, custom
}

// --- Response headers (domain-level headers handler) ---

func buildResponseHeaders(cfg HeadersConfig, advancedMode bool) []any {
	if !cfg.Response.Enabled {
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
		responseBlock := map[string]any{"set": headerSet}
		if resp.Deferred {
			responseBlock["deferred"] = true
		}
		handlers = append(handlers, map[string]any{
			"handler":  "headers",
			"response": responseBlock,
		})
	}

	if resp.CORS {
		handlers = append(handlers, buildCORSHandlers(resp.CORSOrigins)...)
	}

	return handlers
}

func buildAdvancedResponseHeaders(resp ResponseHeaders) []any {
	entries := mergeEntries(resp.Builtin, resp.Custom)
	ops := groupByOperation(entries)

	// Separate CORS origin for multi-origin handling
	var corsOrigins []string
	hasCORS := false
	if vals, ok := ops.set["Access-Control-Allow-Origin"]; ok {
		hasCORS = true
		for _, v := range vals {
			parts := strings.Split(v, ",")
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					corsOrigins = append(corsOrigins, trimmed)
				}
			}
		}
		delete(ops.set, "Access-Control-Allow-Origin")
	}

	var handlers []any

	if !ops.isEmpty() {
		responseBlock := ops.toMap()
		if resp.Deferred {
			responseBlock["deferred"] = true
		}
		handlers = append(handlers, map[string]any{
			"handler":  "headers",
			"response": responseBlock,
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

// --- Domain-level request headers (headers handler, request block) ---

func buildDomainRequestHeaders(cfg HeadersConfig, advancedMode bool) map[string]any {
	req := cfg.Request
	if !req.Enabled {
		return nil
	}
	if advancedMode {
		return buildAdvancedDomainRequestHeaders(req)
	}
	return buildBasicDomainRequestHeaders(req)
}

func buildBasicDomainRequestHeaders(req DomainRequestHeaders) map[string]any {
	headerSet := make(map[string][]string)

	if req.XForwardedFor {
		headerSet["X-Forwarded-For"] = []string{"{http.request.remote.host}"}
	}
	if req.XRealIP {
		headerSet["X-Real-IP"] = []string{"{http.request.remote.host}"}
	}
	if req.XForwardedProto {
		headerSet["X-Forwarded-Proto"] = []string{"{http.request.scheme}"}
	}
	if req.XForwardedHost {
		headerSet["X-Forwarded-Host"] = []string{"{http.request.host}"}
	}
	if req.XRequestID {
		headerSet["X-Request-ID"] = []string{"{http.request.uuid}"}
	}

	if len(headerSet) == 0 {
		return nil
	}
	return map[string]any{"set": headerSet}
}

func buildAdvancedDomainRequestHeaders(req DomainRequestHeaders) map[string]any {
	entries := mergeEntries(req.Builtin, req.Custom)
	ops := groupByOperation(entries)
	if ops.isEmpty() {
		return nil
	}
	return ops.toMap()
}

// --- Header Up (reverse_proxy headers.request) ---

func BuildHeaderUp(cfg HeaderUpConfig, advancedMode bool) map[string]any {
	if !cfg.Enabled {
		return nil
	}
	if advancedMode {
		return buildAdvancedHeaderUp(cfg)
	}
	return buildBasicHeaderUp(cfg)
}

func buildBasicHeaderUp(cfg HeaderUpConfig) map[string]any {
	headerSet := make(map[string][]string)

	if cfg.HostOverride && cfg.HostValue != "" {
		headerSet["Host"] = []string{cfg.HostValue}
	}
	if cfg.Authorization && cfg.AuthValue != "" {
		headerSet["Authorization"] = []string{cfg.AuthValue}
	}

	if len(headerSet) == 0 {
		return nil
	}
	return map[string]any{"set": headerSet}
}

func buildAdvancedHeaderUp(cfg HeaderUpConfig) map[string]any {
	entries := mergeEntries(cfg.Builtin, cfg.Custom)
	ops := groupByOperation(entries)
	if ops.isEmpty() {
		return nil
	}
	return ops.toMap()
}

// --- Header Down (reverse_proxy headers.response) ---

func BuildHeaderDown(cfg HeaderDownConfig, advancedMode bool) map[string]any {
	if !cfg.Enabled {
		return nil
	}
	if advancedMode {
		return buildAdvancedHeaderDown(cfg)
	}
	return buildBasicHeaderDown(cfg)
}

func buildBasicHeaderDown(cfg HeaderDownConfig) map[string]any {
	var deletes []string

	if cfg.StripServer {
		deletes = append(deletes, "Server")
	}
	if cfg.StripPoweredBy {
		deletes = append(deletes, "X-Powered-By")
	}

	if len(deletes) == 0 {
		return nil
	}
	result := map[string]any{"delete": deletes}
	if cfg.Deferred {
		result["deferred"] = true
	}
	return result
}

func buildAdvancedHeaderDown(cfg HeaderDownConfig) map[string]any {
	entries := mergeEntries(cfg.Builtin, cfg.Custom)
	ops := groupByOperation(entries)
	if ops.isEmpty() {
		return nil
	}
	result := ops.toMap()
	if cfg.Deferred {
		result["deferred"] = true
	}
	return result
}

// --- Parse helpers (used by route.go parsers) ---

func appendOpsToEntries(entries *[]HeaderEntry, ops map[string][]string, operation string) {
	for key, vals := range ops {
		for _, val := range vals {
			*entries = append(*entries, HeaderEntry{
				Key:       key,
				Value:     val,
				Operation: operation,
				Enabled:   true,
			})
		}
	}
}

func appendDeleteEntries(entries *[]HeaderEntry, keys []string) {
	for _, key := range keys {
		*entries = append(*entries, HeaderEntry{
			Key:       key,
			Operation: "delete",
			Enabled:   true,
		})
	}
}

func appendReplaceEntries(entries *[]HeaderEntry, replacements map[string][]map[string]string) {
	for key, reps := range replacements {
		for _, rep := range reps {
			*entries = append(*entries, HeaderEntry{
				Key:       key,
				Value:     rep["replace"],
				Operation: "replace",
				Search:    rep["search"],
				Enabled:   true,
			})
		}
	}
}
