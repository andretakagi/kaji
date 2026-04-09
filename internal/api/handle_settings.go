// Global toggles, ACME email, and advanced settings handlers.
package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

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

func handleGlobalTogglesUpdate(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
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
		maybeAutoSnapshot(cc, ss, "Global toggles updated")

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

func handleACMEEmailUpdate(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
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
		maybeAutoSnapshot(cc, ss, "ACME email updated")

		if err := cc.SetACMEEmail(req.Email); err != nil {
			caddyError(w, "handleACMEEmailUpdate", err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
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
