package api

import (
	"reflect"
	"sort"
	"testing"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
)

func ipFilterToggles(listID string) caddy.DomainToggles {
	return caddy.DomainToggles{
		IPFiltering: caddy.IPFilteringOpts{
			Enabled: true,
			ListID:  listID,
			Type:    "whitelist",
		},
	}
}

func TestDomainReferencesLists(t *testing.T) {
	tests := []struct {
		name    string
		domain  config.Domain
		listIDs map[string]bool
		want    bool
	}{
		{
			name: "domain_toggle_match",
			domain: config.Domain{
				Toggles: ipFilterToggles("list1"),
			},
			listIDs: map[string]bool{"list1": true},
			want:    true,
		},
		{
			name: "domain_toggle_disabled",
			domain: config.Domain{
				Toggles: caddy.DomainToggles{
					IPFiltering: caddy.IPFilteringOpts{Enabled: false, ListID: "list1"},
				},
			},
			listIDs: map[string]bool{"list1": true},
			want:    false,
		},
		{
			name: "domain_path_override",
			domain: config.Domain{
				Paths: []config.Path{{
					ToggleOverrides: &caddy.DomainToggles{
						IPFiltering: caddy.IPFilteringOpts{Enabled: true, ListID: "list2"},
					},
				}},
			},
			listIDs: map[string]bool{"list2": true},
			want:    true,
		},
		{
			name: "subdomain_toggle_match",
			domain: config.Domain{
				Subdomains: []config.Subdomain{{
					Toggles: ipFilterToggles("list3"),
				}},
			},
			listIDs: map[string]bool{"list3": true},
			want:    true,
		},
		{
			name: "subdomain_path_override",
			domain: config.Domain{
				Subdomains: []config.Subdomain{{
					Paths: []config.Path{{
						ToggleOverrides: &caddy.DomainToggles{
							IPFiltering: caddy.IPFilteringOpts{Enabled: true, ListID: "list4"},
						},
					}},
				}},
			},
			listIDs: map[string]bool{"list4": true},
			want:    true,
		},
		{
			name: "no_match",
			domain: config.Domain{
				Toggles: ipFilterToggles("other"),
				Paths: []config.Path{{
					ToggleOverrides: &caddy.DomainToggles{
						IPFiltering: caddy.IPFilteringOpts{Enabled: true, ListID: "another"},
					},
				}},
			},
			listIDs: map[string]bool{"target": true},
			want:    false,
		},
		{
			name: "path_override_disabled",
			domain: config.Domain{
				Paths: []config.Path{{
					ToggleOverrides: &caddy.DomainToggles{
						IPFiltering: caddy.IPFilteringOpts{Enabled: false, ListID: "list1"},
					},
				}},
			},
			listIDs: map[string]bool{"list1": true},
			want:    false,
		},
		{
			name: "path_no_overrides",
			domain: config.Domain{
				Paths: []config.Path{{ToggleOverrides: nil}},
			},
			listIDs: map[string]bool{"list1": true},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domainReferencesLists(tt.domain, tt.listIDs)
			if got != tt.want {
				t.Errorf("domainReferencesLists = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindDomainsUsingListDirect(t *testing.T) {
	cfg := &config.AppConfig{
		IPLists: []config.IPList{
			{ID: "list1", Name: "trusted", Type: "whitelist"},
		},
		Domains: []config.Domain{
			{ID: "d1", Name: "example.com", Toggles: ipFilterToggles("list1")},
			{ID: "d2", Name: "other.com", Toggles: ipFilterToggles("not-this-one")},
		},
	}

	got := findDomainsUsingList("list1", cfg)
	want := []affectedDomain{{ID: "d1", Name: "example.com"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFindDomainsUsingListAtAllLevels(t *testing.T) {
	cfg := &config.AppConfig{
		IPLists: []config.IPList{
			{ID: "list1", Name: "trusted", Type: "whitelist"},
		},
		Domains: []config.Domain{
			{
				ID:      "d1",
				Name:    "domain-level.com",
				Toggles: ipFilterToggles("list1"),
			},
			{
				ID:   "d2",
				Name: "domain-path.com",
				Paths: []config.Path{{
					ToggleOverrides: &caddy.DomainToggles{
						IPFiltering: caddy.IPFilteringOpts{Enabled: true, ListID: "list1"},
					},
				}},
			},
			{
				ID:   "d3",
				Name: "subdomain-level.com",
				Subdomains: []config.Subdomain{{
					Toggles: ipFilterToggles("list1"),
				}},
			},
			{
				ID:   "d4",
				Name: "subdomain-path.com",
				Subdomains: []config.Subdomain{{
					Paths: []config.Path{{
						ToggleOverrides: &caddy.DomainToggles{
							IPFiltering: caddy.IPFilteringOpts{Enabled: true, ListID: "list1"},
						},
					}},
				}},
			},
			{ID: "d5", Name: "unrelated.com"},
		},
	}

	got := findDomainsUsingList("list1", cfg)
	gotIDs := make([]string, len(got))
	for i, a := range got {
		gotIDs[i] = a.ID
	}
	sort.Strings(gotIDs)

	want := []string{"d1", "d2", "d3", "d4"}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Errorf("got %v, want %v", gotIDs, want)
	}
}

func TestFindDomainsUsingListDeduplicatesMultipleHits(t *testing.T) {
	cfg := &config.AppConfig{
		IPLists: []config.IPList{
			{ID: "list1", Name: "trusted", Type: "whitelist"},
		},
		Domains: []config.Domain{
			{
				ID:      "d1",
				Name:    "many-refs.com",
				Toggles: ipFilterToggles("list1"),
				Paths: []config.Path{{
					ToggleOverrides: &caddy.DomainToggles{
						IPFiltering: caddy.IPFilteringOpts{Enabled: true, ListID: "list1"},
					},
				}},
				Subdomains: []config.Subdomain{{
					Toggles: ipFilterToggles("list1"),
				}},
			},
		},
	}

	got := findDomainsUsingList("list1", cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 domain, got %d: %+v", len(got), got)
	}
	if got[0].ID != "d1" {
		t.Errorf("got id %s, want d1", got[0].ID)
	}
}

func TestFindDomainsUsingListThroughCompositeParent(t *testing.T) {
	cfg := &config.AppConfig{
		IPLists: []config.IPList{
			{ID: "child", Type: "whitelist"},
			{ID: "parent", Type: "whitelist", Children: []string{"child"}},
		},
		Domains: []config.Domain{
			{ID: "d1", Name: "uses-parent.com", Toggles: ipFilterToggles("parent")},
		},
	}

	// Querying for "child" should also report domains using "parent",
	// since changing child cascades to anything referencing parent.
	got := findDomainsUsingList("child", cfg)
	if len(got) != 1 || got[0].ID != "d1" {
		t.Errorf("got %+v, want d1 via composite parent", got)
	}
}

func TestFindDomainsUsingListEmpty(t *testing.T) {
	cfg := &config.AppConfig{
		IPLists: []config.IPList{{ID: "list1", Type: "whitelist"}},
		Domains: []config.Domain{
			{ID: "d1", Name: "no-refs.com"},
		},
	}
	got := findDomainsUsingList("list1", cfg)
	if len(got) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
}
