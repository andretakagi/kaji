package export

import (
	"strings"
	"testing"
)

func TestMigrateForwardAuth_AddsDefault(t *testing.T) {
	m := map[string]any{}
	changes := migrateForwardAuth(m)

	fa, ok := m["forward_auth"].(map[string]any)
	if !ok {
		t.Fatal("forward_auth not added")
	}
	if fa["enabled"] != false {
		t.Errorf("enabled = %v, want false", fa["enabled"])
	}
	if len(changes) != 1 {
		t.Errorf("changes = %v, want 1 entry", changes)
	}
}

func TestMigrateForwardAuth_PreservesExisting(t *testing.T) {
	m := map[string]any{
		"forward_auth": map[string]any{"enabled": true, "provider": "authelia", "url": "https://auth.example.com"},
	}
	changes := migrateForwardAuth(m)

	fa := m["forward_auth"].(map[string]any)
	if fa["enabled"] != true {
		t.Error("existing forward_auth was overwritten")
	}
	for _, c := range changes {
		if strings.Contains(c, "forward_auth") {
			t.Error("should not report change when forward_auth already exists")
		}
	}
}

func TestMigrateForwardAuth_ConvertsDomainBasicAuth(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"toggles": map[string]any{
					"basic_auth": map[string]any{
						"enabled":  true,
						"username": "admin",
					},
				},
			},
		},
	}
	changes := migrateForwardAuth(m)

	dom := m["domains"].([]any)[0].(map[string]any)
	toggles := dom["toggles"].(map[string]any)

	if _, ok := toggles["basic_auth"]; ok {
		t.Error("old basic_auth field should be removed")
	}
	auth, ok := toggles["auth"].(map[string]any)
	if !ok {
		t.Fatal("auth field not created")
	}
	if auth["mode"] != "basic" {
		t.Errorf("mode = %v, want basic", auth["mode"])
	}
	ba := auth["basic_auth"].(map[string]any)
	if ba["username"] != "admin" {
		t.Errorf("username = %v, want admin", ba["username"])
	}
	if _, ok := ba["enabled"]; ok {
		t.Error("enabled field should be removed from basic_auth")
	}

	found := false
	for _, c := range changes {
		if strings.Contains(c, "example.com") {
			found = true
		}
	}
	if !found {
		t.Errorf("changes should mention example.com, got %v", changes)
	}
}

func TestMigrateForwardAuth_ConvertsDisabledBasicAuth(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"toggles": map[string]any{
					"basic_auth": map[string]any{
						"enabled": false,
					},
				},
			},
		},
	}
	migrateForwardAuth(m)

	dom := m["domains"].([]any)[0].(map[string]any)
	auth := dom["toggles"].(map[string]any)["auth"].(map[string]any)
	if auth["mode"] != "off" {
		t.Errorf("mode = %v, want off (basic_auth was disabled)", auth["mode"])
	}
}

func TestMigrateForwardAuth_SkipsIfAuthExists(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name": "example.com",
				"toggles": map[string]any{
					"basic_auth": map[string]any{"enabled": true},
					"auth":       map[string]any{"mode": "forward"},
				},
			},
		},
	}
	migrateForwardAuth(m)

	toggles := m["domains"].([]any)[0].(map[string]any)["toggles"].(map[string]any)
	auth := toggles["auth"].(map[string]any)
	if auth["mode"] != "forward" {
		t.Error("existing auth field was overwritten")
	}
	if _, ok := toggles["basic_auth"]; ok {
		t.Error("basic_auth should still be cleaned up even when auth exists")
	}
}

func TestMigrateForwardAuth_ConvertsSubdomainAndPaths(t *testing.T) {
	m := map[string]any{
		"domains": []any{
			map[string]any{
				"name":    "example.com",
				"toggles": map[string]any{},
				"subdomains": []any{
					map[string]any{
						"name": "app",
						"toggles": map[string]any{
							"basic_auth": map[string]any{"enabled": true, "username": "user"},
						},
						"paths": []any{
							map[string]any{
								"name": "/api",
								"toggle_overrides": map[string]any{
									"basic_auth": map[string]any{"enabled": false},
								},
							},
						},
					},
				},
				"paths": []any{
					map[string]any{
						"name": "/admin",
						"toggle_overrides": map[string]any{
							"basic_auth": map[string]any{"enabled": true, "username": "root"},
						},
					},
				},
			},
		},
	}
	changes := migrateForwardAuth(m)

	dom := m["domains"].([]any)[0].(map[string]any)

	// Subdomain toggles converted
	sub := dom["subdomains"].([]any)[0].(map[string]any)
	subAuth := sub["toggles"].(map[string]any)["auth"].(map[string]any)
	if subAuth["mode"] != "basic" {
		t.Errorf("subdomain mode = %v, want basic", subAuth["mode"])
	}

	// Subdomain path overrides converted
	subPath := sub["paths"].([]any)[0].(map[string]any)
	subPathAuth := subPath["toggle_overrides"].(map[string]any)["auth"].(map[string]any)
	if subPathAuth["mode"] != "off" {
		t.Errorf("subdomain path mode = %v, want off", subPathAuth["mode"])
	}

	// Domain path overrides converted
	domPath := dom["paths"].([]any)[0].(map[string]any)
	domPathAuth := domPath["toggle_overrides"].(map[string]any)["auth"].(map[string]any)
	if domPathAuth["mode"] != "basic" {
		t.Errorf("domain path mode = %v, want basic", domPathAuth["mode"])
	}

	// Should have changes for forward_auth default + subdomain + subdomain path + domain path
	if len(changes) < 4 {
		t.Errorf("expected at least 4 changes, got %d: %v", len(changes), changes)
	}
}
