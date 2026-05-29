package api

import (
	"errors"
	"testing"
)

func TestCleanDNSProviderError(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want string
	}{
		{
			name: "cloudflare_in_json",
			err:  `caddy rejected config (status 400): {"error":"loading config: provision dns.providers.cloudflare: invalid API token"}`,
			want: "invalid API token",
		},
		{
			name: "cloudflare_without_json",
			err:  "provision dns.providers.cloudflare: token expired",
			want: "token expired",
		},
		{
			name: "generic_provider",
			err:  "provision dns.providers.route53: bad credentials",
			want: "bad credentials",
		},
		{
			name: "generic_provider_in_json",
			err:  `caddy rejected (400): {"error":"loading config: provision dns.providers.duckdns: missing token"}`,
			want: "missing token",
		},
		{
			name: "no_match_fallback",
			err:  "connection refused",
			want: "could not configure DNS challenge provider",
		},
		{
			name: "json_without_provider_chain",
			err:  `something: {"error":"unrelated caddy error"}`,
			want: "could not configure DNS challenge provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanDNSProviderError(errors.New(tt.err)); got != tt.want {
				t.Errorf("cleanDNSProviderError() = %q, want %q", got, tt.want)
			}
		})
	}
}
