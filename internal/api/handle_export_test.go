package api

import (
	"encoding/json"
	"testing"

	"github.com/andretakagi/kaji/internal/config"
)

func TestExtractUpstream(t *testing.T) {
	tests := []struct {
		name string
		rule config.Rule
		want string
	}{
		{
			name: "reverse_proxy",
			rule: config.Rule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: json.RawMessage(`{"upstream":"http://localhost:8080"}`),
			},
			want: "http://localhost:8080",
		},
		{
			name: "not_reverse_proxy",
			rule: config.Rule{
				HandlerType:   "static_response",
				HandlerConfig: json.RawMessage(`{"status_code":200}`),
			},
			want: "",
		},
		{
			name: "invalid_json",
			rule: config.Rule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: json.RawMessage(`not json`),
			},
			want: "",
		},
		{
			name: "empty_config",
			rule: config.Rule{
				HandlerType:   "reverse_proxy",
				HandlerConfig: json.RawMessage(`{}`),
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractUpstream(tt.rule); got != tt.want {
				t.Errorf("extractUpstream() = %q, want %q", got, tt.want)
			}
		})
	}
}
