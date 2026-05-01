// Global toggles, ACME email, and advanced settings handlers.
package api

import (
	"fmt"
	"log"
	"net/http"
	"os"

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

func handleGlobalTogglesUpdate(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var toggles caddy.GlobalToggles
		if !decodeBody(w, r, &toggles) {
			return
		}
		if msg := validateAutoHTTPS(toggles.AutoHTTPS); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		maybeAutoSnapshot(cc, ss, store, version, "Global toggles updated")

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

func handleACMEEmailUpdate(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if msg := validateEmail(req.Email); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		maybeAutoSnapshot(cc, ss, store, version, "ACME email updated")

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
		if !decodeBody(w, r, &req) {
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

func handleDNSProviderGet(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := cc.GetDNSProvider()
		if err != nil {
			caddyError(w, "handleDNSProviderGet", err)
			return
		}
		writeJSON(w, result)
	}
}

func handleDNSProviderUpdate(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Enabled  bool   `json:"enabled"`
			APIToken string `json:"api_token"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		maybeAutoSnapshot(cc, ss, store, version, "DNS provider updated")

		if err := cc.SetDNSProvider(req.APIToken, req.Enabled); err != nil {
			log.Printf("handleDNSProviderUpdate: %v", err)
			writeError(w, cleanDNSProviderError(err), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handlePortsGet(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ports, err := cc.GetPorts()
		if err != nil {
			caddyError(w, "handlePortsGet", err)
			return
		}
		writeJSON(w, ports)
	}
}

func handlePortsUpdate(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var ports caddy.Ports
		if !decodeBody(w, r, &ports) {
			return
		}
		if msg := validatePort(ports.HTTPPort, "HTTP"); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if msg := validatePort(ports.HTTPSPort, "HTTPS"); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if ports.HTTPPort == ports.HTTPSPort {
			writeError(w, "HTTP and HTTPS ports must be different", http.StatusBadRequest)
			return
		}
		maybeAutoSnapshot(cc, ss, store, version, "Ports updated")

		if err := cc.SetPorts(&ports); err != nil {
			caddyError(w, "handlePortsUpdate", err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleCaddyDataDirGet(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		resolved := caddy.ResolveCaddyDataDir(cfg.CaddyDataDir)
		isOverride := "false"
		if cfg.CaddyDataDir != "" {
			isOverride = "true"
		}
		writeJSON(w, map[string]string{
			"caddy_data_dir": resolved,
			"is_override":    isOverride,
		})
	}
}

func handleCaddyDataDirUpdate(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CaddyDataDir string `json:"caddy_data_dir"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		if req.CaddyDataDir != "" {
			info, err := os.Stat(req.CaddyDataDir)
			if err != nil || !info.IsDir() {
				writeError(w, fmt.Sprintf("directory does not exist: %s", req.CaddyDataDir), http.StatusBadRequest)
				return
			}
		}

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.CaddyDataDir = req.CaddyDataDir
			return &c, nil
		}); err != nil {
			log.Printf("handleCaddyDataDirUpdate: save config: %v", err)
			writeError(w, "failed to save config", http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "ok"})
	}
}
