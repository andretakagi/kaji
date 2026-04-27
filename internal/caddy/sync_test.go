package caddy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func rpConfig(t *testing.T, upstream string) json.RawMessage {
	t.Helper()
	return mustMarshal(t, ReverseProxyConfig{Upstream: upstream})
}

func TestBuildDesiredState_EnabledDomains(t *testing.T) {
	domains := []SyncDomain{
		{
			ID:      "rule_aaa",
			Name:    "example.com",
			Enabled: true,
			Toggles: DomainToggles{},
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: rpConfig(t, "localhost:3000"),
				Enabled:       true,
			},
		},
		{
			ID:      "dom_other",
			Name:    "other.com",
			Enabled: true,
			Toggles: DomainToggles{},
			Paths: []SyncPath{
				{
					ID:         "rule_bbb",
					Enabled:    true,
					PathMatch:  "prefix",
					MatchValue: "/api",
					Rule: SyncRule{
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:4000"),
						Enabled:       true,
					},
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
			ID:      "rule_ccc",
			Name:    "disabled.com",
			Enabled: false,
			Toggles: DomainToggles{},
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: rpConfig(t, "localhost:5000"),
				Enabled:       true,
			},
		},
		{
			ID:      "rule_ddd",
			Name:    "enabled.com",
			Enabled: true,
			Toggles: DomainToggles{},
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: rpConfig(t, "localhost:6000"),
				Enabled:       true,
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
			ID:      "rule_e1",
			Name:    "example.com",
			Enabled: true,
			Toggles: DomainToggles{},
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: rpConfig(t, "localhost:7000"),
				Enabled:       true,
			},
			Paths: []SyncPath{
				{
					ID:         "rule_e2",
					Enabled:    false,
					PathMatch:  "prefix",
					MatchValue: "/disabled",
					Rule: SyncRule{
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:7001"),
						Enabled:       true,
					},
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
		t.Error("disabled path kaji_rule_e2 should not be in desired state")
	}
}

func TestBuildDesiredState_ToggleOverride(t *testing.T) {
	domains := []SyncDomain{
		{
			ID:      "dom_override",
			Name:    "example.com",
			Enabled: true,
			Toggles: DomainToggles{
				ForceHTTPS:  true,
				Compression: true,
			},
			Paths: []SyncPath{
				{
					ID:         "rule_override",
					Enabled:    true,
					PathMatch:  "prefix",
					MatchValue: "/app",
					Rule: SyncRule{
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:8000"),
						Enabled:       true,
					},
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

	last := handlers[len(handlers)-1].(map[string]any)
	if last["handler"] != "reverse_proxy" {
		t.Errorf("last handler = %v, want reverse_proxy", last["handler"])
	}

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
			ID:      "rule_ip",
			Name:    "secure.com",
			Enabled: true,
			Toggles: DomainToggles{
				IPFiltering: IPFilteringOpts{
					Enabled: true,
					ListID:  "list_1",
					Type:    "whitelist",
				},
			},
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: rpConfig(t, "localhost:9000"),
				Enabled:       true,
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

func TestBuildDesiredState_HandlerNoneSkipsRoute(t *testing.T) {
	domains := []SyncDomain{
		{
			ID:      "dom_none",
			Name:    "noroot.com",
			Enabled: true,
			Toggles: DomainToggles{},
			Rule: SyncRule{
				HandlerType: "none",
			},
			Paths: []SyncPath{
				{
					ID:         "rule_kept",
					Enabled:    true,
					PathMatch:  "prefix",
					MatchValue: "/api",
					Rule: SyncRule{
						HandlerType:   "reverse_proxy",
						HandlerConfig: rpConfig(t, "localhost:1234"),
						Enabled:       true,
					},
				},
			},
			Subdomains: []SyncSubdomain{
				{
					ID:      "sub_none",
					Name:    "api",
					Enabled: true,
					Toggles: DomainToggles{},
					Rule: SyncRule{
						HandlerType: "none",
					},
					Paths: []SyncPath{
						{
							ID:         "rule_sub_kept",
							Enabled:    true,
							PathMatch:  "prefix",
							MatchValue: "/v1",
							Rule: SyncRule{
								HandlerType:   "reverse_proxy",
								HandlerConfig: rpConfig(t, "localhost:5678"),
								Enabled:       true,
							},
						},
					},
				},
			},
		},
	}

	desired, err := BuildDesiredState(domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := desired["kaji_dom_none"]; ok {
		t.Error("domain root with HandlerType none must not produce a route")
	}
	if _, ok := desired["kaji_sub_none"]; ok {
		t.Error("subdomain root with HandlerType none must not produce a route")
	}
	if _, ok := desired["kaji_rule_kept"]; !ok {
		t.Error("path under HandlerType-none domain should still produce a route")
	}
	if _, ok := desired["kaji_rule_sub_kept"]; !ok {
		t.Error("path under HandlerType-none subdomain should still produce a route")
	}
}

func TestDiffDomains_AddNew(t *testing.T) {
	desired := map[string]json.RawMessage{
		"kaji_rule_new": json.RawMessage(`{"@id":"kaji_rule_new"}`),
	}
	current := map[string]json.RawMessage{}

	adds, updates, deletes := DiffDomains(desired, current, nil)

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

func TestDiffDomains_UpdateChanged(t *testing.T) {
	desired := map[string]json.RawMessage{
		"kaji_rule_x": json.RawMessage(`{"@id":"kaji_rule_x","version":2}`),
	}
	current := map[string]json.RawMessage{
		"kaji_rule_x": json.RawMessage(`{"@id":"kaji_rule_x","version":1}`),
	}

	adds, updates, deletes := DiffDomains(desired, current, nil)

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

func TestDiffDomains_Unchanged(t *testing.T) {
	route := json.RawMessage(`{"@id":"kaji_rule_same","handle":[]}`)
	desired := map[string]json.RawMessage{"kaji_rule_same": route}
	current := map[string]json.RawMessage{"kaji_rule_same": route}

	adds, updates, deletes := DiffDomains(desired, current, nil)

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

func TestDiffDomains_DeleteOrphan(t *testing.T) {
	desired := map[string]json.RawMessage{}
	current := map[string]json.RawMessage{
		"kaji_rule_orphan": json.RawMessage(`{"@id":"kaji_rule_orphan"}`),
	}

	adds, updates, deletes := DiffDomains(desired, current, nil)

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

func TestDiffDomains_DisabledNotDeleted(t *testing.T) {
	desired := map[string]json.RawMessage{}
	current := map[string]json.RawMessage{
		"kaji_rule_disabled": json.RawMessage(`{"@id":"kaji_rule_disabled"}`),
		"kaji_rule_orphan":   json.RawMessage(`{"@id":"kaji_rule_orphan"}`),
	}
	disabled := map[string]bool{
		"kaji_rule_disabled": true,
	}

	_, _, deletes := DiffDomains(desired, current, disabled)

	if len(deletes) != 1 {
		t.Fatalf("deletes count = %d, want 1", len(deletes))
	}
	if deletes[0] != "kaji_rule_orphan" {
		t.Errorf("delete[0] = %q, want kaji_rule_orphan", deletes[0])
	}
}

func TestDiffDomains_Mixed(t *testing.T) {
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

	a, u, d := DiffDomains(desired, current, nil)

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
			ID:      "rule_on",
			Name:    "example.com",
			Enabled: true,
			Rule: SyncRule{
				HandlerType: "reverse_proxy",
				Enabled:     true,
			},
			Paths: []SyncPath{
				{
					ID:        "rule_off",
					Enabled:   false,
					PathMatch: "prefix",
					Rule: SyncRule{
						HandlerType: "reverse_proxy",
						Enabled:     true,
					},
				},
			},
		},
	}

	ids := CollectDisabledIDs(domains)

	if ids["kaji_rule_on"] {
		t.Error("enabled rule should not be in disabled IDs")
	}
	if !ids["kaji_rule_off"] {
		t.Error("disabled path should be in disabled IDs")
	}
}

func TestCollectDisabledIDs_DisabledDomain(t *testing.T) {
	domains := []SyncDomain{
		{
			ID:      "rule_under_disabled",
			Name:    "disabled.com",
			Enabled: false,
			Rule: SyncRule{
				HandlerType: "reverse_proxy",
				Enabled:     true,
			},
			Paths: []SyncPath{
				{
					ID:        "rule_also_disabled",
					Enabled:   false,
					PathMatch: "prefix",
					Rule: SyncRule{
						HandlerType: "reverse_proxy",
						Enabled:     true,
					},
				},
			},
		},
		{
			ID:      "rule_active",
			Name:    "enabled.com",
			Enabled: true,
			Rule: SyncRule{
				HandlerType: "reverse_proxy",
				Enabled:     true,
			},
		},
	}

	ids := CollectDisabledIDs(domains)

	if !ids["kaji_rule_under_disabled"] {
		t.Error("rule under disabled domain should be in disabled IDs")
	}
	if !ids["kaji_rule_also_disabled"] {
		t.Error("disabled path under disabled domain should be in disabled IDs")
	}
	if ids["kaji_rule_active"] {
		t.Error("enabled rule under enabled domain should not be in disabled IDs")
	}
}

func TestSortPaths_Priority(t *testing.T) {
	paths := []SyncPath{
		{ID: "regex", PathMatch: "regex"},
		{ID: "prefix", PathMatch: "prefix"},
		{ID: "exact", PathMatch: "exact"},
	}

	sorted := sortPaths(paths)

	expected := []string{"exact", "prefix", "regex"}
	for i, want := range expected {
		if sorted[i].ID != want {
			t.Errorf("sorted[%d].ID = %q, want %q", i, sorted[i].ID, want)
		}
	}
}

func TestSortPaths_StableOrder(t *testing.T) {
	paths := []SyncPath{
		{ID: "prefix_a", PathMatch: "prefix"},
		{ID: "prefix_b", PathMatch: "prefix"},
		{ID: "prefix_c", PathMatch: "prefix"},
	}

	sorted := sortPaths(paths)

	if sorted[0].ID != "prefix_a" || sorted[1].ID != "prefix_b" || sorted[2].ID != "prefix_c" {
		t.Errorf("stable sort violated: got %s, %s, %s", sorted[0].ID, sorted[1].ID, sorted[2].ID)
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

// mockCaddyServer is a minimal in-memory Caddy admin API mock.
// It holds a map of route ID -> route JSON under a single server ("srv0").
// Mutations via DELETE /id/{id}, PATCH /config/.../routes/{idx}, and
// POST /config/.../routes are applied to the in-memory state so that
// subsequent GET /config/ calls return the updated config.
type mockCaddyServer struct {
	mu     sync.Mutex
	routes []json.RawMessage // ordered list of routes for srv0
}

func newMockCaddyServer(initial []json.RawMessage) *mockCaddyServer {
	out := make([]json.RawMessage, len(initial))
	copy(out, initial)
	return &mockCaddyServer{routes: out}
}

func (m *mockCaddyServer) buildConfig() json.RawMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	routes := make([]json.RawMessage, len(m.routes))
	copy(routes, m.routes)
	cfg := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": map[string]any{
						"routes": routes,
					},
				},
			},
		},
	}
	b, _ := json.Marshal(cfg)
	return b
}

func (m *mockCaddyServer) deleteByID(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, raw := range m.routes {
		var r struct {
			ID string `json:"@id"`
		}
		if json.Unmarshal(raw, &r) == nil && r.ID == id {
			m.routes = append(m.routes[:i], m.routes[i+1:]...)
			return true
		}
	}
	return false
}

func (m *mockCaddyServer) replaceAtIndex(idx int, route json.RawMessage) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idx < 0 || idx >= len(m.routes) {
		return false
	}
	m.routes[idx] = route
	return true
}

func (m *mockCaddyServer) appendRoute(route json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes = append(m.routes, route)
}

// handler returns an http.Handler that speaks enough of the Caddy admin API
// for SyncDomains to operate against.
func (m *mockCaddyServer) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		routesPath := "/config/apps/http/servers/srv0/routes"

		if r.Method == http.MethodGet && r.URL.Path == "/config/" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(m.buildConfig())
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == routesPath {
			m.mu.Lock()
			b, _ := json.Marshal(m.routes)
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}

		if r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, routesPath+"/") {
			idxStr := strings.TrimPrefix(r.URL.Path, routesPath+"/")
			var idx int
			if _, err := fmt.Sscan(idxStr, &idx); err != nil {
				http.Error(w, "bad index", http.StatusBadRequest)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read body", http.StatusInternalServerError)
				return
			}
			if !m.replaceAtIndex(idx, json.RawMessage(body)) {
				http.Error(w, "index out of range", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == routesPath {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read body", http.StatusInternalServerError)
				return
			}
			m.appendRoute(json.RawMessage(body))
			w.WriteHeader(http.StatusOK)
			return
		}

		if strings.Contains(r.URL.Path, "/logs/") || strings.Contains(r.URL.Path, "/logging/") {
			w.WriteHeader(http.StatusOK)
			return
		}

		http.NotFound(w, r)
	})

	mux.HandleFunc("/id/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/id/")
		if !m.deleteByID(id) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	return mux
}

// newMockCaddyClient creates a test Client backed by a mockCaddyServer.
// The httptest.Server is registered for cleanup.
func newMockCaddyClient(t *testing.T, mock *mockCaddyServer) *Client {
	t.Helper()
	srv := httptest.NewServer(mock.handler())
	t.Cleanup(srv.Close)
	return NewClient(func() string { return srv.URL })
}

// kajiRoute returns a minimal kaji-prefixed route JSON for use in mock state.
func kajiRoute(t *testing.T, id, upstream string) json.RawMessage {
	t.Helper()
	r := map[string]any{
		"@id": id,
		"match": []any{
			map[string]any{"host": []string{strings.TrimPrefix(id, "kaji_rule_") + ".example.com"}},
		},
		"handle": []any{
			map[string]any{
				"handler": "reverse_proxy",
				"upstreams": []any{
					map[string]any{"dial": upstream},
				},
			},
		},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("kajiRoute marshal: %v", err)
	}
	return json.RawMessage(b)
}

// syncDomain builds a minimal SyncDomain whose root rule is a reverse proxy.
func syncDomain(ruleID, upstream string) SyncDomain {
	return SyncDomain{
		ID:      ruleID,
		Name:    ruleID + ".example.com",
		Enabled: true,
		Toggles: DomainToggles{},
		Rule: SyncRule{
			HandlerType:   "reverse_proxy",
			HandlerConfig: mustMarshalStatic(ReverseProxyConfig{Upstream: upstream}),
			Enabled:       true,
		},
	}
}

// mustMarshalStatic marshals v to JSON and panics on error.
// Used in test helpers that cannot receive *testing.T.
func mustMarshalStatic(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshalStatic: %v", err))
	}
	return json.RawMessage(b)
}

func TestReadCurrentKajiDomains_EmptyConfig(t *testing.T) {
	mock := newMockCaddyServer(nil)
	cc := newMockCaddyClient(t, mock)

	routes, server, err := ReadCurrentKajiDomains(cc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 0 {
		t.Errorf("routes count = %d, want 0", len(routes))
	}
	if server != "" {
		t.Errorf("server = %q, want empty", server)
	}
}

func TestReadCurrentKajiDomains_IgnoresNonKajiRoutes(t *testing.T) {
	nonKaji := json.RawMessage(`{"@id":"custom_route","match":[{"host":["other.com"]}]}`)
	kajiOne := kajiRoute(t, "kaji_rule_aaa", "localhost:3000")

	mock := newMockCaddyServer([]json.RawMessage{nonKaji, kajiOne})
	cc := newMockCaddyClient(t, mock)

	routes, serverName, err := ReadCurrentKajiDomains(cc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("routes count = %d, want 1", len(routes))
	}
	if _, ok := routes["kaji_rule_aaa"]; !ok {
		t.Error("missing kaji_rule_aaa in result")
	}
	if _, ok := routes["custom_route"]; ok {
		t.Error("non-kaji route should be excluded")
	}
	if serverName != "srv0" {
		t.Errorf("server = %q, want srv0", serverName)
	}
}

func TestReadCurrentKajiDomains_ReadsMultiple(t *testing.T) {
	initial := []json.RawMessage{
		kajiRoute(t, "kaji_rule_x1", "localhost:3001"),
		kajiRoute(t, "kaji_rule_x2", "localhost:3002"),
		json.RawMessage(`{"@id":"unmanaged"}`),
	}
	mock := newMockCaddyServer(initial)
	cc := newMockCaddyClient(t, mock)

	got, _, err := ReadCurrentKajiDomains(cc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("routes count = %d, want 2", len(got))
	}
}

func TestSyncDomains_AddsNewRoute(t *testing.T) {
	mock := newMockCaddyServer(nil)
	cc := newMockCaddyClient(t, mock)

	domains := []SyncDomain{syncDomain("rule_new1", "localhost:4000")}

	result, err := SyncDomains(cc, domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Added != 1 {
		t.Errorf("Added = %d, want 1", result.Added)
	}
	if result.Updated != 0 {
		t.Errorf("Updated = %d, want 0", result.Updated)
	}
	if result.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0", result.Deleted)
	}

	got, _, err := ReadCurrentKajiDomains(cc)
	if err != nil {
		t.Fatalf("ReadCurrentKajiDomains: %v", err)
	}
	if _, ok := got["kaji_rule_new1"]; !ok {
		t.Error("kaji_rule_new1 not found in config after sync")
	}
}

func TestSyncDomains_DeletesOrphanRoute(t *testing.T) {
	orphan := kajiRoute(t, "kaji_rule_orphan", "localhost:9999")
	mock := newMockCaddyServer([]json.RawMessage{orphan})
	cc := newMockCaddyClient(t, mock)

	result, err := SyncDomains(cc, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", result.Deleted)
	}

	got, _, err := ReadCurrentKajiDomains(cc)
	if err != nil {
		t.Fatalf("ReadCurrentKajiDomains: %v", err)
	}
	if _, ok := got["kaji_rule_orphan"]; ok {
		t.Error("kaji_rule_orphan should have been deleted")
	}
}

func TestSyncDomains_UpdatesChangedRoute(t *testing.T) {
	existing := kajiRoute(t, "kaji_rule_upd", "localhost:1111")
	mock := newMockCaddyServer([]json.RawMessage{existing})
	cc := newMockCaddyClient(t, mock)

	// Same rule ID, different upstream - should produce an update
	domains := []SyncDomain{syncDomain("rule_upd", "localhost:2222")}

	result, err := SyncDomains(cc, domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}
	if result.Added != 0 {
		t.Errorf("Added = %d, want 0", result.Added)
	}
	if result.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0", result.Deleted)
	}
}

func TestSyncDomains_MixedAddUpdateDelete(t *testing.T) {
	existing := []json.RawMessage{
		kajiRoute(t, "kaji_rule_upd2", "localhost:1001"),
		kajiRoute(t, "kaji_rule_gone", "localhost:1002"),
	}
	mock := newMockCaddyServer(existing)
	cc := newMockCaddyClient(t, mock)

	domains := []SyncDomain{
		syncDomain("rule_upd2", "localhost:9001"),
		syncDomain("rule_new2", "localhost:9002"),
	}

	result, err := SyncDomains(cc, domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Added != 1 {
		t.Errorf("Added = %d, want 1", result.Added)
	}
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}
	if result.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", result.Deleted)
	}

	got, _, err := ReadCurrentKajiDomains(cc)
	if err != nil {
		t.Fatalf("ReadCurrentKajiDomains: %v", err)
	}
	if _, ok := got["kaji_rule_gone"]; ok {
		t.Error("kaji_rule_gone should have been deleted")
	}
	if _, ok := got["kaji_rule_new2"]; !ok {
		t.Error("kaji_rule_new2 should have been added")
	}
	if _, ok := got["kaji_rule_upd2"]; !ok {
		t.Error("kaji_rule_upd2 should still exist after update")
	}
}

func TestSyncDomains_SkipsDisabledDomainAndRule(t *testing.T) {
	disabledDomainRoute := kajiRoute(t, "kaji_rule_dis_dom", "localhost:7001")
	disabledRuleRoute := kajiRoute(t, "kaji_rule_dis_rule", "localhost:7002")
	mock := newMockCaddyServer([]json.RawMessage{disabledDomainRoute, disabledRuleRoute})
	cc := newMockCaddyClient(t, mock)

	domains := []SyncDomain{
		{
			ID:      "rule_dis_dom",
			Name:    "dis_dom.example.com",
			Enabled: false,
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: mustMarshalStatic(ReverseProxyConfig{Upstream: "localhost:7001"}),
				Enabled:       true,
			},
		},
		{
			ID:      "dom_dis_rule",
			Name:    "dis_rule.example.com",
			Enabled: true,
			Paths: []SyncPath{
				{
					ID:         "rule_dis_rule",
					Enabled:    false,
					PathMatch:  "prefix",
					MatchValue: "/api",
					Rule: SyncRule{
						HandlerType:   "reverse_proxy",
						HandlerConfig: mustMarshalStatic(ReverseProxyConfig{Upstream: "localhost:7002"}),
						Enabled:       true,
					},
				},
			},
		},
	}

	result, err := SyncDomains(cc, domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0 (disabled routes must be protected)", result.Deleted)
	}

	got, _, err := ReadCurrentKajiDomains(cc)
	if err != nil {
		t.Fatalf("ReadCurrentKajiDomains: %v", err)
	}
	if _, ok := got["kaji_rule_dis_dom"]; !ok {
		t.Error("kaji_rule_dis_dom should not be deleted (disabled domain)")
	}
	if _, ok := got["kaji_rule_dis_rule"]; !ok {
		t.Error("kaji_rule_dis_rule should not be deleted (disabled path)")
	}
}

func TestSyncDomains_ErrorOnIPListResolutionFailure(t *testing.T) {
	mock := newMockCaddyServer(nil)
	cc := newMockCaddyClient(t, mock)

	domains := []SyncDomain{
		{
			ID:      "rule_secure",
			Name:    "secure.example.com",
			Enabled: true,
			Toggles: DomainToggles{
				IPFiltering: IPFilteringOpts{
					Enabled: true,
					ListID:  "bad_list",
					Type:    "whitelist",
				},
			},
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: mustMarshalStatic(ReverseProxyConfig{Upstream: "localhost:5000"}),
				Enabled:       true,
			},
		},
	}

	resolveIPs := func(listID string) ([]string, string, error) {
		return nil, "", fmt.Errorf("list %q not found", listID)
	}

	_, err := SyncDomains(cc, domains, resolveIPs)
	if err == nil {
		t.Fatal("expected error when IP list resolution fails")
	}
	if !strings.Contains(err.Error(), "building desired state") {
		t.Errorf("error = %q, want to mention 'building desired state'", err)
	}
}

func TestBuildDesiredState_DomainRuleDisabledOmitsDomainRouteOnly(t *testing.T) {
	domains := []SyncDomain{{
		ID:      "rule_dom_disabled",
		Name:    "example.com",
		Enabled: true,
		Rule: SyncRule{
			HandlerType:   "reverse_proxy",
			HandlerConfig: rpConfig(t, "localhost:3000"),
			Enabled:       false,
		},
		Paths: []SyncPath{{
			ID:         "rule_path_alive",
			Enabled:    true,
			PathMatch:  "prefix",
			MatchValue: "/api/",
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: rpConfig(t, "localhost:4000"),
				Enabled:       true,
			},
		}},
	}}

	desired, err := BuildDesiredState(domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := desired["kaji_rule_dom_disabled"]; ok {
		t.Errorf("domain route should be omitted when Rule.Enabled is false")
	}
	if _, ok := desired["kaji_rule_path_alive"]; !ok {
		t.Errorf("path route should still be emitted when only Rule.Enabled is false")
	}
}

func TestBuildDesiredState_SubdomainRuleDisabledOmitsSubdomainRouteOnly(t *testing.T) {
	domains := []SyncDomain{{
		ID:      "rule_dom_active",
		Name:    "example.com",
		Enabled: true,
		Rule:    SyncRule{HandlerType: "none", Enabled: true},
		Subdomains: []SyncSubdomain{{
			ID:      "rule_sub_disabled",
			Name:    "api",
			Enabled: true,
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: rpConfig(t, "localhost:3000"),
				Enabled:       false,
			},
			Paths: []SyncPath{{
				ID:         "rule_subpath_alive",
				Enabled:    true,
				PathMatch:  "prefix",
				MatchValue: "/v1/",
				Rule: SyncRule{
					HandlerType:   "reverse_proxy",
					HandlerConfig: rpConfig(t, "localhost:4000"),
					Enabled:       true,
				},
			}},
		}},
	}}

	desired, err := BuildDesiredState(domains, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := desired["kaji_rule_sub_disabled"]; ok {
		t.Errorf("subdomain route should be omitted when its Rule.Enabled is false")
	}
	if _, ok := desired["kaji_rule_subpath_alive"]; !ok {
		t.Errorf("subdomain path route should still be emitted")
	}
}

func TestCollectDisabledIDs_RuleDisabled(t *testing.T) {
	domains := []SyncDomain{{
		ID:      "rule_dom_active",
		Enabled: true,
		Rule: SyncRule{
			HandlerType:   "reverse_proxy",
			HandlerConfig: rpConfig(t, "localhost:3000"),
			Enabled:       false,
		},
		Subdomains: []SyncSubdomain{{
			ID:      "rule_sub_active",
			Name:    "api",
			Enabled: true,
			Rule: SyncRule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: rpConfig(t, "localhost:4000"),
				Enabled:       false,
			},
		}},
	}}

	ids := CollectDisabledIDs(domains)
	if !ids["kaji_rule_dom_active"] {
		t.Error("rule-disabled domain should be in disabled IDs (protected from deletion)")
	}
	if !ids["kaji_rule_sub_active"] {
		t.Error("rule-disabled subdomain should be in disabled IDs (protected from deletion)")
	}
}
