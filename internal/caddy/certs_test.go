package caddy

import (
	"os"
	"path/filepath"
	"testing"
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
