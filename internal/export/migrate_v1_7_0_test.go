package export

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMigrateV170RemovesRouteSettings(t *testing.T) {
	m := map[string]any{
		"route_settings": map[string]any{"some-id": map[string]any{"advanced_headers": true}},
	}
	changes := migrateV170(m)
	if _, ok := m["route_settings"]; ok {
		t.Fatal("route_settings should be removed")
	}
	found := false
	for _, c := range changes {
		if c == "removed route_settings" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'removed route_settings' in changes, got %v", changes)
	}
}

func TestMigrateV170RemovesDisabledRoutes(t *testing.T) {
	m := map[string]any{
		"disabled_routes": []any{},
	}
	changes := migrateV170(m)
	if _, ok := m["disabled_routes"]; ok {
		t.Fatal("disabled_routes should be removed")
	}
	found := false
	for _, c := range changes {
		if c == "removed disabled_routes" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'removed disabled_routes' in changes, got %v", changes)
	}
}

func TestMigrateV170CreatesDomains(t *testing.T) {
	m := map[string]any{}
	migrateV170(m)
	domains, ok := m["domains"]
	if !ok {
		t.Fatal("domains array not created")
	}
	arr, ok := domains.([]any)
	if !ok {
		t.Fatalf("domains should be []any, got %T", domains)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty domains array, got %d entries", len(arr))
	}
}

func TestMigrateV170ConvertsDisabledRoutesToDomains(t *testing.T) {
	routeJSON := map[string]any{
		"@id": "kaji_example_com",
		"match": []any{
			map[string]any{"host": []any{"example.com"}},
		},
		"handle": []any{
			map[string]any{
				"handler": "reverse_proxy",
				"upstreams": []any{
					map[string]any{"dial": "localhost:3000"},
				},
			},
		},
		"terminal": true,
	}

	m := map[string]any{
		"disabled_routes": []any{
			map[string]any{
				"id":          "kaji_example_com",
				"server":      "srv0",
				"disabled_at": "2025-01-01T00:00:00Z",
				"route":       routeJSON,
			},
		},
		"route_settings": map[string]any{},
	}

	changes := migrateV170(m)

	if len(changes) < 1 {
		t.Fatalf("expected changes, got %d", len(changes))
	}

	if _, ok := m["disabled_routes"]; ok {
		t.Error("disabled_routes should be removed")
	}
	if _, ok := m["route_settings"]; ok {
		t.Error("route_settings should be removed")
	}

	domains, ok := m["domains"].([]any)
	if !ok {
		t.Fatalf("domains should be []any, got %T", m["domains"])
	}
	if len(domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(domains))
	}

	dom, ok := domains[0].(map[string]any)
	if !ok {
		t.Fatalf("domain entry should be map[string]any, got %T", domains[0])
	}

	if dom["id"] != "dom_migrated_example_com" {
		t.Errorf("domain id = %v, want dom_migrated_example_com", dom["id"])
	}
	if dom["name"] != "example.com" {
		t.Errorf("domain name = %v, want example.com", dom["name"])
	}
	if dom["enabled"] != false {
		t.Errorf("domain enabled = %v, want false", dom["enabled"])
	}

	rule, ok := dom["rule"].(map[string]any)
	if !ok {
		t.Fatalf("expected rule, got %v", dom["rule"])
	}
	if rule["handler_type"] != "reverse_proxy" {
		t.Errorf("handler_type = %v, want reverse_proxy", rule["handler_type"])
	}

	hc, ok := rule["handler_config"].(json.RawMessage)
	if !ok {
		t.Fatalf("handler_config should be json.RawMessage, got %T", rule["handler_config"])
	}
	var cfg map[string]string
	if err := json.Unmarshal(hc, &cfg); err != nil {
		t.Fatalf("parsing handler_config: %v", err)
	}
	if cfg["upstream"] != "localhost:3000" {
		t.Errorf("upstream = %v, want localhost:3000", cfg["upstream"])
	}
}

func TestMigrateV170PreservesExistingDomains(t *testing.T) {
	existing := map[string]any{
		"id":   "dom_existing",
		"name": "existing.com",
	}
	m := map[string]any{
		"domains": []any{existing},
	}
	migrateV170(m)
	domains := m["domains"].([]any)
	if len(domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(domains))
	}
	dom := domains[0].(map[string]any)
	if dom["id"] != "dom_existing" {
		t.Errorf("existing domain was lost")
	}
}

func TestMigrateV170RunsForOldVersion(t *testing.T) {
	m := map[string]any{
		"route_settings": map[string]any{},
	}
	changes, err := RunMigrations(m, "1.6.0")
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if len(changes) == 0 {
		t.Error("expected migration to run for version 1.6.0")
	}
	if _, ok := m["route_settings"]; ok {
		t.Error("route_settings should be removed")
	}
	if _, ok := m["domains"]; !ok {
		t.Error("domains should be created")
	}
}

func TestMigrateV170SkipsForCurrentVersion(t *testing.T) {
	m := map[string]any{}
	changes, err := RunMigrations(m, "1.7.0")
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for version 1.7.0, got %d: %v", len(changes), changes)
	}
}

func TestMigrateV170HandlesNestedSubroute(t *testing.T) {
	routeJSON := map[string]any{
		"@id": "kaji_nested_example_com",
		"match": []any{
			map[string]any{"host": []any{"nested.example.com"}},
		},
		"handle": []any{
			map[string]any{
				"handler": "subroute",
				"routes": []any{
					map[string]any{
						"handle": []any{
							map[string]any{
								"handler": "reverse_proxy",
								"upstreams": []any{
									map[string]any{"dial": "localhost:8080"},
								},
							},
						},
					},
				},
			},
		},
		"terminal": true,
	}

	m := map[string]any{
		"disabled_routes": []any{
			map[string]any{
				"id":     "kaji_nested_example_com",
				"server": "srv0",
				"route":  routeJSON,
			},
		},
	}

	migrateV170(m)

	domains := m["domains"].([]any)
	if len(domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(domains))
	}
	dom := domains[0].(map[string]any)
	rule := dom["rule"].(map[string]any)

	var cfg map[string]string
	json.Unmarshal(rule["handler_config"].(json.RawMessage), &cfg)
	if cfg["upstream"] != "localhost:8080" {
		t.Errorf("upstream = %v, want localhost:8080", cfg["upstream"])
	}
}

func TestSanitizeForID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"example.com", "example_com"},
		{"sub.example.com", "sub_example_com"},
		{"my-app.io", "my_app_io"},
		{"simple", "simple"},
		{"a.b-c_d", "a_b_c_d"},
	}
	for _, tt := range tests {
		got := sanitizeForID(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeForID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractUpstream(t *testing.T) {
	route := `{
		"handle": [{
			"handler": "reverse_proxy",
			"upstreams": [{"dial": "localhost:9000"}]
		}]
	}`
	got := extractUpstream([]byte(route))
	if got != "localhost:9000" {
		t.Errorf("extractUpstream = %q, want localhost:9000", got)
	}

	empty := `{"handle": [{"handler": "encode"}]}`
	got = extractUpstream([]byte(empty))
	if got != "" {
		t.Errorf("extractUpstream with no reverse_proxy = %q, want empty", got)
	}
}

func TestConvertSubdomainRulesExtractsMatching(t *testing.T) {
	dom := map[string]any{
		"name":    "example.com",
		"toggles": map[string]any{"force_https": true},
		"rules": []any{
			map[string]any{
				"id":             "rule_root",
				"enabled":        true,
				"match_type":     "",
				"handler_type":   "reverse_proxy",
				"handler_config": map[string]any{"upstream": "localhost:80"},
			},
			map[string]any{
				"id":             "rule_api",
				"enabled":        true,
				"match_type":     "subdomain",
				"match_value":    "api",
				"handler_type":   "reverse_proxy",
				"handler_config": map[string]any{"upstream": "localhost:9000"},
			},
		},
	}

	change := convertSubdomainRules(dom)
	if change != "converted 1 subdomain rules to subdomain entities for example.com" {
		t.Errorf("change = %q", change)
	}

	rules := dom["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("expected 1 remaining rule, got %d", len(rules))
	}
	if rules[0].(map[string]any)["id"] != "rule_root" {
		t.Errorf("remaining rule id = %v, want rule_root", rules[0])
	}

	subs := dom["subdomains"].([]any)
	if len(subs) != 1 {
		t.Fatalf("expected 1 subdomain, got %d", len(subs))
	}
	sub := subs[0].(map[string]any)
	if sub["id"] != "sub_migrated_api" {
		t.Errorf("subdomain id = %v, want sub_migrated_api", sub["id"])
	}
	if sub["name"] != "api" {
		t.Errorf("subdomain name = %v, want api", sub["name"])
	}
	if sub["enabled"] != true {
		t.Errorf("subdomain enabled = %v, want true", sub["enabled"])
	}
	if sub["handler_type"] != "reverse_proxy" {
		t.Errorf("subdomain handler_type = %v, want reverse_proxy", sub["handler_type"])
	}
	toggles, ok := sub["toggles"].(map[string]any)
	if !ok {
		t.Fatalf("subdomain toggles = %T, want map[string]any", sub["toggles"])
	}
	if toggles["force_https"] != true {
		t.Errorf("subdomain inherited toggles force_https = %v, want true", toggles["force_https"])
	}
}

func TestConvertSubdomainRulesPrefersExplicitOverrides(t *testing.T) {
	dom := map[string]any{
		"name":    "example.com",
		"toggles": map[string]any{"force_https": true, "compression": true},
		"rules": []any{
			map[string]any{
				"id":             "rule_api",
				"enabled":        true,
				"match_type":     "subdomain",
				"match_value":    "api",
				"handler_type":   "reverse_proxy",
				"handler_config": map[string]any{"upstream": "localhost:9000"},
				"toggle_overrides": map[string]any{
					"force_https": false,
				},
			},
		},
	}

	convertSubdomainRules(dom)

	subs := dom["subdomains"].([]any)
	sub := subs[0].(map[string]any)
	toggles := sub["toggles"].(map[string]any)
	if toggles["force_https"] != false {
		t.Errorf("force_https = %v, want false (override)", toggles["force_https"])
	}
	if _, exists := toggles["compression"]; exists {
		t.Error("override should replace toggles entirely, not merge")
	}
}

func TestConvertSubdomainRulesLeavesNonMatchAlone(t *testing.T) {
	dom := map[string]any{
		"name": "example.com",
		"rules": []any{
			map[string]any{
				"id":           "rule_root",
				"match_type":   "",
				"handler_type": "reverse_proxy",
			},
			map[string]any{
				"id":           "rule_path",
				"match_type":   "path",
				"match_value":  "/api",
				"handler_type": "reverse_proxy",
			},
		},
	}

	change := convertSubdomainRules(dom)
	if change != "" {
		t.Errorf("change = %q, want empty", change)
	}

	subs := dom["subdomains"].([]any)
	if len(subs) != 0 {
		t.Errorf("expected 0 subdomains, got %d", len(subs))
	}
	rules := dom["rules"].([]any)
	if len(rules) != 2 {
		t.Errorf("expected 2 rules preserved, got %d", len(rules))
	}
}

func TestConvertSubdomainRulesSkipsEmptyName(t *testing.T) {
	dom := map[string]any{
		"name": "example.com",
		"rules": []any{
			map[string]any{
				"id":           "rule_bad",
				"match_type":   "subdomain",
				"match_value":  "",
				"handler_type": "reverse_proxy",
			},
		},
	}

	change := convertSubdomainRules(dom)
	if change != "" {
		t.Errorf("change = %q, want empty (empty name should be skipped)", change)
	}

	rules := dom["rules"].([]any)
	if len(rules) != 1 {
		t.Errorf("expected the rule to be preserved, got %d rules", len(rules))
	}
	subs := dom["subdomains"].([]any)
	if len(subs) != 0 {
		t.Errorf("expected 0 subdomains, got %d", len(subs))
	}
}

func TestConvertSubdomainRulesNoRulesField(t *testing.T) {
	dom := map[string]any{"name": "example.com"}

	change := convertSubdomainRules(dom)
	if change != "" {
		t.Errorf("change = %q, want empty", change)
	}
	subs, ok := dom["subdomains"].([]any)
	if !ok {
		t.Fatalf("subdomains = %T, want []any", dom["subdomains"])
	}
	if len(subs) != 0 {
		t.Errorf("expected empty subdomains slice")
	}
}

func TestMigrateV170ConvertsSubdomainsForExistingDomain(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name":    "example.com",
				"toggles": map[string]any{},
				"rules": []any{
					map[string]any{
						"id":             "rule_api",
						"match_type":     "subdomain",
						"match_value":    "api",
						"handler_type":   "reverse_proxy",
						"handler_config": map[string]any{"upstream": "localhost:9000"},
					},
				},
			},
		},
	}

	changes := migrateV170(m)
	found := false
	for _, c := range changes {
		if c == "converted 1 subdomain rules to subdomain entities for example.com" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected subdomain conversion change message, got %v", changes)
	}

	dom := m["domains"].([]any)[0].(map[string]any)
	subs := dom["subdomains"].([]any)
	if len(subs) != 1 {
		t.Fatalf("expected 1 subdomain after migration, got %d", len(subs))
	}
}

func TestSplitDomainRulesOnlyPathRules(t *testing.T) {
	dom := map[string]any{
		"name": "example.com",
		"rules": []any{
			map[string]any{
				"id":             "rule_a",
				"enabled":        true,
				"match_type":     "path",
				"path_match":     "prefix",
				"match_value":    "/api",
				"handler_type":   "reverse_proxy",
				"handler_config": map[string]any{"upstream": "localhost:3000"},
			},
			map[string]any{
				"id":             "rule_b",
				"enabled":        false,
				"match_type":     "path",
				"path_match":     "exact",
				"match_value":    "/healthz",
				"handler_type":   "static_response",
				"handler_config": map[string]any{"body": "ok"},
			},
		},
	}

	change := splitDomainRules(dom, "example.com")
	if change != "split 2 legacy rules into domain rule and 2 paths for example.com" {
		t.Errorf("change message = %q", change)
	}
	if _, exists := dom["rules"]; exists {
		t.Fatal("rules key must be removed")
	}

	rule, ok := dom["rule"].(map[string]any)
	if !ok {
		t.Fatalf("rule = %T, want map[string]any", dom["rule"])
	}
	if rule["handler_type"] != "none" {
		t.Errorf("rule.handler_type = %v, want none", rule["handler_type"])
	}
	hc, ok := rule["handler_config"].(map[string]any)
	if !ok {
		t.Fatalf("rule.handler_config = %T, want map[string]any", rule["handler_config"])
	}
	if len(hc) != 0 {
		t.Errorf("rule.handler_config = %v, want empty", hc)
	}

	paths, ok := dom["paths"].([]any)
	if !ok {
		t.Fatalf("paths = %T, want []any", dom["paths"])
	}
	if len(paths) != 2 {
		t.Fatalf("paths length = %d, want 2", len(paths))
	}

	first, ok := paths[0].(map[string]any)
	if !ok {
		t.Fatalf("paths[0] = %T, want map[string]any", paths[0])
	}
	if first["id"] != "rule_a" {
		t.Errorf("paths[0].id = %v, want rule_a", first["id"])
	}
	if first["path_match"] != "prefix" {
		t.Errorf("paths[0].path_match = %v, want prefix", first["path_match"])
	}
	if first["match_value"] != "/api" {
		t.Errorf("paths[0].match_value = %v, want /api", first["match_value"])
	}
	if first["enabled"] != true {
		t.Errorf("paths[0].enabled = %v, want true", first["enabled"])
	}
	firstRule, ok := first["rule"].(map[string]any)
	if !ok {
		t.Fatalf("paths[0].rule = %T, want map[string]any", first["rule"])
	}
	if firstRule["handler_type"] != "reverse_proxy" {
		t.Errorf("paths[0].rule.handler_type = %v, want reverse_proxy", firstRule["handler_type"])
	}
	if !reflect.DeepEqual(firstRule["handler_config"], map[string]any{"upstream": "localhost:3000"}) {
		t.Errorf("paths[0].rule.handler_config = %v", firstRule["handler_config"])
	}

	second := paths[1].(map[string]any)
	if second["id"] != "rule_b" {
		t.Errorf("paths[1].id = %v, want rule_b", second["id"])
	}
	if second["enabled"] != false {
		t.Errorf("paths[1].enabled = %v, want false", second["enabled"])
	}
}

func TestSplitDomainRulesRootPlusPaths(t *testing.T) {
	dom := map[string]any{
		"name": "example.com",
		"rules": []any{
			map[string]any{
				"id":             "rule_root",
				"enabled":        true,
				"match_type":     "",
				"handler_type":   "reverse_proxy",
				"handler_config": map[string]any{"upstream": "localhost:80"},
			},
			map[string]any{
				"id":               "rule_api",
				"enabled":          true,
				"match_type":       "path",
				"path_match":       "prefix",
				"match_value":      "/api",
				"handler_type":     "reverse_proxy",
				"handler_config":   map[string]any{"upstream": "localhost:3000"},
				"advanced_headers": true,
				"toggle_overrides": map[string]any{"compression": true},
			},
		},
	}

	change := splitDomainRules(dom, "example.com")
	if change != "split 2 legacy rules into domain rule and 1 path for example.com" {
		t.Errorf("change message = %q", change)
	}
	if _, exists := dom["rules"]; exists {
		t.Fatal("rules key must be removed")
	}

	rule := dom["rule"].(map[string]any)
	if rule["handler_type"] != "reverse_proxy" {
		t.Errorf("rule.handler_type = %v", rule["handler_type"])
	}
	if !reflect.DeepEqual(rule["handler_config"], map[string]any{"upstream": "localhost:80"}) {
		t.Errorf("rule.handler_config = %v", rule["handler_config"])
	}

	paths := dom["paths"].([]any)
	if len(paths) != 1 {
		t.Fatalf("paths length = %d, want 1", len(paths))
	}
	p := paths[0].(map[string]any)
	if p["id"] != "rule_api" {
		t.Errorf("paths[0].id = %v, want rule_api", p["id"])
	}
	if p["match_value"] != "/api" {
		t.Errorf("paths[0].match_value = %v, want /api", p["match_value"])
	}
	if !reflect.DeepEqual(p["toggle_overrides"], map[string]any{"compression": true}) {
		t.Errorf("paths[0].toggle_overrides = %v", p["toggle_overrides"])
	}
	pRule := p["rule"].(map[string]any)
	if pRule["advanced_headers"] != true {
		t.Errorf("paths[0].rule.advanced_headers = %v, want true", pRule["advanced_headers"])
	}
}

func TestSplitDomainRulesNoRules(t *testing.T) {
	dom := map[string]any{
		"name": "example.com",
	}

	change := splitDomainRules(dom, "example.com")
	if change != "split 0 legacy rules into domain rule and 0 paths for example.com" {
		t.Errorf("change message = %q", change)
	}

	rule := dom["rule"].(map[string]any)
	if rule["handler_type"] != "none" {
		t.Errorf("rule.handler_type = %v, want none", rule["handler_type"])
	}
	hc := rule["handler_config"].(map[string]any)
	if len(hc) != 0 {
		t.Errorf("rule.handler_config = %v, want empty map", hc)
	}

	paths := dom["paths"].([]any)
	if len(paths) != 0 {
		t.Errorf("paths length = %d, want 0", len(paths))
	}
	if _, exists := dom["rules"]; exists {
		t.Error("rules key should be removed")
	}
}

func TestLiftSubdomainRuleNoneNoRules(t *testing.T) {
	sub := map[string]any{
		"name":             "api",
		"handler_type":     "none",
		"handler_config":   map[string]any{},
		"advanced_headers": false,
		"rules":            []any{},
	}

	change := liftSubdomainRule(sub, "example.com/api")
	if change != "lifted handler into subdomain rule and converted 0 rules to paths for example.com/api" {
		t.Errorf("change message = %q", change)
	}

	for _, key := range []string{"handler_type", "handler_config", "advanced_headers", "rules"} {
		if _, exists := sub[key]; exists {
			t.Errorf("legacy field %q should be removed", key)
		}
	}

	rule, ok := sub["rule"].(map[string]any)
	if !ok {
		t.Fatalf("rule = %T, want map[string]any", sub["rule"])
	}
	if rule["handler_type"] != "none" {
		t.Errorf("rule.handler_type = %v, want none", rule["handler_type"])
	}
	if !reflect.DeepEqual(rule["handler_config"], map[string]any{}) {
		t.Errorf("rule.handler_config = %v", rule["handler_config"])
	}
	if rule["advanced_headers"] != false {
		t.Errorf("rule.advanced_headers = %v, want false", rule["advanced_headers"])
	}

	paths, ok := sub["paths"].([]any)
	if !ok {
		t.Fatalf("paths = %T, want []any", sub["paths"])
	}
	if len(paths) != 0 {
		t.Errorf("paths length = %d, want 0", len(paths))
	}
}

func TestLiftSubdomainRuleHandlerPlusPaths(t *testing.T) {
	sub := map[string]any{
		"name":             "api",
		"handler_type":     "reverse_proxy",
		"handler_config":   map[string]any{"upstream": "localhost:9000"},
		"advanced_headers": true,
		"rules": []any{
			map[string]any{
				"id":             "rule_v2",
				"enabled":        true,
				"path_match":     "prefix",
				"match_value":    "/v2",
				"handler_type":   "reverse_proxy",
				"handler_config": map[string]any{"upstream": "localhost:9001"},
			},
			map[string]any{
				"id":             "rule_static",
				"enabled":        true,
				"path_match":     "exact",
				"match_value":    "/ping",
				"handler_type":   "static_response",
				"handler_config": map[string]any{"body": "pong"},
			},
		},
	}

	change := liftSubdomainRule(sub, "example.com/api")
	if change != "lifted handler into subdomain rule and converted 2 rules to paths for example.com/api" {
		t.Errorf("change message = %q", change)
	}

	for _, key := range []string{"handler_type", "handler_config", "advanced_headers", "rules"} {
		if _, exists := sub[key]; exists {
			t.Errorf("legacy field %q should be removed", key)
		}
	}

	rule := sub["rule"].(map[string]any)
	if rule["handler_type"] != "reverse_proxy" {
		t.Errorf("rule.handler_type = %v, want reverse_proxy", rule["handler_type"])
	}
	if !reflect.DeepEqual(rule["handler_config"], map[string]any{"upstream": "localhost:9000"}) {
		t.Errorf("rule.handler_config = %v", rule["handler_config"])
	}
	if rule["advanced_headers"] != true {
		t.Errorf("rule.advanced_headers = %v, want true", rule["advanced_headers"])
	}

	paths := sub["paths"].([]any)
	if len(paths) != 2 {
		t.Fatalf("paths length = %d, want 2", len(paths))
	}

	first := paths[0].(map[string]any)
	if first["id"] != "rule_v2" {
		t.Errorf("paths[0].id = %v, want rule_v2", first["id"])
	}
	if first["path_match"] != "prefix" {
		t.Errorf("paths[0].path_match = %v, want prefix", first["path_match"])
	}
	if first["match_value"] != "/v2" {
		t.Errorf("paths[0].match_value = %v, want /v2", first["match_value"])
	}
	firstRule := first["rule"].(map[string]any)
	if firstRule["handler_type"] != "reverse_proxy" {
		t.Errorf("paths[0].rule.handler_type = %v", firstRule["handler_type"])
	}
	if !reflect.DeepEqual(firstRule["handler_config"], map[string]any{"upstream": "localhost:9001"}) {
		t.Errorf("paths[0].rule.handler_config = %v", firstRule["handler_config"])
	}

	second := paths[1].(map[string]any)
	if second["id"] != "rule_static" {
		t.Errorf("paths[1].id = %v, want rule_static", second["id"])
	}
	secondRule := second["rule"].(map[string]any)
	if secondRule["handler_type"] != "static_response" {
		t.Errorf("paths[1].rule.handler_type = %v, want static_response", secondRule["handler_type"])
	}
}

func TestLiftSubdomainRuleMissingHandlerFallsBackToNone(t *testing.T) {
	sub := map[string]any{
		"name":  "api",
		"rules": []any{},
	}

	change := liftSubdomainRule(sub, "example.com/api")
	if change != "lifted handler into subdomain rule and converted 0 rules to paths for example.com/api" {
		t.Errorf("change message = %q", change)
	}

	rule, ok := sub["rule"].(map[string]any)
	if !ok {
		t.Fatalf("rule = %T, want map[string]any", sub["rule"])
	}
	if rule["handler_type"] != "none" {
		t.Errorf("rule.handler_type = %v, want none", rule["handler_type"])
	}
	if !reflect.DeepEqual(rule["handler_config"], map[string]any{}) {
		t.Errorf("rule.handler_config = %v, want empty map", rule["handler_config"])
	}
	if _, exists := rule["advanced_headers"]; exists {
		t.Errorf("rule.advanced_headers should not be set, got %v", rule["advanced_headers"])
	}
}

func TestMigrateV170PathsTopLevel(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"rules": []any{
					map[string]any{
						"id":             "rule_root",
						"enabled":        true,
						"match_type":     "",
						"handler_type":   "reverse_proxy",
						"handler_config": map[string]any{"upstream": "localhost:80"},
					},
					map[string]any{
						"id":             "rule_api",
						"enabled":        true,
						"match_type":     "path",
						"path_match":     "prefix",
						"match_value":    "/api",
						"handler_type":   "reverse_proxy",
						"handler_config": map[string]any{"upstream": "localhost:3000"},
					},
				},
				"subdomains": []any{
					map[string]any{
						"name":             "api",
						"handler_type":     "reverse_proxy",
						"handler_config":   map[string]any{"upstream": "localhost:9000"},
						"advanced_headers": false,
						"rules": []any{
							map[string]any{
								"id":             "sub_rule_v2",
								"enabled":        true,
								"path_match":     "prefix",
								"match_value":    "/v2",
								"handler_type":   "reverse_proxy",
								"handler_config": map[string]any{"upstream": "localhost:9001"},
							},
						},
					},
				},
			},
		},
	}

	changes := migrateV170Paths(m)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %v", len(changes), changes)
	}
	if changes[0] != "split 2 legacy rules into domain rule and 1 path for example.com" {
		t.Errorf("changes[0] = %q", changes[0])
	}
	if changes[1] != "lifted handler into subdomain rule and converted 1 rule to path for example.com/api" {
		t.Errorf("changes[1] = %q", changes[1])
	}

	domains := m["domains"].([]any)
	dom := domains[0].(map[string]any)
	if _, exists := dom["rules"]; exists {
		t.Error("domain.rules should be removed")
	}
	if dom["rule"].(map[string]any)["handler_type"] != "reverse_proxy" {
		t.Error("domain.rule should be set")
	}
	if len(dom["paths"].([]any)) != 1 {
		t.Error("domain.paths should have 1 entry")
	}

	subs := dom["subdomains"].([]any)
	sub := subs[0].(map[string]any)
	for _, key := range []string{"handler_type", "handler_config", "advanced_headers", "rules"} {
		if _, exists := sub[key]; exists {
			t.Errorf("subdomain.%s should be removed", key)
		}
	}
	if sub["rule"].(map[string]any)["handler_type"] != "reverse_proxy" {
		t.Error("subdomain.rule should be set")
	}
	if len(sub["paths"].([]any)) != 1 {
		t.Error("subdomain.paths should have 1 entry")
	}
}

func TestMigrateV170PathsNoDomains(t *testing.T) {
	m := map[string]any{}
	changes := migrateV170Paths(m)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %v", changes)
	}
}

func TestMigrateV170PathsRunsForOldVersion(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"rules": []any{
					map[string]any{
						"id":             "rule_root",
						"enabled":        true,
						"match_type":     "",
						"handler_type":   "reverse_proxy",
						"handler_config": map[string]any{"upstream": "localhost:80"},
					},
				},
				"subdomains": []any{},
			},
		},
	}
	changes, err := RunMigrations(m, "1.6.0")
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if len(changes) == 0 {
		t.Error("expected paths migration to run for version 1.6.0")
	}

	dom := m["domains"].([]any)[0].(map[string]any)
	if _, exists := dom["rules"]; exists {
		t.Error("domain.rules should be removed after migration")
	}
	if _, exists := dom["rule"]; !exists {
		t.Error("domain.rule should be set after migration")
	}
	if _, exists := dom["paths"]; !exists {
		t.Error("domain.paths should be set after migration")
	}
}
