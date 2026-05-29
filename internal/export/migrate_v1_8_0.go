package export

import "fmt"

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.8.0",
		Summary: "Add log_skip_rules, error_pages, and request_body_max_size defaults",
		Fn:      migrateV180,
	})
}

func migrateV180(m map[string]any) []string {
	var changes []string
	if c := setDefault(m, "log_skip_rules", map[string]any{}); c != "" {
		changes = append(changes, c)
	}
	changes = append(changes, migrateErrorPages(m)...)
	changes = append(changes, migrateRequestBodyMaxSize(m)...)
	return changes
}

func migrateErrorPages(m map[string]any) []string {
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
		if c := setErrorPagesDefault(dom, "domain"); c != "" {
			changes = append(changes, c)
		}
		subs, _ := dom["subdomains"].([]any)
		for _, subRaw := range subs {
			sub, ok := subRaw.(map[string]any)
			if !ok {
				continue
			}
			if c := setErrorPagesDefault(sub, "subdomain"); c != "" {
				changes = append(changes, c)
			}
		}
	}
	return changes
}

func setErrorPagesDefault(entity map[string]any, kind string) string {
	toggles, ok := entity["toggles"].(map[string]any)
	if !ok {
		return ""
	}
	if _, exists := toggles["error_pages"]; exists {
		return ""
	}
	toggles["error_pages"] = []any{}
	name, _ := entity["name"].(string)
	if name == "" {
		name, _ = entity["id"].(string)
	}
	return fmt.Sprintf("added error_pages default for %s %s", kind, name)
}

func migrateRequestBodyMaxSize(m map[string]any) []string {
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
		if c := setToggleDefault(dom, "request_body_max_size", "", "domain"); c != "" {
			changes = append(changes, c)
		}
		subs, _ := dom["subdomains"].([]any)
		for _, subRaw := range subs {
			sub, ok := subRaw.(map[string]any)
			if !ok {
				continue
			}
			if c := setToggleDefault(sub, "request_body_max_size", "", "subdomain"); c != "" {
				changes = append(changes, c)
			}
		}
		paths, _ := dom["paths"].([]any)
		for _, pathRaw := range paths {
			p, ok := pathRaw.(map[string]any)
			if !ok {
				continue
			}
			overrides, ok := p["toggle_overrides"].(map[string]any)
			if ok {
				if _, exists := overrides["request_body_max_size"]; !exists {
					overrides["request_body_max_size"] = ""
					changes = append(changes, "added request_body_max_size default to path toggle override")
				}
			}
		}
		for _, subRaw := range subs {
			sub, ok := subRaw.(map[string]any)
			if !ok {
				continue
			}
			subPaths, _ := sub["paths"].([]any)
			for _, pathRaw := range subPaths {
				p, ok := pathRaw.(map[string]any)
				if !ok {
					continue
				}
				overrides, ok := p["toggle_overrides"].(map[string]any)
				if ok {
					if _, exists := overrides["request_body_max_size"]; !exists {
						overrides["request_body_max_size"] = ""
						changes = append(changes, "added request_body_max_size default to subdomain path toggle override")
					}
				}
			}
		}
	}
	return changes
}

func setToggleDefault(entity map[string]any, key string, defaultVal any, kind string) string {
	toggles, ok := entity["toggles"].(map[string]any)
	if !ok {
		return ""
	}
	if _, exists := toggles[key]; exists {
		return ""
	}
	toggles[key] = defaultVal
	name, _ := entity["name"].(string)
	if name == "" {
		name, _ = entity["id"].(string)
	}
	return fmt.Sprintf("added %s default for %s %s", key, kind, name)
}
