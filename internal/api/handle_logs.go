// Log viewing, streaming, and config handlers.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/logging"
	"github.com/andretakagi/kaji/internal/snapshot"
)

const (
	logFilePollInterval  = 2 * time.Second
	sseKeepaliveInterval = 15 * time.Second
)

func handleLogs(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		q := r.URL.Query()
		params := logging.QueryParams{
			Level: q.Get("level"),
			Host:  q.Get("host"),
		}

		var ok bool
		if params.Limit, ok = parseIntParam(w, q, "limit", 0, 1000); !ok {
			return
		}
		if params.Offset, ok = parseIntParam(w, q, "offset", 0, -1); !ok {
			return
		}
		if params.StatusMin, ok = parseIntParam(w, q, "status_min", 0, -1); !ok {
			return
		}
		if params.StatusMax, ok = parseIntParam(w, q, "status_max", 0, -1); !ok {
			return
		}
		if v := q.Get("since"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeError(w, "invalid since parameter, expected RFC3339 format", http.StatusBadRequest)
				return
			}
			params.Since = t
		}
		if v := q.Get("until"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeError(w, "invalid until parameter, expected RFC3339 format", http.StatusBadRequest)
				return
			}
			params.Until = t
		}

		result, err := logging.QueryLogs(cfg.LogFile, params)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				writeJSON(w, logging.QueryResult{Entries: []logging.LogEntry{}})
				return
			}
			log.Printf("handleLogs: %v", err)
			writeError(w, "failed to query logs", http.StatusInternalServerError)
			return
		}

		writeJSON(w, result)
	}
}

func handleLogStream(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		logFile := store.Get().LogFile
		if logFile == "" {
			writeError(w, "log file not configured", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher.Flush()

		ctx := r.Context()
		lines := make(chan string, 64)

		// If the log file doesn't exist yet (fresh install, no traffic),
		// wait for it to appear rather than returning an error.
		for {
			if _, err := os.Stat(logFile); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(logFilePollInterval):
			}
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- logging.TailFile(ctx, logFile, lines)
		}()

		ticker := time.NewTicker(sseKeepaliveInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
			case line := <-lines:
				line = strings.ReplaceAll(line, "\n", "")
				line = strings.ReplaceAll(line, "\r", "")
				fmt.Fprintf(w, "data: %s\n\n", line)
				flusher.Flush()
			case err := <-errCh:
				if err != nil && err != context.Canceled {
					log.Printf("handleLogStream: %v", err)
				}
				return
			}
		}
	}
}

func handleLogConfigGet(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := cc.GetLoggingConfig()
		if err != nil {
			raw = []byte(`{"logs":{}}`)
		}
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || string(trimmed) == "null" {
			raw = []byte(`{"logs":{}}`)
		}
		var base map[string]json.RawMessage
		if json.Unmarshal(raw, &base) != nil {
			base = map[string]json.RawMessage{}
		}
		dir, _ := json.Marshal(store.Get().LogDir)
		base["log_dir"] = dir
		out, _ := json.Marshal(base)
		writeRawJSON(w, out)
	}
}

func handleLogConfigUpdate(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store) http.HandlerFunc {
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
		if msg := validateLogFilePaths(body, store.Get().LogDir); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		// If any existing sink is being removed, cascade: clear domain mappings.
		var incoming struct {
			Logs map[string]json.RawMessage `json:"logs"`
		}
		if existing, _ := cc.GetLoggingConfig(); existing != nil {
			var current struct {
				Logs map[string]json.RawMessage `json:"logs"`
			}
			if json.Unmarshal(existing, &current) == nil {
				json.Unmarshal(body, &incoming)
				for name := range current.Logs {
					if _, exists := incoming.Logs[name]; !exists {
						if err := cc.ClearDomainsForSink(name); err != nil {
							log.Printf("handleLogConfigUpdate: clear domains for removed sink %q: %v", name, err)
						}
					}
				}
			}
		}

		// When kaji_access writer is set to discard, clear all route mappings.
		if kajiRaw, ok := incoming.Logs["kaji_access"]; ok {
			var sink struct {
				Writer struct {
					Output string `json:"output"`
				} `json:"writer"`
			}
			if json.Unmarshal(kajiRaw, &sink) == nil && sink.Writer.Output == "discard" {
				if err := cc.ClearDomainsForSink("kaji_access"); err != nil {
					log.Printf("handleLogConfigUpdate: clear kaji_access domains: %v", err)
				}
			}
		}

		maybeAutoSnapshot(cc, ss, "Log config updated")

		if err := cc.SetLoggingConfig(body); err != nil {
			caddyError(w, "handleLogConfigUpdate", err)
			return
		}
		// Protected loggers must always exist
		if err := cc.EnsureAccessLogger(); err != nil {
			log.Printf("handleLogConfigUpdate: ensure access logger: %v", err)
		}
		writeJSON(w, map[string]string{"status": "ok"})
		persistCaddyConfig(cc, store)
	}
}

func handleAccessDomains(cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domains, err := cc.GetAllAccessLogDomains()
		if err != nil {
			writeJSON(w, map[string]map[string]string{})
			return
		}
		writeJSON(w, domains)
	}
}

func validateLogFilePaths(body []byte, logDir string) string {
	var cfg struct {
		Logs map[string]struct {
			Writer struct {
				Output   string `json:"output"`
				Filename string `json:"filename"`
			} `json:"writer"`
		} `json:"logs"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return fmt.Sprintf("invalid logging config: %v", err)
	}
	for name, sink := range cfg.Logs {
		if sink.Writer.Output != "file" {
			continue
		}
		cleaned := path.Clean(sink.Writer.Filename)
		if !strings.HasPrefix(cleaned, logDir) {
			return fmt.Sprintf("log '%s': file path must be under %s", name, logDir)
		}
	}
	return ""
}
