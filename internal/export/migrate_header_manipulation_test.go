package export

import "testing"

func TestMigrateHeaderManipulation_RenamesRequestHeaders(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"rule": map[string]any{
					"handler_type": "reverse_proxy",
					"handler_config": map[string]any{
						"upstream":        "localhost:3000",
						"request_headers": map[string]any{"enabled": true},
					},
				},
				"toggles":    map[string]any{},
				"subdomains": []any{},
				"paths":      []any{},
			},
		},
	}

	migrateHeaderManipulation(config)

	hc := config["domains"].([]any)[0].(map[string]any)["rule"].(map[string]any)["handler_config"].(map[string]any)
	if _, ok := hc["request_headers"]; ok {
		t.Error("request_headers should have been renamed")
	}
	if _, ok := hc["header_up"]; !ok {
		t.Error("header_up should exist after rename")
	}
}

func TestMigrateHeaderManipulation_AddsHeaderDownDefault(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"rule": map[string]any{
					"handler_type": "reverse_proxy",
					"handler_config": map[string]any{
						"upstream": "localhost:3000",
					},
				},
				"toggles":    map[string]any{},
				"subdomains": []any{},
				"paths":      []any{},
			},
		},
	}

	migrateHeaderManipulation(config)

	hc := config["domains"].([]any)[0].(map[string]any)["rule"].(map[string]any)["handler_config"].(map[string]any)
	down, ok := hc["header_down"].(map[string]any)
	if !ok {
		t.Fatal("header_down should be set")
	}
	if down["enabled"] != false {
		t.Error("header_down.enabled should default false")
	}
	if down["strip_server"] != false {
		t.Error("header_down.strip_server should default false")
	}
	if down["deferred"] != false {
		t.Error("header_down.deferred should default false")
	}
}

func TestMigrateHeaderManipulation_PreservesExistingHeaderDown(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"rule": map[string]any{
					"handler_type": "reverse_proxy",
					"handler_config": map[string]any{
						"upstream": "localhost:3000",
						"header_down": map[string]any{
							"enabled":      true,
							"strip_server": true,
						},
					},
				},
				"toggles":    map[string]any{},
				"subdomains": []any{},
				"paths":      []any{},
			},
		},
	}

	migrateHeaderManipulation(config)

	hc := config["domains"].([]any)[0].(map[string]any)["rule"].(map[string]any)["handler_config"].(map[string]any)
	down := hc["header_down"].(map[string]any)
	if down["enabled"] != true {
		t.Error("existing header_down.enabled should be preserved")
	}
	if down["strip_server"] != true {
		t.Error("existing header_down.strip_server should be preserved")
	}
}

func TestMigrateHeaderManipulation_BackfillsOperation(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"rule": map[string]any{
					"handler_type": "reverse_proxy",
					"handler_config": map[string]any{
						"upstream": "localhost:3000",
						"header_up": map[string]any{
							"enabled": true,
							"builtin": []any{
								map[string]any{"key": "X-Custom", "value": "foo", "enabled": true},
							},
							"custom": []any{
								map[string]any{"key": "X-Other", "value": "bar", "enabled": true, "operation": "delete"},
							},
						},
					},
				},
				"toggles":    map[string]any{},
				"subdomains": []any{},
				"paths":      []any{},
			},
		},
	}

	migrateHeaderManipulation(config)

	hc := config["domains"].([]any)[0].(map[string]any)["rule"].(map[string]any)["handler_config"].(map[string]any)
	up := hc["header_up"].(map[string]any)

	builtin := up["builtin"].([]any)
	entry := builtin[0].(map[string]any)
	if entry["operation"] != "set" {
		t.Errorf("builtin entry missing operation backfill, got %v", entry["operation"])
	}

	custom := up["custom"].([]any)
	customEntry := custom[0].(map[string]any)
	if customEntry["operation"] != "delete" {
		t.Error("existing operation should not be overwritten")
	}
}

func TestMigrateHeaderManipulation_AddsRequestHeadersToToggles(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"rule": map[string]any{
					"handler_type":   "none",
					"handler_config": map[string]any{},
				},
				"toggles": map[string]any{
					"headers": map[string]any{
						"response": map[string]any{
							"enabled": false,
							"builtin": []any{},
							"custom":  []any{},
						},
					},
				},
				"subdomains": []any{},
				"paths":      []any{},
			},
		},
	}

	migrateHeaderManipulation(config)

	toggles := config["domains"].([]any)[0].(map[string]any)["toggles"].(map[string]any)
	headers := toggles["headers"].(map[string]any)
	req, ok := headers["request"].(map[string]any)
	if !ok {
		t.Fatal("headers.request should be added")
	}
	if req["enabled"] != false {
		t.Error("headers.request.enabled should be false")
	}
	if req["x_forwarded_for"] != false {
		t.Error("headers.request.x_forwarded_for should be false")
	}
}

func TestMigrateHeaderManipulation_PreservesExistingRequestHeaders(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"rule": map[string]any{
					"handler_type":   "none",
					"handler_config": map[string]any{},
				},
				"toggles": map[string]any{
					"headers": map[string]any{
						"request": map[string]any{
							"enabled":   true,
							"x_real_ip": true,
						},
					},
				},
				"subdomains": []any{},
				"paths":      []any{},
			},
		},
	}

	migrateHeaderManipulation(config)

	toggles := config["domains"].([]any)[0].(map[string]any)["toggles"].(map[string]any)
	headers := toggles["headers"].(map[string]any)
	req := headers["request"].(map[string]any)
	if req["enabled"] != true {
		t.Error("existing headers.request.enabled should be preserved")
	}
	if req["x_real_ip"] != true {
		t.Error("existing headers.request.x_real_ip should be preserved")
	}
}

func TestMigrateHeaderManipulation_AddsResponseDeferred(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"rule": map[string]any{
					"handler_type":   "none",
					"handler_config": map[string]any{},
				},
				"toggles": map[string]any{
					"headers": map[string]any{
						"response": map[string]any{
							"enabled": false,
						},
					},
				},
				"subdomains": []any{},
				"paths":      []any{},
			},
		},
	}

	migrateHeaderManipulation(config)

	toggles := config["domains"].([]any)[0].(map[string]any)["toggles"].(map[string]any)
	headers := toggles["headers"].(map[string]any)
	response := headers["response"].(map[string]any)
	if _, ok := response["deferred"]; !ok {
		t.Error("headers.response.deferred should be added")
	}
}

func TestMigrateHeaderManipulation_WalksSubdomains(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":      "dom_1",
				"name":    "example.com",
				"rule":    map[string]any{"handler_type": "none", "handler_config": map[string]any{}},
				"toggles": map[string]any{},
				"paths":   []any{},
				"subdomains": []any{
					map[string]any{
						"id":   "sub_1",
						"name": "api",
						"rule": map[string]any{
							"handler_type": "reverse_proxy",
							"handler_config": map[string]any{
								"upstream":        "localhost:4000",
								"request_headers": map[string]any{"enabled": true},
							},
						},
						"toggles": map[string]any{
							"headers": map[string]any{
								"response": map[string]any{"enabled": false},
							},
						},
						"paths": []any{},
					},
				},
			},
		},
	}

	migrateHeaderManipulation(config)

	sub := config["domains"].([]any)[0].(map[string]any)["subdomains"].([]any)[0].(map[string]any)
	hc := sub["rule"].(map[string]any)["handler_config"].(map[string]any)
	if _, ok := hc["request_headers"]; ok {
		t.Error("subdomain request_headers should be renamed")
	}
	if _, ok := hc["header_up"]; !ok {
		t.Error("subdomain header_up should exist after rename")
	}

	headers := sub["toggles"].(map[string]any)["headers"].(map[string]any)
	if _, ok := headers["request"]; !ok {
		t.Error("subdomain headers.request should be added")
	}
}

func TestMigrateHeaderManipulation_WalksPaths(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":         "dom_1",
				"name":       "example.com",
				"rule":       map[string]any{"handler_type": "none", "handler_config": map[string]any{}},
				"toggles":    map[string]any{},
				"subdomains": []any{},
				"paths": []any{
					map[string]any{
						"id":      "path_1",
						"enabled": true,
						"rule": map[string]any{
							"handler_type": "reverse_proxy",
							"handler_config": map[string]any{
								"upstream":        "localhost:5000",
								"request_headers": map[string]any{"enabled": false},
							},
						},
						"toggle_overrides": map[string]any{
							"headers": map[string]any{
								"response": map[string]any{"enabled": false},
							},
						},
					},
				},
			},
		},
	}

	migrateHeaderManipulation(config)

	path := config["domains"].([]any)[0].(map[string]any)["paths"].([]any)[0].(map[string]any)
	hc := path["rule"].(map[string]any)["handler_config"].(map[string]any)
	if _, ok := hc["request_headers"]; ok {
		t.Error("path request_headers should be renamed")
	}
	if _, ok := hc["header_up"]; !ok {
		t.Error("path header_up should exist after rename")
	}
}

func TestMigrateHeaderManipulation_SkipsNonReverseProxy(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"rule": map[string]any{
					"handler_type":   "static_file",
					"handler_config": map[string]any{"root": "/var/www"},
				},
				"toggles":    map[string]any{},
				"subdomains": []any{},
				"paths":      []any{},
			},
		},
	}

	migrateHeaderManipulation(config)

	hc := config["domains"].([]any)[0].(map[string]any)["rule"].(map[string]any)["handler_config"].(map[string]any)
	if _, ok := hc["header_down"]; ok {
		t.Error("header_down should not be added to non-reverse-proxy rules")
	}
}
