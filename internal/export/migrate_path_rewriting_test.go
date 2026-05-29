package export

import (
	"testing"
)

func TestMigratePathRewriting_AddsDefaults(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"rule": map[string]any{
					"handler_type":   "reverse_proxy",
					"handler_config": map[string]any{"upstream": "http://localhost:8080"},
				},
			},
		},
	}
	changes := migratePathRewriting(m)

	hc := m["domains"].([]any)[0].(map[string]any)["rule"].(map[string]any)["handler_config"].(map[string]any)
	if hc["strip_path_prefix"] != "" {
		t.Errorf("strip_path_prefix = %v, want empty string", hc["strip_path_prefix"])
	}
	if hc["prepend_path_prefix"] != "" {
		t.Errorf("prepend_path_prefix = %v, want empty string", hc["prepend_path_prefix"])
	}
	if len(changes) != 2 {
		t.Errorf("changes count = %d, want 2: %v", len(changes), changes)
	}
}

func TestMigratePathRewriting_PreservesExisting(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"rule": map[string]any{
					"handler_type": "reverse_proxy",
					"handler_config": map[string]any{
						"upstream":            "http://localhost:8080",
						"strip_path_prefix":   "/api",
						"prepend_path_prefix": "/v2",
					},
				},
			},
		},
	}
	changes := migratePathRewriting(m)

	hc := m["domains"].([]any)[0].(map[string]any)["rule"].(map[string]any)["handler_config"].(map[string]any)
	if hc["strip_path_prefix"] != "/api" {
		t.Errorf("strip_path_prefix = %v, want /api", hc["strip_path_prefix"])
	}
	if hc["prepend_path_prefix"] != "/v2" {
		t.Errorf("prepend_path_prefix = %v, want /v2", hc["prepend_path_prefix"])
	}
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none", changes)
	}
}

func TestMigratePathRewriting_SkipsNonReverseProxy(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"rule": map[string]any{
					"handler_type":   "static_response",
					"handler_config": map[string]any{"status_code": 200},
				},
			},
		},
	}
	changes := migratePathRewriting(m)

	hc := m["domains"].([]any)[0].(map[string]any)["rule"].(map[string]any)["handler_config"].(map[string]any)
	if _, ok := hc["strip_path_prefix"]; ok {
		t.Error("strip_path_prefix should not be added to non-reverse_proxy")
	}
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none", changes)
	}
}

func TestMigratePathRewriting_SubdomainsAndPaths(t *testing.T) {
	rpConfig := func() map[string]any {
		return map[string]any{
			"handler_type":   "reverse_proxy",
			"handler_config": map[string]any{"upstream": "http://localhost:8080"},
		}
	}

	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"rule": rpConfig(),
				"subdomains": []any{
					map[string]any{
						"name": "app",
						"rule": rpConfig(),
						"paths": []any{
							map[string]any{"name": "/api", "rule": rpConfig()},
						},
					},
				},
				"paths": []any{
					map[string]any{"name": "/admin", "rule": rpConfig()},
				},
			},
		},
	}
	changes := migratePathRewriting(m)

	// domain + subdomain + subdomain path + domain path = 4 entities, 2 fields each = 8 changes
	if len(changes) != 8 {
		t.Errorf("changes count = %d, want 8: %v", len(changes), changes)
	}
}

func TestMigratePathRewriting_NoDomains(t *testing.T) {
	m := map[string]any{}
	changes := migratePathRewriting(m)
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none", changes)
	}
}
