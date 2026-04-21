package export

import "testing"

func TestMigrateV190RenamesRouteIPLists(t *testing.T) {
	m := map[string]any{
		"route_ip_lists": map[string]any{
			"domain_abc": "list1",
		},
	}
	changes := migrateV190(m)
	if _, ok := m["route_ip_lists"]; ok {
		t.Fatal("route_ip_lists should be removed")
	}
	if _, ok := m["domain_ip_lists"]; !ok {
		t.Fatal("domain_ip_lists should exist")
	}
	found := false
	for _, c := range changes {
		if c == "renamed route_ip_lists to domain_ip_lists" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected rename change message, got %v", changes)
	}
}

func TestMigrateV190NoOpWhenMissing(t *testing.T) {
	m := map[string]any{
		"domains": []any{},
	}
	changes := migrateV190(m)
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %v", changes)
	}
}
