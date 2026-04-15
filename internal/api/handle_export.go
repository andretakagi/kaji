package api

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/export"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func handleExportCaddyfile(cc *caddy.Client, store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := cc.GetConfig()
		if err != nil {
			caddyError(w, "handleExportCaddyfile", err)
			return
		}
		cfg := store.Get()
		content, err := caddy.GenerateCaddyfile(raw, cfg.LogFile)
		if err != nil {
			log.Printf("handleExportCaddyfile: %v", err)
			writeError(w, "failed to generate Caddyfile", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"content": content})
	}
}

func handleExportFull(cc *caddy.Client, store *config.ConfigStore, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		if err := export.BuildZIP(&buf, cc, store, ss, version); err != nil {
			log.Printf("handleExportFull: %v", err)
			writeError(w, "failed to build export", http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("kaji-export-%s.zip", time.Now().Format("2006-01-02"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Write(buf.Bytes())
	}
}

func handleImportCaddyfile(cc *caddy.Client, store *config.ConfigStore, ss *snapshot.Store) http.HandlerFunc {
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

		if err := cc.ValidateConfig(adaptedJSON); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, caddy.ErrValidationRollbackFailed) {
				status = http.StatusInternalServerError
			}
			writeError(w, err.Error(), status)
			return
		}

		configData, err := cc.GetConfig()
		if err != nil {
			log.Printf("handleImportCaddyfile: pre-import snapshot: %v", err)
		} else {
			name := "pre-import-" + time.Now().Format("2006-01-02T15:04:05")
			if _, err := ss.Create(name, "Before Caddyfile import", "auto", configData); err != nil {
				log.Printf("handleImportCaddyfile: pre-import snapshot: %v", err)
			}
		}

		if err := cc.LoadConfig(adaptedJSON); err != nil {
			log.Printf("handleImportCaddyfile: load config: %v", err)
			writeError(w, "failed to load adapted config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		persistCaddyConfig(cc, store)

		settings, err := caddy.ExtractCaddyfileSettings(adaptedJSON)
		if err != nil {
			log.Printf("handleImportCaddyfile: extract settings: %v", err)
		}

		routeCount := 0
		if settings != nil {
			routeCount = settings.RouteCount
		}

		writeJSON(w, map[string]any{
			"status":      "ok",
			"route_count": routeCount,
		})
	}
}

func handleImportFull(cc *caddy.Client, store *config.ConfigStore, ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, export.MaxZIPSize)

		backup, err := export.ParseZIP(r.Body, r.ContentLength)
		if err != nil {
			writeError(w, "invalid backup file: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := cc.ValidateConfig(backup.CaddyConfig); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, caddy.ErrValidationRollbackFailed) {
				status = http.StatusInternalServerError
			}
			writeError(w, err.Error(), status)
			return
		}

		if err := export.Restore(backup, cc, store, ss, true); err != nil {
			log.Printf("handleImportFull: %v", err)
			writeError(w, "import failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		persistCaddyConfig(cc, store)

		snapshotCount := 0
		if backup.Snapshots != nil {
			snapshotCount = len(backup.Snapshots.Index.Snapshots)
		}

		routeCount := caddy.CountRoutes(backup.CaddyConfig)

		writeJSON(w, map[string]any{
			"status":         "ok",
			"route_count":    routeCount,
			"snapshot_count": snapshotCount,
		})
	}
}

func handleSetupImportCaddyfile(cc *caddy.Client) http.HandlerFunc {
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
			log.Printf("handleSetupImportCaddyfile: extract settings: %v", err)
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

func handleSetupImportFull(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, export.MaxZIPSize)

		backup, err := export.ParseZIP(r.Body, r.ContentLength)
		if err != nil {
			writeError(w, "invalid backup file: "+err.Error(), http.StatusBadRequest)
			return
		}

		snapshotCount := 0
		if backup.Snapshots != nil {
			snapshotCount = len(backup.Snapshots.Index.Snapshots)
		}

		resp := map[string]any{
			"status":      "ok",
			"backup_data": backup,
			"summary": map[string]any{
				"auth_enabled":    backup.AppConfig.AuthEnabled,
				"has_api_key":     backup.AppConfig.APIKeyHash != "",
				"caddy_admin_url": backup.AppConfig.CaddyAdminURL,
				"loki_enabled":    backup.AppConfig.Loki.Enabled,
				"ip_lists":        len(backup.AppConfig.IPLists),
				"disabled_routes": len(backup.AppConfig.DisabledRoutes),
				"snapshot_count":  snapshotCount,
			},
		}

		if settings, err := caddy.ExtractCaddyfileSettings(backup.CaddyConfig); err == nil {
			resp["acme_email"] = settings.ACMEEmail
			resp["global_toggles"] = settings.Toggles
			resp["route_count"] = settings.RouteCount
		}

		writeJSON(w, resp)
	}
}
