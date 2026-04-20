package caddy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type SyncDomain struct {
	Name       string
	Enabled    bool
	Toggles    DomainToggles
	Rules      []SyncRule
	Subdomains []SyncSubdomain
}

type SyncSubdomain struct {
	ID              string
	Name            string
	Enabled         bool
	HandlerType     string
	HandlerConfig   json.RawMessage
	Toggles         DomainToggles
	AdvancedHeaders bool
	Rules           []SyncRule
}

type SyncRule struct {
	RuleBuildParams
	Enabled         bool
	ToggleOverrides *DomainToggles
}

type SyncResult struct {
	Added   int
	Updated int
	Deleted int
}

// BuildDesiredState generates a map of CaddyRouteID -> route JSON for all
// enabled domains and enabled rules. Disabled domains and rules are skipped.
// Toggle inheritance is resolved via MergeToggles. IP lists are resolved
// through the provided callback.
func BuildDesiredState(domains []SyncDomain, resolveIPs func(listID string) (ips []string, listType string, err error)) (map[string]json.RawMessage, error) {
	desired := make(map[string]json.RawMessage)

	_, logSkipRuleIDs := CollectAccessLogState(domains)

	for _, dom := range domains {
		if !dom.Enabled {
			continue
		}

		sorted := sortRules(dom.Rules)

		for _, rule := range sorted {
			if !rule.Enabled {
				continue
			}

			toggles := MergeToggles(dom.Toggles, rule.ToggleOverrides)

			var ipListIPs []string
			var ipListType string
			if toggles.IPFiltering.Enabled && toggles.IPFiltering.ListID != "" && resolveIPs != nil {
				ips, typ, err := resolveIPs(toggles.IPFiltering.ListID)
				if err != nil {
					return nil, fmt.Errorf("resolving IP list for rule %s: %w", rule.RuleID, err)
				}
				ipListIPs = ips
				ipListType = typ
			}

			caddyID := CaddyRouteID(rule.RuleID)
			logSkip := logSkipRuleIDs[caddyID]

			routeJSON, err := BuildRuleRoute(dom.Name, rule.RuleBuildParams, toggles, ipListIPs, ipListType, logSkip)
			if err != nil {
				return nil, fmt.Errorf("building route for rule %s: %w", rule.RuleID, err)
			}

			desired[caddyID] = routeJSON
		}

		for _, sub := range dom.Subdomains {
			if !sub.Enabled {
				continue
			}

			subHost := sub.Name + "." + dom.Name
			subToggles := sub.Toggles

			// Build subdomain handler route (skip "none")
			if sub.HandlerType != "none" {
				params := RuleBuildParams{
					RuleID:          sub.ID,
					MatchType:       "",
					HandlerType:     sub.HandlerType,
					HandlerConfig:   sub.HandlerConfig,
					AdvancedHeaders: sub.AdvancedHeaders,
				}

				var ipListIPs []string
				var ipListType string
				if subToggles.IPFiltering.Enabled && subToggles.IPFiltering.ListID != "" && resolveIPs != nil {
					ips, typ, err := resolveIPs(subToggles.IPFiltering.ListID)
					if err != nil {
						return nil, fmt.Errorf("resolving IP list for subdomain %s: %w", sub.ID, err)
					}
					ipListIPs = ips
					ipListType = typ
				}

				caddyID := CaddyRouteID(sub.ID)
				logSkip := logSkipRuleIDs[caddyID]

				routeJSON, err := BuildRuleRoute(subHost, params, subToggles, ipListIPs, ipListType, logSkip)
				if err != nil {
					return nil, fmt.Errorf("building route for subdomain %s: %w", sub.ID, err)
				}

				desired[caddyID] = routeJSON
			}

			// Build subdomain path rules
			sortedSubRules := sortRules(sub.Rules)
			for _, rule := range sortedSubRules {
				if !rule.Enabled {
					continue
				}

				toggles := MergeToggles(subToggles, rule.ToggleOverrides)

				var ipListIPs []string
				var ipListType string
				if toggles.IPFiltering.Enabled && toggles.IPFiltering.ListID != "" && resolveIPs != nil {
					ips, typ, err := resolveIPs(toggles.IPFiltering.ListID)
					if err != nil {
						return nil, fmt.Errorf("resolving IP list for rule %s: %w", rule.RuleID, err)
					}
					ipListIPs = ips
					ipListType = typ
				}

				caddyID := CaddyRouteID(rule.RuleID)
				logSkip := logSkipRuleIDs[caddyID]

				routeJSON, err := BuildRuleRoute(subHost, rule.RuleBuildParams, toggles, ipListIPs, ipListType, logSkip)
				if err != nil {
					return nil, fmt.Errorf("building route for rule %s: %w", rule.RuleID, err)
				}

				desired[caddyID] = routeJSON
			}
		}
	}

	return desired, nil
}

// DiffRoutes compares desired state against current Caddy state. Routes in
// current but not in desired are deleted unless they appear in disabledIDs
// (which protects disabled rules from being treated as orphans).
func DiffRoutes(desired, current map[string]json.RawMessage, disabledIDs map[string]bool) (adds, updates map[string]json.RawMessage, deletes []string) {
	adds = make(map[string]json.RawMessage)
	updates = make(map[string]json.RawMessage)

	for id, desiredRoute := range desired {
		currentRoute, exists := current[id]
		if !exists {
			adds[id] = desiredRoute
			continue
		}
		if !jsonEqual(desiredRoute, currentRoute) {
			updates[id] = desiredRoute
		}
	}

	for id := range current {
		if _, inDesired := desired[id]; inDesired {
			continue
		}
		if disabledIDs[id] {
			continue
		}
		deletes = append(deletes, id)
	}

	sort.Strings(deletes)
	return adds, updates, deletes
}

// CollectDisabledIDs returns the set of CaddyRouteIDs for rules that are
// disabled or belong to disabled domains. These IDs are protected from
// deletion during sync.
func CollectDisabledIDs(domains []SyncDomain) map[string]bool {
	ids := make(map[string]bool)
	for _, dom := range domains {
		for _, rule := range dom.Rules {
			if !dom.Enabled || !rule.Enabled {
				ids[CaddyRouteID(rule.RuleID)] = true
			}
		}
		for _, sub := range dom.Subdomains {
			if !dom.Enabled || !sub.Enabled {
				if sub.HandlerType != "none" {
					ids[CaddyRouteID(sub.ID)] = true
				}
			}
			for _, rule := range sub.Rules {
				if !dom.Enabled || !sub.Enabled || !rule.Enabled {
					ids[CaddyRouteID(rule.RuleID)] = true
				}
			}
		}
	}
	return ids
}

// CollectAccessLogState returns hostname-to-sink mappings for all enabled
// domains/subdomains, and the set of rule IDs that should skip access logging.
func CollectAccessLogState(domains []SyncDomain) (hostnameToSink map[string]string, logSkipRuleIDs map[string]bool) {
	hostnameToSink = make(map[string]string)
	logSkipRuleIDs = make(map[string]bool)

	for _, dom := range domains {
		if !dom.Enabled {
			continue
		}

		hostnameToSink[dom.Name] = dom.Toggles.AccessLog

		for _, rule := range dom.Rules {
			if !rule.Enabled || rule.ToggleOverrides == nil {
				continue
			}
			merged := MergeToggles(dom.Toggles, rule.ToggleOverrides)
			if merged.AccessLog == "" && dom.Toggles.AccessLog != "" {
				logSkipRuleIDs[CaddyRouteID(rule.RuleID)] = true
			}
		}

		for _, sub := range dom.Subdomains {
			if !sub.Enabled {
				continue
			}
			subHost := sub.Name + "." + dom.Name
			hostnameToSink[subHost] = sub.Toggles.AccessLog

			for _, rule := range sub.Rules {
				if !rule.Enabled || rule.ToggleOverrides == nil {
					continue
				}
				merged := MergeToggles(sub.Toggles, rule.ToggleOverrides)
				if merged.AccessLog == "" && sub.Toggles.AccessLog != "" {
					logSkipRuleIDs[CaddyRouteID(rule.RuleID)] = true
				}
			}
		}
	}

	return hostnameToSink, logSkipRuleIDs
}

const kajiRulePrefix = "kaji_rule_"

// ReadCurrentKajiRoutes reads all routes with the kaji_rule_ prefix from
// Caddy's live config. Returns a map of route ID -> route JSON and the
// server name where they were found.
func ReadCurrentKajiRoutes(cc *Client) (map[string]json.RawMessage, string, error) {
	raw, err := cc.GetConfig()
	if err != nil {
		return nil, "", fmt.Errorf("reading caddy config: %w", err)
	}
	if raw == nil {
		return make(map[string]json.RawMessage), "", nil
	}

	var cfg caddyConfigPartial
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, "", fmt.Errorf("parsing caddy config: %w", err)
	}

	routes := make(map[string]json.RawMessage)
	var serverName string
	for name, srv := range cfg.Apps.HTTP.Servers {
		for _, route := range srv.Routes {
			var r struct {
				ID string `json:"@id"`
			}
			if json.Unmarshal(route, &r) != nil {
				continue
			}
			if strings.HasPrefix(r.ID, kajiRulePrefix) {
				routes[r.ID] = route
				serverName = name
			}
		}
	}

	return routes, serverName, nil
}

// SyncDomains orchestrates a full sync: builds desired state from domains,
// reads current kaji routes from Caddy, diffs, and applies changes.
func SyncDomains(cc *Client, domains []SyncDomain, resolveIPs func(string) ([]string, string, error)) (*SyncResult, error) {
	desired, err := BuildDesiredState(domains, resolveIPs)
	if err != nil {
		return nil, fmt.Errorf("building desired state: %w", err)
	}

	current, serverName, err := ReadCurrentKajiRoutes(cc)
	if err != nil {
		return nil, fmt.Errorf("reading current state: %w", err)
	}

	if serverName == "" {
		serverName = "srv0"
	}

	disabledIDs := CollectDisabledIDs(domains)
	adds, updates, deletes := DiffRoutes(desired, current, disabledIDs)

	result := &SyncResult{}

	for _, id := range deletes {
		if err := cc.DeleteByID(id); err != nil {
			return result, fmt.Errorf("deleting route %s: %w", id, err)
		}
		result.Deleted++
	}

	for id, route := range updates {
		if _, err := cc.ReplaceRouteByID(id, route); err != nil {
			return result, fmt.Errorf("updating route %s: %w", id, err)
		}
		result.Updated++
	}

	for _, route := range orderedAdds(adds) {
		if err := cc.AddRoute(serverName, route); err != nil {
			return result, fmt.Errorf("adding route: %w", err)
		}
		result.Added++
	}

	// Set or clear access log entries for each enabled domain and subdomain
	for _, dom := range domains {
		if !dom.Enabled {
			continue
		}
		if err := cc.SetRouteAccessLog(serverName, dom.Name, dom.Toggles.AccessLog); err != nil {
			return result, fmt.Errorf("setting access log for %s: %w", dom.Name, err)
		}
		for _, sub := range dom.Subdomains {
			if !sub.Enabled {
				continue
			}
			subHost := sub.Name + "." + dom.Name
			if err := cc.SetRouteAccessLog(serverName, subHost, sub.Toggles.AccessLog); err != nil {
				return result, fmt.Errorf("setting access log for %s: %w", subHost, err)
			}
		}
	}

	return result, nil
}

// sortRules returns a copy of rules sorted by match specificity. Exact paths
// come first, then prefix, then regex, then root (no match type).
func sortRules(rules []SyncRule) []SyncRule {
	sorted := make([]SyncRule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return rulePriority(sorted[i]) > rulePriority(sorted[j])
	})
	return sorted
}

func rulePriority(r SyncRule) int {
	if r.MatchType == "path" && r.PathMatch == "exact" {
		return 3
	}
	if r.MatchType == "path" && r.PathMatch == "prefix" {
		return 2
	}
	if r.MatchType == "path" && r.PathMatch == "regex" {
		return 1
	}
	return 0
}

func jsonEqual(a, b json.RawMessage) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil {
		return false
	}
	if json.Unmarshal(b, &bv) != nil {
		return false
	}
	// Re-marshal to normalized form for comparison
	aN, err := json.Marshal(av)
	if err != nil {
		return false
	}
	bN, err := json.Marshal(bv)
	if err != nil {
		return false
	}
	return string(aN) == string(bN)
}

// orderedAdds returns routes in deterministic order, sorted by the @id
// extracted from each route.
func orderedAdds(adds map[string]json.RawMessage) []json.RawMessage {
	keys := make([]string, 0, len(adds))
	for k := range adds {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	routes := make([]json.RawMessage, 0, len(keys))
	for _, k := range keys {
		routes = append(routes, adds[k])
	}
	return routes
}
