// Setup wizard and Caddyfile adaptation handlers.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/andretakagi/kaji/internal/auth"
	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/export"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func handleSetupStatus(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{
			"is_first_run":  !config.Exists(),
			"caddy_running": cc.IsReachable(),
		})
	}
}

func handleSetup(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password            string               `json:"password"`
			CaddyAdminURL       string               `json:"caddy_admin_url"`
			ACMEEmail           string               `json:"acme_email"`
			GlobalToggles       *caddy.GlobalToggles `json:"global_toggles"`
			CaddyfileJSON       json.RawMessage      `json:"caddyfile_json"`
			DNSChallengeToken   string               `json:"dns_challenge_token"`
			AutoSnapshotEnabled *bool                `json:"auto_snapshot_enabled"`
			AutoSnapshotLimit   *int                 `json:"auto_snapshot_limit"`
			BackupData          json.RawMessage      `json:"backup_data"`
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

		if !cc.IsReachable() {
			writeError(w, "Caddy is not running. Start Caddy before running setup.", http.StatusBadGateway)
			return
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

		{
			if len(req.BackupData) > 0 {
				var backup export.Backup
				if err := json.Unmarshal(req.BackupData, &backup); err != nil {
					log.Printf("handleSetup: parse backup data: %v", err)
					warnings = append(warnings, "failed to parse backup data: "+err.Error())
				} else {
					restoreWarnings, err := export.Restore(&backup, cc, store, ss, false, version)
					if err != nil {
						log.Printf("handleSetup: restore backup: %v", err)
						warnings = append(warnings, "failed to restore backup: "+err.Error())
					}
					warnings = append(warnings, restoreWarnings...)
					store.Update(func(current config.AppConfig) (*config.AppConfig, error) {
						current.PasswordHash = newCfg.PasswordHash
						current.SessionSecret = newCfg.SessionSecret
						current.AuthEnabled = newCfg.AuthEnabled
						return &current, nil
					})
				}
			} else if len(req.CaddyfileJSON) > 0 {
				if err := cc.LoadConfig(req.CaddyfileJSON); err != nil {
					log.Printf("handleSetup: load caddyfile config: %v", err)
					warnings = append(warnings, "failed to load Caddyfile config: "+err.Error())
				}
			} else {
				listenAddr := cc.HTTPSListenAddr()
				minimalCfg := []byte(fmt.Sprintf(`{"apps":{"http":{"servers":{"srv0":{"listen":[%q]}}},"tls":{"automation":{"policies":[]}}}}`, listenAddr))
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
