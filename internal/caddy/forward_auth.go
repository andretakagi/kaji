package caddy

import (
	"fmt"
	"net/url"
)

// Header presets for forward auth providers. On a 2xx response from the auth
// service, these headers are copied from the auth response into the upstream
// request so the backend knows who the user is.
var forwardAuthHeaderPresets = map[string][]string{
	"authelia": {
		"Remote-User",
		"Remote-Groups",
		"Remote-Email",
		"Remote-Name",
	},
	"authentik": {
		"X-authentik-username",
		"X-authentik-groups",
		"X-authentik-email",
		"X-authentik-name",
		"X-authentik-uid",
	},
	"custom": nil,
}

// ForwardAuthPresetHeaders returns the identity headers for a provider, or nil
// for "custom" / unknown providers.
func ForwardAuthPresetHeaders(provider string) []string {
	return forwardAuthHeaderPresets[provider]
}

// buildForwardAuthHandler builds a Caddy reverse_proxy handler that implements
// forward auth. The handler proxies to the auth service's verification
// endpoint. On 2xx, identity headers are copied into the upstream request and
// processing continues. On non-2xx, the auth service's response (status +
// headers) is returned to the client, which handles login redirects.
func buildForwardAuthHandler(cfg ForwardAuthConfig) (map[string]any, error) {
	parsed, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing forward auth URL: %w", err)
	}

	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	dial := host + ":" + port

	handler := map[string]any{
		"handler":   "reverse_proxy",
		"upstreams": []map[string]string{{"dial": dial}},
		"headers": map[string]any{
			"request": map[string]any{
				"set": map[string][]string{
					"X-Forwarded-Method": {"{http.request.method}"},
					"X-Forwarded-URI":    {"{http.request.uri}"},
				},
			},
		},
		"rewrite": map[string]any{
			"method": "{http.request.method}",
			"uri":    parsed.Path,
		},
	}

	if parsed.Scheme == "https" {
		handler["transport"] = map[string]any{
			"protocol": "http",
			"tls":      map[string]any{},
		}
	}

	// Build the 2xx match route: copy identity headers into the upstream request
	var successRouteHandlers []any
	headers := ForwardAuthPresetHeaders(cfg.Provider)
	if len(headers) > 0 {
		setMap := make(map[string][]string, len(headers))
		for _, h := range headers {
			setMap[h] = []string{"{http.reverse_proxy.header." + h + "}"}
		}
		successRouteHandlers = append(successRouteHandlers, map[string]any{
			"handler": "headers",
			"request": map[string]any{
				"set": setMap,
			},
		})
	}

	successResponse := map[string]any{
		"match": map[string]any{
			"status_code": []int{2},
		},
	}
	if len(successRouteHandlers) > 0 {
		successResponse["routes"] = []map[string]any{
			{"handle": successRouteHandlers},
		}
	}

	// Non-2xx fallback: copy the auth service's response back to the client
	failResponse := map[string]any{
		"routes": []map[string]any{
			{
				"handle": []map[string]any{
					{"handler": "copy_response", "status_code": "{http.reverse_proxy.status_code}"},
					{"handler": "copy_response_headers"},
				},
			},
		},
	}

	handler["handle_response"] = []map[string]any{successResponse, failResponse}

	return handler, nil
}
