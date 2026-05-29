package api

import (
	"net/http/httptest"
	"testing"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
)

func TestPathLabel(t *testing.T) {
	tests := []struct {
		host, match, want string
	}{
		{"example.com", "/api", "example.com/api"},
		{"example.com", "/api/*", "example.com/api/*"},
		{"example.com", "", "example.com (path)"},
	}
	for _, tt := range tests {
		if got := pathLabel(tt.host, tt.match); got != tt.want {
			t.Errorf("pathLabel(%q, %q) = %q, want %q", tt.host, tt.match, got, tt.want)
		}
	}
}

func TestDomainsUsingForwardAuth(t *testing.T) {
	forwardAuth := caddy.DomainToggles{Auth: caddy.AuthToggle{Mode: "forward"}}
	basicAuth := caddy.DomainToggles{Auth: caddy.AuthToggle{Mode: "basic"}}
	forwardOverride := &caddy.DomainToggles{Auth: caddy.AuthToggle{Mode: "forward"}}

	tests := []struct {
		name    string
		domains []config.Domain
		want    []string
	}{
		{
			name:    "no_domains",
			domains: nil,
			want:    nil,
		},
		{
			name: "no_forward_auth",
			domains: []config.Domain{
				{Name: "example.com", Toggles: basicAuth},
			},
			want: nil,
		},
		{
			name: "domain_level",
			domains: []config.Domain{
				{Name: "example.com", Toggles: forwardAuth},
			},
			want: []string{"example.com"},
		},
		{
			name: "subdomain_level",
			domains: []config.Domain{
				{
					Name:    "example.com",
					Toggles: basicAuth,
					Subdomains: []config.Subdomain{
						{Name: "app", Toggles: forwardAuth},
					},
				},
			},
			want: []string{"app.example.com"},
		},
		{
			name: "domain_path_override",
			domains: []config.Domain{
				{
					Name:    "example.com",
					Toggles: basicAuth,
					Paths: []config.Path{
						{MatchValue: "/admin", ToggleOverrides: forwardOverride},
					},
				},
			},
			want: []string{"example.com/admin"},
		},
		{
			name: "subdomain_path_override",
			domains: []config.Domain{
				{
					Name:    "example.com",
					Toggles: basicAuth,
					Subdomains: []config.Subdomain{
						{
							Name:    "app",
							Toggles: basicAuth,
							Paths: []config.Path{
								{MatchValue: "/api", ToggleOverrides: forwardOverride},
							},
						},
					},
				},
			},
			want: []string{"app.example.com/api"},
		},
		{
			name: "path_without_match_value",
			domains: []config.Domain{
				{
					Name:    "example.com",
					Toggles: basicAuth,
					Paths: []config.Path{
						{MatchValue: "", ToggleOverrides: forwardOverride},
					},
				},
			},
			want: []string{"example.com (path)"},
		},
		{
			name: "mixed_all_levels",
			domains: []config.Domain{
				{Name: "a.com", Toggles: forwardAuth},
				{
					Name:    "b.com",
					Toggles: basicAuth,
					Subdomains: []config.Subdomain{
						{Name: "sub", Toggles: forwardAuth},
					},
					Paths: []config.Path{
						{MatchValue: "/secret", ToggleOverrides: forwardOverride},
					},
				},
			},
			want: []string{"a.com", "sub.b.com", "b.com/secret"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Domains = tt.domains
			store := config.NewStore(cfg)

			got := domainsUsingForwardAuth(store)

			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", got)
				}
				return
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %v (len %d), want %v (len %d)", got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateAuthMode(t *testing.T) {
	t.Run("valid_modes", func(t *testing.T) {
		cfg := config.DefaultConfig()
		store := config.NewStore(cfg)

		for _, mode := range []string{"", "off", "basic"} {
			rec := httptest.NewRecorder()
			auth := &caddy.AuthToggle{Mode: mode}
			if !validateAuthMode(rec, store, auth, "") {
				t.Errorf("mode %q rejected, want accepted", mode)
			}
		}
	})

	t.Run("forward_when_enabled", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.ForwardAuth = caddy.ForwardAuthConfig{Enabled: true, Provider: "authelia", URL: "https://auth.example.com"}
		store := config.NewStore(cfg)

		rec := httptest.NewRecorder()
		auth := &caddy.AuthToggle{Mode: "forward"}
		if !validateAuthMode(rec, store, auth, "") {
			t.Error("forward mode rejected when globally enabled")
		}
	})

	t.Run("forward_when_disabled", func(t *testing.T) {
		cfg := config.DefaultConfig()
		store := config.NewStore(cfg)

		rec := httptest.NewRecorder()
		auth := &caddy.AuthToggle{Mode: "forward"}
		if validateAuthMode(rec, store, auth, "test: ") {
			t.Fatal("forward mode accepted when not globally enabled")
		}
		if rec.Code != 400 {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("invalid_mode", func(t *testing.T) {
		cfg := config.DefaultConfig()
		store := config.NewStore(cfg)

		rec := httptest.NewRecorder()
		auth := &caddy.AuthToggle{Mode: "oauth"}
		if validateAuthMode(rec, store, auth, "test: ") {
			t.Fatal("invalid mode accepted")
		}
		if rec.Code != 400 {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}
