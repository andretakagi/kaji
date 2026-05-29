package api

import (
	"testing"

	"github.com/andretakagi/kaji/internal/config"
)

func TestMapsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b map[string]string
		want bool
	}{
		{"both_nil", nil, nil, true},
		{"nil_and_empty", nil, map[string]string{}, true},
		{"equal", map[string]string{"a": "1"}, map[string]string{"a": "1"}, true},
		{"different_value", map[string]string{"a": "1"}, map[string]string{"a": "2"}, false},
		{"different_length", map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}, false},
		{"different_key", map[string]string{"a": "1"}, map[string]string{"b": "1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapsEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("mapsEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestOnlySinksChanged(t *testing.T) {
	base := config.LokiConfig{
		Enabled:              true,
		Endpoint:             "http://loki:3100",
		BearerToken:          "tok",
		TenantID:             "t1",
		BatchSize:            100,
		FlushIntervalSeconds: 5,
		Labels:               map[string]string{"env": "prod"},
		Sinks:                []string{"access"},
	}

	tests := []struct {
		name   string
		modify func(config.LokiConfig) config.LokiConfig
		want   bool
	}{
		{
			name:   "only_sinks_differ",
			modify: func(c config.LokiConfig) config.LokiConfig { c.Sinks = []string{"access", "error"}; return c },
			want:   true,
		},
		{
			name:   "everything_same",
			modify: func(c config.LokiConfig) config.LokiConfig { return c },
			want:   true,
		},
		{
			name:   "endpoint_differs",
			modify: func(c config.LokiConfig) config.LokiConfig { c.Endpoint = "http://other:3100"; return c },
			want:   false,
		},
		{
			name:   "enabled_differs",
			modify: func(c config.LokiConfig) config.LokiConfig { c.Enabled = false; return c },
			want:   false,
		},
		{
			name:   "bearer_token_differs",
			modify: func(c config.LokiConfig) config.LokiConfig { c.BearerToken = "new"; return c },
			want:   false,
		},
		{
			name:   "tenant_id_differs",
			modify: func(c config.LokiConfig) config.LokiConfig { c.TenantID = "t2"; return c },
			want:   false,
		},
		{
			name:   "batch_size_differs",
			modify: func(c config.LokiConfig) config.LokiConfig { c.BatchSize = 200; return c },
			want:   false,
		},
		{
			name:   "flush_interval_differs",
			modify: func(c config.LokiConfig) config.LokiConfig { c.FlushIntervalSeconds = 10; return c },
			want:   false,
		},
		{
			name:   "labels_differ",
			modify: func(c config.LokiConfig) config.LokiConfig { c.Labels = map[string]string{"env": "dev"}; return c },
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newCfg := tt.modify(base)
			if got := onlySinksChanged(base, newCfg); got != tt.want {
				t.Errorf("onlySinksChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}
