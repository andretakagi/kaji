// Auth middleware, access logging, request body limits.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/andretakagi/kaji/internal/auth"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/export"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.size += n
	return n, err
}

func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type accessLogEntry struct {
	Ts         string  `json:"ts"`
	Level      string  `json:"level"`
	Logger     string  `json:"logger"`
	Msg        string  `json:"msg"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Status     int     `json:"status"`
	Duration   float64 `json:"duration"`
	Size       int     `json:"size"`
	RemoteAddr string  `json:"remote_addr"`
}

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		dur := time.Since(start)

		entry := accessLogEntry{
			Ts:         start.Format(time.RFC3339Nano),
			Level:      "info",
			Logger:     "kaji.access",
			Msg:        "handled request",
			Method:     r.Method,
			Path:       r.URL.Path,
			Status:     rec.status,
			Duration:   dur.Seconds(),
			Size:       rec.size,
			RemoteAddr: r.RemoteAddr,
		}

		if rec.status >= 400 {
			entry.Level = "error"
		}

		b, err := json.Marshal(entry)
		if err != nil {
			log.Printf("access log marshal error: %v", err)
			return
		}
		if _, err := os.Stdout.Write(append(b, '\n')); err != nil {
			log.Printf("access log write error: %v", err)
		}
	})
}

func extractBearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return h[7:], true
	}
	return "", false
}

func hasBearerToken(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	return len(h) > 7 && strings.EqualFold(h[:7], "bearer ")
}

var publicPaths = map[string]bool{
	"/api/setup/status": true,
	"/api/auth/login":   true,
	"/api/auth/status":  true,
}

var setupOnlyPaths = map[string]bool{
	"/api/setup":                   true,
	"/api/setup/import/caddyfile":  true,
	"/api/setup/import/full":       true,
	"/api/setup/default-caddyfile": true,
}

const maxRequestBodySize = 1 << 20 // 1 MB

var largeUploadPaths = map[string]bool{
	"/api/import/full":       true,
	"/api/setup/import/full": true,
}

func limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			limit := int64(maxRequestBodySize)
			if largeUploadPaths[r.URL.Path] {
				limit = export.MaxZIPSize
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

func requireAuth(store *config.ConfigStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()

		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		if publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		if setupOnlyPaths[r.URL.Path] && !config.Exists() {
			next.ServeHTTP(w, r)
			return
		}

		// API key auth applies regardless of auth_enabled
		if cfg.APIKeyHash != "" {
			if key, ok := extractBearerToken(r); ok && auth.CheckAPIKey(key, cfg.APIKeyHash) {
				next.ServeHTTP(w, r)
				return
			}
		}

		if !cfg.AuthEnabled {
			// No password auth required, but if an API key is set and we got
			// here, the request didn't provide a valid key
			if cfg.APIKeyHash != "" && hasBearerToken(r) {
				writeError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		token := auth.GetSessionToken(r)
		if token != "" && auth.ValidateSignedToken(token, cfg.SessionSecret, cfg.SessionMaxAge) {
			next.ServeHTTP(w, r)
			return
		}

		writeError(w, "unauthorized", http.StatusUnauthorized)
	})
}
