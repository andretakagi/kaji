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
	_, err := caddy.SyncDomains(cc, syncDomains, ipResolver(store))
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
			Name    string              `json:"name"`
			Toggles caddy.DomainToggles `json:"toggles"`
			Rules   []struct {
				Label           string               `json:"label"`
				MatchType       string               `json:"match_type"`
				PathMatch       string               `json:"path_match"`
				MatchValue      string               `json:"match_value"`
				HandlerType     string               `json:"handler_type"`
				HandlerConfig   json.RawMessage      `json:"handler_config"`
				ToggleOverrides *caddy.DomainToggles `json:"toggle_overrides"`
				AdvancedHeaders bool                 `json:"advanced_headers"`
			} `json:"rules"`
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

		if req.Toggles.BasicAuth.Enabled {
			if req.Toggles.BasicAuth.Username == "" {
				writeError(w, "username is required for basic auth", http.StatusBadRequest)
				return
			}
			if err := hashBasicAuthPassword(&req.Toggles.BasicAuth, ""); err != nil {
				log.Printf("handleCreateDomainFull: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
		}

		hasRoot := false
		for i, rule := range req.Rules {
			if msg := validateMatchType(rule.MatchType); msg != "" {
				writeError(w, fmt.Sprintf("rule %d: %s", i+1, msg), http.StatusBadRequest)
				return
			}
			if msg := validateHandlerType(rule.HandlerType); msg != "" {
				writeError(w, fmt.Sprintf("rule %d: %s", i+1, msg), http.StatusBadRequest)
				return
			}
			if rule.MatchType == "path" {
				if msg := validatePathMatch(rule.PathMatch); msg != "" {
					writeError(w, fmt.Sprintf("rule %d: %s", i+1, msg), http.StatusBadRequest)
					return
				}
			}
			if rule.HandlerType == "reverse_proxy" {
				if !validateReverseProxyConfig(w, rule.HandlerConfig) {
					return
				}
			}
			if rule.HandlerType == "static_response" {
				if !validateStaticResponseConfig(w, rule.HandlerConfig) {
					return
				}
			}
			if rule.HandlerType == "redirect" {
				if !validateRedirectConfig(w, rule.HandlerConfig) {
					return
				}
			}
			if rule.HandlerType == "file_server" {
				if !validateFileServerConfig(w, rule.HandlerConfig) {
					return
				}
			}
			if rule.MatchType == "" {
				if hasRoot {
					writeError(w, "only one root rule is allowed", http.StatusBadRequest)
					return
				}
				hasRoot = true
			}
			if rule.ToggleOverrides != nil && rule.ToggleOverrides.BasicAuth.Enabled {
				if rule.ToggleOverrides.BasicAuth.Username == "" {
					writeError(w, fmt.Sprintf("rule %d: username is required for basic auth", i+1), http.StatusBadRequest)
					return
				}
				if err := hashBasicAuthPassword(&rule.ToggleOverrides.BasicAuth, ""); err != nil {
					log.Printf("handleCreateDomainFull: hash rule password: %v", err)
					writeError(w, "failed to hash password", http.StatusInternalServerError)
					return
				}
			}
		}

		domainID := caddy.GenerateDomainID()
		rules := make([]config.Rule, len(req.Rules))
		for i, rule := range req.Rules {
			rules[i] = config.Rule{
				ID:              caddy.GenerateRuleID(),
				Label:           rule.Label,
				Enabled:         true,
				MatchType:       rule.MatchType,
				PathMatch:       rule.PathMatch,
				MatchValue:      rule.MatchValue,
				HandlerType:     rule.HandlerType,
				HandlerConfig:   rule.HandlerConfig,
				ToggleOverrides: rule.ToggleOverrides,
				AdvancedHeaders: rule.AdvancedHeaders,
			}
		}

		domain := config.Domain{
			ID:         domainID,
			Name:       req.Name,
			Enabled:    true,
			Toggles:    req.Toggles,
			Rules:      rules,
			Subdomains: []config.Subdomain{},
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

		if req.Toggles.BasicAuth.Enabled {
			if req.Toggles.BasicAuth.Username == "" {
				writeError(w, "username is required for basic auth", http.StatusBadRequest)
				return
			}
			var fallbackHash string
			cfg := store.Get()
			if dom := findDomain(cfg, id); dom != nil {
				fallbackHash = dom.Toggles.BasicAuth.PasswordHash
			}
			if err := hashBasicAuthPassword(&req.Toggles.BasicAuth, fallbackHash); err != nil {
				log.Printf("handleUpdateDomain: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
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

func handleCreateRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		var req struct {
			Label           string               `json:"label"`
			MatchType       string               `json:"match_type"`
			PathMatch       string               `json:"path_match"`
			MatchValue      string               `json:"match_value"`
			HandlerType     string               `json:"handler_type"`
			HandlerConfig   json.RawMessage      `json:"handler_config"`
			ToggleOverrides *caddy.DomainToggles `json:"toggle_overrides"`
			AdvancedHeaders bool                 `json:"advanced_headers"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if msg := validateMatchType(req.MatchType); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if msg := validateHandlerType(req.HandlerType); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if req.MatchType == "path" {
			if msg := validatePathMatch(req.PathMatch); msg != "" {
				writeError(w, msg, http.StatusBadRequest)
				return
			}
		}
		if req.HandlerType == "reverse_proxy" {
			if !validateReverseProxyConfig(w, req.HandlerConfig) {
				return
			}
		}
		if req.HandlerType == "static_response" {
			if !validateStaticResponseConfig(w, req.HandlerConfig) {
				return
			}
		}
		if req.HandlerType == "redirect" {
			if !validateRedirectConfig(w, req.HandlerConfig) {
				return
			}
		}
		if req.HandlerType == "file_server" {
			if !validateFileServerConfig(w, req.HandlerConfig) {
				return
			}
		}

		if req.ToggleOverrides != nil && req.ToggleOverrides.BasicAuth.Enabled {
			if req.ToggleOverrides.BasicAuth.Username == "" {
				writeError(w, "username is required for basic auth", http.StatusBadRequest)
				return
			}
			if err := hashBasicAuthPassword(&req.ToggleOverrides.BasicAuth, ""); err != nil {
				log.Printf("handleCreateRule: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
		}

		cfg := store.Get()
		dom := findDomain(cfg, domainID)
		if dom == nil {
			writeError(w, "domain not found", http.StatusNotFound)
			return
		}
		if req.MatchType == "" {
			for _, r := range dom.Rules {
				if r.MatchType == "" {
					writeError(w, "a root rule already exists for this domain", http.StatusConflict)
					return
				}
			}
		}

		ruleID := caddy.GenerateRuleID()
		rule := config.Rule{
			ID:              ruleID,
			Label:           req.Label,
			Enabled:         true,
			MatchType:       req.MatchType,
			PathMatch:       req.PathMatch,
			MatchValue:      req.MatchValue,
			HandlerType:     req.HandlerType,
			HandlerConfig:   req.HandlerConfig,
			ToggleOverrides: req.ToggleOverrides,
			AdvancedHeaders: req.AdvancedHeaders,
		}

		maybeAutoSnapshot(cc, ss, store, version, "Rule created: "+ruleID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, errMutationNotFound
			}
			dom.Rules = append(dom.Rules, rule)
			return &c, nil
		})
		if writeMutateError(w, "handleCreateRule", err, "domain not found", "failed to save rule") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg = store.Get()
		if dom := findDomain(cfg, domainID); dom != nil {
			writeJSON(w, dom)
			return
		}
		writeJSON(w, rule)
	}
}

func handleUpdateRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		ruleID := r.PathValue("ruleId")
		var req struct {
			Label           string               `json:"label"`
			MatchType       string               `json:"match_type"`
			PathMatch       string               `json:"path_match"`
			MatchValue      string               `json:"match_value"`
			HandlerType     string               `json:"handler_type"`
			HandlerConfig   json.RawMessage      `json:"handler_config"`
			ToggleOverrides *caddy.DomainToggles `json:"toggle_overrides"`
			AdvancedHeaders bool                 `json:"advanced_headers"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if msg := validateMatchType(req.MatchType); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if msg := validateHandlerType(req.HandlerType); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if req.MatchType == "path" {
			if msg := validatePathMatch(req.PathMatch); msg != "" {
				writeError(w, msg, http.StatusBadRequest)
				return
			}
		}
		if req.HandlerType == "reverse_proxy" {
			if !validateReverseProxyConfig(w, req.HandlerConfig) {
				return
			}
		}
		if req.HandlerType == "static_response" {
			if !validateStaticResponseConfig(w, req.HandlerConfig) {
				return
			}
		}
		if req.HandlerType == "redirect" {
			if !validateRedirectConfig(w, req.HandlerConfig) {
				return
			}
		}
		if req.HandlerType == "file_server" {
			if !validateFileServerConfig(w, req.HandlerConfig) {
				return
			}
		}

		if req.ToggleOverrides != nil && req.ToggleOverrides.BasicAuth.Enabled {
			if req.ToggleOverrides.BasicAuth.Username == "" {
				writeError(w, "username is required for basic auth", http.StatusBadRequest)
				return
			}
			var fallbackHash string
			cfg := store.Get()
			if dom := findDomain(cfg, domainID); dom != nil {
				for _, r := range dom.Rules {
					if r.ID == ruleID && r.ToggleOverrides != nil {
						fallbackHash = r.ToggleOverrides.BasicAuth.PasswordHash
						break
					}
				}
			}
			if err := hashBasicAuthPassword(&req.ToggleOverrides.BasicAuth, fallbackHash); err != nil {
				log.Printf("handleUpdateRule: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
		}

		maybeAutoSnapshot(cc, ss, store, version, "Rule updated: "+ruleID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, errMutationNotFound
			}
			for i := range dom.Rules {
				if dom.Rules[i].ID == ruleID {
					dom.Rules[i].Label = req.Label
					dom.Rules[i].MatchType = req.MatchType
					dom.Rules[i].PathMatch = req.PathMatch
					dom.Rules[i].MatchValue = req.MatchValue
					dom.Rules[i].HandlerType = req.HandlerType
					dom.Rules[i].HandlerConfig = req.HandlerConfig
					dom.Rules[i].ToggleOverrides = req.ToggleOverrides
					dom.Rules[i].AdvancedHeaders = req.AdvancedHeaders
					return &c, nil
				}
			}
			return nil, errMutationNotFound
		})
		if writeMutateError(w, "handleUpdateRule", err, "rule not found", "failed to update rule") {
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
		if dom := findDomain(cfg, domainID); dom != nil {
			for _, r := range dom.Rules {
				if r.ID == ruleID {
					writeJSON(w, r)
					return
				}
			}
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleDeleteRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		ruleID := r.PathValue("ruleId")

		maybeAutoSnapshot(cc, ss, store, version, "Rule deleted: "+ruleID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, errMutationNotFound
			}
			fresh := make([]config.Rule, 0, len(dom.Rules))
			found := false
			for _, rule := range dom.Rules {
				if rule.ID == ruleID {
					found = true
					continue
				}
				fresh = append(fresh, rule)
			}
			if !found {
				return nil, errMutationNotFound
			}
			dom.Rules = fresh
			return &c, nil
		})
		if writeMutateError(w, "handleDeleteRule", err, "rule not found", "failed to delete rule") {
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

func handleEnableRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return ruleToggleHandler(store, cc, ss, version, true)
}

func handleDisableRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return ruleToggleHandler(store, cc, ss, version, false)
}

func ruleToggleHandler(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		ruleID := r.PathValue("ruleId")

		action := "enabled"
		if !enabled {
			action = "disabled"
		}

		maybeAutoSnapshot(cc, ss, store, version, "Rule "+action+": "+ruleID)

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, errMutationNotFound
			}
			for i := range dom.Rules {
				if dom.Rules[i].ID == ruleID {
					dom.Rules[i].Enabled = enabled
					return &c, nil
				}
			}
			return nil, errMutationNotFound
		})
		if writeMutateError(w, "handleToggleRule", err, "rule not found", "failed to update rule") {
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
