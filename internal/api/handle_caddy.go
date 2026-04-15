// Caddy service control, config proxy, upstreams, and Caddyfile export.
package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/system"
)

const caddyReadyTimeout = 10 * time.Second

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
		if err := cc.WaitReady(caddyReadyTimeout); err != nil {
			log.Printf("handleStart: caddy started but admin API not ready: %v", err)
			writeError(w, "caddy process started but admin API is not responding", http.StatusBadGateway)
			return
		}
		loadSavedCaddyConfig(cc, store)
		ensureLoggers(cc)
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
		if err := cc.WaitReady(caddyReadyTimeout); err != nil {
			log.Printf("handleRestart: caddy restarted but admin API not ready: %v", err)
			writeError(w, "caddy process restarted but admin API is not responding", http.StatusBadGateway)
			return
		}
		loadSavedCaddyConfig(cc, store)
		ensureLoggers(cc)
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
