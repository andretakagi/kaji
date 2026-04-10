// Snapshot CRUD handlers.
package api

import (
	"log"
	"net/http"
	"time"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func handleSnapshotList(ss *snapshot.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, ss.GetIndex())
	}
}

func handleSnapshotCreate(ss *snapshot.Store, cc *caddy.Client) http.HandlerFunc {
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

		configData, err := cc.GetConfig()
		if err != nil {
			caddyError(w, "handleSnapshotCreate", err)
			return
		}

		snap, err := ss.Create(req.Name, req.Description, "manual", configData)
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

func handleSnapshotRestore(ss *snapshot.Store, cc *caddy.Client, store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, "snapshot id is required", http.StatusBadRequest)
			return
		}

		configData, err := ss.ReadConfig(id)
		if err != nil {
			writeError(w, err.Error(), http.StatusNotFound)
			return
		}

		if err := cc.LoadConfig(configData); err != nil {
			caddyError(w, "handleSnapshotRestore", err)
			return
		}

		if err := ss.SetCurrent(id); err != nil {
			log.Printf("handleSnapshotRestore: set current: %v", err)
			writeError(w, "restored config but failed to update snapshot index", http.StatusInternalServerError)
			return
		}

		persistCaddyConfig(cc, store)
		writeJSON(w, map[string]string{"status": "ok"})
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
