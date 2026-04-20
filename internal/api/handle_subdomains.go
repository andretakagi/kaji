package api

import (
	"encoding/json"
	"fmt"
	"log"
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

func handleCreateSubdomain(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		var req struct {
			Name            string               `json:"name"`
			HandlerType     string               `json:"handler_type"`
			HandlerConfig   json.RawMessage      `json:"handler_config"`
			Toggles         *caddy.DomainToggles `json:"toggles"`
			AdvancedHeaders bool                 `json:"advanced_headers"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		name := strings.TrimSpace(req.Name)
		if msg := validateSubdomainName(name); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		if msg := validateSubdomainHandlerType(req.HandlerType); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		if req.HandlerType != "none" {
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

		if toggles.BasicAuth.Enabled {
			if toggles.BasicAuth.Username == "" {
				writeError(w, "username is required for basic auth", http.StatusBadRequest)
				return
			}
			if err := hashBasicAuthPassword(&toggles.BasicAuth, ""); err != nil {
				log.Printf("handleCreateSubdomain: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
		}

		sub := config.Subdomain{
			ID:              caddy.GenerateSubdomainID(),
			Name:            name,
			Enabled:         true,
			HandlerType:     req.HandlerType,
			HandlerConfig:   req.HandlerConfig,
			Toggles:         toggles,
			AdvancedHeaders: req.AdvancedHeaders,
			Rules:           []config.Rule{},
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain created: "+name+"."+dom.Name)

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, fmt.Errorf("domain not found")
			}
			d.Subdomains = append(d.Subdomains, sub)
			return &c, nil
		}); err != nil {
			if err.Error() == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			log.Printf("handleCreateSubdomain: save config: %v", err)
			writeError(w, "failed to save subdomain", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleCreateSubdomain", err)
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
			Name            string              `json:"name"`
			HandlerType     string              `json:"handler_type"`
			HandlerConfig   json.RawMessage     `json:"handler_config"`
			Toggles         caddy.DomainToggles `json:"toggles"`
			AdvancedHeaders bool                `json:"advanced_headers"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		name := strings.TrimSpace(req.Name)
		if msg := validateSubdomainName(name); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		if msg := validateSubdomainHandlerType(req.HandlerType); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		if req.HandlerType != "none" {
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
		}

		if req.Toggles.BasicAuth.Enabled {
			if req.Toggles.BasicAuth.Username == "" {
				writeError(w, "username is required for basic auth", http.StatusBadRequest)
				return
			}
			var fallbackHash string
			cfg := store.Get()
			if d := findDomain(cfg, domainID); d != nil {
				if s := findSubdomain(d, subID); s != nil {
					fallbackHash = s.Toggles.BasicAuth.PasswordHash
				}
			}
			if err := hashBasicAuthPassword(&req.Toggles.BasicAuth, fallbackHash); err != nil {
				log.Printf("handleUpdateSubdomain: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain updated: "+subID)

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, fmt.Errorf("domain not found")
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, fmt.Errorf("subdomain not found")
			}
			for _, other := range d.Subdomains {
				if other.ID != subID && strings.EqualFold(other.Name, name) {
					return nil, fmt.Errorf("duplicate name")
				}
			}
			s.Name = name
			s.HandlerType = req.HandlerType
			s.HandlerConfig = req.HandlerConfig
			s.Toggles = req.Toggles
			s.AdvancedHeaders = req.AdvancedHeaders
			return &c, nil
		}); err != nil {
			errMsg := err.Error()
			if errMsg == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: subdomain not found" {
				writeError(w, "subdomain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: duplicate name" {
				writeError(w, "a subdomain with this name already exists", http.StatusConflict)
				return
			}
			log.Printf("handleUpdateSubdomain: save config: %v", err)
			writeError(w, "failed to update subdomain", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleUpdateSubdomain", err)
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

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain deleted: "+subID)

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, fmt.Errorf("domain not found")
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
				return nil, fmt.Errorf("subdomain not found")
			}
			d.Subdomains = fresh
			return &c, nil
		}); err != nil {
			errMsg := err.Error()
			if errMsg == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: subdomain not found" {
				writeError(w, "subdomain not found", http.StatusNotFound)
				return
			}
			log.Printf("handleDeleteSubdomain: save config: %v", err)
			writeError(w, "failed to delete subdomain", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleDeleteSubdomain", err)
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

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, fmt.Errorf("domain not found")
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, fmt.Errorf("subdomain not found")
			}
			s.Enabled = enabled
			return &c, nil
		}); err != nil {
			errMsg := err.Error()
			if errMsg == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: subdomain not found" {
				writeError(w, "subdomain not found", http.StatusNotFound)
				return
			}
			log.Printf("handleToggleSubdomain: save config: %v", err)
			writeError(w, "failed to update subdomain", http.StatusInternalServerError)
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain "+action+": "+subID)

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleToggleSubdomain", err)
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

func handleCreateSubdomainRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
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
		if req.MatchType != "path" {
			writeError(w, "subdomain rules must have match_type path", http.StatusBadRequest)
			return
		}
		if msg := validatePathMatch(req.PathMatch); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if msg := validateHandlerType(req.HandlerType); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
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
				log.Printf("handleCreateSubdomainRule: hash password: %v", err)
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
		if findSubdomain(dom, subID) == nil {
			writeError(w, "subdomain not found", http.StatusNotFound)
			return
		}

		rule := config.Rule{
			ID:              caddy.GenerateRuleID(),
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

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain rule created: "+rule.ID)

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, fmt.Errorf("domain not found")
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, fmt.Errorf("subdomain not found")
			}
			s.Rules = append(s.Rules, rule)
			return &c, nil
		}); err != nil {
			errMsg := err.Error()
			if errMsg == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: subdomain not found" {
				writeError(w, "subdomain not found", http.StatusNotFound)
				return
			}
			log.Printf("handleCreateSubdomainRule: save config: %v", err)
			writeError(w, "failed to save rule", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleCreateSubdomainRule", err)
			return
		}

		persistCaddyConfig(cc, store)

		cfg = store.Get()
		if d := findDomain(cfg, domainID); d != nil {
			writeJSON(w, d)
			return
		}
		writeJSON(w, rule)
	}
}

func handleUpdateSubdomainRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
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
		if req.MatchType != "path" {
			writeError(w, "subdomain rules must have match_type path", http.StatusBadRequest)
			return
		}
		if msg := validatePathMatch(req.PathMatch); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}
		if msg := validateHandlerType(req.HandlerType); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
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
			if d := findDomain(cfg, domainID); d != nil {
				if s := findSubdomain(d, subID); s != nil {
					for _, rule := range s.Rules {
						if rule.ID == ruleID && rule.ToggleOverrides != nil {
							fallbackHash = rule.ToggleOverrides.BasicAuth.PasswordHash
							break
						}
					}
				}
			}
			if err := hashBasicAuthPassword(&req.ToggleOverrides.BasicAuth, fallbackHash); err != nil {
				log.Printf("handleUpdateSubdomainRule: hash password: %v", err)
				writeError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain rule updated: "+ruleID)

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, fmt.Errorf("domain not found")
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, fmt.Errorf("subdomain not found")
			}
			for i := range s.Rules {
				if s.Rules[i].ID == ruleID {
					s.Rules[i].Label = req.Label
					s.Rules[i].MatchType = req.MatchType
					s.Rules[i].PathMatch = req.PathMatch
					s.Rules[i].MatchValue = req.MatchValue
					s.Rules[i].HandlerType = req.HandlerType
					s.Rules[i].HandlerConfig = req.HandlerConfig
					s.Rules[i].ToggleOverrides = req.ToggleOverrides
					s.Rules[i].AdvancedHeaders = req.AdvancedHeaders
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
			if errMsg == "applying config update: subdomain not found" {
				writeError(w, "subdomain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: rule not found" {
				writeError(w, "rule not found", http.StatusNotFound)
				return
			}
			log.Printf("handleUpdateSubdomainRule: save config: %v", err)
			writeError(w, "failed to update rule", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleUpdateSubdomainRule", err)
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

func handleDeleteSubdomainRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
		ruleID := r.PathValue("ruleId")

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain rule deleted: "+ruleID)

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, fmt.Errorf("domain not found")
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, fmt.Errorf("subdomain not found")
			}
			fresh := make([]config.Rule, 0, len(s.Rules))
			found := false
			for _, rule := range s.Rules {
				if rule.ID == ruleID {
					found = true
					continue
				}
				fresh = append(fresh, rule)
			}
			if !found {
				return nil, fmt.Errorf("rule not found")
			}
			s.Rules = fresh
			return &c, nil
		}); err != nil {
			errMsg := err.Error()
			if errMsg == "applying config update: domain not found" {
				writeError(w, "domain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: subdomain not found" {
				writeError(w, "subdomain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: rule not found" {
				writeError(w, "rule not found", http.StatusNotFound)
				return
			}
			log.Printf("handleDeleteSubdomainRule: save config: %v", err)
			writeError(w, "failed to delete rule", http.StatusInternalServerError)
			return
		}

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleDeleteSubdomainRule", err)
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

func handleEnableSubdomainRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return subdomainRuleToggleHandler(store, cc, ss, version, true)
}

func handleDisableSubdomainRule(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return subdomainRuleToggleHandler(store, cc, ss, version, false)
}

func subdomainRuleToggleHandler(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("id")
		subID := r.PathValue("subId")
		ruleID := r.PathValue("ruleId")

		action := "enabled"
		if !enabled {
			action = "disabled"
		}

		if err := store.Update(func(c config.AppConfig) (*config.AppConfig, error) {
			d := findDomain(&c, domainID)
			if d == nil {
				return nil, fmt.Errorf("domain not found")
			}
			s := findSubdomain(d, subID)
			if s == nil {
				return nil, fmt.Errorf("subdomain not found")
			}
			for i := range s.Rules {
				if s.Rules[i].ID == ruleID {
					s.Rules[i].Enabled = enabled
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
			if errMsg == "applying config update: subdomain not found" {
				writeError(w, "subdomain not found", http.StatusNotFound)
				return
			}
			if errMsg == "applying config update: rule not found" {
				writeError(w, "rule not found", http.StatusNotFound)
				return
			}
			log.Printf("handleToggleSubdomainRule: save config: %v", err)
			writeError(w, "failed to update rule", http.StatusInternalServerError)
			return
		}

		maybeAutoSnapshot(cc, ss, store, version, "Subdomain rule "+action+": "+ruleID)

		if err := syncAfterMutation(cc, store); err != nil {
			caddyError(w, "handleToggleSubdomainRule", err)
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
