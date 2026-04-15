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

func handleImportCaddyfile(cc *caddy.Client, store *config.ConfigStore, ss *snapshot.Store, version string) http.HandlerFunc {
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

		data, err := buildSnapshotData(cc, store, version)
		if err != nil {
			log.Printf("handleImportCaddyfile: pre-import snapshot: %v", err)
			writeError(w, "failed to create pre-import snapshot: "+err.Error(), http.StatusInternalServerError)
			return
		}
		name := "pre-import-" + time.Now().Format("2006-01-02T15:04:05")
		if _, err := ss.Create(name, "Before Caddyfile import", "auto", data); err != nil {
			log.Printf("handleImportCaddyfile: pre-import snapshot: %v", err)
			writeError(w, "failed to create pre-import snapshot: "+err.Error(), http.StatusInternalServerError)
			return
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

		adminListen := ""
		if settings != nil {
			adminListen = settings.AdminListen
		}
		if adminListen == "" {
			adminListen = caddy.ParseCaddyfileAdminAddr(req.Caddyfile)
		}

		if adminListen != "" {
			adminURL := "http://" + adminListen
			if err := store.Update(func(cur config.AppConfig) (*config.AppConfig, error) {
				cur.CaddyAdminURL = adminURL
				return &cur, nil
			}); err != nil {
				log.Printf("handleImportCaddyfile: update admin URL: %v", err)
			}
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

func handleImportFull(cc *caddy.Client, store *config.ConfigStore, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backup, err := export.ParseZIP(r.Body, r.ContentLength, version)
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

		restoreWarnings, err := export.Restore(backup, cc, store, ss, true, version)
		if err != nil {
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

		resp := map[string]any{
			"status":         "ok",
			"route_count":    routeCount,
			"snapshot_count": snapshotCount,
		}
		if len(backup.MigrationLog) > 0 {
			resp["migrated_from"] = backup.Manifest.KajiVersion
			resp["migration_log"] = backup.MigrationLog
		}
		if len(restoreWarnings) > 0 {
			resp["warnings"] = restoreWarnings
		}
		writeJSON(w, resp)
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

		routes := caddy.ExtractReviewRoutes(adaptedJSON)

		adminListen := settings.AdminListen
		if adminListen == "" {
			adminListen = caddy.ParseCaddyfileAdminAddr(req.Caddyfile)
		}

		resp := map[string]any{
			"acme_email":     settings.ACMEEmail,
			"global_toggles": settings.Toggles,
			"route_count":    settings.RouteCount,
			"adapted_config": adaptedJSON,
			"routes":         routes,
		}
		if adminListen != "" {
			resp["admin_listen"] = adminListen
		}
		writeJSON(w, resp)
	}
}

func handleSetupImportFull(cc *caddy.Client, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backup, err := export.ParseZIP(r.Body, r.ContentLength, version)
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
		if len(backup.MigrationLog) > 0 {
			resp["migrated_from"] = backup.Manifest.KajiVersion
			resp["migration_log"] = backup.MigrationLog
		}

		routes := caddy.ExtractReviewRoutes(backup.CaddyConfig)
		for _, dr := range backup.AppConfig.DisabledRoutes {
			params, err := caddy.ParseRouteParams(dr.Route)
			if err != nil || params.Domain == "" {
				continue
			}
			routes = append(routes, caddy.ReviewRoute{
				Domain:   params.Domain,
				Upstream: params.Upstream,
				Enabled:  false,
			})
		}

		type reviewIPList struct {
			Name       string `json:"name"`
			Type       string `json:"type"`
			EntryCount int    `json:"entry_count"`
		}
		ipLists := make([]reviewIPList, 0, len(backup.AppConfig.IPLists))
		for _, l := range backup.AppConfig.IPLists {
			ipLists = append(ipLists, reviewIPList{
				Name:       l.Name,
				Type:       l.Type,
				EntryCount: len(l.IPs),
			})
		}

		type reviewSnapshot struct {
			Name      string `json:"name"`
			Type      string `json:"type"`
			CreatedAt string `json:"created_at"`
		}
		var snapshots []reviewSnapshot
		if backup.Snapshots != nil {
			for _, s := range backup.Snapshots.Index.Snapshots {
				snapshots = append(snapshots, reviewSnapshot{
					Name:      s.Name,
					Type:      s.Type,
					CreatedAt: s.CreatedAt,
				})
			}
		}

		resp["review"] = map[string]any{
			"routes": routes,
			"logging": map[string]any{
				"log_file":      backup.AppConfig.LogFile,
				"log_dir":       backup.AppConfig.LogDir,
				"loki_enabled":  backup.AppConfig.Loki.Enabled,
				"loki_endpoint": backup.AppConfig.Loki.Endpoint,
			},
			"ip_lists":  ipLists,
			"snapshots": snapshots,
		}

		writeJSON(w, resp)
	}
}
