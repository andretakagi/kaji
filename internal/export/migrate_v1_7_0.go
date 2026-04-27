package export

import (
	"encoding/json"
	"fmt"
	"regexp"
)

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.7.0",
		Summary: "Convert flat routes to domain-centric model; split rules into rule + paths",
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

	// Convert subdomain rules to first-class subdomain entities
	domainsRaw, ok := m["domains"]
	if ok {
		if domains, ok := domainsRaw.([]any); ok {
			for _, domRaw := range domains {
				dom, ok := domRaw.(map[string]any)
				if !ok {
					continue
				}
				if c := convertSubdomainRules(dom); c != "" {
					changes = append(changes, c)
				}
			}
		}
	}

	// Split domain rules into rule + paths and lift subdomain handler into subdomain.rule
	changes = append(changes, migrateV170Paths(m)...)

	return changes
}

func migrateV170Paths(m map[string]any) []string {
	var changes []string
	domainsRaw, ok := m["domains"].([]any)
	if !ok {
		return changes
	}
	for _, dRaw := range domainsRaw {
		dom, ok := dRaw.(map[string]any)
		if !ok {
			continue
		}
		domName, _ := dom["name"].(string)
		if domName == "" {
			domName, _ = dom["id"].(string)
		}
		if c := splitDomainRules(dom, domName); c != "" {
			changes = append(changes, c)
		}
		if subsRaw, ok := dom["subdomains"].([]any); ok {
			for _, sRaw := range subsRaw {
				sub, ok := sRaw.(map[string]any)
				if !ok {
					continue
				}
				subName, _ := sub["name"].(string)
				if subName == "" {
					subName, _ = sub["id"].(string)
				}
				target := subName
				if domName != "" {
					target = domName + "/" + subName
				}
				if c := liftSubdomainRule(sub, target); c != "" {
					changes = append(changes, c)
				}
			}
		}
	}
	return changes
}

func splitDomainRules(dom map[string]any, domName string) string {
	rules, _ := dom["rules"].([]any)
	var rootRule map[string]any
	paths := make([]any, 0, len(rules))
	for _, r := range rules {
		rule, ok := r.(map[string]any)
		if !ok {
			continue
		}
		mt, _ := rule["match_type"].(string)
		if mt == "" {
			if rootRule == nil {
				rootRule = ruleToOwnRule(rule)
			}
			continue
		}
		paths = append(paths, ruleToPath(rule))
	}
	if rootRule == nil {
		rootRule = noneRule()
	}
	dom["rule"] = rootRule
	dom["paths"] = paths
	delete(dom, "rules")
	return fmt.Sprintf("split %d legacy %s into domain rule and %d %s for %s",
		len(rules), pluralize(len(rules), "rule", "rules"),
		len(paths), pluralize(len(paths), "path", "paths"),
		domName)
}

func liftSubdomainRule(sub map[string]any, target string) string {
	handlerType, _ := sub["handler_type"].(string)
	if handlerType == "" {
		sub["rule"] = noneRule()
	} else {
		sub["rule"] = map[string]any{
			"handler_type":     sub["handler_type"],
			"handler_config":   sub["handler_config"],
			"advanced_headers": sub["advanced_headers"],
		}
	}
	rules, _ := sub["rules"].([]any)
	paths := make([]any, 0, len(rules))
	for _, r := range rules {
		rule, ok := r.(map[string]any)
		if !ok {
			continue
		}
		paths = append(paths, ruleToPath(rule))
	}
	sub["paths"] = paths
	delete(sub, "handler_type")
	delete(sub, "handler_config")
	delete(sub, "advanced_headers")
	delete(sub, "rules")
	return fmt.Sprintf("lifted handler into subdomain rule and converted %d %s to %s for %s",
		len(rules), pluralize(len(rules), "rule", "rules"),
		pluralize(len(rules), "path", "paths"),
		target)
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func ruleToOwnRule(rule map[string]any) map[string]any {
	return map[string]any{
		"handler_type":     rule["handler_type"],
		"handler_config":   rule["handler_config"],
		"advanced_headers": rule["advanced_headers"],
	}
}

func ruleToPath(rule map[string]any) map[string]any {
	out := map[string]any{
		"id":          rule["id"],
		"label":       rule["label"],
		"enabled":     rule["enabled"],
		"path_match":  rule["path_match"],
		"match_value": rule["match_value"],
		"rule": map[string]any{
			"handler_type":     rule["handler_type"],
			"handler_config":   rule["handler_config"],
			"advanced_headers": rule["advanced_headers"],
		},
	}
	if to, ok := rule["toggle_overrides"]; ok {
		out["toggle_overrides"] = to
	}
	return out
}

func noneRule() map[string]any {
	return map[string]any{
		"handler_type":   "none",
		"handler_config": map[string]any{},
	}
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

func convertSubdomainRules(dom map[string]any) string {
	rulesRaw, ok := dom["rules"]
	if !ok {
		dom["subdomains"] = []any{}
		return ""
	}
	rules, ok := rulesRaw.([]any)
	if !ok {
		dom["subdomains"] = []any{}
		return ""
	}

	var subdomains []any
	remaining := []any{}
	for _, ruleRaw := range rules {
		rule, ok := ruleRaw.(map[string]any)
		if !ok {
			remaining = append(remaining, ruleRaw)
			continue
		}
		matchType, _ := rule["match_type"].(string)
		if matchType != "subdomain" {
			remaining = append(remaining, ruleRaw)
			continue
		}

		name, _ := rule["match_value"].(string)
		if name == "" {
			remaining = append(remaining, ruleRaw)
			continue
		}

		handlerType, _ := rule["handler_type"].(string)
		handlerConfig := rule["handler_config"]
		enabled, _ := rule["enabled"].(bool)
		advancedHeaders, _ := rule["advanced_headers"].(bool)

		toggles := map[string]any{}
		if overrides, ok := rule["toggle_overrides"].(map[string]any); ok {
			toggles = overrides
		} else if domToggles, ok := dom["toggles"].(map[string]any); ok {
			copied, err := json.Marshal(domToggles)
			if err == nil {
				var t map[string]any
				if json.Unmarshal(copied, &t) == nil {
					toggles = t
				}
			}
		}

		sanitized := sanitizeForID(name)
		sub := map[string]any{
			"id":               "sub_migrated_" + sanitized,
			"name":             name,
			"enabled":          enabled,
			"handler_type":     handlerType,
			"handler_config":   handlerConfig,
			"toggles":          toggles,
			"advanced_headers": advancedHeaders,
			"rules":            []any{},
		}
		subdomains = append(subdomains, sub)
	}

	if len(subdomains) == 0 {
		dom["subdomains"] = []any{}
		return ""
	}

	dom["rules"] = remaining
	dom["subdomains"] = subdomains

	domName, _ := dom["name"].(string)
	if domName == "" {
		domName, _ = dom["id"].(string)
	}
	return fmt.Sprintf("converted %d subdomain rules to subdomain entities for %s", len(subdomains), domName)
}
