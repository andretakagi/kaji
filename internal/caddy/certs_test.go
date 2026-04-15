package caddy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExpectedCertDomains(t *testing.T) {
	cases := []struct {
		name string
		json string
		want []string
	}{
		{
			name: "nil input",
			json: "",
			want: nil,
		},
		{
			name: "invalid JSON",
			json: "{bad",
			want: nil,
		},
		{
			name: "no servers",
			json: `{"apps":{"http":{"servers":{}}}}`,
			want: nil,
		},
		{
			name: "kaji route with host",
			json: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"@id":"kaji_example_com","match":[{"host":["example.com"]}],"handle":[]}
			]}}}}}`,
			want: []string{"example.com"},
		},
		{
			name: "non-kaji route is ignored",
			json: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"@id":"other_route","match":[{"host":["example.com"]}],"handle":[]}
			]}}}}}`,
			want: nil,
		},
		{
			name: "multiple domains across routes",
			json: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"@id":"kaji_a","match":[{"host":["a.example.com"]}],"handle":[]},
				{"@id":"kaji_b","match":[{"host":["b.example.com"]}],"handle":[]}
			]}}}}}`,
			want: []string{"a.example.com", "b.example.com"},
		},
		{
			name: "multiple hosts in single match",
			json: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"@id":"kaji_multi","match":[{"host":["one.com","two.com"]}],"handle":[]}
			]}}}}}`,
			want: []string{"one.com", "two.com"},
		},
		{
			name: "duplicate domains are deduplicated",
			json: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"@id":"kaji_a","match":[{"host":["example.com"]}],"handle":[]},
				{"@id":"kaji_b","match":[{"host":["example.com"]}],"handle":[]}
			]}}}}}`,
			want: []string{"example.com"},
		},
		{
			name: "case-insensitive dedup",
			json: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"@id":"kaji_a","match":[{"host":["Example.COM"]}],"handle":[]},
				{"@id":"kaji_b","match":[{"host":["example.com"]}],"handle":[]}
			]}}}}}`,
			want: []string{"Example.COM"},
		},
		{
			name: "auto_https disabled skips routes without force-https subroute",
			json: `{"apps":{"http":{"servers":{"srv0":{
				"automatic_https":{"disable":true},
				"routes":[
					{"@id":"kaji_plain","match":[{"host":["plain.com"]}],"handle":[]}
				]
			}}}}}`,
			want: nil,
		},
		{
			name: "auto_https disabled but route has force-https subroute",
			json: `{"apps":{"http":{"servers":{"srv0":{
				"automatic_https":{"disable":true},
				"routes":[
					{"@id":"kaji_forced","match":[{"host":["forced.com"]}],"handle":[
						{"handler":"subroute","routes":[
							{"match":[{"protocol":"http"}],"handle":[]}
						]}
					]}
				]
			}}}}}`,
			want: []string{"forced.com"},
		},
		{
			name: "route without match is harmless",
			json: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"@id":"kaji_nomatch","handle":[]}
			]}}}}}`,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var input []byte
			if tc.json != "" {
				input = []byte(tc.json)
			}
			got := ExpectedCertDomains(input)
			if !strSliceEqual(got, tc.want) {
				t.Errorf("ExpectedCertDomains() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMergeMissingCerts(t *testing.T) {
	t.Run("no expected domains returns original list", func(t *testing.T) {
		certs := []CertInfo{{Domain: "a.com", Status: "valid", DaysLeft: 90}}
		got := MergeMissingCerts(certs, nil)
		if len(got) != 1 || got[0].Domain != "a.com" {
			t.Errorf("expected original list unchanged, got %v", got)
		}
	})

	t.Run("empty expected domains returns original list", func(t *testing.T) {
		certs := []CertInfo{{Domain: "a.com", Status: "valid", DaysLeft: 90}}
		got := MergeMissingCerts(certs, []string{})
		if len(got) != 1 || got[0].Domain != "a.com" {
			t.Errorf("expected original list unchanged, got %v", got)
		}
	})

	t.Run("domain already present is not added", func(t *testing.T) {
		certs := []CertInfo{{Domain: "a.com", SANs: []string{"a.com"}, Status: "valid", DaysLeft: 90}}
		got := MergeMissingCerts(certs, []string{"a.com"})
		if len(got) != 1 {
			t.Errorf("expected 1 cert, got %d", len(got))
		}
	})

	t.Run("domain present as SAN is not added", func(t *testing.T) {
		certs := []CertInfo{{Domain: "main.com", SANs: []string{"main.com", "alias.com"}, Status: "valid", DaysLeft: 90}}
		got := MergeMissingCerts(certs, []string{"alias.com"})
		if len(got) != 1 {
			t.Errorf("expected 1 cert, got %d", len(got))
		}
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		certs := []CertInfo{{Domain: "Example.COM", SANs: []string{"Example.COM"}, Status: "valid", DaysLeft: 90}}
		got := MergeMissingCerts(certs, []string{"example.com"})
		if len(got) != 1 {
			t.Errorf("expected case-insensitive match, got %d certs", len(got))
		}
	})

	t.Run("missing domain is added with correct fields", func(t *testing.T) {
		got := MergeMissingCerts(nil, []string{"new.com"})
		if len(got) != 1 {
			t.Fatalf("expected 1 cert, got %d", len(got))
		}
		c := got[0]
		if c.Domain != "new.com" {
			t.Errorf("Domain = %q, want new.com", c.Domain)
		}
		if c.Status != "missing" {
			t.Errorf("Status = %q, want missing", c.Status)
		}
		if c.DaysLeft != -1 {
			t.Errorf("DaysLeft = %d, want -1", c.DaysLeft)
		}
	})

	t.Run("missing certs are sorted before valid certs", func(t *testing.T) {
		certs := []CertInfo{
			{Domain: "ok.com", SANs: []string{"ok.com"}, Status: "valid", DaysLeft: 90},
		}
		got := MergeMissingCerts(certs, []string{"missing.com"})
		if len(got) != 2 {
			t.Fatalf("expected 2 certs, got %d", len(got))
		}
		if got[0].Status != "missing" {
			t.Errorf("first cert status = %q, want missing", got[0].Status)
		}
		if got[1].Status != "valid" {
			t.Errorf("second cert status = %q, want valid", got[1].Status)
		}
	})

	t.Run("mixed statuses maintain priority order", func(t *testing.T) {
		certs := []CertInfo{
			{Domain: "valid.com", SANs: []string{"valid.com"}, Status: "valid", DaysLeft: 90},
			{Domain: "expired.com", SANs: []string{"expired.com"}, Status: "expired", DaysLeft: 0},
		}
		got := MergeMissingCerts(certs, []string{"new.com"})
		if len(got) != 3 {
			t.Fatalf("expected 3 certs, got %d", len(got))
		}
		wantOrder := []string{"missing", "expired", "valid"}
		for i, want := range wantOrder {
			if got[i].Status != want {
				t.Errorf("cert[%d].Status = %q, want %q", i, got[i].Status, want)
			}
		}
	})
}

func TestDeleteCertificate(t *testing.T) {
	t.Run("deletes existing certificate directory", func(t *testing.T) {
		dataDir := t.TempDir()
		certDir := filepath.Join(dataDir, "certificates", "acme-v02", "example.com")
		if err := os.MkdirAll(certDir, 0o755); err != nil {
			t.Fatal(err)
		}
		certFile := filepath.Join(certDir, "example.com.crt")
		if err := os.WriteFile(certFile, []byte("cert"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := DeleteCertificate(dataDir, "acme-v02", "example.com"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(certDir); !os.IsNotExist(err) {
			t.Error("certificate directory should have been removed")
		}
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		dataDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dataDir, "certificates"), 0o755); err != nil {
			t.Fatal(err)
		}

		err := DeleteCertificate(dataDir, "acme-v02", "noexist.com")
		if err == nil {
			t.Fatal("expected error for non-existent directory")
		}
	})

	t.Run("rejects path traversal in issuer", func(t *testing.T) {
		dataDir := t.TempDir()
		err := DeleteCertificate(dataDir, "../../../etc", "passwd")
		if err == nil {
			t.Fatal("expected error for path traversal in issuer")
		}
	})

	t.Run("rejects path traversal in domain", func(t *testing.T) {
		dataDir := t.TempDir()
		err := DeleteCertificate(dataDir, "acme-v02", "../../etc/passwd")
		if err == nil {
			t.Fatal("expected error for path traversal in domain")
		}
	})
}

// writeSelfSignedCert generates a self-signed certificate and writes it to disk.
// Returns the path to the .crt file.
func writeSelfSignedCert(t *testing.T, dir, issuerKey, domain string, tmpl *x509.Certificate) string {
	t.Helper()

	certDir := filepath.Join(dir, "certificates", issuerKey, domain)
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	if tmpl.SerialNumber == nil {
		tmpl.SerialNumber = big.NewInt(1)
	}
	if tmpl.NotBefore.IsZero() {
		tmpl.NotBefore = time.Now().Add(-1 * time.Hour)
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	certFile := filepath.Join(certDir, domain+".crt")
	f, err := os.Create(certFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return certFile
}

func TestParseCertFileStatusValid(t *testing.T) {
	dir := t.TempDir()
	tmpl := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "valid.example.com"},
		DNSNames: []string{"valid.example.com", "alt.example.com"},
		NotAfter: time.Now().Add(90 * 24 * time.Hour),
	}
	writeSelfSignedCert(t, dir, "acme-v02.api.letsencrypt.org-directory", "valid.example.com", tmpl)

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("got %d certs, want 1", len(certs))
	}

	c := certs[0]
	if c.Domain != "valid.example.com" {
		t.Errorf("Domain = %q, want valid.example.com", c.Domain)
	}
	if c.Status != "valid" {
		t.Errorf("Status = %q, want valid", c.Status)
	}
	if c.DaysLeft < 89 {
		t.Errorf("DaysLeft = %d, expected ~90", c.DaysLeft)
	}
	if !c.Managed {
		t.Error("Managed should be true for letsencrypt issuer key")
	}
	if c.Fingerprint == "" {
		t.Error("Fingerprint should not be empty")
	}
	if len(c.SANs) != 2 || c.SANs[0] != "valid.example.com" || c.SANs[1] != "alt.example.com" {
		t.Errorf("SANs = %v, want [valid.example.com alt.example.com]", c.SANs)
	}
	if c.IssuerKey != "acme-v02.api.letsencrypt.org-directory" {
		t.Errorf("IssuerKey = %q, want acme-v02.api.letsencrypt.org-directory", c.IssuerKey)
	}
}

func TestParseCertFileStatusExpired(t *testing.T) {
	dir := t.TempDir()
	tmpl := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "expired.example.com"},
		DNSNames: []string{"expired.example.com"},
		NotAfter: time.Now().Add(-24 * time.Hour),
	}
	writeSelfSignedCert(t, dir, "acme-v02", "expired.example.com", tmpl)

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("got %d certs, want 1", len(certs))
	}
	if certs[0].Status != "expired" {
		t.Errorf("Status = %q, want expired", certs[0].Status)
	}
	if certs[0].DaysLeft != 0 {
		t.Errorf("DaysLeft = %d, want 0 for expired cert", certs[0].DaysLeft)
	}
}

func TestParseCertFileStatusExpiring(t *testing.T) {
	dir := t.TempDir()

	// Exactly at 30-day boundary
	tmpl := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "expiring.example.com"},
		DNSNames: []string{"expiring.example.com"},
		NotAfter: time.Now().Add(30 * 24 * time.Hour),
	}
	writeSelfSignedCert(t, dir, "acme-v02", "expiring.example.com", tmpl)

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("got %d certs, want 1", len(certs))
	}
	if certs[0].Status != "expiring" {
		t.Errorf("Status = %q, want expiring (at 30-day boundary)", certs[0].Status)
	}
}

func TestParseCertFileStatusValidAt31Days(t *testing.T) {
	dir := t.TempDir()
	tmpl := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "ok.example.com"},
		DNSNames: []string{"ok.example.com"},
		NotAfter: time.Now().Add(31*24*time.Hour + time.Hour),
	}
	writeSelfSignedCert(t, dir, "acme-v02", "ok.example.com", tmpl)

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if certs[0].Status != "valid" {
		t.Errorf("Status = %q, want valid (31 days out)", certs[0].Status)
	}
}

func TestParseCertFileDomainFallbackToDNSNames(t *testing.T) {
	dir := t.TempDir()
	// No CommonName, but has DNSNames
	tmpl := &x509.Certificate{
		DNSNames: []string{"dns-fallback.example.com"},
		NotAfter: time.Now().Add(90 * 24 * time.Hour),
	}
	writeSelfSignedCert(t, dir, "acme-v02", "dns-fallback.example.com", tmpl)

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("got %d certs, want 1", len(certs))
	}
	if certs[0].Domain != "dns-fallback.example.com" {
		t.Errorf("Domain = %q, want dns-fallback.example.com (fallback to DNSNames[0])", certs[0].Domain)
	}
}

func TestParseCertFileDomainFallbackToDirectoryName(t *testing.T) {
	dir := t.TempDir()
	// No CommonName, no DNSNames - should fall back to directory name
	tmpl := &x509.Certificate{
		NotAfter: time.Now().Add(90 * 24 * time.Hour),
	}
	writeSelfSignedCert(t, dir, "acme-v02", "dir-fallback.example.com", tmpl)

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("got %d certs, want 1", len(certs))
	}
	if certs[0].Domain != "dir-fallback.example.com" {
		t.Errorf("Domain = %q, want dir-fallback.example.com (fallback to directory name)", certs[0].Domain)
	}
}

func TestParseCertFileManagedFlag(t *testing.T) {
	cases := []struct {
		issuerKey string
		managed   bool
	}{
		{"acme-v02.api.letsencrypt.org-directory", true},
		{"acme.zerossl.com-v2-DV90", true},
		{"local", false},
		{"custom-ca", false},
		{"ACME-UPPERCASE", true},
	}

	for _, tc := range cases {
		t.Run(tc.issuerKey, func(t *testing.T) {
			dir := t.TempDir()
			tmpl := &x509.Certificate{
				Subject:  pkix.Name{CommonName: "test.com"},
				DNSNames: []string{"test.com"},
				NotAfter: time.Now().Add(90 * 24 * time.Hour),
			}
			writeSelfSignedCert(t, dir, tc.issuerKey, "test.com", tmpl)

			certs, err := ReadCertificates(dir)
			if err != nil {
				t.Fatalf("ReadCertificates: %v", err)
			}
			if len(certs) != 1 {
				t.Fatalf("got %d certs, want 1", len(certs))
			}
			if certs[0].Managed != tc.managed {
				t.Errorf("Managed = %v for issuerKey %q, want %v", certs[0].Managed, tc.issuerKey, tc.managed)
			}
		})
	}
}

func TestReadCertificatesSortOrder(t *testing.T) {
	dir := t.TempDir()

	// Valid cert (90 days)
	writeSelfSignedCert(t, dir, "acme", "valid.com", &x509.Certificate{
		Subject:  pkix.Name{CommonName: "valid.com"},
		DNSNames: []string{"valid.com"},
		NotAfter: time.Now().Add(90 * 24 * time.Hour),
	})
	// Expired cert
	writeSelfSignedCert(t, dir, "acme", "expired.com", &x509.Certificate{
		Subject:  pkix.Name{CommonName: "expired.com"},
		DNSNames: []string{"expired.com"},
		NotAfter: time.Now().Add(-24 * time.Hour),
	})
	// Expiring cert (15 days)
	writeSelfSignedCert(t, dir, "acme", "expiring.com", &x509.Certificate{
		Subject:  pkix.Name{CommonName: "expiring.com"},
		DNSNames: []string{"expiring.com"},
		NotAfter: time.Now().Add(15 * 24 * time.Hour),
	})

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if len(certs) != 3 {
		t.Fatalf("got %d certs, want 3", len(certs))
	}

	// Expected order: expired, expiring, valid
	wantOrder := []string{"expired", "expiring", "valid"}
	for i, want := range wantOrder {
		if certs[i].Status != want {
			t.Errorf("certs[%d].Status = %q, want %q", i, certs[i].Status, want)
		}
	}
}

func TestReadCertificatesNoCertsDir(t *testing.T) {
	dir := t.TempDir()

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if certs != nil {
		t.Errorf("expected nil for missing certificates dir, got %v", certs)
	}
}

func TestReadCertificatesSkipsFiles(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certificates")
	if err := os.MkdirAll(certsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a file (not a directory) inside certificates/
	if err := os.WriteFile(filepath.Join(certsDir, "not-a-dir"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(certs) != 0 {
		t.Errorf("expected 0 certs (file should be skipped), got %d", len(certs))
	}
}

func TestReadCertificatesSkipsBadPEM(t *testing.T) {
	dir := t.TempDir()
	certDir := filepath.Join(dir, "certificates", "acme", "bad.com")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "bad.com.crt"), []byte("not a PEM"), 0o644); err != nil {
		t.Fatal(err)
	}

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(certs) != 0 {
		t.Errorf("expected 0 certs (bad PEM should be skipped), got %d", len(certs))
	}
}

func TestReadCertificatesMultipleIssuers(t *testing.T) {
	dir := t.TempDir()

	writeSelfSignedCert(t, dir, "acme-v02", "a.com", &x509.Certificate{
		Subject:  pkix.Name{CommonName: "a.com"},
		DNSNames: []string{"a.com"},
		NotAfter: time.Now().Add(90 * 24 * time.Hour),
	})
	writeSelfSignedCert(t, dir, "local", "b.com", &x509.Certificate{
		Subject:  pkix.Name{CommonName: "b.com"},
		DNSNames: []string{"b.com"},
		NotAfter: time.Now().Add(90 * 24 * time.Hour),
	})

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if len(certs) != 2 {
		t.Fatalf("got %d certs, want 2", len(certs))
	}

	domains := map[string]bool{}
	for _, c := range certs {
		domains[c.Domain] = true
	}
	if !domains["a.com"] || !domains["b.com"] {
		t.Errorf("expected both a.com and b.com, got %v", domains)
	}
}

func TestParseCertFileFingerprint(t *testing.T) {
	dir := t.TempDir()
	tmpl := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "fp.example.com"},
		DNSNames: []string{"fp.example.com"},
		NotAfter: time.Now().Add(90 * 24 * time.Hour),
	}
	writeSelfSignedCert(t, dir, "acme", "fp.example.com", tmpl)

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("got %d certs, want 1", len(certs))
	}

	fp := certs[0].Fingerprint
	if len(fp) != 64 {
		t.Errorf("Fingerprint length = %d, want 64 hex chars (sha256)", len(fp))
	}
	for _, ch := range fp {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			t.Errorf("Fingerprint contains non-hex char %q", string(ch))
			break
		}
	}
}

func TestResolveCaddyDataDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CADDY_DATA_DIR", "")

	got := ResolveCaddyDataDir(dir)
	if got != dir {
		t.Errorf("ResolveCaddyDataDir(%q) = %q, want override to win", dir, got)
	}
}

func TestResolveCaddyDataDirOverrideNotExist(t *testing.T) {
	t.Setenv("CADDY_DATA_DIR", "")

	got := ResolveCaddyDataDir("/nonexistent/override/path")
	// Override doesn't exist, falls through to env/candidates/fallback
	if got == "/nonexistent/override/path" {
		t.Error("should not return override when directory doesn't exist")
	}
}

func TestResolveCaddyDataDirEnvVar(t *testing.T) {
	envDir := t.TempDir()
	t.Setenv("CADDY_DATA_DIR", envDir)

	got := ResolveCaddyDataDir("")
	if got != envDir {
		t.Errorf("ResolveCaddyDataDir() = %q, want env var %q", got, envDir)
	}
}

func TestResolveCaddyDataDirEnvVarNotExist(t *testing.T) {
	t.Setenv("CADDY_DATA_DIR", "/nonexistent/env/path")

	got := ResolveCaddyDataDir("")
	if got == "/nonexistent/env/path" {
		t.Error("should not return env var when directory doesn't exist")
	}
}

func TestResolveCaddyDataDirFallback(t *testing.T) {
	t.Setenv("CADDY_DATA_DIR", "")

	// With no override, no env var, and no OS candidates existing,
	// it should return the fallback /data/caddy
	got := ResolveCaddyDataDir("")
	if got != "/data/caddy" {
		// On macOS, ~/Library/Application Support/Caddy or ~/.local/share/caddy
		// might actually exist. Only check fallback if none of the candidates exist.
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir: %v", err)
		}
		candidates := []string{
			"/data/caddy",
			filepath.Join(home, "Library", "Application Support", "Caddy"),
			filepath.Join(home, ".local", "share", "caddy"),
		}
		found := false
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && info.IsDir() {
				found = true
				if got != c {
					t.Errorf("ResolveCaddyDataDir() = %q, expected candidate %q", got, c)
				}
				break
			}
		}
		if !found && got != "/data/caddy" {
			t.Errorf("ResolveCaddyDataDir() = %q, want fallback /data/caddy", got)
		}
	}
}

func TestResolveCaddyDataDirOverrideBeatsEnv(t *testing.T) {
	overrideDir := t.TempDir()
	envDir := t.TempDir()
	t.Setenv("CADDY_DATA_DIR", envDir)

	got := ResolveCaddyDataDir(overrideDir)
	if got != overrideDir {
		t.Errorf("ResolveCaddyDataDir() = %q, want override %q to beat env %q", got, overrideDir, envDir)
	}
}

func TestResolveCaddyDataDirOverrideIsFile(t *testing.T) {
	// If the override path exists but is a file (not a directory), skip it
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CADDY_DATA_DIR", "")

	got := ResolveCaddyDataDir(filePath)
	if got == filePath {
		t.Error("should not return override when path is a file, not a directory")
	}
}

func TestCertFilePath(t *testing.T) {
	want := filepath.Join("/data/caddy", "certificates", "acme", "example.com", "example.com.crt")
	got := CertFilePath("/data/caddy", "acme", "example.com")
	if got != want {
		t.Errorf("CertFilePath = %q, want %q", got, want)
	}
}

func TestStatusPriority(t *testing.T) {
	cases := []struct {
		status string
		want   int
	}{
		{"missing", -1},
		{"expired", 0},
		{"expiring", 1},
		{"valid", 3},
		{"unknown", 4},
	}
	for _, tc := range cases {
		got := statusPriority(tc.status)
		if got != tc.want {
			t.Errorf("statusPriority(%q) = %d, want %d", tc.status, got, tc.want)
		}
	}
}

func TestParseCertFileNotAfterFormat(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	notAfter := now.Add(60 * 24 * time.Hour)
	tmpl := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "time.example.com"},
		DNSNames: []string{"time.example.com"},
		NotAfter: notAfter,
	}
	writeSelfSignedCert(t, dir, "acme", "time.example.com", tmpl)

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("got %d certs, want 1", len(certs))
	}

	// Verify NotBefore and NotAfter are in RFC3339 format
	if _, err := time.Parse(time.RFC3339, certs[0].NotBefore); err != nil {
		t.Errorf("NotBefore %q is not valid RFC3339: %v", certs[0].NotBefore, err)
	}
	if _, err := time.Parse(time.RFC3339, certs[0].NotAfter); err != nil {
		t.Errorf("NotAfter %q is not valid RFC3339: %v", certs[0].NotAfter, err)
	}
}

func TestReadCertificatesIssuerField(t *testing.T) {
	dir := t.TempDir()
	tmpl := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "issuer.example.com"},
		Issuer:   pkix.Name{CommonName: "Test CA"},
		DNSNames: []string{"issuer.example.com"},
		NotAfter: time.Now().Add(90 * 24 * time.Hour),
	}
	writeSelfSignedCert(t, dir, "acme", "issuer.example.com", tmpl)

	certs, err := ReadCertificates(dir)
	if err != nil {
		t.Fatalf("ReadCertificates: %v", err)
	}
	// Self-signed, so issuer CN = subject CN (CreateCertificate uses the parent's subject)
	if certs[0].Issuer != "issuer.example.com" {
		t.Errorf("Issuer = %q, want issuer.example.com (self-signed)", certs[0].Issuer)
	}
}

func TestReadCertificatesDaysLeftConsistentWithStatus(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name     string
		domain   string
		notAfter time.Duration
		status   string
	}{
		{"5-days", "five.com", 5 * 24 * time.Hour, "expiring"},
		{"29-days", "twentynine.com", 29 * 24 * time.Hour, "expiring"},
		{"0-days-expired", "zero.com", -1 * time.Hour, "expired"},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subdir := t.TempDir()
			tmpl := &x509.Certificate{
				SerialNumber: big.NewInt(int64(i + 10)),
				Subject:      pkix.Name{CommonName: tc.domain},
				DNSNames:     []string{tc.domain},
				NotAfter:     time.Now().Add(tc.notAfter),
			}
			writeSelfSignedCert(t, subdir, "acme", tc.domain, tmpl)
			certs, err := ReadCertificates(subdir)
			if err != nil {
				t.Fatalf("ReadCertificates: %v", err)
			}
			if len(certs) != 1 {
				t.Fatalf("got %d certs, want 1", len(certs))
			}
			if certs[0].Status != tc.status {
				t.Errorf("Status = %q, want %q (DaysLeft=%d)", certs[0].Status, tc.status, certs[0].DaysLeft)
			}
		})
	}
	_ = fmt.Sprint(dir) // silence unused
}

func strSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
