package export

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.8.0",
		Summary: "Move request headers from domain toggles into per-rule reverse_proxy config",
		Fn:      migrateV180,
	})
}

func migrateV180(m map[string]any) []string {
	var changes []string

	domainsRaw, ok := m["domains"]
	if !ok {
		return changes
	}
	domains, ok := domainsRaw.([]any)
	if !ok {
		return changes
	}

	for _, domRaw := range domains {
		dom, ok := domRaw.(map[string]any)
		if !ok {
			continue
		}

		// Extract request headers from domain toggles
		var requestData any
		if toggles, ok := dom["toggles"].(map[string]any); ok {
			if headers, ok := toggles["headers"].(map[string]any); ok {
				requestData = headers["request"]
				delete(headers, "request")
			}
		}

		rulesRaw, ok := dom["rules"]
		if !ok {
			continue
		}
		rules, ok := rulesRaw.([]any)
		if !ok {
			continue
		}

		for _, ruleRaw := range rules {
			rule, ok := ruleRaw.(map[string]any)
			if !ok {
				continue
			}

			// Copy domain-level request headers into each reverse_proxy rule's handler_config
			if requestData != nil {
				handlerType, _ := rule["handler_type"].(string)
				if handlerType == "reverse_proxy" || handlerType == "" {
					if hc, ok := rule["handler_config"].(map[string]any); ok {
						if _, exists := hc["request_headers"]; !exists {
							hc["request_headers"] = requestData
						}
					}
				}
			}

			// Clean request from toggle_overrides.headers on each rule
			if overrides, ok := rule["toggle_overrides"].(map[string]any); ok {
				if headers, ok := overrides["headers"].(map[string]any); ok {
					delete(headers, "request")
				}
			}
		}

		if requestData != nil {
			domName, _ := dom["name"].(string)
			if domName == "" {
				domName, _ = dom["id"].(string)
			}
			changes = append(changes, "moved request headers from domain toggles to rule handler_config for "+domName)
		}
	}

	return changes
}
