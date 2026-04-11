package caddy

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type CertInfo struct {
	Domain      string   `json:"domain"`
	SANs        []string `json:"sans"`
	Issuer      string   `json:"issuer"`
	NotBefore   string   `json:"not_before"`
	NotAfter    string   `json:"not_after"`
	DaysLeft    int      `json:"days_left"`
	Status      string   `json:"status"`
	Managed     bool     `json:"managed"`
	IssuerKey   string   `json:"issuer_key"`
	Fingerprint string   `json:"fingerprint"`
}

func ResolveCaddyDataDir(override string) string {
	if override != "" {
		if info, err := os.Stat(override); err == nil && info.IsDir() {
			return override
		}
	}

	if env := os.Getenv("CADDY_DATA_DIR"); env != "" {
		if info, err := os.Stat(env); err == nil && info.IsDir() {
			return env
		}
	}

	candidates := []string{"/data/caddy"}
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, "Library", "Application Support", "Caddy"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "share", "caddy"))
	}

	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}

	return "/data/caddy"
}

func ReadCertificates(dataDir string) ([]CertInfo, error) {
	certsDir := filepath.Join(dataDir, "certificates")
	if _, err := os.Stat(certsDir); os.IsNotExist(err) {
		return nil, nil
	}

	issuers, err := os.ReadDir(certsDir)
	if err != nil {
		return nil, fmt.Errorf("reading certificates dir: %w", err)
	}

	var certs []CertInfo
	for _, issuerEntry := range issuers {
		if !issuerEntry.IsDir() {
			continue
		}
		issuerKey := issuerEntry.Name()
		issuerDir := filepath.Join(certsDir, issuerKey)

		domains, err := os.ReadDir(issuerDir)
		if err != nil {
			continue
		}
		for _, domainEntry := range domains {
			if !domainEntry.IsDir() {
				continue
			}
			domain := domainEntry.Name()
			certFile := filepath.Join(issuerDir, domain, domain+".crt")

			ci, err := parseCertFile(certFile, issuerKey, domain)
			if err != nil {
				continue
			}
			certs = append(certs, *ci)
		}
	}

	sort.Slice(certs, func(i, j int) bool {
		pi := statusPriority(certs[i].Status)
		pj := statusPriority(certs[j].Status)
		if pi != pj {
			return pi < pj
		}
		return certs[i].DaysLeft < certs[j].DaysLeft
	})

	return certs, nil
}

func parseCertFile(path, issuerKey, domain string) (*CertInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	name := cert.Subject.CommonName
	if name == "" && len(cert.DNSNames) > 0 {
		name = cert.DNSNames[0]
	}
	if name == "" {
		name = domain
	}

	now := time.Now()
	hoursLeft := cert.NotAfter.Sub(now).Hours()
	daysLeft := int(math.Floor(hoursLeft / 24))
	if daysLeft < 0 {
		daysLeft = 0
	}

	fingerprint := fmt.Sprintf("%x", sha256.Sum256(cert.Raw))

	managed := strings.Contains(strings.ToLower(issuerKey), "acme") ||
		strings.Contains(strings.ToLower(issuerKey), "letsencrypt") ||
		strings.Contains(strings.ToLower(issuerKey), "zerossl")

	var status string
	switch {
	case now.After(cert.NotAfter):
		status = "expired"
	case daysLeft < 7:
		status = "critical"
	case daysLeft <= 30:
		status = "expiring"
	default:
		status = "valid"
	}

	return &CertInfo{
		Domain:      name,
		SANs:        cert.DNSNames,
		Issuer:      cert.Issuer.CommonName,
		NotBefore:   cert.NotBefore.Format(time.RFC3339),
		NotAfter:    cert.NotAfter.Format(time.RFC3339),
		DaysLeft:    daysLeft,
		Status:      status,
		Managed:     managed,
		IssuerKey:   issuerKey,
		Fingerprint: fingerprint,
	}, nil
}

func statusPriority(status string) int {
	switch status {
	case "missing":
		return -1
	case "expired":
		return 0
	case "critical":
		return 1
	case "expiring":
		return 2
	case "valid":
		return 3
	default:
		return 4
	}
}

func DeleteCertificate(dataDir, issuerKey, domain string) error {
	certsRoot := filepath.Join(dataDir, "certificates")
	dir := filepath.Join(certsRoot, issuerKey, domain)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving certificate path: %w", err)
	}
	absCertsRoot, err := filepath.Abs(certsRoot)
	if err != nil {
		return fmt.Errorf("resolving certificates root: %w", err)
	}
	if !strings.HasPrefix(absDir, absCertsRoot+string(filepath.Separator)) {
		return fmt.Errorf("invalid certificate path")
	}
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		return fmt.Errorf("certificate directory not found: %s", dir)
	}
	return os.RemoveAll(absDir)
}

func CertFilePath(dataDir, issuerKey, domain string) string {
	return filepath.Join(dataDir, "certificates", issuerKey, domain, domain+".crt")
}

func ExpectedCertDomains(configJSON []byte) []string {
	if configJSON == nil {
		return nil
	}

	var cfg caddyConfigPartial
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return nil
	}

	autoHTTPS := "on"
	for _, srv := range cfg.Apps.HTTP.Servers {
		if srv.AutoHTTPS != nil {
			if srv.AutoHTTPS.Disable {
				autoHTTPS = "off"
			} else if srv.AutoHTTPS.DisableRedirects {
				autoHTTPS = "disable_redirects"
			}
		}
		break
	}

	var domains []string
	seen := map[string]bool{}

	for _, srv := range cfg.Apps.HTTP.Servers {
		for _, rawRoute := range srv.Routes {
			var route struct {
				ID    string `json:"@id"`
				Match []struct {
					Host []string `json:"host"`
				} `json:"match"`
				Handle []json.RawMessage `json:"handle"`
			}
			if json.Unmarshal(rawRoute, &route) != nil {
				continue
			}
			if !strings.HasPrefix(route.ID, "kaji_") {
				continue
			}

			hasHTTPS := autoHTTPS != "off"
			if !hasHTTPS {
				for _, h := range route.Handle {
					var handler struct {
						Handler string `json:"handler"`
						Routes  []struct {
							Match  []json.RawMessage `json:"match"`
							Handle []json.RawMessage `json:"handle"`
						} `json:"routes"`
					}
					if json.Unmarshal(h, &handler) != nil || handler.Handler != "subroute" {
						continue
					}
					if isForceHTTPSSubroute(handler.Routes) {
						hasHTTPS = true
						break
					}
				}
			}

			if !hasHTTPS {
				continue
			}

			for _, m := range route.Match {
				for _, host := range m.Host {
					lower := strings.ToLower(host)
					if !seen[lower] {
						seen[lower] = true
						domains = append(domains, host)
					}
				}
			}
		}
	}

	return domains
}

func MergeMissingCerts(certs []CertInfo, expectedDomains []string) []CertInfo {
	if len(expectedDomains) == 0 {
		return certs
	}

	have := map[string]bool{}
	for _, c := range certs {
		have[strings.ToLower(c.Domain)] = true
		for _, san := range c.SANs {
			have[strings.ToLower(san)] = true
		}
	}

	for _, domain := range expectedDomains {
		if have[strings.ToLower(domain)] {
			continue
		}
		certs = append(certs, CertInfo{
			Domain:   domain,
			SANs:     []string{},
			Status:   "missing",
			DaysLeft: -1,
		})
	}

	sort.Slice(certs, func(i, j int) bool {
		pi := statusPriority(certs[i].Status)
		pj := statusPriority(certs[j].Status)
		if pi != pj {
			return pi < pj
		}
		return certs[i].DaysLeft < certs[j].DaysLeft
	})

	return certs
}
