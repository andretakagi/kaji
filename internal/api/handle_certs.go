package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
)

func handleCertificatesList(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := store.Get()
		dataDir := caddy.ResolveCaddyDataDir(cfg.CaddyDataDir)
		certs, err := caddy.ReadCertificates(dataDir)
		if err != nil {
			log.Printf("handleCertificatesList: %v", err)
			writeError(w, "failed to read certificates", http.StatusInternalServerError)
			return
		}
		if certs == nil {
			certs = []caddy.CertInfo{}
		}

		configJSON, err := cc.GetConfig()
		if err == nil {
			expected := caddy.ExpectedCertDomains(configJSON)
			certs = caddy.MergeMissingCerts(certs, expected)
		}

		writeJSON(w, certs)
	}
}

func handleCertificateRenew(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IssuerKey string `json:"issuer_key"`
			Domain    string `json:"domain"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.IssuerKey == "" || req.Domain == "" {
			writeError(w, "issuer_key and domain are required", http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(req.IssuerKey, "/\\") || strings.ContainsAny(req.Domain, "/\\") {
			writeError(w, "invalid issuer_key or domain", http.StatusBadRequest)
			return
		}

		cfg := store.Get()
		dataDir := caddy.ResolveCaddyDataDir(cfg.CaddyDataDir)
		certPath := caddy.CertFilePath(dataDir, req.IssuerKey, req.Domain)

		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			writeError(w, "certificate not found", http.StatusNotFound)
			return
		}

		keyPath := strings.TrimSuffix(certPath, ".crt") + ".key"
		if err := os.Remove(certPath); err != nil {
			log.Printf("handleCertificateRenew: remove crt: %v", err)
			writeError(w, "failed to remove certificate file", http.StatusInternalServerError)
			return
		}
		if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
			log.Printf("handleCertificateRenew: remove key: %v", err)
		}

		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleCertificateDelete(store *config.ConfigStore, cc *caddy.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issuer := r.PathValue("issuer")
		domain := r.PathValue("domain")
		if issuer == "" || domain == "" {
			writeError(w, "issuer and domain are required", http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(issuer, "/\\") || strings.ContainsAny(domain, "/\\") {
			writeError(w, "invalid issuer or domain", http.StatusBadRequest)
			return
		}

		if r.URL.Query().Get("force") != "true" {
			configJSON, err := cc.GetConfig()
			if err == nil && containsDomain(configJSON, domain) {
				writeError(w, fmt.Sprintf("domain %q has an active route - use force=true to delete anyway", domain), http.StatusConflict)
				return
			}
		}

		cfg := store.Get()
		dataDir := caddy.ResolveCaddyDataDir(cfg.CaddyDataDir)
		if err := caddy.DeleteCertificate(dataDir, issuer, domain); err != nil {
			log.Printf("handleCertificateDelete: %v", err)
			writeError(w, "failed to delete certificate", http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleCertificateDownload(store *config.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issuer := r.PathValue("issuer")
		domain := r.PathValue("domain")
		if issuer == "" || domain == "" {
			writeError(w, "issuer and domain are required", http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(issuer, "/\\") || strings.ContainsAny(domain, "/\\") {
			writeError(w, "invalid issuer or domain", http.StatusBadRequest)
			return
		}

		cfg := store.Get()
		dataDir := caddy.ResolveCaddyDataDir(cfg.CaddyDataDir)
		certPath := caddy.CertFilePath(dataDir, issuer, domain)

		data, err := os.ReadFile(certPath)
		if err != nil {
			if os.IsNotExist(err) {
				writeError(w, "certificate not found", http.StatusNotFound)
				return
			}
			log.Printf("handleCertificateDownload: %v", err)
			writeError(w, "failed to read certificate", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", domain+".crt"))
		w.Write(data)
	}
}

func containsDomain(configJSON []byte, domain string) bool {
	var cfg struct {
		Apps struct {
			HTTP struct {
				Servers map[string]struct {
					Routes []struct {
						Match []struct {
							Host []string `json:"host"`
						} `json:"match"`
					} `json:"routes"`
				} `json:"servers"`
			} `json:"http"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return false
	}
	for _, srv := range cfg.Apps.HTTP.Servers {
		for _, route := range srv.Routes {
			for _, m := range route.Match {
				for _, h := range m.Host {
					if h == domain {
						return true
					}
				}
			}
		}
	}
	return false
}
