package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/export"
	"github.com/andretakagi/kaji/internal/snapshot"
)

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
			ID:      domainID,
			Name:    req.Name,
			Enabled: true,
			Toggles: req.Toggles,
			Rules:   rules,
		}

		maybeAutoSnapshot(cc, ss, store, version, "Domain created: "+req.Name)

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			c.Domains = append(c.Domains, domain)
			return &c, nil
		}); err != nil {
			log.Printf("handleCreateDomainFull: save config: %v", err)
			writeError(w, "failed to save domain", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleCreateDomainFull", err)
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

		var updated *config.Domain
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, id)
			if dom == nil {
				return nil, fmt.Errorf("domain not found")
			}
			dom.Name = req.Name
			dom.Toggles = req.Toggles
			updated = dom
			return &c, nil
		}); err != nil {
			if err.Error() == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			log.Printf("handleUpdateDomain: save config: %v", err)
			writeError(w, "failed to update domain", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleUpdateDomain", err)
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
		dom := findDomain(cfg, id)
		if dom != nil {
			writeJSON(w, dom)
		} else {
			writeJSON(w, updated)
		}
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

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			fresh := make([]config.Domain, 0, len(c.Domains))
			for _, d := range c.Domains {
				if d.ID != id {
					fresh = append(fresh, d)
				}
			}
			c.Domains = fresh
			return &c, nil
		}); err != nil {
			log.Printf("handleDeleteDomain: save config: %v", err)
			writeError(w, "failed to delete domain", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleDeleteDomain", err)
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

		var domName string
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, id)
			if dom == nil {
				return nil, fmt.Errorf("domain not found")
			}
			dom.Enabled = enabled
			domName = dom.Name
			return &c, nil
		}); err != nil {
			if err.Error() == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			log.Printf("handleToggleDomain: save config: %v", err)
			writeError(w, "failed to update domain", http.StatusInternalServerError)
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Domain "+action+": "+domName)

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleToggleDomain", err)
			return
		}

		persistCaddyConfig(cc, store)

		cfg := store.Get()
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

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, fmt.Errorf("domain not found")
			}
			dom.Rules = append(dom.Rules, rule)
			return &c, nil
		}); err != nil {
			if err.Error() == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			log.Printf("handleCreateRule: save config: %v", err)
			writeError(w, "failed to save rule", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleCreateRule", err)
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

		var updated *config.Rule
		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, fmt.Errorf("domain not found")
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
					updated = &dom.Rules[i]
					return &c, nil
				}
			}
			return nil, fmt.Errorf("rule not found")
		}); err != nil {
			errMsg := err.Error()
			if errMsg == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: rule not found" {
				writeError(w, "rule not found", http.StatusNotFound)
				return
			}
			log.Printf("handleUpdateRule: save config: %v", err)
			writeError(w, "failed to update rule", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleUpdateRule", err)
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
		writeJSON(w, updated)
	}
}

func handleDeleteRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		ruleID := r.PathValue("ruleId")

		maybeAutoSnapshot(cc, ss, store, version, "Rule deleted: "+ruleID)

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, fmt.Errorf("domain not found")
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
				return nil, fmt.Errorf("rule not found")
			}
			dom.Rules = fresh
			return &c, nil
		}); err != nil {
			errMsg := err.Error()
			if errMsg == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: rule not found" {
				writeError(w, "rule not found", http.StatusNotFound)
				return
			}
			log.Printf("handleDeleteRule: save config: %v", err)
			writeError(w, "failed to delete rule", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleDeleteRule", err)
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

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			dom := findDomain(&c, domainID)
			if dom == nil {
				return nil, fmt.Errorf("domain not found")
			}
			for i := range dom.Rules {
				if dom.Rules[i].ID == ruleID {
					dom.Rules[i].Enabled = enabled
					return &c, nil
				}
			}
			return nil, fmt.Errorf("rule not found")
		}); err != nil {
			errMsg := err.Error()
			if errMsg == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: rule not found" {
				writeError(w, "rule not found", http.StatusNotFound)
				return
			}
			log.Printf("handleToggleRule: save config: %v", err)
			writeError(w, "failed to update rule", http.StatusInternalServerError)
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Rule "+action+": "+ruleID)

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleToggleRule", err)
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
