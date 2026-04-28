package export

import "fmt"

func init() {
	migrations = append(migrations, Migration{
		Before:  "",
		Summary: "Rename request_headers to header_up, add header_down and domain request headers",
		Fn:      migrateHeaderManipulation,
	})
}

func migrateHeaderManipulation(m map[string]any) []string {
	var changes []string
	domains, ok := m["domains"].([]any)
	if !ok {
		return changes
	}
	for _, dRaw := range domains {
		dom, ok := dRaw.(map[string]any)
		if !ok {
			continue
		}
		changes = append(changes, migrateHeaderManipulationToggles(dom)...)
		changes = append(changes, migrateHeaderManipulationRule(dom)...)
		if subs, ok := dom["subdomains"].([]any); ok {
			for _, sRaw := range subs {
				sub, ok := sRaw.(map[string]any)
				if !ok {
					continue
				}
				changes = append(changes, migrateHeaderManipulationToggles(sub)...)
				changes = append(changes, migrateHeaderManipulationRule(sub)...)
				changes = append(changes, migrateHeaderManipulationPaths(sub)...)
			}
		}
		changes = append(changes, migrateHeaderManipulationPaths(dom)...)
	}
	return changes
}

func migrateHeaderManipulationPaths(parent map[string]any) []string {
	var changes []string
	paths, ok := parent["paths"].([]any)
	if !ok {
		return changes
	}
	for _, pRaw := range paths {
		path, ok := pRaw.(map[string]any)
		if !ok {
			continue
		}
		if overrides, ok := path["toggle_overrides"].(map[string]any); ok {
			changes = append(changes, migrateHeaderManipulationToggles(overrides)...)
		}
		changes = append(changes, migrateHeaderManipulationRule(path)...)
	}
	return changes
}

func migrateHeaderManipulationToggles(entity map[string]any) []string {
	var changes []string
	toggles, ok := entity["toggles"].(map[string]any)
	if !ok {
		return changes
	}
	headers, ok := toggles["headers"].(map[string]any)
	if !ok {
		return changes
	}

	if _, exists := headers["request"]; !exists {
		headers["request"] = map[string]any{
			"enabled":            false,
			"x_forwarded_for":   false,
			"x_real_ip":         false,
			"x_forwarded_proto": false,
			"x_forwarded_host":  false,
			"x_request_id":      false,
			"builtin":           []any{},
			"custom":            []any{},
		}
		name := entityName(entity)
		changes = append(changes, fmt.Sprintf("added headers.request default for %s", name))
	}

	if response, ok := headers["response"].(map[string]any); ok {
		if c := setDefault(response, "deferred", false); c != "" {
			changes = append(changes, c)
		}
		backfillOperationOnEntries(response, "builtin")
		backfillOperationOnEntries(response, "custom")
	}

	if request, ok := headers["request"].(map[string]any); ok {
		backfillOperationOnEntries(request, "builtin")
		backfillOperationOnEntries(request, "custom")
	}

	return changes
}

func migrateHeaderManipulationRule(entity map[string]any) []string {
	var changes []string
	rule, ok := entity["rule"].(map[string]any)
	if !ok {
		return changes
	}
	if ht, _ := rule["handler_type"].(string); ht != "reverse_proxy" {
		return changes
	}
	hc, ok := rule["handler_config"].(map[string]any)
	if !ok {
		return changes
	}

	if c := renameField(hc, "request_headers", "header_up"); c != "" {
		changes = append(changes, c)
	}

	if c := setDefault(hc, "header_down", map[string]any{
		"enabled":          false,
		"strip_server":     false,
		"strip_powered_by": false,
		"deferred":         false,
		"builtin":          []any{},
		"custom":           []any{},
	}); c != "" {
		changes = append(changes, c)
	}

	if up, ok := hc["header_up"].(map[string]any); ok {
		backfillOperationOnEntries(up, "builtin")
		backfillOperationOnEntries(up, "custom")
	}
	if down, ok := hc["header_down"].(map[string]any); ok {
		backfillOperationOnEntries(down, "builtin")
		backfillOperationOnEntries(down, "custom")
	}

	return changes
}

func backfillOperationOnEntries(m map[string]any, key string) {
	entries, ok := m[key].([]any)
	if !ok {
		return
	}
	for _, eRaw := range entries {
		entry, ok := eRaw.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := entry["operation"]; !exists {
			entry["operation"] = "set"
		}
	}
}

func entityName(entity map[string]any) string {
	if name, ok := entity["name"].(string); ok && name != "" {
		return name
	}
	if id, ok := entity["id"].(string); ok && id != "" {
		return id
	}
	return "unknown"
}
