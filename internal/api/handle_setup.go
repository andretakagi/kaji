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
	"github.com/andretakagi/kaji/internal/snapshot"
)

func handleSetupStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"is_first_run": !config.Exists()})
	}
}

func handleSetup(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password           string               `json:"password"`
			CaddyAdminURL      string               `json:"caddy_admin_url"`
			ACMEEmail          string               `json:"acme_email"`
			GlobalToggles      *caddy.GlobalToggles `json:"global_toggles"`
			CaddyfileJSON      json.RawMessage      `json:"caddyfile_json"`
			DNSChallengeToken  string               `json:"dns_challenge_token"`
			AutoSnapshotEnabled *bool               `json:"auto_snapshot_enabled"`
			AutoSnapshotLimit  *int                 `json:"auto_snapshot_limit"`
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
		var dnsError string
		caddyUp := cc.IsReachable()

		if caddyUp {
			if len(req.CaddyfileJSON) > 0 {
				if err := cc.LoadConfig(req.CaddyfileJSON); err != nil {
					log.Printf("handleSetup: load caddyfile config: %v", err)
					warnings = append(warnings, "failed to load Caddyfile config: "+err.Error())
				}
			} else {
				minimalCfg := []byte(`{"apps":{"http":{"servers":{"srv0":{"listen":[":443"]}}},"tls":{"automation":{"policies":[]}}}}`)
				if err := cc.LoadConfig(minimalCfg); err != nil {
					log.Printf("handleSetup: load minimal config: %v", err)
					warnings = append(warnings, "failed to initialize Caddy config: "+err.Error())
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
			if req.DNSChallengeToken != "" {
				if err := cc.SetDNSProvider(req.DNSChallengeToken, true); err != nil {
					log.Printf("handleSetup: set DNS provider: %v", err)
					dnsError = cleanDNSProviderError(err)
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
		} else {
			log.Println("handleSetup: Caddy not reachable, skipping proxy configuration")
		}

		if req.AutoSnapshotEnabled != nil {
			limit := 10
			if req.AutoSnapshotLimit != nil {
				limit = *req.AutoSnapshotLimit
			}
			if err := ss.UpdateSettings(*req.AutoSnapshotEnabled, limit); err != nil {
				log.Printf("handleSetup: update snapshot settings: %v", err)
				warnings = append(warnings, "failed to configure snapshot settings: "+err.Error())
			}
		}

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
		if dnsError != "" {
			resp["dns_error"] = dnsError
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

// cleanDNSProviderError extracts a user-facing message from a Caddy DNS
// provider error. Caddy wraps the real problem in a deep provisioning chain
// that isn't useful to show in the UI.
func cleanDNSProviderError(err error) string {
	msg := err.Error()

	// The Caddy JSON error is embedded in the Go error string. Extract it.
	if idx := strings.Index(msg, `{"error":"`); idx >= 0 {
		var parsed struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(msg[idx:]), &parsed) == nil && parsed.Error != "" {
			msg = parsed.Error
		}
	}

	// Pull out just the cloudflare module's message from the provisioning chain.
	if idx := strings.Index(msg, "provision dns.providers.cloudflare: "); idx >= 0 {
		return msg[idx+len("provision dns.providers.cloudflare: "):]
	}

	// Generic DNS provider fallback.
	if idx := strings.Index(msg, "provision dns.providers."); idx >= 0 {
		tail := msg[idx+len("provision dns.providers."):]
		if colonIdx := strings.Index(tail, ": "); colonIdx >= 0 {
			return tail[colonIdx+2:]
		}
	}

	return "could not configure DNS challenge provider"
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
