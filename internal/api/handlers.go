// All /api/* HTTP handlers: setup, auth, routes, settings, logs.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andretakagi/kaji/internal/auth"
	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/logging"
	"github.com/andretakagi/kaji/internal/system"
)

func RegisterRoutes(mux *http.ServeMux, store *config.ConfigStore, mgr system.CaddyManager, cc *caddy.Client, version string) http.Handler {
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"version": version})
	})

	mux.HandleFunc("GET /api/setup/status", handleSetupStatus())
	mux.HandleFunc("POST /api/setup", handleSetup(store, cc))
	mux.HandleFunc("POST /api/setup/adapt-caddyfile", handleAdaptCaddyfile(cc))
	mux.HandleFunc("GET /api/setup/default-caddyfile", handleDefaultCaddyfile())

	mux.HandleFunc("GET /api/auth/status", handleAuthStatus(store))
	mux.HandleFunc("POST /api/auth/login", handleLogin(store))
	mux.HandleFunc("POST /api/auth/logout", handleLogout(store))
	mux.HandleFunc("PUT /api/auth/password", handlePasswordChange(store))

	mux.HandleFunc("GET /api/caddy/status", handleStatus(mgr))
	mux.HandleFunc("POST /api/caddy/start", handleStart(mgr, cc, store))
	mux.HandleFunc("POST /api/caddy/stop", handleStop(mgr))
	mux.HandleFunc("POST /api/caddy/restart", handleRestart(mgr, cc, store))
	mux.HandleFunc("GET /api/caddy/config", handleConfigProxy(cc))
	mux.HandleFunc("GET /api/caddy/config/{path...}", handleConfigProxy(cc))
	mux.HandleFunc("POST /api/caddy/load", handleConfigLoad(store, cc))
	mux.HandleFunc("GET /api/caddy/upstreams", handleUpstreams(cc))
	mux.HandleFunc("POST /api/routes", handleCreateRoute(store, cc))
	mux.HandleFunc("DELETE /api/routes/{id}", handleDeleteRoute(store, cc))
	mux.HandleFunc("PUT /api/routes/{id}", handleUpdateRoute(store, cc))
	mux.HandleFunc("POST /api/routes/disable", handleDisableRoute(store, cc))
	mux.HandleFunc("POST /api/routes/enable", handleEnableRoute(store, cc))
	mux.HandleFunc("GET /api/routes/disabled", handleDisabledRoutes(store))
	mux.HandleFunc("GET /api/logs", handleLogs(store))
	mux.HandleFunc("GET /api/logs/stream", handleLogStream(store))
	mux.HandleFunc("GET /api/logs/config", handleLogConfigGet(cc))
	mux.HandleFunc("PUT /api/logs/config", handleLogConfigUpdate(store, cc))
	mux.HandleFunc("GET /api/logs/access-domains", handleAccessDomains(cc))
	mux.HandleFunc("GET /api/caddyfile", handleCaddyfileExport(cc, store))
	mux.HandleFunc("GET /api/settings/global-toggles", handleGlobalTogglesGet(cc))
	mux.HandleFunc("PUT /api/settings/global-toggles", handleGlobalTogglesUpdate(store, cc))
	mux.HandleFunc("GET /api/settings/acme-email", handleACMEEmailGet(cc))
	mux.HandleFunc("PUT /api/settings/acme-email", handleACMEEmailUpdate(store, cc))
	mux.HandleFunc("PUT /api/settings/auth", handleAuthToggle(store))
	mux.HandleFunc("GET /api/settings/api-key", handleAPIKeyStatus(store))
	mux.HandleFunc("POST /api/settings/api-key", handleAPIKeyGenerate(store))
	mux.HandleFunc("DELETE /api/settings/api-key", handleAPIKeyRevoke(store))
	mux.HandleFunc("GET /api/settings/advanced", handleAdvancedGet(store))
	mux.HandleFunc("PUT /api/settings/advanced", handleAdvancedUpdate(store))

	return accessLog(limitRequestBody(requireAuth(store, mux)))
}

func handleSetupStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"is_first_run": !config.Exists()})
	}
}

func handleSetup(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password      string               `json:"password"`
			CaddyAdminURL string               `json:"caddy_admin_url"`
			ACMEEmail     string               `json:"acme_email"`
			GlobalToggles *caddy.GlobalToggles `json:"global_toggles"`
			CaddyfileJSON json.RawMessage      `json:"caddyfile_json"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		sessionSecret, err := auth.GenerateSessionToken()
		if err != nil {
			log.Printf("handleSetup: generate session secret: %v", err)
			writeError(w, "failed to generate session secret", http.StatusInternalServerError)
			return
		}

		newCfg := config.DefaultConfig()
		newCfg.SessionSecret = sessionSecret

		if req.Password != "" {
			hash, err := auth.HashPassword(req.Password)
			if err != nil {
				log.Printf("handleSetup: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
			newCfg.AuthEnabled = true
			newCfg.PasswordHash = hash
		}

		if req.CaddyAdminURL != "" {
			if msg := validateCaddyAdminURL(req.CaddyAdminURL); msg != "" {
				writeError(w, msg, http.StatusBadRequest)
				return
			}
			newCfg.CaddyAdminURL = req.CaddyAdminURL
		}
		if err := store.Update(func(_ config.AppConfig) (*config.AppConfig, error) {
			if config.Exists() {
				return nil, config.ErrSetupDone
			}
			return newCfg, nil
		}); err != nil {
			if errors.Is(err, config.ErrSetupDone) {
				writeError(w, "setup already completed", http.StatusConflict)
				return
			}
			log.Printf("handleSetup: save config: %v", err)
			writeError(w, "failed to save config", http.StatusInternalServerError)
			return
		}

		var warnings []string

		if len(req.CaddyfileJSON) > 0 {
			if err := cc.LoadConfig(req.CaddyfileJSON); err != nil {
				log.Printf("handleSetup: load caddyfile config: %v", err)
				warnings = append(warnings, "failed to load Caddyfile config: "+err.Error())
			}
		}

		if req.ACMEEmail != "" {
			if err := cc.SetACMEEmail(req.ACMEEmail); err != nil {
				log.Printf("handleSetup: set ACME email: %v", err)
				warnings = append(warnings, "failed to set ACME email: "+err.Error())
			}
		}
		if req.GlobalToggles != nil {
			if err := cc.SetGlobalToggles(req.GlobalToggles); err != nil {
				log.Printf("handleSetup: set global toggles: %v", err)
				warnings = append(warnings, "failed to set global toggles: "+err.Error())
			}
		}

		persistCaddyConfig(cc, store)

		if newCfg.AuthEnabled {
			token, err := auth.GenerateSessionToken()
			if err != nil {
				log.Printf("handleSetup: generate session token: %v", err)
				writeJSON(w, map[string]any{"status": "ok", "warning": "setup succeeded but session creation failed, please log in manually"})
				return
			}
			cfg := store.Get()
			auth.SetSessionCookie(w, r, auth.SignToken(token, cfg.SessionSecret), sessionMaxAge(cfg), cfg.SecureCookies)
		}

		resp := map[string]any{"status": "ok"}
		if len(warnings) > 0 {
			resp["warnings"] = warnings
		}
		writeJSON(w, resp)
	}
}

func handleAdaptCaddyfile(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Caddyfile string `json:"caddyfile"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Caddyfile) == "" {
			writeError(w, "caddyfile content is required", http.StatusBadRequest)
			return
		}

		adaptedJSON, err := cc.AdaptCaddyfile(req.Caddyfile)
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "unreachable") {
				writeError(w, "Caddy must be running to parse a Caddyfile", http.StatusBadGateway)
				return
			}
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		settings, err := caddy.ExtractCaddyfileSettings(adaptedJSON)
		if err != nil {
			log.Printf("handleAdaptCaddyfile: extract settings: %v", err)
			writeError(w, "failed to extract settings from adapted config", http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"acme_email":     settings.ACMEEmail,
			"global_toggles": settings.Toggles,
			"route_count":    settings.RouteCount,
			"adapted_config": adaptedJSON,
		})
	}
}

func handleDefaultCaddyfile() http.HandlerFunc {
	defaultPath := "/etc/caddy/Caddyfile"
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(defaultPath)
		if err != nil {
			writeJSON(w, map[string]string{
				"content": "",
				"path":    defaultPath,
			})
			return
		}
		writeJSON(w, map[string]string{
			"content": string(data),
			"path":    defaultPath,
		})
	}
}

func handleAuthStatus(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		authenticated := !cfg.AuthEnabled
		if cfg.AuthEnabled {
			token := auth.GetSessionToken(r)
			authenticated = token != "" && auth.ValidateSignedToken(token, cfg.SessionSecret, cfg.SessionMaxAge)
		}
		writeJSON(w, map[string]any{
			"auth_enabled":  cfg.AuthEnabled,
			"authenticated": authenticated,
			"has_password":  cfg.PasswordHash != "",
		})
	}
}

func handleLogin(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		if !cfg.AuthEnabled {
			writeError(w, "authentication is disabled", http.StatusConflict)
			return
		}

		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if !auth.CheckPassword(cfg.PasswordHash, req.Password) {
			writeError(w, "invalid password", http.StatusUnauthorized)
			return
		}

		token, err := auth.GenerateSessionToken()
		if err != nil {
			log.Printf("handleLogin: generate session token: %v", err)
			writeError(w, "failed to create session", http.StatusInternalServerError)
			return
		}
		auth.SetSessionCookie(w, r, auth.SignToken(token, cfg.SessionSecret), sessionMaxAge(cfg), cfg.SecureCookies)
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleLogout(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		auth.ClearSessionCookie(w, r, cfg.SecureCookies)
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handlePasswordChange(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !store.Get().AuthEnabled {
			writeError(w, "authentication is disabled", http.StatusConflict)
			return
		}

		var req struct {
			NewPassword string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.NewPassword == "" {
			writeError(w, "new password is required", http.StatusBadRequest)
			return
		}

		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			log.Printf("handlePasswordChange: hash password: %v", err)
			writeError(w, "failed to hash password", http.StatusInternalServerError)
			return
		}

		newSecret, err := auth.GenerateSessionToken()
		if err != nil {
			log.Printf("handlePasswordChange: generate session secret: %v", err)
			writeError(w, "failed to generate session secret", http.StatusInternalServerError)
			return
		}

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.PasswordHash = hash
			c.SessionSecret = newSecret
			return &c, nil
		}); err != nil {
			log.Printf("handlePasswordChange: save config: %v", err)
			writeError(w, "failed to save config", http.StatusInternalServerError)
			return
		}

		token, err := auth.GenerateSessionToken()
		if err != nil {
			log.Printf("handlePasswordChange: generate session token: %v", err)
			writeJSON(w, map[string]string{"status": "ok", "warning": "password changed but session creation failed, please log in manually"})
			return
		}
		cfg := store.Get()
		auth.SetSessionCookie(w, r, auth.SignToken(token, cfg.SessionSecret), sessionMaxAge(cfg), cfg.SecureCookies)

		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleStatus(mgr system.CaddyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		running, err := mgr.Status()
		if err != nil {
			log.Printf("handleStatus: %v", err)
			writeError(w, "failed to check caddy status", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]bool{"running": running})
	}
}

func handleStart(mgr system.CaddyManager, cc *caddy.Client, store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Start(); err != nil {
			log.Printf("handleStart: %v", err)
			writeError(w, "failed to start caddy", http.StatusInternalServerError)
			return
		}
		loadSavedCaddyConfig(cc, store)
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleStop(mgr system.CaddyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Stop(); err != nil {
			log.Printf("handleStop: %v", err)
			writeError(w, "failed to stop caddy", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleRestart(mgr system.CaddyManager, cc *caddy.Client, store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Restart(); err != nil {
			log.Printf("handleRestart: %v", err)
			writeError(w, "failed to restart caddy", http.StatusInternalServerError)
			return
		}
		loadSavedCaddyConfig(cc, store)
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleConfigProxy(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := url.PathUnescape(r.PathValue("path"))
		if err != nil {
			writeError(w, "invalid config path", http.StatusBadRequest)
			return
		}
		cleaned := path.Clean("/" + p)
		if cleaned != "/"+p {
			writeError(w, "invalid config path", http.StatusBadRequest)
			return
		}

		cfg, err := cc.GetConfigPath(p)
		if err != nil {
			caddyError(w, "handleConfigProxy", err)
			return
		}
		if len(cfg) == 0 || !json.Valid(cfg) {
			log.Printf("handleConfigProxy: caddy returned invalid JSON for path %q (len=%d)", p, len(cfg))
			writeError(w, "caddy returned an invalid config response", http.StatusBadGateway)
			return
		}
		writeRawJSON(w, cfg)
	}
}

func handleConfigLoad(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		if !json.Valid(body) {
			writeError(w, "request body is not valid JSON", http.StatusBadRequest)
			return
		}
		if err := cc.LoadConfig(body); err != nil {
			caddyError(w, "handleConfigLoad", err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func loadSavedCaddyConfig(cc *caddy.Client, store *config.ConfigStore) {
	cfg := store.Get()
	saved, err := os.ReadFile(cfg.CaddyConfigPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("loadSavedCaddyConfig: read config: %v", err)
		}
		return
	}
	if err := cc.LoadConfig(saved); err != nil {
		log.Printf("loadSavedCaddyConfig: %v", err)
	}
}

func handleUpstreams(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upstreams, err := cc.GetUpstreams()
		if err != nil {
			// On a fresh Caddy with no reverse proxies, this endpoint
			// may not exist. Return an empty array instead of an error.
			upstreams = []byte(`[]`)
		}
		writeRawJSON(w, upstreams)
	}
}

func handleCaddyfileExport(cc *caddy.Client, store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := cc.GetConfig()
		if err != nil {
			caddyError(w, "handleCaddyfileExport", err)
			return
		}
		cfg := store.Get()
		content, err := caddy.GenerateCaddyfile(raw, cfg.LogFile)
		if err != nil {
			log.Printf("handleCaddyfileExport: %v", err)
			writeError(w, "failed to generate Caddyfile", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"content": content})
	}
}

func handleCreateRoute(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
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

func handleDeleteRoute(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		routeID := r.PathValue("id")
		if routeID == "" {
			writeError(w, "route id is required", http.StatusBadRequest)
			return
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

func handleUpdateRoute(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
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

func handleDisableRoute(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
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

		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleEnableRoute(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
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

func handleLogs(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		q := r.URL.Query()
		params := logging.QueryParams{
			Level: q.Get("level"),
			Host:  q.Get("host"),
		}

		var ok bool
		if params.Limit, ok = parseIntParam(w, q, "limit", 0, 1000); !ok {
			return
		}
		if params.Offset, ok = parseIntParam(w, q, "offset", 0, -1); !ok {
			return
		}
		if params.StatusMin, ok = parseIntParam(w, q, "status_min", 0, -1); !ok {
			return
		}
		if params.StatusMax, ok = parseIntParam(w, q, "status_max", 0, -1); !ok {
			return
		}
		if v := q.Get("since"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeError(w, "invalid since parameter, expected RFC3339 format", http.StatusBadRequest)
				return
			}
			params.Since = t
		}
		if v := q.Get("until"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeError(w, "invalid until parameter, expected RFC3339 format", http.StatusBadRequest)
				return
			}
			params.Until = t
		}

		result, err := logging.QueryLogs(cfg.LogFile, params)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				writeJSON(w, logging.QueryResult{Entries: []logging.LogEntry{}})
				return
			}
			log.Printf("handleLogs: %v", err)
			writeError(w, "failed to query logs", http.StatusInternalServerError)
			return
		}

		writeJSON(w, result)
	}
}

func handleLogStream(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		logFile := store.Get().LogFile
		if logFile == "" {
			writeError(w, "log file not configured", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher.Flush()

		ctx := r.Context()
		lines := make(chan string, 64)

		// If the log file doesn't exist yet (fresh install, no traffic),
		// wait for it to appear rather than returning an error.
		for {
			if _, err := os.Stat(logFile); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- logging.TailFile(ctx, logFile, lines)
		}()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
			case line := <-lines:
				line = strings.ReplaceAll(line, "\n", "")
				line = strings.ReplaceAll(line, "\r", "")
				fmt.Fprintf(w, "data: %s\n\n", line)
				flusher.Flush()
			case err := <-errCh:
				if err != nil && err != context.Canceled {
					log.Printf("handleLogStream: %v", err)
				}
				return
			}
		}
	}
}

func handleLogConfigGet(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := cc.GetLoggingConfig()
		if err != nil {
			// On a fresh Caddy instance, the logging path doesn't exist yet.
			// Return an empty config instead of an error.
			raw = []byte(`{"logs":{}}`)
		}
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || string(trimmed) == "null" {
			raw = []byte(`{"logs":{}}`)
		}
		writeRawJSON(w, raw)
	}
}

func handleLogConfigUpdate(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	dockerMode := os.Getenv("CADDY_GUI_MODE") == "docker"
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		if !json.Valid(body) {
			writeError(w, "request body is not valid JSON", http.StatusBadRequest)
			return
		}
		if dockerMode {
			if msg := validateLogFilePaths(body); msg != "" {
				writeError(w, msg, http.StatusBadRequest)
				return
			}
		}
		// If any existing sink is being removed, cascade: clear domain mappings.
		var incoming struct {
			Logs map[string]json.RawMessage `json:"logs"`
		}
		if existing, _ := cc.GetLoggingConfig(); existing != nil {
			var current struct {
				Logs map[string]json.RawMessage `json:"logs"`
			}
			if json.Unmarshal(existing, &current) == nil {
				json.Unmarshal(body, &incoming)
				for name := range current.Logs {
					if _, exists := incoming.Logs[name]; !exists {
						_ = cc.ClearDomainsForSink(name)
					}
				}
			}
		}

		if err := cc.SetLoggingConfig(body); err != nil {
			caddyError(w, "handleLogConfigUpdate", err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleAccessDomains(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domains, err := cc.GetAllAccessLogDomains()
		if err != nil {
			writeJSON(w, map[string]map[string]string{})
			return
		}
		writeJSON(w, domains)
	}
}

const dockerLogDir = "/var/log/caddy/"

// validateLogFilePaths checks that any file-output sinks write to the
// writable log volume. In Docker the container runs read-only, so only
// /var/log/caddy/ is available.
func validateLogFilePaths(body []byte) string {
	var cfg struct {
		Logs map[string]struct {
			Writer struct {
				Output   string `json:"output"`
				Filename string `json:"filename"`
			} `json:"writer"`
		} `json:"logs"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return fmt.Sprintf("invalid logging config: %v", err)
	}
	for name, sink := range cfg.Logs {
		if sink.Writer.Output != "file" {
			continue
		}
		cleaned := path.Clean(sink.Writer.Filename)
		if !strings.HasPrefix(cleaned, dockerLogDir) {
			return fmt.Sprintf("log '%s': file path must be under %s (the container filesystem is read-only)", name, dockerLogDir)
		}
	}
	return ""
}

func handleGlobalTogglesGet(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := cc.GetGlobalToggles()
		if err != nil {
			caddyError(w, "handleGlobalTogglesGet", err)
			return
		}
		writeJSON(w, t)
	}
}

func handleGlobalTogglesUpdate(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var toggles caddy.GlobalToggles
		if err := json.NewDecoder(r.Body).Decode(&toggles); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if msg := validateAutoHTTPS(toggles.AutoHTTPS); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if err := cc.SetGlobalToggles(&toggles); err != nil {
			caddyError(w, "handleGlobalTogglesUpdate", err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleACMEEmailGet(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email, err := cc.GetACMEEmail()
		if err != nil {
			caddyError(w, "handleACMEEmailGet", err)
			return
		}
		writeJSON(w, map[string]string{"email": email})
	}
}

func handleACMEEmailUpdate(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if msg := validateEmail(req.Email); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if err := cc.SetACMEEmail(req.Email); err != nil {
			caddyError(w, "handleACMEEmailUpdate", err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleAuthToggle(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AuthEnabled bool   `json:"auth_enabled"`
			Password    string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.AuthEnabled {
			cfg := store.Get()

			newSecret, err := auth.GenerateSessionToken()
			if err != nil {
				log.Printf("handleAuthToggle: generate session secret: %v", err)
				writeError(w, "failed to generate session secret", http.StatusInternalServerError)
				return
			}

			if cfg.PasswordHash == "" {
				if req.Password == "" {
					writeError(w, "password is required to enable auth", http.StatusBadRequest)
					return
				}
				hash, err := auth.HashPassword(req.Password)
				if err != nil {
					log.Printf("handleAuthToggle: hash password: %v", err)
					writeError(w, "failed to hash password", http.StatusInternalServerError)
					return
				}
				if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
					c.AuthEnabled = true
					c.PasswordHash = hash
					c.SessionSecret = newSecret
					return &c, nil
				}); err != nil {
					log.Printf("handleAuthToggle: save config: %v", err)
					writeError(w, "failed to save config", http.StatusInternalServerError)
					return
				}
			} else {
				if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
					c.AuthEnabled = true
					c.SessionSecret = newSecret
					return &c, nil
				}); err != nil {
					log.Printf("handleAuthToggle: save config: %v", err)
					writeError(w, "failed to save config", http.StatusInternalServerError)
					return
				}
			}

			cfg = store.Get()
			token, err := auth.GenerateSessionToken()
			if err != nil {
				log.Printf("handleAuthToggle: generate session token: %v", err)
				writeJSON(w, map[string]string{"status": "ok", "warning": "auth enabled but session creation failed, please log in manually"})
				return
			}
			auth.SetSessionCookie(w, r, auth.SignToken(token, cfg.SessionSecret), sessionMaxAge(cfg), cfg.SecureCookies)
		} else {
			if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
				c.AuthEnabled = false
				c.PasswordHash = ""
				return &c, nil
			}); err != nil {
				log.Printf("handleAuthToggle: save config: %v", err)
				writeError(w, "failed to save config", http.StatusInternalServerError)
				return
			}
		}

		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleAPIKeyStatus(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"has_api_key": store.Get().APIKeyHash != ""})
	}
}

func handleAPIKeyGenerate(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := auth.GenerateAPIKey()
		if err != nil {
			log.Printf("handleAPIKeyGenerate: %v", err)
			writeError(w, "failed to generate API key", http.StatusInternalServerError)
			return
		}
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.APIKeyHash = auth.HashAPIKey(key)
			return &c, nil
		}); err != nil {
			log.Printf("handleAPIKeyGenerate: save config: %v", err)
			writeError(w, "failed to save config", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"api_key": key})
	}
}

func handleAPIKeyRevoke(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.APIKeyHash = ""
			return &c, nil
		}); err != nil {
			log.Printf("handleAPIKeyRevoke: save config: %v", err)
			writeError(w, "failed to save config", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleAdvancedGet(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		writeJSON(w, map[string]string{
			"caddy_admin_url": cfg.CaddyAdminURL,
		})
	}
}

func handleAdvancedUpdate(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CaddyAdminURL string `json:"caddy_admin_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if msg := validateCaddyAdminURL(req.CaddyAdminURL); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.CaddyAdminURL = req.CaddyAdminURL
			return &c, nil
		}); err != nil {
			log.Printf("handleAdvancedUpdate: save config: %v", err)
			writeError(w, "failed to save config", http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func sessionMaxAge(cfg *config.AppConfig) int {
	if cfg.SessionMaxAge > 0 {
		return cfg.SessionMaxAge
	}
	return auth.DefaultSessionMaxAge
}

func hashBasicAuthPassword(ba *caddy.BasicAuth, fallbackHash string) error {
	if ba.Password != "" {
		hash, err := auth.HashPassword(ba.Password)
		if err != nil {
			return fmt.Errorf("hashing basic auth password: %w", err)
		}
		ba.PasswordHash = hash
		ba.Password = ""
	} else if ba.PasswordHash == "" {
		if fallbackHash == "" {
			return errors.New("password is required for basic auth")
		}
		ba.PasswordHash = fallbackHash
	}
	return nil
}

var (
	persistMu    sync.Mutex
	persistTimer *time.Timer
)

const persistDelay = 500 * time.Millisecond

// persistCaddyConfig debounces config persistence. Each call resets a short
// timer so rapid sequential mutations collapse into a single persist of the
// latest state, avoiding ordering races between goroutines.
func persistCaddyConfig(cc *caddy.Client, store *config.ConfigStore) {
	persistMu.Lock()
	defer persistMu.Unlock()
	if persistTimer != nil {
		persistTimer.Stop()
	}
	persistTimer = time.AfterFunc(persistDelay, func() {
		doPersistCaddyConfig(cc, store)
	})
}

func doPersistCaddyConfig(cc *caddy.Client, store *config.ConfigStore) {
	raw, err := cc.GetConfig()
	if err != nil {
		log.Printf("persistCaddyConfig: fetch config: %v", err)
		return
	}
	cfg := store.Get()
	if err := os.MkdirAll(filepath.Dir(cfg.CaddyConfigPath), 0755); err != nil {
		log.Printf("persistCaddyConfig: create directory: %v", err)
		return
	}
	tmp := cfg.CaddyConfigPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		log.Printf("persistCaddyConfig: write temp file: %v", err)
		return
	}
	if err := os.Rename(tmp, cfg.CaddyConfigPath); err != nil {
		os.Remove(tmp)
		log.Printf("persistCaddyConfig: rename: %v", err)
	}
}

// parseIntParam reads an integer query parameter, clamped to [min, max].
// Returns (0, true) if absent, (n, true) on success, or (0, false) if invalid
// (error already written to w). Use max < 0 to skip upper clamping.
func parseIntParam(w http.ResponseWriter, q url.Values, name string, min, max int) (int, bool) {
	v := q.Get(name)
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		writeError(w, "invalid "+name+" parameter", http.StatusBadRequest)
		return 0, false
	}
	if n < min {
		n = min
	}
	if max >= 0 && n > max {
		n = max
	}
	return n, true
}

func caddyError(w http.ResponseWriter, handler string, err error) {
	log.Printf("%s: %v", handler, err)
	errStr := err.Error()
	var msg string
	switch {
	case strings.Contains(errStr, "unreachable"):
		msg = "caddy admin API is unreachable - is Caddy running?"
	default:
		msg = extractCaddyMessage(errStr)
	}
	writeError(w, msg, http.StatusBadGateway)
}

// extractCaddyMessage pulls the human-readable error out of Caddy's JSON
// response embedded in our error string. Caddy returns {"error":"..."} and
// we wrap that as "caddy rejected ... (status N): {json}".
func extractCaddyMessage(errStr string) string {
	const fallback = "caddy returned an error - check server logs for details"
	idx := strings.Index(errStr, "{")
	if idx < 0 {
		return fallback
	}
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(errStr[idx:]), &parsed) != nil || parsed.Error == "" {
		return fallback
	}
	// Strip the verbose "loading new config:" prefix chain that Caddy nests
	msg := parsed.Error
	for _, prefix := range []string{"loading new config: ", "loading config: "} {
		msg = strings.TrimPrefix(msg, prefix)
	}
	msg = stripGoStructs(msg)
	return msg
}

// goStructRe matches Go struct literals like &logging.FileWriter{...} that
// Caddy sometimes dumps into error messages. These are noise for end users.
var goStructRe = regexp.MustCompile(`\s*(?:using\s+)?&\w+(?:\.\w+)*\{[^}]*\}`)

func stripGoStructs(msg string) string {
	cleaned := goStructRe.ReplaceAllString(msg, "")
	cleaned = strings.ReplaceAll(cleaned, ": : ", ": ")
	return strings.TrimSpace(cleaned)
}

func setAPIHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
}

func writeError(w http.ResponseWriter, msg string, code int) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]string{"error": msg}); err != nil {
		log.Printf("writeError: failed to encode error response: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setAPIHeaders(w)
	w.WriteHeader(code)
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("writeError: write error: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		log.Printf("writeJSON: failed to encode response: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setAPIHeaders(w)
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("writeJSON: write error: %v", err)
	}
}

func writeRawJSON(w http.ResponseWriter, raw []byte) {
	setAPIHeaders(w)
	if _, err := w.Write(raw); err != nil {
		log.Printf("writeRawJSON: write error: %v", err)
	}
}
