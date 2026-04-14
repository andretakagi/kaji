package api

import (
	"log"
	"net/http"
	"time"

	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/logging"
)

func handleLokiStatus(pipeline *logging.LokiPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		running, sinks := pipeline.GetStatus()

		type sinkStatusJSON struct {
			Tailing       bool   `json:"tailing"`
			LastPushAt    string `json:"last_push_at"`
			EntriesPushed int64  `json:"entries_pushed"`
			LastError     string `json:"last_error"`
		}

		result := struct {
			Running bool                      `json:"running"`
			Sinks   map[string]sinkStatusJSON `json:"sinks"`
		}{
			Running: running,
			Sinks:   make(map[string]sinkStatusJSON),
		}

		for name, s := range sinks {
			lastPush := ""
			if !s.LastPushAt.IsZero() {
				lastPush = s.LastPushAt.Format(time.RFC3339)
			}
			result.Sinks[name] = sinkStatusJSON{
				Tailing:       s.Tailing,
				LastPushAt:    lastPush,
				EntriesPushed: s.EntriesPushed,
				LastError:     s.LastError,
			}
		}

		writeJSON(w, result)
	}
}

func handleLokiConfigGet(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		writeJSON(w, cfg.Loki)
	}
}

func handleLokiConfigUpdate(store *config.ConfigStore, pipeline *logging.LokiPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req config.LokiConfig
		if !decodeBody(w, r, &req) {
			return
		}

		old := store.Get().Loki

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.Loki = req
			return &c, nil
		}); err != nil {
			writeError(w, "failed to save loki config", http.StatusInternalServerError)
			log.Printf("handleLokiConfigUpdate: %v", err)
			return
		}

		if onlySinksChanged(old, req) {
			pipeline.Reconfigure()
		} else {
			pipeline.Restart()
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func onlySinksChanged(old, new config.LokiConfig) bool {
	return old.Enabled == new.Enabled &&
		old.Endpoint == new.Endpoint &&
		old.BearerToken == new.BearerToken &&
		old.TenantID == new.TenantID &&
		old.BatchSize == new.BatchSize &&
		old.FlushIntervalSeconds == new.FlushIntervalSeconds &&
		mapsEqual(old.Labels, new.Labels)
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func handleLokiTest(store *config.ConfigStore, pipeline *logging.LokiPipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		if cfg.Loki.Endpoint == "" {
			writeJSON(w, map[string]any{
				"success": false,
				"message": "No Loki endpoint configured",
			})
			return
		}

		pusher := pipeline.GetPusher()
		if pusher == nil {
			pusher = logging.NewLokiPusher(cfg.Loki.Endpoint, cfg.Loki.BearerToken, cfg.Loki.TenantID, nil, nil)
		}

		if err := pusher.SendTestEntry(cfg.Loki.Endpoint, cfg.Loki.BearerToken, cfg.Loki.TenantID); err != nil {
			writeJSON(w, map[string]any{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, map[string]any{
			"success": true,
			"message": "Test entry sent successfully",
		})
	}
}
