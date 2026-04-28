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

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
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
		var arr []json.RawMessage
		if err := json.Unmarshal(rules.AdvancedRaw, &arr); err != nil {
			return "advanced_raw must be a valid JSON array"
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
