package api

import (
	"log"
	"net/http"
	"strings"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

// pathRequest is the body shape for creating or updating a Path. The nested
// rule cannot be type "none" - paths exist to dispatch traffic somewhere.
type pathRequest struct {
	Label           string               `json:"label,omitempty"`
	PathMatch       string               `json:"path_match"`
	MatchValue      string               `json:"match_value"`
	Rule            updateRuleRequest    `json:"rule"`
	ToggleOverrides *caddy.DomainToggles `json:"toggle_overrides,omitempty"`
}

// validatePathRequest checks that path_match is exact|prefix|regex,
// match_value is non-empty, the nested rule has a non-"none" handler type,
// and any rule handler config is valid. Hashes any toggle override password,
// falling back to fallbackHash so update callers can preserve the existing
// hash when the client sends the request without retyping the password.
// errPrefix is prepended to messages so callers can disambiguate which path
// in a list is invalid.
func validatePathRequest(w http.ResponseWriter, p *pathRequest, fallbackHash, errPrefix string) bool {
	if msg := validatePathMatch(p.PathMatch); msg != "" {
		writeError(w, errPrefix+msg, http.StatusBadRequest)
		return false
	}
	if strings.TrimSpace(p.MatchValue) == "" {
		writeError(w, errPrefix+"match_value is required", http.StatusBadRequest)
		return false
	}
	if p.Rule.HandlerType == "none" {
		writeError(w, errPrefix+"rule handler_type cannot be none", http.StatusBadRequest)
		return false
	}
	if !validateRuleHandler(w, p.Rule.HandlerType, p.Rule.HandlerConfig) {
		return false
	}
	if p.ToggleOverrides != nil && p.ToggleOverrides.BasicAuth.Enabled {
		if p.ToggleOverrides.BasicAuth.Username == "" {
			writeError(w, errPrefix+"username is required for basic auth", http.StatusBadRequest)
			return false
		}
		if err := hashBasicAuthPassword(&p.ToggleOverrides.BasicAuth, fallbackHash); err != nil {
			log.Printf("validatePathRequest: hash password: %v", err)
			writeError(w, "failed to hash password", http.StatusInternalServerError)
			return false
		}
	}
	return true
}

// existingPathHash returns the stored basic-auth password hash for the path
// with the given ID, or "" if the path or its toggle override is absent.
func existingPathHash(paths []config.Path, pathID string) string {
	idx := findPath(paths, pathID)
	if idx < 0 {
		return ""
	}
	if paths[idx].ToggleOverrides == nil {
		return ""
	}
	return paths[idx].ToggleOverrides.BasicAuth.PasswordHash
}

func pathFromRequest(p pathRequest) config.Path {
	return config.Path{
		ID:         caddy.GeneratePathID(),
		Label:      p.Label,
		Enabled:    true,
		PathMatch:  p.PathMatch,
		MatchValue: p.MatchValue,
		Rule: config.Rule{
			HandlerType:     p.Rule.HandlerType,
			HandlerConfig:   p.Rule.HandlerConfig,
			AdvancedHeaders: p.Rule.AdvancedHeaders,
		},
		ToggleOverrides: p.ToggleOverrides,
	}
}

// findPath locates a path by ID inside the given list. Returns the index or -1.
func findPath(paths []config.Path, pathID string) int {
	for i := range paths {
		if paths[i].ID == pathID {
			return i
		}
	}
	return -1
}

// applyPathUpdate copies fields from req into existing, preserving the ID and
// enabled state.
func applyPathUpdate(existing *config.Path, req pathRequest) {
	existing.Label = req.Label
	existing.PathMatch = req.PathMatch
	existing.MatchValue = req.MatchValue
	existing.Rule = config.Rule{
		HandlerType:     req.Rule.HandlerType,
		HandlerConfig:   req.Rule.HandlerConfig,
		AdvancedHeaders: req.Rule.AdvancedHeaders,
	}
	existing.ToggleOverrides = req.ToggleOverrides
}

// --- Domain paths ---

func handleCreateDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		var req pathRequest
		if !decodeBody(w, r, &req) {
			return
		}
		if !validatePathRequest(w, &req, "", "") {
			return
		}

		cfg := store.Get()
		if findDomain(cfg, domainID) == nil {
			writeError(w, "domain not found", http.StatusNotFound)
			return
		}

		path := pathFromRequest(req)

		maybeAutoSnapshot(cc, ss, store, version, "Path created: "+path.ID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, errMutationNotFound
			}
			dom.Paths = append(dom.Paths, path)
			return &c, nil
		})
		if writeMutateError(w, "handleCreateDomainPath", err, "domain not found", "failed to save path") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg = store.Get()
		if dom := findDomain(cfg, domainID); dom != nil {
			writeJSON(w, dom)
			return
		}
		writeJSON(w, path)
	}
}

func handleUpdateDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		pathID := r.PathValue("pathId")
		var req pathRequest
		if !decodeBody(w, r, &req) {
			return
		}

		var fallbackHash string
		if dom := findDomain(store.Get(), domainID); dom != nil {
			fallbackHash = existingPathHash(dom.Paths, pathID)
		}
		if !validatePathRequest(w, &req, fallbackHash, "") {
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Path updated: "+pathID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, errMutationNotFound
			}
			idx := findPath(dom.Paths, pathID)
			if idx < 0 {
				return nil, errMutationNotFound
			}
			applyPathUpdate(&dom.Paths[idx], req)
			return &c, nil
		})
		if writeMutateError(w, "handleUpdateDomainPath", err, "path not found", "failed to update path") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
		if dom := findDomain(cfg, domainID); dom != nil {
			writeJSON(w, dom)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleDeleteDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		pathID := r.PathValue("pathId")

		maybeAutoSnapshot(cc, ss, store, version, "Path deleted: "+pathID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, errMutationNotFound
			}
			fresh := make([]config.Path, 0, len(dom.Paths))
			found := false
			for _, p := range dom.Paths {
				if p.ID == pathID {
					found = true
					continue
				}
				fresh = append(fresh, p)
			}
			if !found {
				return nil, errMutationNotFound
			}
			dom.Paths = fresh
			return &c, nil
		})
		if writeMutateError(w, "handleDeleteDomainPath", err, "path not found", "failed to delete path") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
		if dom := findDomain(cfg, domainID); dom != nil {
			writeJSON(w, dom)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleEnableDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return domainPathToggleHandler(store, cc, ss, version, true)
}

func handleDisableDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return domainPathToggleHandler(store, cc, ss, version, false)
}

func domainPathToggleHandler(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		pathID := r.PathValue("pathId")

		action := "enabled"
		if !enabled {
			action = "disabled"
		}

		maybeAutoSnapshot(cc, ss, store, version, "Path "+action+": "+pathID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, errMutationNotFound
			}
			idx := findPath(dom.Paths, pathID)
			if idx < 0 {
				return nil, errMutationNotFound
			}
			dom.Paths[idx].Enabled = enabled
			return &c, nil
		})
		if writeMutateError(w, "handleToggleDomainPath", err, "path not found", "failed to update path") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
		if dom := findDomain(cfg, domainID); dom != nil {
			writeJSON(w, dom)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

// --- Subdomain paths ---

func handleCreateSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
		var req pathRequest
		if !decodeBody(w, r, &req) {
			return
		}
		if !validatePathRequest(w, &req, "", "") {
			return
		}

		cfg := store.Get()
		dom := findDomain(cfg, domainID)
		if dom == nil {
			writeError(w, "domain not found", http.StatusNotFound)
			return
		}
		if findSubdomain(dom, subID) == nil {
			writeError(w, "subdomain not found", http.StatusNotFound)
			return
		}

		path := pathFromRequest(req)

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain path created: "+path.ID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, errMutationNotFound
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, errMutationNotFound
			}
			s.Paths = append(s.Paths, path)
			return &c, nil
		})
		if writeMutateError(w, "handleCreateSubdomainPath", err, "subdomain not found", "failed to save path") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg = store.Get()
		if d := findDomain(cfg, domainID); d != nil {
			writeJSON(w, d)
			return
		}
		writeJSON(w, path)
	}
}

func handleUpdateSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
		pathID := r.PathValue("pathId")
		var req pathRequest
		if !decodeBody(w, r, &req) {
			return
		}

		var fallbackHash string
		if dom := findDomain(store.Get(), domainID); dom != nil {
			if sub := findSubdomain(dom, subID); sub != nil {
				fallbackHash = existingPathHash(sub.Paths, pathID)
			}
		}
		if !validatePathRequest(w, &req, fallbackHash, "") {
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain path updated: "+pathID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, errMutationNotFound
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, errMutationNotFound
			}
			idx := findPath(s.Paths, pathID)
			if idx < 0 {
				return nil, errMutationNotFound
			}
			applyPathUpdate(&s.Paths[idx], req)
			return &c, nil
		})
		if writeMutateError(w, "handleUpdateSubdomainPath", err, "path not found", "failed to update path") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
		if d := findDomain(cfg, domainID); d != nil {
			writeJSON(w, d)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleDeleteSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
		pathID := r.PathValue("pathId")

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain path deleted: "+pathID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, errMutationNotFound
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, errMutationNotFound
			}
			fresh := make([]config.Path, 0, len(s.Paths))
			found := false
			for _, p := range s.Paths {
				if p.ID == pathID {
					found = true
					continue
				}
				fresh = append(fresh, p)
			}
			if !found {
				return nil, errMutationNotFound
			}
			s.Paths = fresh
			return &c, nil
		})
		if writeMutateError(w, "handleDeleteSubdomainPath", err, "path not found", "failed to delete path") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
		if d := findDomain(cfg, domainID); d != nil {
			writeJSON(w, d)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleEnableSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return subdomainPathToggleHandler(store, cc, ss, version, true)
}

func handleDisableSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return subdomainPathToggleHandler(store, cc, ss, version, false)
}

func subdomainPathToggleHandler(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
		pathID := r.PathValue("pathId")

		action := "enabled"
		if !enabled {
			action = "disabled"
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain path "+action+": "+pathID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, errMutationNotFound
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, errMutationNotFound
			}
			idx := findPath(s.Paths, pathID)
			if idx < 0 {
				return nil, errMutationNotFound
			}
			s.Paths[idx].Enabled = enabled
			return &c, nil
		})
		if writeMutateError(w, "handleToggleSubdomainPath", err, "path not found", "failed to update path") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
		if d := findDomain(cfg, domainID); d != nil {
			writeJSON(w, d)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}
