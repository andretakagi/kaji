package api

import (
	"strings"
	"testing"
)

func TestValidateDomain(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"valid single label would fail", "example", "invalid domain address"},
		{"valid two labels", "example.com", ""},
		{"valid subdomain", "sub.example.com", ""},
		{"valid wildcard", "*.example.com", ""},
		{"empty", "", "domain is required"},
		{"wildcard only", "*.", "domain is required"},
		{"too long", strings.Repeat("a", 254), "domain name too long"},
		{"control character", "exa\x01mple.com", "domain contains control characters"},
		{"del character", "exa\x7fmple.com", "domain contains control characters"},
		{"empty label trailing dot", "example.com.", "domain has an empty label"},
		{"empty label double dot", "example..com", "domain has an empty label"},
		{"label too long", "example." + strings.Repeat("a", 64) + ".com", "domain label too long (max 63 characters)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateDomain(tc.input)
			if got != tc.want {
				t.Errorf("validateDomain(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateUpstream(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"valid ip port", "127.0.0.1:8080", ""},
		{"valid hostname port", "myservice:3000", ""},
		{"empty", "", "upstream is required"},
		{"no port", "127.0.0.1", "upstream must be host:port (e.g. 127.0.0.1:8080 or myservice:3000)"},
		{"empty host", ":8080", "upstream host is empty"},
		{"port zero", "localhost:0", "upstream port must be a number between 1 and 65535"},
		{"port too large", "localhost:65536", "upstream port must be a number between 1 and 65535"},
		{"port not a number", "localhost:abc", "upstream port must be a number between 1 and 65535"},
		{"port min valid", "localhost:1", ""},
		{"port max valid", "localhost:65535", ""},
		{"ipv6", "[::1]:9000", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateUpstream(tc.input)
			if got != tc.want {
				t.Errorf("validateUpstream(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"valid", "user@example.com", ""},
		{"valid subdomain", "user@mail.example.com", ""},
		{"empty", "", "email is required"},
		{"too long", strings.Repeat("a", 255), "email is too long"},
		{"control character", "use\x01r@example.com", "email contains control characters"},
		{"del character", "user\x7f@example.com", "email contains control characters"},
		{"no at sign", "userexample.com", "invalid email format"},
		{"at at start", "@example.com", "invalid email format"},
		{"no dot in domain", "user@example", "invalid email format"},
		{"trailing dot in domain", "user@example.", "invalid email format"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateEmail(tc.input)
			if got != tc.want {
				t.Errorf("validateEmail(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateCaddyAdminURL(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"valid http", "http://localhost:2019", ""},
		{"valid https", "https://localhost:2019", ""},
		{"valid http with path", "http://localhost:2019/admin", ""},
		{"empty", "", "Caddy admin URL must not be empty"},
		{"no scheme", "localhost:2019", "invalid Caddy admin URL: must be http or https with a valid host"},
		{"ftp scheme", "ftp://localhost:2019", "invalid Caddy admin URL: must be http or https with a valid host"},
		{"no host", "http://", "invalid Caddy admin URL: must be http or https with a valid host"},
		{"scheme only", "http://", "invalid Caddy admin URL: must be http or https with a valid host"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateCaddyAdminURL(tc.input)
			if got != tc.want {
				t.Errorf("validateCaddyAdminURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateServerName(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"valid letters", "myserver", ""},
		{"valid with digits", "server1", ""},
		{"valid with hyphen", "my-server", ""},
		{"valid with underscore", "my_server", ""},
		{"valid mixed", "My_Server-01", ""},
		{"empty", "", "server name is required"},
		{"too long", strings.Repeat("a", 65), "server name too long (max 64 characters)"},
		{"max length valid", strings.Repeat("a", 64), ""},
		{"space", "my server", "server name must contain only letters, digits, hyphens, and underscores"},
		{"dot", "my.server", "server name must contain only letters, digits, hyphens, and underscores"},
		{"at sign", "my@server", "server name must contain only letters, digits, hyphens, and underscores"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateServerName(tc.input)
			if got != tc.want {
				t.Errorf("validateServerName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateAutoHTTPS(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"on", "on", ""},
		{"off", "off", ""},
		{"disable_redirects", "disable_redirects", ""},
		{"empty", "", "auto_https must be one of: on, off, disable_redirects"},
		{"uppercase ON", "ON", "auto_https must be one of: on, off, disable_redirects"},
		{"invalid value", "auto", "auto_https must be one of: on, off, disable_redirects"},
		{"true", "true", "auto_https must be one of: on, off, disable_redirects"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateAutoHTTPS(tc.input)
			if got != tc.want {
				t.Errorf("validateAutoHTTPS(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateLBStrategy(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"round_robin", "round_robin", ""},
		{"first", "first", ""},
		{"least_conn", "least_conn", ""},
		{"random", "random", ""},
		{"ip_hash", "ip_hash", ""},
		{"empty", "", "load balancing strategy must be one of: round_robin, first, least_conn, random, ip_hash"},
		{"uppercase", "ROUND_ROBIN", "load balancing strategy must be one of: round_robin, first, least_conn, random, ip_hash"},
		{"invalid", "weighted", "load balancing strategy must be one of: round_robin, first, least_conn, random, ip_hash"},
		{"partial match", "round", "load balancing strategy must be one of: round_robin, first, least_conn, random, ip_hash"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateLBStrategy(tc.input)
			if got != tc.want {
				t.Errorf("validateLBStrategy(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
