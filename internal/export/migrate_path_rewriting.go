package export

func init() {
	migrations = append(migrations, Migration{
		Before:  "",
		Summary: "Add strip_path_prefix and prepend_path_prefix to reverse proxy configs",
		Fn:      migratePathRewriting,
	})
}

func migratePathRewriting(m map[string]any) []string {
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
		changes = append(changes, migratePathRewritingRule(dom)...)
		if subs, ok := dom["subdomains"].([]any); ok {
			for _, sRaw := range subs {
				sub, ok := sRaw.(map[string]any)
				if !ok {
					continue
				}
				changes = append(changes, migratePathRewritingRule(sub)...)
				changes = append(changes, migratePathRewritingPaths(sub)...)
			}
		}
		changes = append(changes, migratePathRewritingPaths(dom)...)
	}
	return changes
}

func migratePathRewritingPaths(parent map[string]any) []string {
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
		changes = append(changes, migratePathRewritingRule(path)...)
	}
	return changes
}

func migratePathRewritingRule(entity map[string]any) []string {
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
	if c := setDefault(hc, "strip_path_prefix", ""); c != "" {
		changes = append(changes, c)
	}
	if c := setDefault(hc, "prepend_path_prefix", ""); c != "" {
		changes = append(changes, c)
	}
	return changes
}
