package export

import "testing"

func TestMigrateV1100DefaultsRuleEnabled(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"rule": map[string]any{"handler_type": "reverse_proxy"},
				"paths": []any{
					map[string]any{
						"id":   "p1",
						"rule": map[string]any{"handler_type": "reverse_proxy"},
					},
				},
				"subdomains": []any{
					map[string]any{
						"name": "api",
						"rule": map[string]any{"handler_type": "none"},
						"paths": []any{
							map[string]any{
								"id":   "sp1",
								"rule": map[string]any{"handler_type": "reverse_proxy"},
							},
						},
					},
				},
			},
		},
	}

	changes := migrateV1100(m)
	if len(changes) != 4 {
		t.Fatalf("expected 4 changes, got %d: %v", len(changes), changes)
	}

	dom := m["domains"].([]any)[0].(map[string]any)
	if dom["rule"].(map[string]any)["enabled"] != true {
		t.Error("domain rule.enabled should default to true")
	}
	domPath := dom["paths"].([]any)[0].(map[string]any)
	if domPath["rule"].(map[string]any)["enabled"] != true {
		t.Error("domain path rule.enabled should default to true")
	}
	sub := dom["subdomains"].([]any)[0].(map[string]any)
	if sub["rule"].(map[string]any)["enabled"] != true {
		t.Error("subdomain rule.enabled should default to true")
	}
	subPath := sub["paths"].([]any)[0].(map[string]any)
	if subPath["rule"].(map[string]any)["enabled"] != true {
		t.Error("subdomain path rule.enabled should default to true")
	}
}

func TestMigrateV1100PreservesExistingFalse(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"rule": map[string]any{"handler_type": "reverse_proxy", "enabled": false},
			},
		},
	}

	changes := migrateV1100(m)
	if len(changes) != 0 {
		t.Errorf("expected no changes when enabled is already set, got %v", changes)
	}

	dom := m["domains"].([]any)[0].(map[string]any)
	if dom["rule"].(map[string]any)["enabled"] != false {
		t.Error("existing rule.enabled=false should be preserved")
	}
}

func TestMigrateV1100NoOpWhenMissing(t *testing.T) {
	m := map[string]any{}
	changes := migrateV1100(m)
	if len(changes) != 0 {
		t.Errorf("expected no changes when domains missing, got %v", changes)
	}
}

func TestMigrateV1100SkipsRuleWithoutMap(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"rule": "not a map",
			},
		},
	}
	changes := migrateV1100(m)
	if len(changes) != 0 {
		t.Errorf("expected no changes when rule is not a map, got %v", changes)
	}
}
