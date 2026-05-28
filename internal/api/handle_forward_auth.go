package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
	"github.com/andretakagi/kaji/internal/snapshot"
)

func handleForwardAuthGet(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, store.Get().ForwardAuth)
	}
}

func handleForwardAuthUpdate(store *config.ConfigStore, cc *caddy.Client, ss *snapshot.Store, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req caddy.ForwardAuthConfig
		if !decodeBody(w, r, &req) {
			return
		}

		if req.Enabled {
			if req.URL == "" {
				writeError(w, "URL is required when forward auth is enabled", http.StatusBadRequest)
				return
			}
			parsed, err := url.Parse(req.URL)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				writeError(w, "URL must include scheme and host (e.g. https://auth.example.com)", http.StatusBadRequest)
				return
			}
			switch req.Provider {
			case "authelia", "authentik", "custom":
			default:
				writeError(w, fmt.Sprintf("provider must be authelia, authentik, or custom (got %q)", req.Provider), http.StatusBadRequest)
				return
			}
		}

		if !req.Enabled {
			if using := domainsUsingForwardAuth(store); len(using) > 0 {
				writeError(w, fmt.Sprintf("cannot disable forward auth while domains use it: %s", strings.Join(using, ", ")), http.StatusConflict)
				return
			}
		}

		maybeAutoSnapshot(cc, ss, store, version, "Forward auth updated")

		err := mutateAndSync(store, cc, func(c config.AppConfig) (*config.AppConfig, error) {
			c.ForwardAuth = req
			return &c, nil
		})
		if writeMutateError(w, "handleForwardAuthUpdate", err, "", "failed to save forward auth config") {
			return
		}

		persistCaddyConfig(cc, store)
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func domainsUsingForwardAuth(store *config.ConfigStore) []string {
	cfg := store.Get()
	var using []string
	for _, dom := range cfg.Domains {
		if dom.Toggles.Auth.Mode == "forward" {
			using = append(using, dom.Name)
		}
		for _, sub := range dom.Subdomains {
			subHost := sub.Name + "." + dom.Name
			if sub.Toggles.Auth.Mode == "forward" {
				using = append(using, subHost)
			}
			for _, p := range sub.Paths {
				if p.ToggleOverrides != nil && p.ToggleOverrides.Auth.Mode == "forward" {
					using = append(using, pathLabel(subHost, p.MatchValue))
				}
			}
		}
		for _, p := range dom.Paths {
			if p.ToggleOverrides != nil && p.ToggleOverrides.Auth.Mode == "forward" {
				using = append(using, pathLabel(dom.Name, p.MatchValue))
			}
		}
	}
	return using
}

func pathLabel(host, matchValue string) string {
	if matchValue != "" {
		return host + matchValue
	}
	return host + " (path)"
}

func validateAuthMode(w http.ResponseWriter, store *config.ConfigStore, auth *caddy.AuthToggle, errPrefix string) bool {
	switch auth.Mode {
	case "", "off", "basic", "forward":
	default:
		writeError(w, fmt.Sprintf("%sauth mode must be off, basic, or forward", errPrefix), http.StatusBadRequest)
		return false
	}
	if auth.Mode == "forward" {
		cfg := store.Get()
		if !cfg.ForwardAuth.Enabled {
			writeError(w, fmt.Sprintf("%sforward auth is not configured globally; set it up in Settings first", errPrefix), http.StatusBadRequest)
			return false
		}
	}
	return true
}
