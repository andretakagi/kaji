// Cascade logic for IP list changes: finds affected routes and rebuilds them.
package api

import (
	"log"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
)

type affectedRoute struct {
	RouteID string
	Domain  string
}

// findDirectRoutesUsingList returns routes that directly reference the given list
// via the RouteIPLists config map.
func findDirectRoutesUsingList(listID string, store *config.ConfigStore, cc *caddy.Client) []affectedRoute {
	cfg := store.Get()
	var routes []affectedRoute

	for routeID, boundListID := range cfg.RouteIPLists {
		if boundListID != listID {
			continue
		}
		domain := ""
		if raw, err := cc.GetRouteByID(routeID); err == nil {
			if params, err := caddy.ParseRouteParams(raw); err == nil {
				domain = params.Domain
			}
		}
		routes = append(routes, affectedRoute{RouteID: routeID, Domain: domain})
	}

	return routes
}

// findRoutesUsingList returns all routes affected by a change to the given list,
// including routes that use composite lists containing this list as a child.
func findRoutesUsingList(listID string, allLists []config.IPList, store *config.ConfigStore, cc *caddy.Client) []affectedRoute {
	// Collect IDs of all lists that include listID (directly or transitively as a child)
	affectedListIDs := map[string]bool{listID: true}
	changed := true
	for changed {
		changed = false
		for _, l := range allLists {
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

	cfg := store.Get()
	seen := map[string]bool{}
	var routes []affectedRoute

	for routeID, boundListID := range cfg.RouteIPLists {
		if !affectedListIDs[boundListID] {
			continue
		}
		if seen[routeID] {
			continue
		}
		seen[routeID] = true

		domain := ""
		if raw, err := cc.GetRouteByID(routeID); err == nil {
			if params, err := caddy.ParseRouteParams(raw); err == nil {
				domain = params.Domain
			}
		}
		routes = append(routes, affectedRoute{RouteID: routeID, Domain: domain})
	}

	return routes
}

// cascadeIPListChange finds all routes affected by a change to the given IP list
// and rebuilds each one with the updated resolved IPs.
func cascadeIPListChange(listID string, store *config.ConfigStore, cc *caddy.Client) {
	cfg := store.Get()
	affected := findRoutesUsingList(listID, cfg.IPLists, store, cc)
	for _, ar := range affected {
		rebuildRoute(ar.RouteID, store, cc)
	}
}

// rebuildRoute reads a route from Caddy, re-resolves its IP list, rebuilds it,
// and replaces it in place.
func rebuildRoute(routeID string, store *config.ConfigStore, cc *caddy.Client) {
	raw, err := cc.GetRouteByID(routeID)
	if err != nil {
		log.Printf("rebuildRoute: failed to get route %s: %v", routeID, err)
		return
	}

	params, err := caddy.ParseRouteParams(raw)
	if err != nil {
		log.Printf("rebuildRoute: failed to parse route %s: %v", routeID, err)
		return
	}

	cfg := store.Get()
	listID := cfg.RouteIPLists[routeID]

	if listID != "" {
		resolved, err := caddy.ResolveIPList(listID, cfg.IPLists)
		if err != nil {
			log.Printf("rebuildRoute: failed to resolve IP list for route %s: %v", routeID, err)
			// List resolution failed (list was deleted, etc.) - rebuild without IP filtering
			params.IPListIPs = nil
			params.IPListType = ""
		} else {
			params.IPListIPs = resolved
			for _, l := range cfg.IPLists {
				if l.ID == listID {
					params.IPListType = l.Type
					break
				}
			}
		}
	} else {
		params.IPListIPs = nil
		params.IPListType = ""
	}

	route, err := caddy.BuildRoute(params)
	if err != nil {
		log.Printf("rebuildRoute: failed to build route %s: %v", routeID, err)
		return
	}

	if _, err := cc.ReplaceRouteByID(routeID, route); err != nil {
		log.Printf("rebuildRoute: failed to replace route %s: %v", routeID, err)
		return
	}

	// Re-set access log config since the route was replaced
	if params.Toggles.AccessLog != "" {
		server, err := cc.FindRouteServer(routeID)
		if err == nil {
			_ = cc.SetRouteAccessLog(server, params.Domain, params.Toggles.AccessLog)
		}
	}

}
