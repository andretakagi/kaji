package export

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.10.0",
		Summary: "Default rule.enabled to true on every domain, subdomain, and path",
		Fn:      migrateV1100,
	})
}

func migrateV1100(m map[string]any) []string {
	var changes []string

	domains, ok := m["domains"].([]any)
	if !ok {
		return changes
	}

	for _, domRaw := range domains {
		dom, ok := domRaw.(map[string]any)
		if !ok {
			continue
		}

		if defaulted := defaultRuleEnabled(dom); defaulted {
			changes = append(changes, "defaulted rule.enabled=true on "+ruleScope(dom, "domain"))
		}

		if paths, ok := dom["paths"].([]any); ok {
			for _, pRaw := range paths {
				if p, ok := pRaw.(map[string]any); ok && defaultRuleEnabled(p) {
					changes = append(changes, "defaulted rule.enabled=true on "+ruleScope(p, "path"))
				}
			}
		}

		if subs, ok := dom["subdomains"].([]any); ok {
			for _, sRaw := range subs {
				sub, ok := sRaw.(map[string]any)
				if !ok {
					continue
				}
				if defaultRuleEnabled(sub) {
					changes = append(changes, "defaulted rule.enabled=true on "+ruleScope(sub, "subdomain"))
				}
				if paths, ok := sub["paths"].([]any); ok {
					for _, pRaw := range paths {
						if p, ok := pRaw.(map[string]any); ok && defaultRuleEnabled(p) {
							changes = append(changes, "defaulted rule.enabled=true on "+ruleScope(p, "path"))
						}
					}
				}
			}
		}
	}

	return changes
}

func defaultRuleEnabled(parent map[string]any) bool {
	rule, ok := parent["rule"].(map[string]any)
	if !ok {
		return false
	}
	if _, exists := rule["enabled"]; exists {
		return false
	}
	rule["enabled"] = true
	return true
}

func ruleScope(parent map[string]any, kind string) string {
	if name, ok := parent["name"].(string); ok && name != "" {
		return kind + " " + name
	}
	if id, ok := parent["id"].(string); ok && id != "" {
		return kind + " " + id
	}
	return kind
}
