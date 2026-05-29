package api

import (
	"testing"
)

func TestConfigsUsingDomain(t *testing.T) {
	tests := []struct {
		name   string
		config string
		domain string
		want   []string
	}{
		{
			name: "single_match",
			config: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"match":[{"host":["example.com"]}]}
			]}}}}}`,
			domain: "example.com",
			want:   []string{"example.com"},
		},
		{
			name: "no_match",
			config: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"match":[{"host":["other.com"]}]}
			]}}}}}`,
			domain: "example.com",
			want:   []string{},
		},
		{
			name: "multi_host_route",
			config: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"match":[{"host":["example.com","www.example.com"]}]}
			]}}}}}`,
			domain: "example.com",
			want:   []string{"example.com, www.example.com"},
		},
		{
			name: "multiple_matching_routes",
			config: `{"apps":{"http":{"servers":{"srv0":{"routes":[
				{"match":[{"host":["example.com"]}]},
				{"match":[{"host":["example.com","api.example.com"]}]}
			]}}}}}`,
			domain: "example.com",
			want:   []string{"example.com", "example.com, api.example.com"},
		},
		{
			name: "across_servers",
			config: `{"apps":{"http":{"servers":{
				"srv0":{"routes":[{"match":[{"host":["example.com"]}]}]},
				"srv1":{"routes":[{"match":[{"host":["example.com","internal.example.com"]}]}]}
			}}}}`,
			domain: "example.com",
			want:   []string{"example.com", "example.com, internal.example.com"},
		},
		{
			name:   "invalid_json",
			config: `not json`,
			domain: "example.com",
			want:   nil,
		},
		{
			name:   "empty_config",
			config: `{"apps":{"http":{"servers":{}}}}`,
			domain: "example.com",
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configsUsingDomain([]byte(tt.config), tt.domain)

			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", got)
				}
				return
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %v (len %d), want %v (len %d)", got, len(got), tt.want, len(tt.want))
			}

			// Map iteration order is non-deterministic for across_servers,
			// so check that all expected values are present.
			remaining := make(map[string]bool, len(tt.want))
			for _, w := range tt.want {
				remaining[w] = true
			}
			for _, g := range got {
				if !remaining[g] {
					t.Errorf("unexpected result entry %q", g)
				}
				delete(remaining, g)
			}
			for w := range remaining {
				t.Errorf("missing expected entry %q", w)
			}
		})
	}
}
