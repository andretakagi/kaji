package export

import (
	"reflect"
	"testing"
)

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

	change := splitDomainRules(dom)
	if change != "split 2 legacy rules into rule + 2 paths" {
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

	change := splitDomainRules(dom)
	if change != "split 2 legacy rules into rule + 1 paths" {
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

	change := splitDomainRules(dom)
	if change != "split 0 legacy rules into rule + 0 paths" {
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

	change := liftSubdomainRule(sub)
	if change != "lifted handler into rule, converted 0 rules to paths" {
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

	change := liftSubdomainRule(sub)
	if change != "lifted handler into rule, converted 2 rules to paths" {
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
	if changes[0] != "example.com: split 2 legacy rules into rule + 1 paths" {
		t.Errorf("changes[0] = %q", changes[0])
	}
	if changes[1] != "example.com/api: lifted handler into rule, converted 1 rules to paths" {
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
