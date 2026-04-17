package caddy

import (
	"encoding/json"
	"testing"
)

func rpConfig(t *testing.T, upstream string) json.RawMessage {
	t.Helper()
	return mustMarshal(t, ReverseProxyConfig{Upstream: upstream})
}

func TestBuildDesiredState_EnabledDomains(t *testing.T) {
	domains := []SyncDomain{
		{
			Name:    "example.com",
			Enabled: true,
			Toggles: DomainToggles{},
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{
						RuleID:        "rule_aaa",
						MatchType:     "",
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:3000"),
					},
					Enabled: true,
				},
			},
		},
		{
			Name:    "other.com",
			Enabled: true,
			Toggles: DomainToggles{},
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{
						RuleID:        "rule_bbb",
						MatchType:     "subdomain",
						MatchValue:    "api",
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:4000"),
					},
					Enabled: true,
				},
			},
		},
	}

	desired, err := BuildDesiredState(domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(desired) != 2 {
		t.Fatalf("desired count = %d, want 2", len(desired))
	}

	if _, ok := desired["kaji_rule_aaa"]; !ok {
		t.Error("missing kaji_rule_aaa in desired state")
	}
	if _, ok := desired["kaji_rule_bbb"]; !ok {
		t.Error("missing kaji_rule_bbb in desired state")
	}
}

func TestBuildDesiredState_DisabledDomain(t *testing.T) {
	domains := []SyncDomain{
		{
			Name:    "disabled.com",
			Enabled: false,
			Toggles: DomainToggles{},
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{
						RuleID:        "rule_ccc",
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:5000"),
					},
					Enabled: true,
				},
			},
		},
		{
			Name:    "enabled.com",
			Enabled: true,
			Toggles: DomainToggles{},
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{
						RuleID:        "rule_ddd",
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:6000"),
					},
					Enabled: true,
				},
			},
		},
	}

	desired, err := BuildDesiredState(domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(desired) != 1 {
		t.Fatalf("desired count = %d, want 1", len(desired))
	}

	if _, ok := desired["kaji_rule_ccc"]; ok {
		t.Error("disabled domain rule should not be in desired state")
	}
	if _, ok := desired["kaji_rule_ddd"]; !ok {
		t.Error("missing kaji_rule_ddd from enabled domain")
	}
}

func TestBuildDesiredState_DisabledRule(t *testing.T) {
	domains := []SyncDomain{
		{
			Name:    "example.com",
			Enabled: true,
			Toggles: DomainToggles{},
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{
						RuleID:        "rule_e1",
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:7000"),
					},
					Enabled: true,
				},
				{
					RuleBuildParams: RuleBuildParams{
						RuleID:        "rule_e2",
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:7001"),
					},
					Enabled: false,
				},
			},
		},
	}

	desired, err := BuildDesiredState(domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(desired) != 1 {
		t.Fatalf("desired count = %d, want 1", len(desired))
	}

	if _, ok := desired["kaji_rule_e1"]; !ok {
		t.Error("missing enabled rule kaji_rule_e1")
	}
	if _, ok := desired["kaji_rule_e2"]; ok {
		t.Error("disabled rule kaji_rule_e2 should not be in desired state")
	}
}

func TestBuildDesiredState_ToggleOverride(t *testing.T) {
	domains := []SyncDomain{
		{
			Name:    "example.com",
			Enabled: true,
			Toggles: DomainToggles{
				ForceHTTPS:  true,
				Compression: true,
			},
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{
						RuleID:        "rule_override",
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:8000"),
					},
					Enabled: true,
					ToggleOverrides: &DomainToggles{
						ForceHTTPS:  false,
						Compression: false,
					},
				},
			},
		},
	}

	desired, err := BuildDesiredState(domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	routeJSON := desired["kaji_rule_override"]
	route := unmarshalRoute(t, routeJSON)
	handlers := route["handle"].([]any)

	// With overrides disabling force_https and compression, we should only
	// see the reverse_proxy handler (plus any default response header handlers)
	last := handlers[len(handlers)-1].(map[string]any)
	if last["handler"] != "reverse_proxy" {
		t.Errorf("last handler = %v, want reverse_proxy", last["handler"])
	}

	// No subroute (force HTTPS) or encode (compression) handler should be present
	for _, h := range handlers {
		hm := h.(map[string]any)
		if hm["handler"] == "subroute" {
			t.Error("unexpected subroute handler with force_https overridden to false")
		}
		if hm["handler"] == "encode" {
			t.Error("unexpected encode handler with compression overridden to false")
		}
	}
}

func TestBuildDesiredState_WithIPList(t *testing.T) {
	domains := []SyncDomain{
		{
			Name:    "secure.com",
			Enabled: true,
			Toggles: DomainToggles{
				IPFiltering: IPFilteringOpts{
					Enabled: true,
					ListID:  "list_1",
					Type:    "whitelist",
				},
			},
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{
						RuleID:        "rule_ip",
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:9000"),
					},
					Enabled: true,
				},
			},
		},
	}

	resolveIPs := func(listID string) ([]string, string, error) {
		if listID == "list_1" {
			return []string{"10.0.0.0/8"}, "whitelist", nil
		}
		return nil, "", nil
	}

	desired, err := BuildDesiredState(domains, resolveIPs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	routeJSON := desired["kaji_rule_ip"]
	route := unmarshalRoute(t, routeJSON)
	handlers := route["handle"].([]any)

	foundIPFilter := false
	for _, h := range handlers {
		hm := h.(map[string]any)
		if hm["handler"] == "subroute" {
			routes, ok := hm["routes"].([]any)
			if !ok || len(routes) == 0 {
				continue
			}
			r := routes[0].(map[string]any)
			matchList, ok := r["match"].([]any)
			if !ok || len(matchList) == 0 {
				continue
			}
			m := matchList[0].(map[string]any)
			if _, hasNot := m["not"]; hasNot {
				foundIPFilter = true
			}
		}
	}
	if !foundIPFilter {
		t.Error("expected IP filtering subroute with whitelist 'not' matcher")
	}
}

func TestBuildDesiredState_EmptyDomains(t *testing.T) {
	desired, err := BuildDesiredState(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(desired) != 0 {
		t.Errorf("desired count = %d, want 0", len(desired))
	}
}

func TestDiffRoutes_AddNew(t *testing.T) {
	desired := map[string]json.RawMessage{
		"kaji_rule_new": json.RawMessage(`{"@id":"kaji_rule_new"}`),
	}
	current := map[string]json.RawMessage{}

	adds, updates, deletes := DiffRoutes(desired, current, nil)

	if len(adds) != 1 {
		t.Fatalf("adds count = %d, want 1", len(adds))
	}
	if _, ok := adds["kaji_rule_new"]; !ok {
		t.Error("missing kaji_rule_new in adds")
	}
	if len(updates) != 0 {
		t.Errorf("updates count = %d, want 0", len(updates))
	}
	if len(deletes) != 0 {
		t.Errorf("deletes count = %d, want 0", len(deletes))
	}
}

func TestDiffRoutes_UpdateChanged(t *testing.T) {
	desired := map[string]json.RawMessage{
		"kaji_rule_x": json.RawMessage(`{"@id":"kaji_rule_x","version":2}`),
	}
	current := map[string]json.RawMessage{
		"kaji_rule_x": json.RawMessage(`{"@id":"kaji_rule_x","version":1}`),
	}

	adds, updates, deletes := DiffRoutes(desired, current, nil)

	if len(adds) != 0 {
		t.Errorf("adds count = %d, want 0", len(adds))
	}
	if len(updates) != 1 {
		t.Fatalf("updates count = %d, want 1", len(updates))
	}
	if _, ok := updates["kaji_rule_x"]; !ok {
		t.Error("missing kaji_rule_x in updates")
	}
	if len(deletes) != 0 {
		t.Errorf("deletes count = %d, want 0", len(deletes))
	}
}

func TestDiffRoutes_Unchanged(t *testing.T) {
	route := json.RawMessage(`{"@id":"kaji_rule_same","handle":[]}`)
	desired := map[string]json.RawMessage{"kaji_rule_same": route}
	current := map[string]json.RawMessage{"kaji_rule_same": route}

	adds, updates, deletes := DiffRoutes(desired, current, nil)

	if len(adds) != 0 {
		t.Errorf("adds count = %d, want 0", len(adds))
	}
	if len(updates) != 0 {
		t.Errorf("updates count = %d, want 0", len(updates))
	}
	if len(deletes) != 0 {
		t.Errorf("deletes count = %d, want 0", len(deletes))
	}
}

func TestDiffRoutes_DeleteOrphan(t *testing.T) {
	desired := map[string]json.RawMessage{}
	current := map[string]json.RawMessage{
		"kaji_rule_orphan": json.RawMessage(`{"@id":"kaji_rule_orphan"}`),
	}

	adds, updates, deletes := DiffRoutes(desired, current, nil)

	if len(adds) != 0 {
		t.Errorf("adds count = %d, want 0", len(adds))
	}
	if len(updates) != 0 {
		t.Errorf("updates count = %d, want 0", len(updates))
	}
	if len(deletes) != 1 {
		t.Fatalf("deletes count = %d, want 1", len(deletes))
	}
	if deletes[0] != "kaji_rule_orphan" {
		t.Errorf("delete[0] = %q, want kaji_rule_orphan", deletes[0])
	}
}

func TestDiffRoutes_DisabledNotDeleted(t *testing.T) {
	desired := map[string]json.RawMessage{}
	current := map[string]json.RawMessage{
		"kaji_rule_disabled": json.RawMessage(`{"@id":"kaji_rule_disabled"}`),
		"kaji_rule_orphan":   json.RawMessage(`{"@id":"kaji_rule_orphan"}`),
	}
	disabled := map[string]bool{
		"kaji_rule_disabled": true,
	}

	_, _, deletes := DiffRoutes(desired, current, disabled)

	if len(deletes) != 1 {
		t.Fatalf("deletes count = %d, want 1", len(deletes))
	}
	if deletes[0] != "kaji_rule_orphan" {
		t.Errorf("delete[0] = %q, want kaji_rule_orphan", deletes[0])
	}
}

func TestDiffRoutes_Mixed(t *testing.T) {
	desired := map[string]json.RawMessage{
		"kaji_rule_new":       json.RawMessage(`{"@id":"kaji_rule_new"}`),
		"kaji_rule_changed":   json.RawMessage(`{"@id":"kaji_rule_changed","v":2}`),
		"kaji_rule_unchanged": json.RawMessage(`{"@id":"kaji_rule_unchanged"}`),
	}
	current := map[string]json.RawMessage{
		"kaji_rule_changed":   json.RawMessage(`{"@id":"kaji_rule_changed","v":1}`),
		"kaji_rule_unchanged": json.RawMessage(`{"@id":"kaji_rule_unchanged"}`),
		"kaji_rule_stale":     json.RawMessage(`{"@id":"kaji_rule_stale"}`),
	}

	a, u, d := DiffRoutes(desired, current, nil)

	if len(a) != 1 {
		t.Errorf("adds count = %d, want 1", len(a))
	}
	if len(u) != 1 {
		t.Errorf("updates count = %d, want 1", len(u))
	}
	if len(d) != 1 {
		t.Errorf("deletes count = %d, want 1", len(d))
	}
}

func TestCollectDisabledIDs_DisabledRules(t *testing.T) {
	domains := []SyncDomain{
		{
			Name:    "example.com",
			Enabled: true,
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{RuleID: "rule_on"},
					Enabled:         true,
				},
				{
					RuleBuildParams: RuleBuildParams{RuleID: "rule_off"},
					Enabled:         false,
				},
			},
		},
	}

	ids := CollectDisabledIDs(domains)

	if ids["kaji_rule_on"] {
		t.Error("enabled rule should not be in disabled IDs")
	}
	if !ids["kaji_rule_off"] {
		t.Error("disabled rule should be in disabled IDs")
	}
}

func TestCollectDisabledIDs_DisabledDomain(t *testing.T) {
	domains := []SyncDomain{
		{
			Name:    "disabled.com",
			Enabled: false,
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{RuleID: "rule_under_disabled"},
					Enabled:         true,
				},
				{
					RuleBuildParams: RuleBuildParams{RuleID: "rule_also_disabled"},
					Enabled:         false,
				},
			},
		},
		{
			Name:    "enabled.com",
			Enabled: true,
			Rules: []SyncRule{
				{
					RuleBuildParams: RuleBuildParams{RuleID: "rule_active"},
					Enabled:         true,
				},
			},
		},
	}

	ids := CollectDisabledIDs(domains)

	if !ids["kaji_rule_under_disabled"] {
		t.Error("rule under disabled domain should be in disabled IDs")
	}
	if !ids["kaji_rule_also_disabled"] {
		t.Error("disabled rule under disabled domain should be in disabled IDs")
	}
	if ids["kaji_rule_active"] {
		t.Error("enabled rule under enabled domain should not be in disabled IDs")
	}
}

func TestSortRules_Priority(t *testing.T) {
	rules := []SyncRule{
		{RuleBuildParams: RuleBuildParams{RuleID: "root", MatchType: ""}},
		{RuleBuildParams: RuleBuildParams{RuleID: "regex", MatchType: "path", PathMatch: "regex"}},
		{RuleBuildParams: RuleBuildParams{RuleID: "prefix", MatchType: "path", PathMatch: "prefix"}},
		{RuleBuildParams: RuleBuildParams{RuleID: "exact", MatchType: "path", PathMatch: "exact"}},
		{RuleBuildParams: RuleBuildParams{RuleID: "subdomain", MatchType: "subdomain"}},
	}

	sorted := sortRules(rules)

	expected := []string{"exact", "prefix", "subdomain", "regex", "root"}
	for i, want := range expected {
		if sorted[i].RuleID != want {
			t.Errorf("sorted[%d].RuleID = %q, want %q", i, sorted[i].RuleID, want)
		}
	}
}

func TestSortRules_StableOrder(t *testing.T) {
	rules := []SyncRule{
		{RuleBuildParams: RuleBuildParams{RuleID: "prefix_a", MatchType: "path", PathMatch: "prefix"}},
		{RuleBuildParams: RuleBuildParams{RuleID: "prefix_b", MatchType: "path", PathMatch: "prefix"}},
		{RuleBuildParams: RuleBuildParams{RuleID: "prefix_c", MatchType: "path", PathMatch: "prefix"}},
	}

	sorted := sortRules(rules)

	if sorted[0].RuleID != "prefix_a" || sorted[1].RuleID != "prefix_b" || sorted[2].RuleID != "prefix_c" {
		t.Errorf("stable sort violated: got %s, %s, %s", sorted[0].RuleID, sorted[1].RuleID, sorted[2].RuleID)
	}
}

func TestJsonEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b json.RawMessage
		want bool
	}{
		{
			name: "identical",
			a:    json.RawMessage(`{"a":1}`),
			b:    json.RawMessage(`{"a":1}`),
			want: true,
		},
		{
			name: "different key order",
			a:    json.RawMessage(`{"a":1,"b":2}`),
			b:    json.RawMessage(`{"b":2,"a":1}`),
			want: true,
		},
		{
			name: "different whitespace",
			a:    json.RawMessage(`{ "a" : 1 }`),
			b:    json.RawMessage(`{"a":1}`),
			want: true,
		},
		{
			name: "different values",
			a:    json.RawMessage(`{"a":1}`),
			b:    json.RawMessage(`{"a":2}`),
			want: false,
		},
		{
			name: "extra key",
			a:    json.RawMessage(`{"a":1}`),
			b:    json.RawMessage(`{"a":1,"b":2}`),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("jsonEqual = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderedAdds(t *testing.T) {
	adds := map[string]json.RawMessage{
		"kaji_rule_c": json.RawMessage(`{"@id":"kaji_rule_c"}`),
		"kaji_rule_a": json.RawMessage(`{"@id":"kaji_rule_a"}`),
		"kaji_rule_b": json.RawMessage(`{"@id":"kaji_rule_b"}`),
	}

	ordered := orderedAdds(adds)

	if len(ordered) != 3 {
		t.Fatalf("ordered count = %d, want 3", len(ordered))
	}

	var ids []string
	for _, raw := range ordered {
		var r struct {
			ID string `json:"@id"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		ids = append(ids, r.ID)
	}

	expected := []string{"kaji_rule_a", "kaji_rule_b", "kaji_rule_c"}
	for i, want := range expected {
		if ids[i] != want {
			t.Errorf("ordered[%d] = %q, want %q", i, ids[i], want)
		}
	}
}
