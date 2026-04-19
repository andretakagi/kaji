package export

import (
	"testing"
)

func TestMigrateV180MovesRequestHeaders(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"toggles": map[string]any{
					"headers": map[string]any{
						"response": map[string]any{"enabled": true},
						"request": map[string]any{
							"enabled":       true,
							"host_override": true,
							"host_value":    "backend.local",
						},
					},
				},
				"rules": []any{
					map[string]any{
						"handler_type":   "reverse_proxy",
						"handler_config": map[string]any{"upstream": "localhost:3000"},
					},
				},
			},
		},
	}

	changes := migrateV180(m)

	if len(changes) == 0 {
		t.Fatal("expected changes")
	}

	dom := m["domains"].([]any)[0].(map[string]any)

	// Request should be removed from toggles headers
	headers := dom["toggles"].(map[string]any)["headers"].(map[string]any)
	if _, ok := headers["request"]; ok {
		t.Error("request should be removed from domain toggles headers")
	}

	// Request headers should be in handler_config
	rule := dom["rules"].([]any)[0].(map[string]any)
	hc := rule["handler_config"].(map[string]any)
	rh, ok := hc["request_headers"]
	if !ok {
		t.Fatal("request_headers not found in handler_config")
	}
	rhMap := rh.(map[string]any)
	if rhMap["host_override"] != true {
		t.Errorf("host_override = %v, want true", rhMap["host_override"])
	}
	if rhMap["host_value"] != "backend.local" {
		t.Errorf("host_value = %v, want backend.local", rhMap["host_value"])
	}
}

func TestMigrateV180OverrideTakesPriority(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "override.com",
				"toggles": map[string]any{
					"headers": map[string]any{
						"request": map[string]any{
							"enabled":    true,
							"host_value": "domain-level.local",
						},
					},
				},
				"rules": []any{
					map[string]any{
						"handler_type":   "reverse_proxy",
						"handler_config": map[string]any{"upstream": "localhost:4000"},
						"toggle_overrides": map[string]any{
							"headers": map[string]any{
								"request": map[string]any{
									"enabled":    true,
									"host_value": "rule-level.local",
								},
							},
						},
					},
				},
			},
		},
	}

	migrateV180(m)

	dom := m["domains"].([]any)[0].(map[string]any)
	rule := dom["rules"].([]any)[0].(map[string]any)
	hc := rule["handler_config"].(map[string]any)
	rh := hc["request_headers"].(map[string]any)
	if rh["host_value"] != "rule-level.local" {
		t.Errorf("host_value = %v, want rule-level.local (override should take priority)", rh["host_value"])
	}

	// Override request should also be cleaned up
	overrides := rule["toggle_overrides"].(map[string]any)
	overrideHeaders := overrides["headers"].(map[string]any)
	if _, ok := overrideHeaders["request"]; ok {
		t.Error("request should be removed from toggle_overrides headers")
	}
}

func TestMigrateV180SkipsStaticResponseRules(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "static.com",
				"toggles": map[string]any{
					"headers": map[string]any{
						"request": map[string]any{
							"enabled":    true,
							"host_value": "should-not-appear.local",
						},
					},
				},
				"rules": []any{
					map[string]any{
						"handler_type":   "static_response",
						"handler_config": map[string]any{"status_code": "200", "body": "OK"},
					},
				},
			},
		},
	}

	migrateV180(m)

	dom := m["domains"].([]any)[0].(map[string]any)
	rule := dom["rules"].([]any)[0].(map[string]any)
	hc := rule["handler_config"].(map[string]any)
	if _, ok := hc["request_headers"]; ok {
		t.Error("static_response rules should not receive request_headers")
	}
}

func TestMigrateV180NoDomains(t *testing.T) {
	m := map[string]any{}
	changes := migrateV180(m)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for missing domains, got %d", len(changes))
	}
}

func TestMigrateV180NoRequestHeaders(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "no-request.com",
				"toggles": map[string]any{
					"headers": map[string]any{
						"response": map[string]any{"enabled": true},
					},
				},
				"rules": []any{
					map[string]any{
						"handler_type":   "reverse_proxy",
						"handler_config": map[string]any{"upstream": "localhost:5000"},
					},
				},
			},
		},
	}

	changes := migrateV180(m)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes when no request headers exist, got %d", len(changes))
	}
}

func TestMigrateV180PreservesExistingRequestHeaders(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "existing.com",
				"toggles": map[string]any{
					"headers": map[string]any{
						"request": map[string]any{
							"enabled":    true,
							"host_value": "new.local",
						},
					},
				},
				"rules": []any{
					map[string]any{
						"handler_type": "reverse_proxy",
						"handler_config": map[string]any{
							"upstream": "localhost:6000",
							"request_headers": map[string]any{
								"enabled":    true,
								"host_value": "already-set.local",
							},
						},
					},
				},
			},
		},
	}

	migrateV180(m)

	dom := m["domains"].([]any)[0].(map[string]any)
	rule := dom["rules"].([]any)[0].(map[string]any)
	hc := rule["handler_config"].(map[string]any)
	rh := hc["request_headers"].(map[string]any)
	if rh["host_value"] != "already-set.local" {
		t.Errorf("host_value = %v, want already-set.local (existing should not be overwritten)", rh["host_value"])
	}
}

func TestMigrateV180RunsForVersion170(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "versioned.com",
				"toggles": map[string]any{
					"headers": map[string]any{
						"request": map[string]any{"enabled": true},
					},
				},
				"rules": []any{
					map[string]any{
						"handler_type":   "reverse_proxy",
						"handler_config": map[string]any{"upstream": "localhost:7000"},
					},
				},
			},
		},
	}

	changes, err := RunMigrations(m, "1.7.0")
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if len(changes) == 0 {
		t.Error("expected migration to run for version 1.7.0")
	}
}

func TestMigrateV180SkipsForVersion180(t *testing.T) {
	m := map[string]any{}
	changes, err := RunMigrations(m, "1.8.0")
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for version 1.8.0, got %d", len(changes))
	}
}
