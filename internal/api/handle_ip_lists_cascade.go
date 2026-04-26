// Cascade logic for IP list changes: finds domains affected by a list change.
package api

import (
	"github.com/andretakagi/kaji/internal/config"
)

type affectedDomain struct {
	ID   string
	Name string
}

// findDomainsUsingList returns domains whose toggles or rule overrides reference
// the given list ID, either directly or through composite lists that include it.
func findDomainsUsingList(listID string, cfg *config.AppConfig) []affectedDomain {
	// Collect IDs of all lists that include listID (directly or transitively)
	affectedListIDs := map[string]bool{listID: true}
	changed := true
	for changed {
		changed = false
		for _, l := range cfg.IPLists {
			if affectedListIDs[l.ID] {
				continue
			}
			for _, childID := range l.Children {
				if affectedListIDs[childID] {
					affectedListIDs[l.ID] = true
					changed = true
					break
				}
			}
		}
	}

	seen := map[string]bool{}
	var result []affectedDomain

	for _, d := range cfg.Domains {
		if seen[d.ID] {
			continue
		}
		if domainReferencesLists(d, affectedListIDs) {
			seen[d.ID] = true
			result = append(result, affectedDomain{ID: d.ID, Name: d.Name})
		}
	}

	return result
}

func domainReferencesLists(d config.Domain, listIDs map[string]bool) bool {
	if d.Toggles.IPFiltering.Enabled && listIDs[d.Toggles.IPFiltering.ListID] {
		return true
	}
	for _, p := range d.Paths {
		if p.ToggleOverrides != nil && p.ToggleOverrides.IPFiltering.Enabled && listIDs[p.ToggleOverrides.IPFiltering.ListID] {
			return true
		}
	}
	for _, s := range d.Subdomains {
		if s.Toggles.IPFiltering.Enabled && listIDs[s.Toggles.IPFiltering.ListID] {
			return true
		}
		for _, p := range s.Paths {
			if p.ToggleOverrides != nil && p.ToggleOverrides.IPFiltering.Enabled && listIDs[p.ToggleOverrides.IPFiltering.ListID] {
				return true
			}
		}
	}
	return false
}
