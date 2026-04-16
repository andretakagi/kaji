package export

import (
	"testing"
)

func TestMigrateV170AddsRouteSettings(t *testing.T) {
	m := map[string]any{
		"listen_addr": ":8080",
	}
	changes := migrateV170(m)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	rs, ok := m["route_settings"]
	if !ok {
		t.Fatal("route_settings not added")
	}
	if _, ok := rs.(map[string]any); !ok {
		t.Errorf("route_settings should be map[string]any, got %T", rs)
	}
}

func TestMigrateV170PreservesExistingRouteSettings(t *testing.T) {
	existing := map[string]any{"some-route-id": map[string]any{"notes": "important"}}
	m := map[string]any{
		"route_settings": existing,
	}
	changes := migrateV170(m)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes when route_settings exists, got %d: %v", len(changes), changes)
	}
	rs := m["route_settings"].(map[string]any)
	if _, ok := rs["some-route-id"]; !ok {
		t.Error("existing route_settings data was lost")
	}
}

func TestMigrateV170RunsForOldVersion(t *testing.T) {
	m := map[string]any{}
	changes, err := RunMigrations(m, "1.6.0")
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	found := false
	for _, c := range changes {
		if c != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected migration to run for version 1.6.0")
	}
	if _, ok := m["route_settings"]; !ok {
		t.Error("route_settings not added for version 1.6.0")
	}
}

func TestMigrateV170SkipsForCurrentVersion(t *testing.T) {
	m := map[string]any{}
	changes, err := RunMigrations(m, "1.7.0")
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	for _, c := range changes {
		if c == "added route_settings (default: map[])" {
			t.Error("migration should not run for version 1.7.0")
		}
	}
	if _, ok := m["route_settings"]; ok {
		t.Error("route_settings should not be added for version 1.7.0")
	}
}
