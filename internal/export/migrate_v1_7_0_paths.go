package export

import "fmt"

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.7.0",
		Summary: "Split domain rules into rule + paths; lift subdomain handler into subdomain.rule",
		Fn:      migrateV170Paths,
	})
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
		name, _ := dom["name"].(string)
		if c := splitDomainRules(dom); c != "" {
			changes = append(changes, fmt.Sprintf("%s: %s", name, c))
		}
		if subsRaw, ok := dom["subdomains"].([]any); ok {
			for _, sRaw := range subsRaw {
				sub, ok := sRaw.(map[string]any)
				if !ok {
					continue
				}
				if c := liftSubdomainRule(sub); c != "" {
					changes = append(changes, fmt.Sprintf("%s/%v: %s", name, sub["name"], c))
				}
			}
		}
	}
	return changes
}

func splitDomainRules(dom map[string]any) string {
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
	return fmt.Sprintf("split %d legacy rules into rule + %d paths", len(rules), len(paths))
}

func liftSubdomainRule(sub map[string]any) string {
	sub["rule"] = map[string]any{
		"handler_type":     sub["handler_type"],
		"handler_config":   sub["handler_config"],
		"advanced_headers": sub["advanced_headers"],
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
	return fmt.Sprintf("lifted handler into rule, converted %d rules to paths", len(rules))
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
