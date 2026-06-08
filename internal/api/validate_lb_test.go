package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andretakagi/kaji/internal/caddy"
)

func TestValidateLoadBalancing(t *testing.T) {
	tests := []struct {
		name    string
		lb      caddy.LoadBalancing
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "disabled always passes",
			lb:     caddy.LoadBalancing{Enabled: false, Strategy: "nonsense"},
			wantOK: true,
		},
		{
			name:    "unknown strategy rejected",
			lb:      caddy.LoadBalancing{Enabled: true, Strategy: "weighted", Upstreams: []string{"localhost:8081"}},
			wantOK:  false,
			wantMsg: "load balancing strategy must be one of",
		},
		{
			name:    "missing additional upstream rejected",
			lb:      caddy.LoadBalancing{Enabled: true, Strategy: "round_robin"},
			wantOK:  false,
			wantMsg: "at least one additional upstream",
		},
		{
			name: "weighted valid with matching weights",
			lb: caddy.LoadBalancing{
				Enabled:   true,
				Strategy:  "weighted_round_robin",
				Upstreams: []string{"localhost:8081"},
				Weights:   []int{2, 1},
			},
			wantOK: true,
		},
		{
			name: "weighted valid with no weights",
			lb: caddy.LoadBalancing{
				Enabled:   true,
				Strategy:  "weighted_round_robin",
				Upstreams: []string{"localhost:8081"},
			},
			wantOK: true,
		},
		{
			name: "weighted rejects wrong weight count",
			lb: caddy.LoadBalancing{
				Enabled:   true,
				Strategy:  "weighted_round_robin",
				Upstreams: []string{"localhost:8081"},
				Weights:   []int{2},
			},
			wantOK:  false,
			wantMsg: "one weight per upstream",
		},
		{
			name: "weighted rejects zero weight",
			lb: caddy.LoadBalancing{
				Enabled:   true,
				Strategy:  "weighted_round_robin",
				Upstreams: []string{"localhost:8081"},
				Weights:   []int{1, 0},
			},
			wantOK:  false,
			wantMsg: "weights must be 1 or greater",
		},
		{
			name: "query requires key",
			lb: caddy.LoadBalancing{
				Enabled:   true,
				Strategy:  "query",
				Upstreams: []string{"localhost:8081"},
			},
			wantOK:  false,
			wantMsg: "query parameter name",
		},
		{
			name: "query valid with key",
			lb: caddy.LoadBalancing{
				Enabled:   true,
				Strategy:  "query",
				Upstreams: []string{"localhost:8081"},
				Key:       "session",
			},
			wantOK: true,
		},
		{
			name: "header requires key",
			lb: caddy.LoadBalancing{
				Enabled:   true,
				Strategy:  "header",
				Upstreams: []string{"localhost:8081"},
			},
			wantOK:  false,
			wantMsg: "header field name",
		},
		{
			name: "cookie valid without key",
			lb: caddy.LoadBalancing{
				Enabled:   true,
				Strategy:  "cookie",
				Upstreams: []string{"localhost:8081"},
			},
			wantOK: true,
		},
		{
			name: "cookie rejects overlong secret",
			lb: caddy.LoadBalancing{
				Enabled:   true,
				Strategy:  "cookie",
				Upstreams: []string{"localhost:8081"},
				Secret:    strings.Repeat("x", 257),
			},
			wantOK:  false,
			wantMsg: "cookie secret is too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ok := validateLoadBalancing(rec, tt.lb)
			if ok != tt.wantOK {
				t.Fatalf("validateLoadBalancing = %v, want %v (body: %s)", ok, tt.wantOK, rec.Body.String())
			}
			if !tt.wantOK && !strings.Contains(rec.Body.String(), tt.wantMsg) {
				t.Errorf("error body = %q, want substring %q", rec.Body.String(), tt.wantMsg)
			}
		})
	}
}
