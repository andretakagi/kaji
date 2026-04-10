// Setup wizard and Caddyfile adaptation handlers.
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/andretakagi/kaji/internal/auth"
	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
)

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
		if !decodeBody(w, r, &req) {
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

		if err := cc.EnsureDefaultLogger(); err != nil {
			log.Printf("handleSetup: ensure default logger: %v", err)
			warnings = append(warnings, "failed to initialize default logger: "+err.Error())
		}

		if err := cc.EnsureAccessLogger(); err != nil {
			log.Printf("handleSetup: ensure access logger: %v", err)
			warnings = append(warnings, "failed to initialize access logger: "+err.Error())
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
		if !decodeBody(w, r, &req) {
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
