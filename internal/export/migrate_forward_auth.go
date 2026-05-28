package export

import "fmt"

func init() {
	migrations = append(migrations, Migration{
		Before:  "",
		Summary: "Convert basic_auth toggle to auth structure, add forward_auth default",
		Fn:      migrateForwardAuth,
	})
}

func migrateForwardAuth(m map[string]any) []string {
	var changes []string
	if c := setDefault(m, "forward_auth", map[string]any{
		"enabled":  false,
		"provider": "",
		"url":      "",
	}); c != "" {
		changes = append(changes, c)
	}
	domains, ok := m["domains"].([]any)
	if !ok {
		return changes
	}
	for _, dRaw := range domains {
		dom, ok := dRaw.(map[string]any)
		if !ok {
			continue
		}
		changes = append(changes, migrateBasicAuthToggles(dom)...)
		if subs, ok := dom["subdomains"].([]any); ok {
			for _, sRaw := range subs {
				sub, ok := sRaw.(map[string]any)
				if !ok {
					continue
				}
				changes = append(changes, migrateBasicAuthToggles(sub)...)
				changes = append(changes, migrateBasicAuthPaths(sub)...)
			}
		}
		changes = append(changes, migrateBasicAuthPaths(dom)...)
	}
	return changes
}

func migrateBasicAuthPaths(parent map[string]any) []string {
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
		overrides, ok := path["toggle_overrides"].(map[string]any)
		if !ok {
			continue
		}
		if c := convertBasicAuthField(overrides, entityName(path)); c != "" {
			changes = append(changes, c)
		}
	}
	return changes
}

func migrateBasicAuthToggles(entity map[string]any) []string {
	toggles, ok := entity["toggles"].(map[string]any)
	if !ok {
		return nil
	}
	if c := convertBasicAuthField(toggles, entityName(entity)); c != "" {
		return []string{c}
	}
	return nil
}

func convertBasicAuthField(toggles map[string]any, name string) string {
	oldBA, ok := toggles["basic_auth"].(map[string]any)
	if !ok {
		return ""
	}
	if _, exists := toggles["auth"]; exists {
		delete(toggles, "basic_auth")
		return ""
	}

	mode := "off"
	if enabled, ok := oldBA["enabled"].(bool); ok && enabled {
		mode = "basic"
	}
	delete(oldBA, "enabled")

	toggles["auth"] = map[string]any{
		"mode":       mode,
		"basic_auth": oldBA,
	}
	delete(toggles, "basic_auth")

	return fmt.Sprintf("converted basic_auth to auth structure for %s", name)
}
