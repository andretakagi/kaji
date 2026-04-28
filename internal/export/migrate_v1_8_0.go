package export

import "fmt"

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.8.0",
		Summary: "Add log_skip_rules for per-sink skip conditions, add error_pages to domain toggles",
		Fn:      migrateV180,
	})
}

func migrateV180(m map[string]any) []string {
	var changes []string
	if c := setDefault(m, "log_skip_rules", map[string]any{}); c != "" {
		changes = append(changes, c)
	}
	changes = append(changes, migrateErrorPages(m)...)
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
