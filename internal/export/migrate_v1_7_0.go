package export

import (
	"encoding/json"
	"regexp"
)

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.7.0",
		Summary: "Convert flat routes to domain-centric model",
		Fn:      migrateV170,
	})
}

func migrateV170(m map[string]any) []string {
	var changes []string

	// Convert disabled_routes to domain entries
	if raw, ok := m["disabled_routes"]; ok {
		if routes, ok := raw.([]any); ok && len(routes) > 0 {
			domains := ensureDomains(m)
			for _, entry := range routes {
				route, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				domain := disabledRouteToDomain(route)
				if domain != nil {
					domains = append(domains, domain)
				}
			}
			m["domains"] = domains
			changes = append(changes, "converted disabled_routes to domain entries")
		}
	}

	if c := removeField(m, "disabled_routes"); c != "" {
		changes = append(changes, c)
	}

	if c := removeField(m, "route_settings"); c != "" {
		changes = append(changes, c)
	}

	if c := setDefault(m, "domains", []any{}); c != "" {
		changes = append(changes, c)
	}

	return changes
}

func disabledRouteToDomain(entry map[string]any) map[string]any {
	routeRaw, ok := entry["route"]
	if !ok {
		return nil
	}

	var routeBytes []byte
	switch v := routeRaw.(type) {
	case string:
		routeBytes = []byte(v)
	case json.RawMessage:
		routeBytes = []byte(v)
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		routeBytes = b
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		routeBytes = b
	}

	var route struct {
		Match []struct {
			Host []string `json:"host"`
		} `json:"match"`
		Handle []json.RawMessage `json:"handle"`
	}
	if json.Unmarshal(routeBytes, &route) != nil {
		return nil
	}

	domainName := ""
	if len(route.Match) > 0 && len(route.Match[0].Host) > 0 {
		domainName = route.Match[0].Host[0]
	}
	if domainName == "" {
		return nil
	}

	upstream := extractUpstream(routeBytes)
	sanitized := sanitizeForID(domainName)

	handlerConfig := map[string]any{"upstream": upstream}
	if upstream == "" {
		handlerConfig = map[string]any{"upstream": "localhost:80"}
	}
	hcBytes, _ := json.Marshal(handlerConfig)

	return map[string]any{
		"id":      "dom_migrated_" + sanitized,
		"name":    domainName,
		"enabled": false,
		"toggles": map[string]any{},
		"rules": []any{
			map[string]any{
				"id":             "rule_migrated_" + sanitized,
				"enabled":        true,
				"match_type":     "",
				"handler_type":   "reverse_proxy",
				"handler_config": json.RawMessage(hcBytes),
			},
		},
	}
}

func extractUpstream(routeJSON []byte) string {
	var route struct {
		Handle []json.RawMessage `json:"handle"`
	}
	if json.Unmarshal(routeJSON, &route) != nil {
		return ""
	}

	for _, h := range route.Handle {
		if u := extractUpstreamFromHandler(h); u != "" {
			return u
		}
	}
	return ""
}

func extractUpstreamFromHandler(h json.RawMessage) string {
	var handler struct {
		Handler   string `json:"handler"`
		Upstreams []struct {
			Dial string `json:"dial"`
		} `json:"upstreams"`
		Routes []struct {
			Handle []json.RawMessage `json:"handle"`
		} `json:"routes"`
	}
	if json.Unmarshal(h, &handler) != nil {
		return ""
	}

	if handler.Handler == "reverse_proxy" && len(handler.Upstreams) > 0 {
		return handler.Upstreams[0].Dial
	}

	if handler.Handler == "subroute" {
		for _, route := range handler.Routes {
			for _, nested := range route.Handle {
				if u := extractUpstreamFromHandler(nested); u != "" {
					return u
				}
			}
		}
	}

	return ""
}

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

func sanitizeForID(s string) string {
	return nonAlphanumeric.ReplaceAllString(s, "_")
}

func ensureDomains(m map[string]any) []any {
	if existing, ok := m["domains"]; ok {
		if arr, ok := existing.([]any); ok {
			return arr
		}
	}
	return []any{}
}
