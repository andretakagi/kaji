package api

import (
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
// match_value is non-empty, and delegates rule validation to validateRule
// with allowNone=false (paths must dispatch traffic somewhere). Hashes any
// toggle override password, falling back to fallbackHash so update callers
// can preserve the existing hash when the client sends the request without
// retyping the password. errPrefix is prepended to messages so callers can
// disambiguate which path in a list is invalid.
func validatePathRequest(w http.ResponseWriter, p *pathRequest, fallbackHash, errPrefix string) bool {
	if msg := validatePathMatch(p.PathMatch); msg != "" {
		writeError(w, errPrefix+msg, http.StatusBadRequest)
		return false
	}
	if strings.TrimSpace(p.MatchValue) == "" {
		writeError(w, errPrefix+"match_value is required", http.StatusBadRequest)
		return false
	}
	if !validateRule(w, p.Rule, false) {
		return false
	}
	if p.ToggleOverrides != nil {
		if !validateAndHashBasicAuth(w, &p.ToggleOverrides.BasicAuth, fallbackHash, errPrefix, "validatePathRequest") {
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
			Enabled:         true,
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
	existing.Rule.HandlerType = req.Rule.HandlerType
	existing.Rule.HandlerConfig = req.Rule.HandlerConfig
	existing.Rule.AdvancedHeaders = req.Rule.AdvancedHeaders
	existing.ToggleOverrides = req.ToggleOverrides
}

// pathScope identifies the path container a request targets. An empty subID
// means a domain-level path; otherwise the path lives under the named
// subdomain.
type pathScope struct {
	domainID string
	subID    string
}

// pathScopeFromRequest extracts a pathScope from the URL path values. If
// includeSub is true, the route was registered with a {subId} placeholder.
func pathScopeFromRequest(r *http.Request, includeSub bool) pathScope {
	scope := pathScope{domainID: r.PathValue("id")}
	if includeSub {
		scope.subID = r.PathValue("subId")
	}
	return scope
}

// resolvePathContainer returns a pointer to the slice that owns the paths in
// the given scope, plus the parent domain (always) and parent subdomain (only
// for subdomain-scoped paths). Returns nil pointers when the parents are
// missing - callers should treat that as 404.
func resolvePathContainer(c *config.AppConfig, scope pathScope) (paths *[]config.Path, dom *config.Domain, sub *config.Subdomain) {
	dom = findDomain(c, scope.domainID)
	if dom == nil {
		return nil, nil, nil
	}
	if scope.subID == "" {
		return &dom.Paths, dom, nil
	}
	sub = findSubdomain(dom, scope.subID)
	if sub == nil {
		return nil, dom, nil
	}
	return &sub.Paths, dom, sub
}

// pathSnapshotLabel produces a consistent, human-readable description of a
// path mutation for the snapshot log: e.g. "Path created: example.com prefix
// /api/*" or "Subdomain path updated: api.example.com regex ^/v1/.*$".
func pathSnapshotLabel(action string, dom *config.Domain, sub *config.Subdomain, p *config.Path) string {
	host := ""
	if dom != nil {
		host = dom.Name
		if sub != nil {
			host = sub.Name + "." + dom.Name
		}
	}
	prefix := "Path"
	if sub != nil {
		prefix = "Subdomain path"
	}
	parts := []string{prefix, action + ":"}
	if host != "" {
		parts = append(parts, host)
	}
	if p != nil {
		if p.PathMatch != "" {
			parts = append(parts, p.PathMatch)
		}
		if p.MatchValue != "" {
			parts = append(parts, p.MatchValue)
		}
	}
	return strings.Join(parts, " ")
}

// returnDomainOr writes the refreshed domain JSON (so the frontend can replace
// its local copy) or, in the unlikely case the domain has vanished between
// mutation and read, falls back to the supplied alt payload.
func returnDomainOr(w http.ResponseWriter, store *config.ConfigStore, domainID string, alt any) {
	if dom := findDomain(store.Get(), domainID); dom != nil {
		writeJSON(w, dom)
		return
	}
	writeJSON(w, alt)
}

// createPath is the shared body for handleCreate{Domain,Subdomain}Path.
func createPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, scopeOf func(*http.Request) pathScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := scopeOf(r)
		var req pathRequest
		if !decodeBody(w, r, &req) {
			return
		}
		if !validatePathRequest(w, &req, "", "") {
			return
		}

		_, dom, sub := resolvePathContainer(store.Get(), scope)
		if dom == nil {
			writeError(w, "domain not found", http.StatusNotFound)
			return
		}
		if scope.subID != "" && sub == nil {
			writeError(w, "subdomain not found", http.StatusNotFound)
			return
		}

		path := pathFromRequest(req)

		maybeAutoSnapshot(cc, ss, store, version, pathSnapshotLabel("created", dom, sub, &path))

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			paths, _, _ := resolvePathContainer(&c, scope)
			if paths == nil {
				return nil, errMutationNotFound
			}
			*paths = append(*paths, path)
			return &c, nil
		})
		if writeMutateError(w, "createPath", err, "path container not found", "failed to save path") {
			return
		}

		persistCaddyConfig(cc, store)
		returnDomainOr(w, store, scope.domainID, path)
	}
}

// updatePath is the shared body for handleUpdate{Domain,Subdomain}Path.
func updatePath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, scopeOf func(*http.Request) pathScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := scopeOf(r)
		pathID := r.PathValue("pathId")
		var req pathRequest
		if !decodeBody(w, r, &req) {
			return
		}

		var fallbackHash string
		paths, dom, sub := resolvePathContainer(store.Get(), scope)
		if paths != nil {
			fallbackHash = existingPathHash(*paths, pathID)
		}
		if !validatePathRequest(w, &req, fallbackHash, "") {
			return
		}

		var labelPath *config.Path
		if paths != nil {
			if idx := findPath(*paths, pathID); idx >= 0 {
				labelCopy := (*paths)[idx]
				labelPath = &labelCopy
				labelPath.PathMatch = req.PathMatch
				labelPath.MatchValue = req.MatchValue
			}
		}
		maybeAutoSnapshot(cc, ss, store, version, pathSnapshotLabel("updated", dom, sub, labelPath))

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			ps, _, _ := resolvePathContainer(&c, scope)
			if ps == nil {
				return nil, errMutationNotFound
			}
			idx := findPath(*ps, pathID)
			if idx < 0 {
				return nil, errMutationNotFound
			}
			applyPathUpdate(&(*ps)[idx], req)
			return &c, nil
		})
		if writeMutateError(w, "updatePath", err, "path not found", "failed to update path") {
			return
		}

		persistCaddyConfig(cc, store)
		returnDomainOr(w, store, scope.domainID, map[string]string{"status": "ok"})
	}
}

// deletePath is the shared body for handleDelete{Domain,Subdomain}Path.
func deletePath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, scopeOf func(*http.Request) pathScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := scopeOf(r)
		pathID := r.PathValue("pathId")

		paths, dom, sub := resolvePathContainer(store.Get(), scope)
		var labelPath *config.Path
		if paths != nil {
			if idx := findPath(*paths, pathID); idx >= 0 {
				p := (*paths)[idx]
				labelPath = &p
			}
		}
		maybeAutoSnapshot(cc, ss, store, version, pathSnapshotLabel("deleted", dom, sub, labelPath))

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			ps, _, _ := resolvePathContainer(&c, scope)
			if ps == nil {
				return nil, errMutationNotFound
			}
			fresh := make([]config.Path, 0, len(*ps))
			found := false
			for _, p := range *ps {
				if p.ID == pathID {
					found = true
					continue
				}
				fresh = append(fresh, p)
			}
			if !found {
				return nil, errMutationNotFound
			}
			*ps = fresh
			return &c, nil
		})
		if writeMutateError(w, "deletePath", err, "path not found", "failed to delete path") {
			return
		}

		persistCaddyConfig(cc, store)
		returnDomainOr(w, store, scope.domainID, map[string]string{"status": "ok"})
	}
}

// togglePath is the shared body for handle{Enable,Disable}{Domain,Subdomain}Path.
func togglePath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, scopeOf func(*http.Request) pathScope, enabled bool) http.HandlerFunc {
	action := "enabled"
	if !enabled {
		action = "disabled"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		scope := scopeOf(r)
		pathID := r.PathValue("pathId")

		paths, dom, sub := resolvePathContainer(store.Get(), scope)
		var labelPath *config.Path
		if paths != nil {
			if idx := findPath(*paths, pathID); idx >= 0 {
				p := (*paths)[idx]
				labelPath = &p
			}
		}
		maybeAutoSnapshot(cc, ss, store, version, pathSnapshotLabel(action, dom, sub, labelPath))

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			ps, _, _ := resolvePathContainer(&c, scope)
			if ps == nil {
				return nil, errMutationNotFound
			}
			idx := findPath(*ps, pathID)
			if idx < 0 {
				return nil, errMutationNotFound
			}
			(*ps)[idx].Enabled = enabled
			return &c, nil
		})
		if writeMutateError(w, "togglePath", err, "path not found", "failed to update path") {
			return
		}

		persistCaddyConfig(cc, store)
		returnDomainOr(w, store, scope.domainID, map[string]string{"status": "ok"})
	}
}

// --- Domain paths ---

func handleCreateDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return createPath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, false) })
}

func handleUpdateDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return updatePath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, false) })
}

func handleDeleteDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return deletePath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, false) })
}

func handleEnableDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return togglePath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, false) }, true)
}

func handleDisableDomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return togglePath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, false) }, false)
}

// --- Subdomain paths ---

func handleCreateSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return createPath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, true) })
}

func handleUpdateSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return updatePath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, true) })
}

func handleDeleteSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return deletePath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, true) })
}

func handleEnableSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return togglePath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, true) }, true)
}

func handleDisableSubdomainPath(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return togglePath(store, cc, ss, version, func(r *http.Request) pathScope { return pathScopeFromRequest(r, true) }, false)
}
