package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func handleLogSkipRulesGet(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sinkName := r.PathValue("sinkName")
		if sinkName == "" {
			writeError(w, "sink name is required", http.StatusBadRequest)
			return
		}

		cfg := store.Get()
		rules, ok := cfg.LogSkipRules[sinkName]
		if !ok {
			writeJSON(w, config.LogSkipConfig{
				Mode:        "basic",
				Conditions:  []config.SkipCondition{},
				AdvancedRaw: nil,
			})
			return
		}

		if rules.Conditions == nil {
			rules.Conditions = []config.SkipCondition{}
		}
		writeJSON(w, rules)
	}
}

func handleLogSkipRulesPut(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sinkName := r.PathValue("sinkName")
		if sinkName == "" {
			writeError(w, "sink name is required", http.StatusBadRequest)
			return
		}
		if sinkName == "default" {
			writeError(w, "skip rules cannot be set on the default sink", http.StatusBadRequest)
			return
		}

		loggingRaw, err := cc.GetLoggingConfig()
		if err != nil || loggingRaw == nil {
			writeError(w, "sink not found", http.StatusNotFound)
			return
		}
		var loggingCfg struct {
			Logs map[string]json.RawMessage `json:"logs"`
		}
		if json.Unmarshal(loggingRaw, &loggingCfg) != nil {
			writeError(w, "sink not found", http.StatusNotFound)
			return
		}
		if _, ok := loggingCfg.Logs[sinkName]; !ok {
			writeError(w, "sink not found", http.StatusNotFound)
			return
		}

		var rules config.LogSkipConfig
		if !decodeBody(w, r, &rules) {
			return
		}

		if msg := validateLogSkipRules(rules); msg != "" {
			writeError(w, msg, http.StatusBadRequest)
			return
		}

		if rules.Conditions == nil {
			rules.Conditions = []config.SkipCondition{}
		}

		err = mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			maybeAutoSnapshot(cc, ss, store, version, "Log skip rules updated")
			if c.LogSkipRules == nil {
				c.LogSkipRules = make(map[string]config.LogSkipConfig)
			}
			c.LogSkipRules[sinkName] = rules
			return &c, nil
		})
		if writeMutateError(w, "handleLogSkipRulesPut", err, "sink not found", "failed to save skip rules") {
			return
		}

		log.Printf("handleLogSkipRulesPut: saved skip rules for sink %q", sinkName)
		writeJSON(w, rules)
		persistCaddyConfig(cc, store)
	}
}

func validateLogSkipRules(rules config.LogSkipConfig) string {
	if rules.Mode != "basic" && rules.Mode != "advanced" {
		return "mode must be basic or advanced"
	}

	if rules.Mode == "advanced" {
		if len(rules.AdvancedRaw) == 0 {
			return "advanced_raw is required when mode is advanced"
		}
		var sets []map[string]json.RawMessage
		if err := json.Unmarshal(rules.AdvancedRaw, &sets); err != nil {
			return "advanced_raw must be a valid JSON array of matcher objects"
		}
		allowed := map[string]bool{"path": true, "path_regexp": true, "header": true, "remote_ip": true}
		for i, set := range sets {
			if len(set) == 0 {
				return fmt.Sprintf("advanced_raw[%d]: matcher object must have at least one key", i)
			}
			for key := range set {
				if !allowed[key] {
					return fmt.Sprintf("advanced_raw[%d]: unknown matcher %q (allowed: path, path_regexp, header, remote_ip)", i, key)
				}
			}
		}
	}

	for i, c := range rules.Conditions {
		switch c.Type {
		case "path", "path_regexp", "remote_ip":
		case "header":
			if c.Key == "" {
				return fmt.Sprintf("condition %d: key is required for header type", i)
			}
		default:
			return fmt.Sprintf("condition %d: type must be path, path_regexp, header, or remote_ip", i)
		}
		if c.Value == "" {
			return fmt.Sprintf("condition %d: value is required", i)
		}
	}

	return ""
}
