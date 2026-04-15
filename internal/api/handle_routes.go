// Route CRUD handlers.
package api

import (
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
		if !decodeBody(w, r, &req) {
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
		if !validateLoadBalancing(w, req.Toggles.LoadBalancing) {
			return
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

		if err := resolveIPFiltering(store, req.Toggles, &params); err != nil {
			writeError(w, fmt.Sprintf("IP list error: %v", err), http.StatusBadRequest)
			return
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

		resp := map[string]any{"status": "ok", "@id": caddy.GenerateRouteID(req.Domain)}
		if err := cc.SetRouteAccessLog(req.Server, req.Domain, req.Toggles.AccessLog); err != nil {
			log.Printf("handleCreateRoute: set access log: %v", err)
			resp["warning"] = "Route saved, but access logging could not be configured"
		}
		writeJSON(w, resp)
		persistCaddyConfig(cc, store)
		trackRouteIPList(store, routeID, req.Toggles)
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

		info := lookupRouteSink(cc, routeID)

		desc := "Route deleted: " + routeID
		if info.domain != "" {
			desc = "Route deleted: " + info.domain
		}
		maybeAutoSnapshot(cc, ss, desc)

		if err := cc.DeleteByID(routeID); err != nil {
			caddyError(w, "handleDeleteRoute", err)
			return
		}

		resp := map[string]any{"status": "ok"}
		if info.domain != "" && info.server != "" {
			if err := cc.SetRouteAccessLog(info.server, info.domain, ""); err != nil {
				log.Printf("handleDeleteRoute: clear access log: %v", err)
				resp["warning"] = "Route deleted, but its access log entry could not be cleaned up"
			}
			if info.sink != "" {
				if referenced, err := cc.IsSinkReferenced(info.sink); err == nil && !referenced {
					if err := cc.DeleteConfigPath(caddy.LogSinkPath(info.sink)); err != nil {
						log.Printf("handleDeleteRoute: remove unused log sink: %v", err)
					}
				}
			}
		}
		writeJSON(w, resp)
		persistCaddyConfig(cc, store)
		trackRouteIPList(store, routeID, caddy.RouteToggles{})
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
		if !decodeBody(w, r, &req) {
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

		if !validateLoadBalancing(w, req.Toggles.LoadBalancing) {
			return
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

		if err := resolveIPFiltering(store, req.Toggles, &params); err != nil {
			writeError(w, fmt.Sprintf("IP list error: %v", err), http.StatusBadRequest)
			return
		}

		route, err := caddy.BuildRoute(params)
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Capture old sink before replacing
		oldInfo := lookupRouteSink(cc, routeID)

		maybeAutoSnapshot(cc, ss, "Route updated: "+req.Domain)

		server, err := cc.ReplaceRouteByID(routeID, route)
		if err != nil {
			caddyError(w, "handleUpdateRoute", err)
			return
		}

		resp := map[string]any{"status": "ok"}
		if err := cc.SetRouteAccessLog(server, req.Domain, req.Toggles.AccessLog); err != nil {
			log.Printf("handleUpdateRoute: set access log: %v", err)
			resp["warning"] = "Route saved, but access logging could not be updated"
		}
		if oldInfo.sink != "" && oldInfo.sink != req.Toggles.AccessLog {
			if referenced, err := cc.IsSinkReferenced(oldInfo.sink); err == nil && !referenced {
				if err := cc.DeleteConfigPath(caddy.LogSinkPath(oldInfo.sink)); err != nil {
					log.Printf("handleUpdateRoute: remove unused log sink: %v", err)
				}
			}
		}
		writeJSON(w, resp)
		persistCaddyConfig(cc, store)
		trackRouteIPList(store, routeID, req.Toggles)
	}
}

func handleDisableRoute(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"@id"`
		}
		if !decodeBody(w, r, &req) {
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
		resp := map[string]any{"status": "ok"}
		if parsed, err := caddy.ParseRouteParams(route); err == nil && parsed.Domain != "" {
			if err := cc.SetRouteAccessLog(server, parsed.Domain, ""); err != nil {
				log.Printf("handleDisableRoute: clear access log: %v", err)
				resp["warning"] = "Route disabled, but its access log entry could not be cleaned up"
			}
		}

		writeJSON(w, resp)
		persistCaddyConfig(cc, store)
	}
}

func handleEnableRoute(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"@id"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.ID == "" {
			writeError(w, "@id is required", http.StatusBadRequest)
			return
		}

		cfg := store.Get()
		var disabled config.DisabledRoute
		found := false
		for _, dr := range cfg.DisabledRoutes {
			if dr.ID == req.ID {
				disabled = dr
				found = true
				break
			}
		}
		if !found {
			writeError(w, "route not found in disabled list", http.StatusNotFound)
			return
		}

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			fresh := make([]config.DisabledRoute, 0, len(c.DisabledRoutes))
			for _, dr := range c.DisabledRoutes {
				if dr.ID != req.ID {
					fresh = append(fresh, dr)
				}
			}
			c.DisabledRoutes = fresh
			return &c, nil
		}); err != nil {
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
		resp := map[string]any{"status": "ok"}
		if parsed, err := caddy.ParseRouteParams(disabled.Route); err == nil && parsed.Toggles.AccessLog != "" {
			if err := cc.SetRouteAccessLog(disabled.Server, parsed.Domain, parsed.Toggles.AccessLog); err != nil {
				log.Printf("handleEnableRoute: restore access log: %v", err)
				resp["warning"] = "Route enabled, but access logging could not be restored"
			}
		}

		writeJSON(w, resp)
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
