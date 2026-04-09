// Route CRUD handlers.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func handleCreateRoute(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Server   string             `json:"server"`
			Domain   string             `json:"domain"`
			Upstream string             `json:"upstream"`
			Toggles  caddy.RouteToggles `json:"toggles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if msg := validateDomain(req.Domain); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if msg := validateUpstream(req.Upstream); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if req.Server == "" {
			req.Server = "srv0"
		}
		if msg := validateServerName(req.Server); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if req.Toggles.LoadBalancing.Enabled {
			if msg := validateLBStrategy(req.Toggles.LoadBalancing.Strategy); msg != "" {
				writeError(w, msg, http.StatusBadRequest)
				return
			}
			if len(req.Toggles.LoadBalancing.Upstreams) == 0 {
				writeError(w, "load balancing requires at least one additional upstream", http.StatusBadRequest)
				return
			}
			for _, u := range req.Toggles.LoadBalancing.Upstreams {
				if msg := validateUpstream(u); msg != "" {
					writeError(w, "additional upstream: "+msg, http.StatusBadRequest)
					return
				}
			}
		}

		if req.Toggles.BasicAuth.Enabled {
			if req.Toggles.BasicAuth.Username == "" {
				writeError(w, "username is required for basic auth", http.StatusBadRequest)
				return
			}
			if req.Toggles.BasicAuth.Password == "" && req.Toggles.BasicAuth.PasswordHash == "" {
				writeError(w, "password is required for basic auth", http.StatusBadRequest)
				return
			}
			if err := hashBasicAuthPassword(&req.Toggles.BasicAuth, ""); err != nil {
				log.Printf("handleCreateRoute: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
		}

		routeID := caddy.GenerateRouteID(req.Domain)
		if _, err := cc.GetRouteByID(routeID); err == nil {
			writeError(w, "a route for this domain already exists", http.StatusConflict)
			return
		}

		params := caddy.RouteParams{
			Domain:   req.Domain,
			Upstream: req.Upstream,
			Toggles:  req.Toggles,
		}
		route, err := caddy.BuildRoute(params)
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		maybeAutoSnapshot(cc, ss, "Route created: "+req.Domain)

		if err := cc.AddRoute(req.Server, route); err != nil {
			caddyError(w, "handleCreateRoute", err)
			return
		}

		if err := cc.SetRouteAccessLog(req.Server, req.Domain, req.Toggles.AccessLog); err != nil {
			log.Printf("handleCreateRoute: set access log: %v", err)
		}
		writeJSON(w, map[string]any{"status": "ok", "@id": caddy.GenerateRouteID(req.Domain)})
		persistCaddyConfig(cc, store)
	}
}

func handleDeleteRoute(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		routeID := r.PathValue("id")
		if routeID == "" {
			writeError(w, "route id is required", http.StatusBadRequest)
			return
		}

		// If the route is disabled, it lives in Kaji's config, not Caddy
		cfg := store.Get()
		for _, dr := range cfg.DisabledRoutes {
			if dr.ID == routeID {
				if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
					fresh := make([]config.DisabledRoute, 0, len(c.DisabledRoutes))
					for _, d := range c.DisabledRoutes {
						if d.ID != routeID {
							fresh = append(fresh, d)
						}
					}
					c.DisabledRoutes = fresh
					return &c, nil
				}); err != nil {
					log.Printf("handleDeleteRoute: remove disabled route: %v", err)
					writeError(w, "failed to remove disabled route", http.StatusInternalServerError)
					return
				}
				writeJSON(w, map[string]string{"status": "ok"})
				return
			}
		}

		var domain, server, oldSink string
		if raw, err := cc.GetRouteByID(routeID); err == nil {
			var route struct {
				Match []struct {
					Host []string `json:"host"`
				} `json:"match"`
			}
			if json.Unmarshal(raw, &route) == nil && len(route.Match) > 0 && len(route.Match[0].Host) > 0 {
				domain = route.Match[0].Host[0]
			}
			server, _ = cc.FindRouteServer(routeID)
			if domain != "" && server != "" {
				if domainSinks, err := cc.GetAccessLogDomains(server); err == nil {
					oldSink = domainSinks[domain]
				}
			}
		}

		desc := "Route deleted: " + routeID
		if domain != "" {
			desc = "Route deleted: " + domain
		}
		maybeAutoSnapshot(cc, ss, desc)

		if err := cc.DeleteByID(routeID); err != nil {
			caddyError(w, "handleDeleteRoute", err)
			return
		}

		if domain != "" && server != "" {
			_ = cc.SetRouteAccessLog(server, domain, "")
			if oldSink != "" {
				if referenced, err := cc.IsSinkReferenced(oldSink); err == nil && !referenced {
					_ = cc.DeleteConfigPath("logging/logs/" + oldSink)
				}
			}
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleUpdateRoute(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		routeID := r.PathValue("id")
		if routeID == "" {
			writeError(w, "route id is required", http.StatusBadRequest)
			return
		}

		var req struct {
			Domain   string             `json:"domain"`
			Upstream string             `json:"upstream"`
			Toggles  caddy.RouteToggles `json:"toggles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if msg := validateDomain(req.Domain); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if msg := validateUpstream(req.Upstream); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		if req.Toggles.LoadBalancing.Enabled {
			if msg := validateLBStrategy(req.Toggles.LoadBalancing.Strategy); msg != "" {
				writeError(w, msg, http.StatusBadRequest)
				return
			}
			if len(req.Toggles.LoadBalancing.Upstreams) == 0 {
				writeError(w, "load balancing requires at least one additional upstream", http.StatusBadRequest)
				return
			}
			for _, u := range req.Toggles.LoadBalancing.Upstreams {
				if msg := validateUpstream(u); msg != "" {
					writeError(w, "additional upstream: "+msg, http.StatusBadRequest)
					return
				}
			}
		}

		if req.Toggles.BasicAuth.Enabled {
			if req.Toggles.BasicAuth.Username == "" {
				writeError(w, "username is required for basic auth", http.StatusBadRequest)
				return
			}
			var fallbackHash string
			if req.Toggles.BasicAuth.Password == "" {
				if raw, err := cc.GetRouteByID(routeID); err == nil {
					existing, _ := caddy.ParseRouteParams(raw)
					fallbackHash = existing.Toggles.BasicAuth.PasswordHash
				}
			}
			if err := hashBasicAuthPassword(&req.Toggles.BasicAuth, fallbackHash); err != nil {
				log.Printf("handleUpdateRoute: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
		}

		params := caddy.RouteParams{
			ID:       routeID,
			Domain:   req.Domain,
			Upstream: req.Upstream,
			Toggles:  req.Toggles,
		}
		route, err := caddy.BuildRoute(params)
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Capture old sink before replacing
		var oldSink string
		if raw, err := cc.GetRouteByID(routeID); err == nil {
			oldParams, _ := caddy.ParseRouteParams(raw)
			oldDomain := oldParams.Domain
			oldServer, _ := cc.FindRouteServer(routeID)
			if oldDomain != "" && oldServer != "" {
				if domainSinks, err := cc.GetAccessLogDomains(oldServer); err == nil {
					oldSink = domainSinks[oldDomain]
				}
			}
		}

		maybeAutoSnapshot(cc, ss, "Route updated: "+req.Domain)

		server, err := cc.ReplaceRouteByID(routeID, route)
		if err != nil {
			caddyError(w, "handleUpdateRoute", err)
			return
		}

		if err := cc.SetRouteAccessLog(server, req.Domain, req.Toggles.AccessLog); err != nil {
			log.Printf("handleUpdateRoute: set access log: %v", err)
		}
		if oldSink != "" && oldSink != req.Toggles.AccessLog {
			if referenced, err := cc.IsSinkReferenced(oldSink); err == nil && !referenced {
				_ = cc.DeleteConfigPath("logging/logs/" + oldSink)
			}
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleDisableRoute(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"@id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.ID == "" {
			writeError(w, "@id is required", http.StatusBadRequest)
			return
		}

		route, err := cc.GetRouteByID(req.ID)
		if err != nil {
			caddyError(w, "handleDisableRoute", err)
			return
		}

		server, err := cc.FindRouteServer(req.ID)
		if err != nil {
			caddyError(w, "handleDisableRoute", err)
			return
		}

		entry := config.DisabledRoute{
			ID:         req.ID,
			Server:     server,
			DisabledAt: time.Now().UTC().Format(time.RFC3339),
			Route:      route,
		}
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			fresh := make([]config.DisabledRoute, len(c.DisabledRoutes), len(c.DisabledRoutes)+1)
			copy(fresh, c.DisabledRoutes)
			c.DisabledRoutes = append(fresh, entry)
			return &c, nil
		}); err != nil {
			log.Printf("handleDisableRoute: save config: %v", err)
			writeError(w, "failed to save disabled route to config", http.StatusInternalServerError)
			return
		}

		maybeAutoSnapshot(cc, ss, "Route disabled: "+req.ID)

		if err := cc.DeleteByID(req.ID); err != nil {
			// Roll back: remove the entry we just saved
			if rbErr := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
				fresh := make([]config.DisabledRoute, 0, len(c.DisabledRoutes))
				for _, dr := range c.DisabledRoutes {
					if dr.ID != req.ID {
						fresh = append(fresh, dr)
					}
				}
				c.DisabledRoutes = fresh
				return &c, nil
			}); rbErr != nil {
				log.Printf("handleDisableRoute: rollback failed: %v", rbErr)
			}
			caddyError(w, "handleDisableRoute", err)
			return
		}

		// Remove the domain from logger_names so the logs page
		// no longer lists it under the access log sink.
		if parsed, err := caddy.ParseRouteParams(route); err == nil && parsed.Domain != "" {
			_ = cc.SetRouteAccessLog(server, parsed.Domain, "")
		}

		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleEnableRoute(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"@id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.ID == "" {
			writeError(w, "@id is required", http.StatusBadRequest)
			return
		}

		var disabled config.DisabledRoute
		errNotFound := fmt.Errorf("route not found in disabled list")

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			fresh := make([]config.DisabledRoute, 0, len(c.DisabledRoutes))
			found := false
			for _, dr := range c.DisabledRoutes {
				if dr.ID == req.ID {
					disabled = dr
					found = true
				} else {
					fresh = append(fresh, dr)
				}
			}
			if !found {
				return nil, errNotFound
			}
			c.DisabledRoutes = fresh
			return &c, nil
		}); errors.Is(err, errNotFound) {
			writeError(w, "route not found in disabled list", http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("handleEnableRoute: save config: %v", err)
			writeError(w, "failed to remove route from disabled list", http.StatusInternalServerError)
			return
		}

		maybeAutoSnapshot(cc, ss, "Route enabled: "+req.ID)

		if err := cc.AddRoute(disabled.Server, disabled.Route); err != nil {
			// Roll back: re-add the entry to the disabled list
			if rbErr := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
				fresh := make([]config.DisabledRoute, len(c.DisabledRoutes), len(c.DisabledRoutes)+1)
				copy(fresh, c.DisabledRoutes)
				c.DisabledRoutes = append(fresh, disabled)
				return &c, nil
			}); rbErr != nil {
				log.Printf("handleEnableRoute: rollback failed: %v", rbErr)
			}
			caddyError(w, "handleEnableRoute", err)
			return
		}

		// Restore the domain's logger_names entry if the route
		// had an access log sink configured before it was disabled.
		if parsed, err := caddy.ParseRouteParams(disabled.Route); err == nil && parsed.Toggles.AccessLog != "" {
			_ = cc.SetRouteAccessLog(disabled.Server, parsed.Domain, parsed.Toggles.AccessLog)
		}

		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleDisabledRoutes(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		routes := cfg.DisabledRoutes
		if routes == nil {
			routes = []config.DisabledRoute{}
		}
		writeJSON(w, routes)
	}
}
