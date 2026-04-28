// Endpoint registration and shared API utilities.
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andretakagi/kaji/internal/auth"
	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/logging"
	"github.com/andretakagi/kaji/internal/snapshot"
	"github.com/andretakagi/kaji/internal/system"
)

func RegisterRoutes(mux *http.ServeMux, store *config.ConfigStore, mgr system.CaddyManager, cc *caddy.Client, ss *snapshot.Store, pipeline *logging.LokiPipeline, version string) http.Handler {
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"version": version})
	})

	mux.HandleFunc("GET /api/setup/status", handleSetupStatus(cc))
	mux.HandleFunc("POST /api/setup", handleSetup(store, cc, ss, version))
	mux.HandleFunc("POST /api/setup/import/caddyfile", handleSetupImportCaddyfile(cc))
	mux.HandleFunc("POST /api/setup/import/full", handleSetupImportFull(cc, version))
	mux.HandleFunc("GET /api/setup/default-caddyfile", handleDefaultCaddyfile())

	mux.HandleFunc("GET /api/auth/status", handleAuthStatus(store))
	mux.HandleFunc("POST /api/auth/login", handleLogin(store))
	mux.HandleFunc("POST /api/auth/logout", handleLogout(store))
	mux.HandleFunc("PUT /api/auth/password", handlePasswordChange(store))

	mux.HandleFunc("GET /api/caddy/status", handleStatus(mgr))
	mux.HandleFunc("POST /api/caddy/start", handleStart(mgr, cc, store))
	mux.HandleFunc("POST /api/caddy/stop", handleStop(mgr))
	mux.HandleFunc("POST /api/caddy/restart", handleRestart(mgr, cc, store))
	mux.HandleFunc("GET /api/caddy/config", handleConfigProxy(cc))
	mux.HandleFunc("GET /api/caddy/config/{path...}", handleConfigProxy(cc))
	mux.HandleFunc("POST /api/caddy/load", handleConfigLoad(store, cc))
	mux.HandleFunc("GET /api/caddy/upstreams", handleUpstreams(cc))
	// Domain management
	mux.HandleFunc("GET /api/domains", handleListDomains(store))
	mux.HandleFunc("POST /api/domains/full", handleCreateDomainFull(store, cc, ss, version))
	mux.HandleFunc("GET /api/domains/{id}", handleGetDomain(store))
	mux.HandleFunc("PUT /api/domains/{id}", handleUpdateDomain(store, cc, ss, version))
	mux.HandleFunc("DELETE /api/domains/{id}", handleDeleteDomain(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/enable", handleEnableDomain(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/disable", handleDisableDomain(store, cc, ss, version))
	mux.HandleFunc("PUT /api/domains/{id}/rule", handleUpdateDomainRule(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/rule/enable", handleEnableDomainRule(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/rule/disable", handleDisableDomainRule(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/paths", handleCreateDomainPath(store, cc, ss, version))
	mux.HandleFunc("PUT /api/domains/{id}/paths/{pathId}", handleUpdateDomainPath(store, cc, ss, version))
	mux.HandleFunc("DELETE /api/domains/{id}/paths/{pathId}", handleDeleteDomainPath(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/paths/{pathId}/enable", handleEnableDomainPath(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/paths/{pathId}/disable", handleDisableDomainPath(store, cc, ss, version))
	// Subdomain management
	mux.HandleFunc("POST /api/domains/{id}/subdomains", handleCreateSubdomain(store, cc, ss, version))
	mux.HandleFunc("PUT /api/domains/{id}/subdomains/{subId}", handleUpdateSubdomain(store, cc, ss, version))
	mux.HandleFunc("DELETE /api/domains/{id}/subdomains/{subId}", handleDeleteSubdomain(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/subdomains/{subId}/enable", handleEnableSubdomain(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/subdomains/{subId}/disable", handleDisableSubdomain(store, cc, ss, version))
	mux.HandleFunc("PUT /api/domains/{id}/subdomains/{subId}/rule", handleUpdateSubdomainRule(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/subdomains/{subId}/rule/enable", handleEnableSubdomainRule(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/subdomains/{subId}/rule/disable", handleDisableSubdomainRule(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/subdomains/{subId}/paths", handleCreateSubdomainPath(store, cc, ss, version))
	mux.HandleFunc("PUT /api/domains/{id}/subdomains/{subId}/paths/{pathId}", handleUpdateSubdomainPath(store, cc, ss, version))
	mux.HandleFunc("DELETE /api/domains/{id}/subdomains/{subId}/paths/{pathId}", handleDeleteSubdomainPath(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/subdomains/{subId}/paths/{pathId}/enable", handleEnableSubdomainPath(store, cc, ss, version))
	mux.HandleFunc("POST /api/domains/{id}/subdomains/{subId}/paths/{pathId}/disable", handleDisableSubdomainPath(store, cc, ss, version))

	mux.HandleFunc("GET /api/logs", handleLogs(store))
	mux.HandleFunc("GET /api/logs/stream", handleLogStream(store))
	mux.HandleFunc("GET /api/logs/config", handleLogConfigGet(store, cc))
	mux.HandleFunc("PUT /api/logs/config", handleLogConfigUpdate(store, cc, ss, version))
	mux.HandleFunc("GET /api/logs/access-domains", handleAccessDomains(cc))
	mux.HandleFunc("GET /api/export/caddyfile", handleExportCaddyfile(cc, store))
	mux.HandleFunc("GET /api/export/full", handleExportFull(cc, store, ss, version))
	mux.HandleFunc("POST /api/import/caddyfile", handleImportCaddyfile(cc, store, ss, version))
	mux.HandleFunc("POST /api/import/full", handleImportFull(cc, store, ss, version))
	mux.HandleFunc("GET /api/settings/global-toggles", handleGlobalTogglesGet(cc))
	mux.HandleFunc("PUT /api/settings/global-toggles", handleGlobalTogglesUpdate(store, cc, ss, version))
	mux.HandleFunc("GET /api/settings/acme-email", handleACMEEmailGet(cc))
	mux.HandleFunc("PUT /api/settings/acme-email", handleACMEEmailUpdate(store, cc, ss, version))
	mux.HandleFunc("GET /api/settings/dns-provider", handleDNSProviderGet(cc))
	mux.HandleFunc("PUT /api/settings/dns-provider", handleDNSProviderUpdate(store, cc, ss, version))
	mux.HandleFunc("PUT /api/settings/auth", handleAuthToggle(store))
	mux.HandleFunc("GET /api/settings/api-key", handleAPIKeyStatus(store))
	mux.HandleFunc("POST /api/settings/api-key", handleAPIKeyGenerate(store))
	mux.HandleFunc("DELETE /api/settings/api-key", handleAPIKeyRevoke(store))
	mux.HandleFunc("GET /api/settings/advanced", handleAdvancedGet(store))
	mux.HandleFunc("PUT /api/settings/advanced", handleAdvancedUpdate(store))

	mux.HandleFunc("GET /api/ip-lists", handleListIPLists(store))
	mux.HandleFunc("POST /api/ip-lists", handleCreateIPList(store))
	mux.HandleFunc("GET /api/ip-lists/bindings", handleDomainIPListBindings(store))
	mux.HandleFunc("PUT /api/ip-lists/{id}", handleUpdateIPList(store, cc, ss, version))
	mux.HandleFunc("DELETE /api/ip-lists/{id}", handleDeleteIPList(store, cc, ss, version))
	mux.HandleFunc("GET /api/ip-lists/{id}/usage", handleIPListUsage(store, cc))

	mux.HandleFunc("GET /api/snapshots", handleSnapshotList(ss))
	mux.HandleFunc("POST /api/snapshots", handleSnapshotCreate(ss, cc, store, version))
	mux.HandleFunc("POST /api/snapshots/{id}/restore", handleSnapshotRestore(ss, cc, store, version))
	mux.HandleFunc("PUT /api/snapshots/{id}", handleSnapshotUpdate(ss))
	mux.HandleFunc("DELETE /api/snapshots/{id}", handleSnapshotDelete(ss))
	mux.HandleFunc("PUT /api/snapshots/settings", handleSnapshotSettings(ss))

	mux.HandleFunc("GET /api/certificates", handleCertificatesList(store, cc))
	mux.HandleFunc("POST /api/certificates/renew", handleCertificateRenew(store))
	mux.HandleFunc("DELETE /api/certificates/{issuer}/{domain}", handleCertificateDelete(store, cc))
	mux.HandleFunc("GET /api/certificates/{issuer}/{domain}/download", handleCertificateDownload(store))
	mux.HandleFunc("GET /api/settings/caddy-data-dir", handleCaddyDataDirGet(store))
	mux.HandleFunc("PUT /api/settings/caddy-data-dir", handleCaddyDataDirUpdate(store))

	mux.HandleFunc("GET /api/loki/status", handleLokiStatus(pipeline))
	mux.HandleFunc("GET /api/loki/config", handleLokiConfigGet(store))
	mux.HandleFunc("PUT /api/loki/config", handleLokiConfigUpdate(store, pipeline))
	mux.HandleFunc("POST /api/loki/test", handleLokiTest(store))

	mux.HandleFunc("GET /api/log-skip-rules/{sinkName}", handleLogSkipRulesGet(store))
	mux.HandleFunc("PUT /api/log-skip-rules/{sinkName}", handleLogSkipRulesPut(store, cc, ss, version))

	return accessLog(limitRequestBody(requireAuth(store, mux)))
}

func buildSnapshotData(cc *caddy.Client, store *config.ConfigStore, version string) (*snapshot.Data, error) {
	caddyConfig, err := cc.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("fetching caddy config: %w", err)
	}

	cfg := store.Get()
	stripped := *cfg
	stripped.StripCredentials()

	appConfigJSON, err := json.Marshal(stripped)
	if err != nil {
		return nil, fmt.Errorf("marshaling app config: %w", err)
	}

	return &snapshot.Data{
		KajiVersion: version,
		CaddyConfig: caddyConfig,
		AppConfig:   json.RawMessage(appConfigJSON),
	}, nil
}

func maybeAutoSnapshot(cc *caddy.Client, ss *snapshot.Store, store *config.ConfigStore, version, description string) {
	if !ss.IsAutoEnabled() {
		return
	}
	data, err := buildSnapshotData(cc, store, version)
	if err != nil {
		log.Printf("auto-snapshot: %v", err)
		return
	}
	name := "auto-" + time.Now().Format("2006-01-02T15:04:05")
	if _, err := ss.Create(name, description, "auto", data); err != nil {
		log.Printf("auto-snapshot: failed to create: %v", err)
	}
}

func loadSavedCaddyConfig(cc *caddy.Client, store *config.ConfigStore) {
	cfg := store.Get()
	saved, err := os.ReadFile(cfg.CaddyConfigPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("loadSavedCaddyConfig: read config: %v", err)
		}
		return
	}
	if err := cc.LoadConfig(saved); err != nil {
		log.Printf("loadSavedCaddyConfig: %v", err)
	}
}

func ensureLoggers(cc *caddy.Client) {
	if err := cc.EnsureDefaultLogger(); err != nil {
		log.Printf("ensureLoggers: default: %v", err)
	}
	if err := cc.EnsureAccessLogger(); err != nil {
		log.Printf("ensureLoggers: access: %v", err)
	}
}

func sessionMaxAge(cfg *config.AppConfig) int {
	if cfg.SessionMaxAge > 0 {
		return cfg.SessionMaxAge
	}
	return auth.DefaultSessionMaxAge
}

func hashBasicAuthPassword(ba *caddy.BasicAuth, fallbackHash string) error {
	if ba.Password != "" {
		hash, err := auth.HashPassword(ba.Password)
		if err != nil {
			return fmt.Errorf("hashing basic auth password: %w", err)
		}
		ba.PasswordHash = hash
		ba.Password = ""
	} else if ba.PasswordHash == "" {
		if fallbackHash == "" {
			return errors.New("password is required for basic auth")
		}
		ba.PasswordHash = fallbackHash
	}
	return nil
}

// validateAndHashBasicAuth performs the standard basic-auth gate used by every
// domain/subdomain/path handler: if BasicAuth is enabled, require a username,
// then hash the new password (falling back to fallbackHash when the client
// omits one so existing hashes survive updates). errPrefix is prepended to the
// missing-username message so list contexts can disambiguate. logPrefix names
// the calling handler in the 500 log line. Returns false (and writes the HTTP
// response) on any failure.
func validateAndHashBasicAuth(w http.ResponseWriter, ba *caddy.BasicAuth, fallbackHash, errPrefix, logPrefix string) bool {
	if !ba.Enabled {
		return true
	}
	if ba.Username == "" {
		writeError(w, errPrefix+"username is required for basic auth", http.StatusBadRequest)
		return false
	}
	if err := hashBasicAuthPassword(ba, fallbackHash); err != nil {
		log.Printf("%s: hash password: %v", logPrefix, err)
		writeError(w, "failed to hash password", http.StatusInternalServerError)
		return false
	}
	return true
}

var (
	persistMu    sync.Mutex
	persistTimer *time.Timer
)

const persistDelay = 500 * time.Millisecond

// persistCaddyConfig debounces config persistence. Each call resets a short
// timer so rapid sequential mutations collapse into a single persist of the
// latest state, avoiding ordering races between goroutines.
func persistCaddyConfig(cc *caddy.Client, store *config.ConfigStore) {
	persistMu.Lock()
	defer persistMu.Unlock()
	if persistTimer != nil {
		persistTimer.Stop()
	}
	persistTimer = time.AfterFunc(persistDelay, func() {
		doPersistCaddyConfig(cc, store)
	})
}

func doPersistCaddyConfig(cc *caddy.Client, store *config.ConfigStore) {
	raw, err := cc.GetConfig()
	if err != nil {
		log.Printf("persistCaddyConfig: fetch config: %v", err)
		return
	}
	cfg := store.Get()
	if err := os.MkdirAll(filepath.Dir(cfg.CaddyConfigPath), 0755); err != nil {
		log.Printf("persistCaddyConfig: create directory: %v", err)
		return
	}
	tmp := cfg.CaddyConfigPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		log.Printf("persistCaddyConfig: write temp file: %v", err)
		return
	}
	if err := os.Rename(tmp, cfg.CaddyConfigPath); err != nil {
		os.Remove(tmp)
		log.Printf("persistCaddyConfig: rename: %v", err)
	}
}

// parseIntParam reads an integer query parameter, clamped to [min, max].
// Returns (0, true) if absent, (n, true) on success, or (0, false) if invalid
// (error already written to w). Use max < 0 to skip upper clamping.
func parseIntParam(w http.ResponseWriter, q url.Values, name string, min, max int) (int, bool) {
	v := q.Get(name)
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		writeError(w, "invalid "+name+" parameter", http.StatusBadRequest)
		return 0, false
	}
	if n < min {
		n = min
	}
	if max >= 0 && n > max {
		n = max
	}
	return n, true
}

func caddyError(w http.ResponseWriter, handler string, err error) {
	log.Printf("%s: %v", handler, err)
	var msg string
	switch {
	case errors.Is(err, caddy.ErrConnectionRefused):
		msg = "Caddy admin API refused the connection - is Caddy running?"
	case errors.Is(err, caddy.ErrTimeout):
		msg = "Caddy admin API didn't respond in time. The change may not have applied - refresh to verify."
	case errors.Is(err, caddy.ErrConnectionReset):
		msg = "Connection to Caddy was reset. The change may not have applied - refresh to verify."
	case errors.Is(err, caddy.ErrTransport):
		msg = "Couldn't reach Caddy admin API. Check the server logs for details."
	default:
		msg = extractCaddyMessage(err.Error())
	}
	writeError(w, msg, http.StatusBadGateway)
}

// extractCaddyMessage pulls the human-readable error out of Caddy's JSON
// response embedded in our error string. Caddy returns {"error":"..."} and
// we wrap that as "caddy rejected ... (status N): {json}".
func extractCaddyMessage(errStr string) string {
	const fallback = "caddy returned an error - check server logs for details"
	idx := strings.Index(errStr, "{")
	if idx < 0 {
		return fallback
	}
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(errStr[idx:]), &parsed) != nil || parsed.Error == "" {
		return fallback
	}
	// Strip the verbose "loading new config:" prefix chain that Caddy nests
	msg := parsed.Error
	for _, prefix := range []string{"loading new config: ", "loading config: "} {
		msg = strings.TrimPrefix(msg, prefix)
	}
	msg = stripGoStructs(msg)
	return msg
}

// goStructRe matches Go struct literals like &logging.FileWriter{...} that
// Caddy sometimes dumps into error messages. These are noise for end users.
var goStructRe = regexp.MustCompile(`\s*(?:using\s+)?&\w+(?:\.\w+)*\{[^}]*\}`)

func stripGoStructs(msg string) string {
	cleaned := goStructRe.ReplaceAllString(msg, "")
	cleaned = strings.ReplaceAll(cleaned, ": : ", ": ")
	return strings.TrimSpace(cleaned)
}

func setAPIHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
}

func writeError(w http.ResponseWriter, msg string, code int) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]string{"error": msg}); err != nil {
		log.Printf("writeError: failed to encode error response: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setAPIHeaders(w)
	w.WriteHeader(code)
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("writeError: write error: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		log.Printf("writeJSON: failed to encode response: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setAPIHeaders(w)
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("writeJSON: write error: %v", err)
	}
}

func writeRawJSON(w http.ResponseWriter, raw []byte) {
	setAPIHeaders(w)
	if _, err := w.Write(raw); err != nil {
		log.Printf("writeRawJSON: write error: %v", err)
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}
