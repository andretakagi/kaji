package export

import (
	"encoding/json"
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

	rules, ok := dom["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %v", dom["rules"])
	}

	rule, ok := rules[0].(map[string]any)
	if !ok {
		t.Fatalf("rule should be map[string]any, got %T", rules[0])
	}
	if rule["id"] != "rule_migrated_example_com" {
		t.Errorf("rule id = %v, want rule_migrated_example_com", rule["id"])
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
	rules := dom["rules"].([]any)
	rule := rules[0].(map[string]any)

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
