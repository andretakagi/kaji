package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func findSubdomain(dom *config.Domain, subID string) *config.Subdomain {
	for i := range dom.Subdomains {
		if dom.Subdomains[i].ID == subID {
			return &dom.Subdomains[i]
		}
	}
	return nil
}

// subdomainHost renders "<subdomain>.<domain>" for snapshot descriptions, or
// the subdomain ID alone if the parents have already been removed.
func subdomainHost(cfg *config.AppConfig, domainID, subID string) string {
	if dom := findDomain(cfg, domainID); dom != nil {
		if sub := findSubdomain(dom, subID); sub != nil {
			return sub.Name + "." + dom.Name
		}
	}
	return subID
}

type subdomainRequest struct {
	Name    string               `json:"name"`
	Toggles *caddy.DomainToggles `json:"toggles"`
	Rule    updateRuleRequest    `json:"rule"`
	Paths   []pathRequest        `json:"paths"`
}

func handleCreateSubdomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		var req subdomainRequest
		if !decodeBody(w, r, &req) {
			return
		}

		name := strings.TrimSpace(req.Name)
		if msg := validateSubdomainName(name); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		if !validateRule(w, req.Rule, true) {
			return
		}

		cfg := store.Get()
		dom := findDomain(cfg, domainID)
		if dom == nil {
			writeError(w, "domain not found", http.StatusNotFound)
			return
		}

		for _, s := range dom.Subdomains {
			if strings.EqualFold(s.Name, name) {
				writeError(w, "a subdomain with this name already exists", http.StatusConflict)
				return
			}
		}

		toggles := dom.Toggles
		if req.Toggles != nil {
			toggles = *req.Toggles
		}

		if !validateAndHashBasicAuth(w, &toggles.BasicAuth, "", "", "handleCreateSubdomain") {
			return
		}

		paths := make([]config.Path, len(req.Paths))
		for i, p := range req.Paths {
			if !validatePathRequest(w, &p, "", fmt.Sprintf("path %d: ", i+1)) {
				return
			}
			paths[i] = pathFromRequest(p)
		}

		sub := config.Subdomain{
			ID:      caddy.GenerateSubdomainID(),
			Name:    name,
			Enabled: true,
			Toggles: toggles,
			Rule: config.Rule{
				HandlerType:     req.Rule.HandlerType,
				HandlerConfig:   req.Rule.HandlerConfig,
				AdvancedHeaders: req.Rule.AdvancedHeaders,
				Enabled:         true,
			},
			Paths: paths,
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain created: "+name+"."+dom.Name)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, errMutationNotFound
			}
			d.Subdomains = append(d.Subdomains, sub)
			return &c, nil
		})
		if writeMutateError(w, "handleCreateSubdomain", err, "domain not found", "failed to save subdomain") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg = store.Get()
		if d := findDomain(cfg, domainID); d != nil {
			writeJSON(w, d)
			return
		}
		writeJSON(w, sub)
	}
}

func handleUpdateSubdomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
		var req struct {
			Name    string              `json:"name"`
			Enabled *bool               `json:"enabled"`
			Toggles caddy.DomainToggles `json:"toggles"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		name := strings.TrimSpace(req.Name)
		if msg := validateSubdomainName(name); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		var fallbackHash string
		if d := findDomain(store.Get(), domainID); d != nil {
			if s := findSubdomain(d, subID); s != nil {
				fallbackHash = s.Toggles.BasicAuth.PasswordHash
			}
		}
		if !validateAndHashBasicAuth(w, &req.Toggles.BasicAuth, fallbackHash, "", "handleUpdateSubdomain") {
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain updated: "+subdomainHost(store.Get(), domainID, subID))

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, errMutationNotFound
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, errMutationNotFound
			}
			for _, other := range d.Subdomains {
				if other.ID != subID && strings.EqualFold(other.Name, name) {
					return nil, conflictErr("a subdomain with this name already exists")
				}
			}
			s.Name = name
			s.Toggles = req.Toggles
			if req.Enabled != nil {
				s.Enabled = *req.Enabled
			}
			return &c, nil
		})
		if writeMutateError(w, "handleUpdateSubdomain", err, "subdomain not found", "failed to update subdomain") {
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

func handleDeleteSubdomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain deleted: "+subdomainHost(store.Get(), domainID, subID))

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, errMutationNotFound
			}
			fresh := make([]config.Subdomain, 0, len(d.Subdomains))
			found := false
			for _, s := range d.Subdomains {
				if s.ID == subID {
					found = true
					continue
				}
				fresh = append(fresh, s)
			}
			if !found {
				return nil, errMutationNotFound
			}
			d.Subdomains = fresh
			return &c, nil
		})
		if writeMutateError(w, "handleDeleteSubdomain", err, "subdomain not found", "failed to delete subdomain") {
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

func handleEnableSubdomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return subdomainToggleHandler(store, cc, ss, version, true)
}

func handleDisableSubdomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return subdomainToggleHandler(store, cc, ss, version, false)
}

func subdomainToggleHandler(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")

		action := "enabled"
		if !enabled {
			action = "disabled"
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain "+action+": "+subdomainHost(store.Get(), domainID, subID))

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, errMutationNotFound
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, errMutationNotFound
			}
			s.Enabled = enabled
			return &c, nil
		})
		if writeMutateError(w, "handleToggleSubdomain", err, "subdomain not found", "failed to update subdomain") {
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

func handleUpdateSubdomainRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
		var req updateRuleRequest
		if !decodeBody(w, r, &req) {
			return
		}
		if !validateRule(w, req, true) {
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain rule updated: "+subdomainHost(store.Get(), domainID, subID))

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, errMutationNotFound
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, errMutationNotFound
			}
			s.Rule.HandlerType = req.HandlerType
			s.Rule.HandlerConfig = req.HandlerConfig
			s.Rule.AdvancedHeaders = req.AdvancedHeaders
			return &c, nil
		})
		if writeMutateError(w, "handleUpdateSubdomainRule", err, "subdomain not found", "failed to update rule") {
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
