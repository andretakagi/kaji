package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/export"
	"github.com/andretakagi/kaji/internal/snapshot"
)

// mutationMu serializes mutateAndSync calls so the snapshot taken before a
// mutation can't be invalidated by a concurrent successful mutation in another
// handler. Kaji is single-user; serializing all mutations is acceptable and
// makes rollback safe without restructuring ConfigStore.
var mutationMu sync.Mutex

func ipResolver(store *config.ConfigStore) func(string) ([]string, string, error) {
	return func(listID string) ([]string, string, error) {
		cfg := store.Get()
		entries := config.IPListsToEntries(cfg.IPLists)
		ips, err := caddy.ResolveIPList(listID, entries)
		if err != nil {
			return nil, "", err
		}
		for _, l := range cfg.IPLists {
			if l.ID == listID {
				return ips, l.Type, nil
			}
		}
		return nil, "", fmt.Errorf("IP list %s not found", listID)
	}
}

func syncAfterMutation(cc *caddy.Client, store *config.ConfigStore) error {
	cfg := store.Get()
	syncDomains := export.ToSyncDomains(cfg.Domains)
	skipRules := export.ToSyncSkipRules(cfg.LogSkipRules)
	_, err := caddy.SyncDomains(cc, syncDomains, ipResolver(store), skipRules)
	return err
}

// errSyncFailed wraps a Caddy sync error returned from mutateAndSync, so
// callers can tell it apart from a local-store error and route it through
// caddyError for a user-friendly message.
type errSyncFailed struct{ err error }

func (e *errSyncFailed) Error() string { return e.err.Error() }
func (e *errSyncFailed) Unwrap() error { return e.err }

// errMutationNotFound is returned from the mutate function passed to
// mutateAndSync when the target object (domain, subdomain, rule, etc.)
// doesn't exist. Callers use writeMutateError to translate this into a 404.
var errMutationNotFound = errors.New("not found")

// mutationConflict is returned from a mutate function when the request would
// violate a uniqueness invariant (e.g. duplicate subdomain name). Translated
// to a 409 by writeMutateError using msg as the user-facing message.
type mutationConflict struct{ msg string }

func (e *mutationConflict) Error() string { return e.msg }

func conflictErr(msg string) error { return &mutationConflict{msg: msg} }

// writeMutateError translates a mutateAndSync error into an HTTP response.
// Returns true if an error was written (so callers can return early). The
// default message is used for unexpected local-store errors. Sync errors are
// routed through caddyError; sentinel errors map to 404 / 409 with the
// supplied user-facing messages.
func writeMutateError(w http.ResponseWriter, handler string, err error, notFoundMsg, defaultMsg string) bool {
	if err == nil {
		return false
	}
	var syncErr *errSyncFailed
	if errors.As(err, &syncErr) {
		caddyError(w, handler, syncErr.err)
		return true
	}
	if errors.Is(err, errMutationNotFound) {
		writeError(w, notFoundMsg, http.StatusNotFound)
		return true
	}
	var conflict *mutationConflict
	if errors.As(err, &conflict) {
		writeError(w, conflict.msg, http.StatusConflict)
		return true
	}
	log.Printf("%s: %v", handler, err)
	writeError(w, defaultMsg, http.StatusInternalServerError)
	return true
}

// mutateAndSync applies mutate to the local config, then syncs the result to
// Caddy. If the Caddy sync fails for any reason, the local config is rolled
// back to the pre-mutation snapshot so Kaji's UI doesn't show state that
// disagrees with what's actually live in Caddy.
//
// On sync failure the returned error is wrapped in *errSyncFailed - callers
// should check for it with errors.As and pass the unwrapped error to
// caddyError. Errors from mutate or the local store update are returned
// unwrapped.
//
// The mutationMu lock prevents another mutation from interleaving between
// our store.Update and our rollback. If mutate itself returns an error, the
// store was never touched, so no rollback is needed.
func mutateAndSync(store *config.ConfigStore, cc *caddy.Client, mutate func(c config.AppConfig) (*config.AppConfig, error)) error {
	mutationMu.Lock()
	defer mutationMu.Unlock()

	prev := store.Get()

	if err := store.Update(mutate); err != nil {
		return err
	}

	if err := syncAfterMutation(cc, store); err != nil {
		if rbErr := store.Update(func(_ config.AppConfig) (*config.AppConfig, error) {
			restored := *prev
			return &restored, nil
		}); rbErr != nil {
			log.Printf("mutateAndSync: rollback failed after sync error %v: %v", err, rbErr)
		}
		return &errSyncFailed{err: err}
	}

	return nil
}

func findDomain(cfg *config.AppConfig, id string) *config.Domain {
	for i := range cfg.Domains {
		if cfg.Domains[i].ID == id {
			return &cfg.Domains[i]
		}
	}
	return nil
}

// updateRuleRequest is the shared body shape for setting a Rule on a domain
// or subdomain. Used by handleUpdateDomainRule, handleUpdateSubdomainRule, and
// nested inside path requests.
type updateRuleRequest struct {
	HandlerType     string          `json:"handler_type"`
	HandlerConfig   json.RawMessage `json:"handler_config"`
	AdvancedHeaders bool            `json:"advanced_headers"`
}

func handleListDomains(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		domains := cfg.Domains
		if domains == nil {
			domains = []config.Domain{}
		}
		writeJSON(w, domains)
	}
}

func handleGetDomain(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		cfg := store.Get()
		dom := findDomain(cfg, id)
		if dom == nil {
			writeError(w, "domain not found", http.StatusNotFound)
			return
		}
		writeJSON(w, dom)
	}
}

func handleCreateDomainFull(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name       string              `json:"name"`
			Toggles    caddy.DomainToggles `json:"toggles"`
			Rule       updateRuleRequest   `json:"rule"`
			Subdomains []subdomainRequest  `json:"subdomains"`
			Paths      []pathRequest       `json:"paths"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if msg := validateDomain(req.Name); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		cfg := store.Get()
		for _, d := range cfg.Domains {
			if strings.EqualFold(d.Name, req.Name) {
				writeError(w, "a domain with this name already exists", http.StatusConflict)
				return
			}
		}

		if !validateAndHashBasicAuth(w, &req.Toggles.BasicAuth, "", "", "handleCreateDomainFull") {
			return
		}

		if !validateRule(w, req.Rule, true) {
			return
		}

		paths := make([]config.Path, len(req.Paths))
		for i, p := range req.Paths {
			if !validatePathRequest(w, &p, "", fmt.Sprintf("path %d: ", i+1)) {
				return
			}
			paths[i] = pathFromRequest(p)
		}

		subdomains := make([]config.Subdomain, len(req.Subdomains))
		seenSub := make(map[string]struct{}, len(req.Subdomains))
		for i, s := range req.Subdomains {
			subName := strings.TrimSpace(s.Name)
			if msg := validateSubdomainName(subName); msg != "" {
				writeError(w, fmt.Sprintf("subdomain %d: %s", i+1, msg), http.StatusBadRequest)
				return
			}
			lower := strings.ToLower(subName)
			if _, dup := seenSub[lower]; dup {
				writeError(w, fmt.Sprintf("subdomain %d: a subdomain with this name already exists", i+1), http.StatusConflict)
				return
			}
			seenSub[lower] = struct{}{}

			if !validateRule(w, s.Rule, true) {
				return
			}

			subToggles := req.Toggles
			if s.Toggles != nil {
				subToggles = *s.Toggles
			}
			if !validateAndHashBasicAuth(w, &subToggles.BasicAuth, "", fmt.Sprintf("subdomain %d: ", i+1), "handleCreateDomainFull") {
				return
			}

			subPaths := make([]config.Path, len(s.Paths))
			for j, p := range s.Paths {
				if !validatePathRequest(w, &p, "", fmt.Sprintf("subdomain %d path %d: ", i+1, j+1)) {
					return
				}
				subPaths[j] = pathFromRequest(p)
			}

			subdomains[i] = config.Subdomain{
				ID:      caddy.GenerateSubdomainID(),
				Name:    subName,
				Enabled: true,
				Toggles: subToggles,
				Rule: config.Rule{
					HandlerType:     s.Rule.HandlerType,
					HandlerConfig:   s.Rule.HandlerConfig,
					AdvancedHeaders: s.Rule.AdvancedHeaders,
					Enabled:         true,
				},
				Paths: subPaths,
			}
		}

		domain := config.Domain{
			ID:      caddy.GenerateDomainID(),
			Name:    req.Name,
			Enabled: true,
			Toggles: req.Toggles,
			Rule: config.Rule{
				HandlerType:     req.Rule.HandlerType,
				HandlerConfig:   req.Rule.HandlerConfig,
				AdvancedHeaders: req.Rule.AdvancedHeaders,
				Enabled:         true,
			},
			Subdomains: subdomains,
			Paths:      paths,
		}

		maybeAutoSnapshot(cc, ss, store, version, "Domain created: "+req.Name)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			c.Domains = append(c.Domains, domain)
			return &c, nil
		})
		if writeMutateError(w, "handleCreateDomainFull", err, "", "failed to save domain") {
			return
		}

		persistCaddyConfig(cc, store)
		writeJSON(w, domain)
	}
}

func handleUpdateDomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			Name    string              `json:"name"`
			Toggles caddy.DomainToggles `json:"toggles"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if msg := validateDomain(req.Name); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		var fallbackHash string
		if dom := findDomain(store.Get(), id); dom != nil {
			fallbackHash = dom.Toggles.BasicAuth.PasswordHash
		}
		if !validateAndHashBasicAuth(w, &req.Toggles.BasicAuth, fallbackHash, "", "handleUpdateDomain") {
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Domain updated: "+req.Name)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, id)
			if dom == nil {
				return nil, errMutationNotFound
			}
			dom.Name = req.Name
			dom.Toggles = req.Toggles
			return &c, nil
		})
		if writeMutateError(w, "handleUpdateDomain", err, "domain not found", "failed to update domain") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
		writeJSON(w, findDomain(cfg, id))
	}
}

func handleDeleteDomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		cfg := store.Get()
		dom := findDomain(cfg, id)
		if dom == nil {
			writeError(w, "domain not found", http.StatusNotFound)
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Domain deleted: "+dom.Name)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			fresh := make([]config.Domain, 0, len(c.Domains))
			for _, d := range c.Domains {
				if d.ID != id {
					fresh = append(fresh, d)
				}
			}
			c.Domains = fresh
			return &c, nil
		})
		if writeMutateError(w, "handleDeleteDomain", err, "domain not found", "failed to delete domain") {
			return
		}

		persistCaddyConfig(cc, store)
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleEnableDomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return domainToggleHandler(store, cc, ss, version, true)
}

func handleDisableDomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return domainToggleHandler(store, cc, ss, version, false)
}

func domainToggleHandler(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		action := "enabled"
		if !enabled {
			action = "disabled"
		}

		cfg := store.Get()
		existing := findDomain(cfg, id)
		if existing == nil {
			writeError(w, "domain not found", http.StatusNotFound)
			return
		}
		maybeAutoSnapshot(cc, ss, store, version, "Domain "+action+": "+existing.Name)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, id)
			if dom == nil {
				return nil, errMutationNotFound
			}
			dom.Enabled = enabled
			return &c, nil
		})
		if writeMutateError(w, "handleToggleDomain", err, "domain not found", "failed to update domain") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg = store.Get()
		if dom := findDomain(cfg, id); dom != nil {
			writeJSON(w, dom)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleEnableDomainRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return domainRuleToggleHandler(store, cc, ss, version, true)
}

func handleDisableDomainRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return domainRuleToggleHandler(store, cc, ss, version, false)
}

func domainRuleToggleHandler(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		action := "enabled"
		if !enabled {
			action = "disabled"
		}

		cfg := store.Get()
		existing := findDomain(cfg, id)
		if existing == nil {
			writeError(w, "domain not found", http.StatusNotFound)
			return
		}
		maybeAutoSnapshot(cc, ss, store, version, "Domain rule "+action+": "+existing.Name)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, id)
			if dom == nil {
				return nil, errMutationNotFound
			}
			dom.Rule.Enabled = enabled
			return &c, nil
		})
		if writeMutateError(w, "handleToggleDomainRule", err, "domain not found", "failed to update rule") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg = store.Get()
		if dom := findDomain(cfg, id); dom != nil {
			writeJSON(w, dom)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleUpdateDomainRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		var req updateRuleRequest
		if !decodeBody(w, r, &req) {
			return
		}
		if !validateRule(w, req, true) {
			return
		}

		domName := domainID
		if dom := findDomain(store.Get(), domainID); dom != nil {
			domName = dom.Name
		}
		maybeAutoSnapshot(cc, ss, store, version, "Domain rule updated: "+domName)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, errMutationNotFound
			}
			dom.Rule.HandlerType = req.HandlerType
			dom.Rule.HandlerConfig = req.HandlerConfig
			dom.Rule.AdvancedHeaders = req.AdvancedHeaders
			return &c, nil
		})
		if writeMutateError(w, "handleUpdateDomainRule", err, "domain not found", "failed to update rule") {
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
