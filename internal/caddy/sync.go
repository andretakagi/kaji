package caddy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type SyncRule struct {
	HandlerType     string
	HandlerConfig   json.RawMessage
	AdvancedHeaders bool
	Enabled         bool
}

type SyncPath struct {
	ID              string
	Enabled         bool
	PathMatch       string
	MatchValue      string
	Rule            SyncRule
	ToggleOverrides *DomainToggles
}

type SyncSubdomain struct {
	ID      string
	Name    string
	Enabled bool
	Toggles DomainToggles
	Rule    SyncRule
	Paths   []SyncPath
}

type SyncDomain struct {
	ID         string
	Name       string
	Enabled    bool
	Toggles    DomainToggles
	Rule       SyncRule
	Subdomains []SyncSubdomain
	Paths      []SyncPath
}

type SyncResult struct {
	Added   int
	Updated int
	Deleted int
}

// BuildLogSkipRoute generates a non-terminal vars route that sets log_skip=true
// for requests matching the given skip rules and hostname. Returns nil, nil when
// there are no matchers to emit (empty conditions in basic mode, or no
// advanced_raw in advanced mode).
func BuildLogSkipRoute(domainID, hostname string, rule LogSkipRule) (json.RawMessage, error) {
	routeID := "kaji_logskip_" + domainID

	var matchSets []map[string]any

	if rule.Mode == "advanced" {
		if len(rule.AdvancedRaw) == 0 {
			return nil, nil
		}
		var sets []map[string]any
		if err := json.Unmarshal(rule.AdvancedRaw, &sets); err != nil {
			return nil, fmt.Errorf("parsing advanced_raw for %s: %w", domainID, err)
		}
		for _, s := range sets {
			m := make(map[string]any, len(s)+1)
			for k, v := range s {
				m[k] = v
			}
			m["host"] = []string{hostname}
			matchSets = append(matchSets, m)
		}
	} else {
		for _, c := range rule.Conditions {
			m := map[string]any{"host": []string{hostname}}
			switch c.Type {
			case "path":
				m["path"] = []string{c.Value}
			case "path_regexp":
				m["path_regexp"] = map[string]any{"pattern": c.Value}
			case "header":
				m["header"] = map[string]any{c.Key: []string{c.Value}}
			case "remote_ip":
				m["remote_ip"] = map[string]any{"ranges": []string{c.Value}}
			default:
				continue
			}
			matchSets = append(matchSets, m)
		}
	}

	if len(matchSets) == 0 {
		return nil, nil
	}

	route := map[string]any{
		"@id":      routeID,
		"match":    matchSets,
		"handle":   []any{map[string]any{"handler": "vars", "log_skip": true}},
		"terminal": false,
	}
	data, err := json.Marshal(route)
	if err != nil {
		return nil, fmt.Errorf("marshaling skip route for %s: %w", domainID, err)
	}
	return json.RawMessage(data), nil
}

// BuildDesiredState generates a map of CaddyDomainID -> route JSON for all
// enabled domains, subdomains, and paths. Disabled entries and entries with
// HandlerType "none" are skipped. Toggle inheritance is resolved via
// MergeToggles. IP lists are resolved through the provided callback.
// logSkipRules maps sink names to their skip rules; when a domain's AccessLog
// sink has rules, a kaji_logskip_<id> route is also emitted.
func BuildDesiredState(domains []SyncDomain, resolveIPs func(listID string) (ips []string, listType string, err error), logSkipRules map[string]LogSkipRule) (map[string]json.RawMessage, error) {
	desired := make(map[string]json.RawMessage)

	_, logSkipIDs := CollectAccessLogState(domains)

	for _, dom := range domains {
		if !dom.Enabled {
			continue
		}

		if dom.Rule.HandlerType != "" && dom.Rule.HandlerType != "none" && dom.Rule.Enabled {
			params := RuleBuildParams{
				RuleID:          dom.ID,
				HandlerType:     dom.Rule.HandlerType,
				HandlerConfig:   dom.Rule.HandlerConfig,
				AdvancedHeaders: dom.Rule.AdvancedHeaders,
			}

			ips, ipType, err := resolveToggleIPs(dom.Toggles, resolveIPs, "domain "+dom.ID)
			if err != nil {
				return nil, err
			}

			caddyID := CaddyDomainID(dom.ID)
			routeJSON, err := BuildRuleDomain(dom.Name, params, dom.Toggles, ips, ipType, logSkipIDs[caddyID])
			if err != nil {
				return nil, fmt.Errorf("building route for domain %s: %w", dom.ID, err)
			}
			desired[caddyID] = routeJSON
		}

		if dom.Toggles.AccessLog != "" {
			if rule, ok := logSkipRules[dom.Toggles.AccessLog]; ok {
				skipJSON, err := BuildLogSkipRoute(dom.ID, dom.Name, rule)
				if err != nil {
					return nil, fmt.Errorf("building skip route for domain %s: %w", dom.ID, err)
				}
				if skipJSON != nil {
					desired["kaji_logskip_"+dom.ID] = skipJSON
				}
			}
		}

		if err := emitPathRoutes(dom.Name, dom.Paths, dom.Toggles, resolveIPs, logSkipIDs, desired); err != nil {
			return nil, err
		}

		for _, sub := range dom.Subdomains {
			if !sub.Enabled {
				continue
			}
			subHost := sub.Name + "." + dom.Name

			if sub.Rule.HandlerType != "" && sub.Rule.HandlerType != "none" && sub.Rule.Enabled {
				params := RuleBuildParams{
					RuleID:          sub.ID,
					HandlerType:     sub.Rule.HandlerType,
					HandlerConfig:   sub.Rule.HandlerConfig,
					AdvancedHeaders: sub.Rule.AdvancedHeaders,
				}

				ips, ipType, err := resolveToggleIPs(sub.Toggles, resolveIPs, "subdomain "+sub.ID)
				if err != nil {
					return nil, err
				}

				caddyID := CaddyDomainID(sub.ID)
				routeJSON, err := BuildRuleDomain(subHost, params, sub.Toggles, ips, ipType, logSkipIDs[caddyID])
				if err != nil {
					return nil, fmt.Errorf("building route for subdomain %s: %w", sub.ID, err)
				}
				desired[caddyID] = routeJSON
			}

			if sub.Toggles.AccessLog != "" {
				if rule, ok := logSkipRules[sub.Toggles.AccessLog]; ok {
					skipJSON, err := BuildLogSkipRoute(sub.ID, subHost, rule)
					if err != nil {
						return nil, fmt.Errorf("building skip route for subdomain %s: %w", sub.ID, err)
					}
					if skipJSON != nil {
						desired["kaji_logskip_"+sub.ID] = skipJSON
					}
				}
			}

			if err := emitPathRoutes(subHost, sub.Paths, sub.Toggles, resolveIPs, logSkipIDs, desired); err != nil {
				return nil, err
			}
		}
	}

	return desired, nil
}

// emitPathRoutes builds and writes routes for every enabled path under the
// given host. Toggle overrides are merged against parentToggles, and disabled
// paths or paths with HandlerType "" or "none" are skipped.
func emitPathRoutes(
	host string,
	paths []SyncPath,
	parentToggles DomainToggles,
	resolveIPs func(string) ([]string, string, error),
	logSkipIDs map[string]bool,
	desired map[string]json.RawMessage,
) error {
	for _, p := range sortPaths(paths) {
		if !p.Enabled {
			continue
		}
		if p.Rule.HandlerType == "" || p.Rule.HandlerType == "none" {
			continue
		}

		toggles := MergeToggles(parentToggles, p.ToggleOverrides)
		params := RuleBuildParams{
			RuleID:          p.ID,
			MatchType:       "path",
			PathMatch:       p.PathMatch,
			MatchValue:      p.MatchValue,
			HandlerType:     p.Rule.HandlerType,
			HandlerConfig:   p.Rule.HandlerConfig,
			AdvancedHeaders: p.Rule.AdvancedHeaders,
		}

		ips, ipType, err := resolveToggleIPs(toggles, resolveIPs, "path "+p.ID)
		if err != nil {
			return err
		}

		caddyID := CaddyDomainID(p.ID)
		routeJSON, err := BuildRuleDomain(host, params, toggles, ips, ipType, logSkipIDs[caddyID])
		if err != nil {
			return fmt.Errorf("building route for path %s: %w", p.ID, err)
		}
		desired[caddyID] = routeJSON
	}
	return nil
}

// resolveToggleIPs runs the resolveIPs callback when the toggles call for IP
// filtering. Returns empty slices when filtering is off or no callback is set.
func resolveToggleIPs(toggles DomainToggles, resolveIPs func(string) ([]string, string, error), context string) ([]string, string, error) {
	if !toggles.IPFiltering.Enabled || toggles.IPFiltering.ListID == "" || resolveIPs == nil {
		return nil, "", nil
	}
	ips, typ, err := resolveIPs(toggles.IPFiltering.ListID)
	if err != nil {
		return nil, "", fmt.Errorf("resolving IP list for %s: %w", context, err)
	}
	return ips, typ, nil
}

// DiffDomains compares desired state against current Caddy state. Domains in
// current but not in desired are deleted unless they appear in disabledIDs
// (which protects disabled rules from being treated as orphans).
func DiffDomains(desired, current map[string]json.RawMessage, disabledIDs map[string]bool) (adds, updates map[string]json.RawMessage, deletes []string) {
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

// CollectDisabledIDs returns the set of CaddyDomainIDs for entries that are
// disabled (domain-level, rule-level, or by a disabled parent). These IDs are
// protected from deletion during sync. Entries with HandlerType "none" are
// skipped because they are intentionally absent from Caddy.
// Skip routes (kaji_logskip_*) follow the same disabled/enabled status as their
// parent domain or subdomain.
func CollectDisabledIDs(domains []SyncDomain) map[string]bool {
	ids := make(map[string]bool)
	for _, dom := range domains {
		if dom.Rule.HandlerType != "" && dom.Rule.HandlerType != "none" {
			if !dom.Enabled || !dom.Rule.Enabled {
				ids[CaddyDomainID(dom.ID)] = true
			}
		}
		if !dom.Enabled {
			ids["kaji_logskip_"+dom.ID] = true
		}
		for _, p := range dom.Paths {
			if p.Rule.HandlerType == "" || p.Rule.HandlerType == "none" {
				continue
			}
			if !dom.Enabled || !p.Enabled {
				ids[CaddyDomainID(p.ID)] = true
			}
		}
		for _, sub := range dom.Subdomains {
			if sub.Rule.HandlerType != "" && sub.Rule.HandlerType != "none" {
				if !dom.Enabled || !sub.Enabled || !sub.Rule.Enabled {
					ids[CaddyDomainID(sub.ID)] = true
				}
			}
			if !dom.Enabled || !sub.Enabled {
				ids["kaji_logskip_"+sub.ID] = true
			}
			for _, p := range sub.Paths {
				if p.Rule.HandlerType == "" || p.Rule.HandlerType == "none" {
					continue
				}
				if !dom.Enabled || !sub.Enabled || !p.Enabled {
					ids[CaddyDomainID(p.ID)] = true
				}
			}
		}
	}
	return ids
}

// CollectAccessLogState returns hostname-to-sink mappings for all enabled
// domains/subdomains, and the set of route IDs that should skip access logging
// because a path override clears the sink.
func CollectAccessLogState(domains []SyncDomain) (hostnameToSink map[string]string, logSkipIDs map[string]bool) {
	hostnameToSink = make(map[string]string)
	logSkipIDs = make(map[string]bool)

	for _, dom := range domains {
		if !dom.Enabled {
			continue
		}

		hostnameToSink[dom.Name] = dom.Toggles.AccessLog

		for _, p := range dom.Paths {
			if !p.Enabled || p.ToggleOverrides == nil {
				continue
			}
			merged := MergeToggles(dom.Toggles, p.ToggleOverrides)
			if merged.AccessLog == "" && dom.Toggles.AccessLog != "" {
				logSkipIDs[CaddyDomainID(p.ID)] = true
			}
		}

		for _, sub := range dom.Subdomains {
			if !sub.Enabled {
				continue
			}
			subHost := sub.Name + "." + dom.Name
			hostnameToSink[subHost] = sub.Toggles.AccessLog

			for _, p := range sub.Paths {
				if !p.Enabled || p.ToggleOverrides == nil {
					continue
				}
				merged := MergeToggles(sub.Toggles, p.ToggleOverrides)
				if merged.AccessLog == "" && sub.Toggles.AccessLog != "" {
					logSkipIDs[CaddyDomainID(p.ID)] = true
				}
			}
		}
	}

	return hostnameToSink, logSkipIDs
}

const kajiRoutePrefix = "kaji_"

// ReadCurrentKajiDomains reads all Kaji-managed routes from Caddy's live
// config. Returns a map of domain ID -> domain JSON and the server name
// where they were found.
func ReadCurrentKajiDomains(cc *Client) (map[string]json.RawMessage, string, error) {
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
			if strings.HasPrefix(r.ID, kajiRoutePrefix) {
				routes[r.ID] = route
				serverName = name
			}
		}
	}

	return routes, serverName, nil
}

// SyncDomains orchestrates a full sync: builds desired state from domains,
// reads current kaji domains from Caddy, diffs, and applies changes.
func SyncDomains(cc *Client, domains []SyncDomain, resolveIPs func(string) ([]string, string, error), logSkipRules map[string]LogSkipRule) (*SyncResult, error) {
	desired, err := BuildDesiredState(domains, resolveIPs, logSkipRules)
	if err != nil {
		return nil, fmt.Errorf("building desired state: %w", err)
	}

	current, serverName, err := ReadCurrentKajiDomains(cc)
	if err != nil {
		return nil, fmt.Errorf("reading current state: %w", err)
	}

	if serverName == "" {
		serverName = "srv0"
	}

	disabledIDs := CollectDisabledIDs(domains)
	adds, updates, deletes := DiffDomains(desired, current, disabledIDs)

	result := &SyncResult{}

	for _, id := range deletes {
		if err := cc.DeleteByID(id); err != nil {
			return result, fmt.Errorf("deleting domain %s: %w", id, err)
		}
		result.Deleted++
	}

	for id, route := range updates {
		if _, err := cc.ReplaceDomainByID(id, route); err != nil {
			return result, fmt.Errorf("updating domain %s: %w", id, err)
		}
		result.Updated++
	}

	for _, route := range orderedAdds(adds) {
		if err := cc.AddDomain(serverName, route); err != nil {
			return result, fmt.Errorf("adding domain: %w", err)
		}
		result.Added++
	}

	for _, dom := range domains {
		if !dom.Enabled {
			continue
		}
		if err := cc.SetDomainAccessLog(serverName, dom.Name, dom.Toggles.AccessLog); err != nil {
			return result, fmt.Errorf("setting access log for %s: %w", dom.Name, err)
		}
		for _, sub := range dom.Subdomains {
			if !sub.Enabled {
				continue
			}
			subHost := sub.Name + "." + dom.Name
			if err := cc.SetDomainAccessLog(serverName, subHost, sub.Toggles.AccessLog); err != nil {
				return result, fmt.Errorf("setting access log for %s: %w", subHost, err)
			}
		}
	}

	errorsConfig, err := BuildHandleErrorsRoutes(domains)
	if err != nil {
		return result, fmt.Errorf("building handle_errors config: %w", err)
	}
	if errorsConfig != nil {
		if err := cc.SetHandleErrors(serverName, errorsConfig); err != nil {
			return result, fmt.Errorf("setting handle_errors: %w", err)
		}
	} else {
		_ = cc.DeleteHandleErrors(serverName)
	}

	return result, nil
}

// sortPaths returns a copy of paths sorted by match specificity. Exact paths
// come first, then prefix, then regex.
func sortPaths(paths []SyncPath) []SyncPath {
	sorted := make([]SyncPath, len(paths))
	copy(sorted, paths)
	sort.SliceStable(sorted, func(i, j int) bool {
		return pathPriority(sorted[i]) > pathPriority(sorted[j])
	})
	return sorted
}

func pathPriority(p SyncPath) int {
	switch p.PathMatch {
	case "exact":
		return 3
	case "prefix":
		return 2
	case "regex":
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
