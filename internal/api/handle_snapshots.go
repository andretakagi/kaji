// Snapshot CRUD handlers.
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/export"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func handleSnapshotList(ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, ss.GetIndex())
	}
}

func handleSnapshotCreate(ss *snapshot.Store, cc *caddy.Client, store *config.ConfigStore, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.Name == "" {
			req.Name = "manual-" + time.Now().Format("2006-01-02T15:04:05")
		}

		data, err := buildSnapshotData(cc, store, version)
		if err != nil {
			log.Printf("handleSnapshotCreate: %v", err)
			writeError(w, "failed to capture config", http.StatusInternalServerError)
			return
		}

		snap, err := ss.Create(req.Name, req.Description, "manual", data)
		if err != nil {
			log.Printf("handleSnapshotCreate: %v", err)
			writeError(w, "failed to create snapshot", http.StatusInternalServerError)
			return
		}
		writeJSON(w, snap)
	}
}

func handleSnapshotDelete(ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, "snapshot id is required", http.StatusBadRequest)
			return
		}

		if err := ss.Delete(id); err != nil {
			writeError(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleSnapshotRestore(ss *snapshot.Store, cc *caddy.Client, store *config.ConfigStore, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, "snapshot id is required", http.StatusBadRequest)
			return
		}

		data, err := ss.ReadData(id)
		if err != nil {
			writeError(w, err.Error(), http.StatusNotFound)
			return
		}

		if err := cc.ValidateConfig(data.CaddyConfig); err != nil {
			if errors.Is(err, caddy.ErrValidationRollbackFailed) {
				caddyError(w, "handleSnapshotRestore", err)
				return
			}
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := cc.LoadConfig(data.CaddyConfig); err != nil {
			caddyError(w, "handleSnapshotRestore", err)
			return
		}

		if err := ss.SetCurrent(id); err != nil {
			log.Printf("handleSnapshotRestore: set current: %v", err)
			writeError(w, "restored config but failed to update snapshot index", http.StatusInternalServerError)
			return
		}

		persistCaddyConfig(cc, store)

		resp := map[string]any{"status": "ok"}

		if data.AppConfig != nil {
			var configMap map[string]any
			if err := json.Unmarshal(data.AppConfig, &configMap); err != nil {
				log.Printf("handleSnapshotRestore: parse app config: %v", err)
				resp["warnings"] = []string{"Caddy config restored but app config could not be parsed"}
				writeJSON(w, resp)
				return
			}

			if data.KajiVersion != "" {
				migrationLog, err := export.RunMigrations(configMap, data.KajiVersion)
				if err != nil {
					log.Printf("handleSnapshotRestore: migration: %v", err)
					resp["warnings"] = []string{"Caddy config restored but app config migration failed"}
					writeJSON(w, resp)
					return
				}
				if len(migrationLog) > 0 {
					resp["migration_log"] = migrationLog
				}
			}

			migratedJSON, err := json.Marshal(configMap)
			if err != nil {
				log.Printf("handleSnapshotRestore: re-encode config: %v", err)
				resp["warnings"] = []string{"Caddy config restored but app config could not be applied"}
				writeJSON(w, resp)
				return
			}

			var imported config.AppConfig
			if err := json.Unmarshal(migratedJSON, &imported); err != nil {
				log.Printf("handleSnapshotRestore: unmarshal migrated config: %v", err)
				resp["warnings"] = []string{"Caddy config restored but app config could not be applied"}
				writeJSON(w, resp)
				return
			}

			if err := store.Update(func(current config.AppConfig) (*config.AppConfig, error) {
				imported.PreserveCredentials(&current)
				return &imported, nil
			}); err != nil {
				log.Printf("handleSnapshotRestore: update app config: %v", err)
				resp["warnings"] = []string{"Caddy config restored but app config update failed: " + err.Error()}
				writeJSON(w, resp)
				return
			}
		} else {
			resp["legacy"] = true
		}

		writeJSON(w, resp)
	}
}

func handleSnapshotUpdate(ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, "snapshot id is required", http.StatusBadRequest)
			return
		}

		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		if err := ss.Update(id, req.Name, req.Description); err != nil {
			writeError(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleSnapshotSettings(ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AutoSnapshotEnabled bool `json:"auto_snapshot_enabled"`
			AutoSnapshotLimit   int  `json:"auto_snapshot_limit"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.AutoSnapshotLimit < 1 {
			req.AutoSnapshotLimit = 50
		}

		if err := ss.UpdateSettings(req.AutoSnapshotEnabled, req.AutoSnapshotLimit); err != nil {
			log.Printf("handleSnapshotSettings: %v", err)
			writeError(w, "failed to update snapshot settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}
