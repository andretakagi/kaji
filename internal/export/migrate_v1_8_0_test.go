package export

import "testing"

func TestMigrateV180_ErrorPages(t *testing.T) {
	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"toggles": map[string]any{
					"force_https": true,
				},
				"subdomains": []any{
					map[string]any{
						"id":   "sub_1",
						"name": "api",
						"toggles": map[string]any{
							"compression": true,
						},
					},
				},
			},
			map[string]any{
				"id":   "dom_2",
				"name": "other.com",
				"toggles": map[string]any{
					"error_pages": []any{},
				},
			},
		},
	}

	changes := migrateV180(config)

	domToggles := config["domains"].([]any)[0].(map[string]any)["toggles"].(map[string]any)
	if _, ok := domToggles["error_pages"]; !ok {
		t.Error("domain toggles missing error_pages")
	}

	subToggles := config["domains"].([]any)[0].(map[string]any)["subdomains"].([]any)[0].(map[string]any)["toggles"].(map[string]any)
	if _, ok := subToggles["error_pages"]; !ok {
		t.Error("subdomain toggles missing error_pages")
	}

	dom2Toggles := config["domains"].([]any)[1].(map[string]any)["toggles"].(map[string]any)
	if _, ok := dom2Toggles["error_pages"]; !ok {
		t.Error("dom2 toggles missing error_pages")
	}

	hasErrorPagesChange := false
	for _, c := range changes {
		if c == "added error_pages default for domain example.com" {
			hasErrorPagesChange = true
		}
	}
	if !hasErrorPagesChange {
		t.Errorf("expected migration change for example.com, got: %v", changes)
	}

	hasDom2Change := false
	for _, c := range changes {
		if c == "added error_pages default for domain other.com" {
			hasDom2Change = true
		}
	}
	if hasDom2Change {
		t.Error("should not add error_pages to domain that already has it")
	}
}

func TestMigrateV180_LoadBalancingParams(t *testing.T) {
	reverseProxyRule := func(lb map[string]any) map[string]any {
		return map[string]any{
			"handler_type": "reverse_proxy",
			"handler_config": map[string]any{
				"upstream":       "localhost:8080",
				"load_balancing": lb,
			},
		}
	}

	config := map[string]any{
		"domains": []any{
			map[string]any{
				"id":   "dom_1",
				"name": "example.com",
				"rule": reverseProxyRule(map[string]any{
					"enabled":  true,
					"strategy": "round_robin",
				}),
				"subdomains": []any{
					map[string]any{
						"id":   "sub_1",
						"name": "api",
						"rule": reverseProxyRule(map[string]any{
							"enabled": false,
						}),
					},
				},
				"paths": []any{
					map[string]any{
						"id":   "path_1",
						"rule": reverseProxyRule(map[string]any{"enabled": false}),
					},
					map[string]any{
						"id": "path_static",
						"rule": map[string]any{
							"handler_type":   "static_response",
							"handler_config": map[string]any{"status_code": "200"},
						},
					},
				},
			},
			map[string]any{
				"id":   "dom_2",
				"name": "other.com",
				"rule": reverseProxyRule(map[string]any{
					"enabled": false,
					"key":     "existing",
					"weights": []any{2, 1},
					"secret":  "keep",
				}),
			},
		},
	}

	changes := migrateLoadBalancingParams(config)

	getLB := func(rule map[string]any) map[string]any {
		return rule["handler_config"].(map[string]any)["load_balancing"].(map[string]any)
	}

	dom1 := config["domains"].([]any)[0].(map[string]any)
	domLB := getLB(dom1["rule"].(map[string]any))
	for _, field := range []string{"weights", "key", "secret"} {
		if _, ok := domLB[field]; !ok {
			t.Errorf("domain load_balancing missing %s default", field)
		}
	}

	subLB := getLB(dom1["subdomains"].([]any)[0].(map[string]any)["rule"].(map[string]any))
	if _, ok := subLB["key"]; !ok {
		t.Error("subdomain load_balancing missing key default")
	}

	pathLB := getLB(dom1["paths"].([]any)[0].(map[string]any)["rule"].(map[string]any))
	if _, ok := pathLB["weights"]; !ok {
		t.Error("path load_balancing missing weights default")
	}

	// Non-reverse_proxy rules must be left untouched.
	staticHC := dom1["paths"].([]any)[1].(map[string]any)["rule"].(map[string]any)["handler_config"].(map[string]any)
	if _, ok := staticHC["load_balancing"]; ok {
		t.Error("static_response handler should not gain load_balancing")
	}

	dom2LB := getLB(config["domains"].([]any)[1].(map[string]any)["rule"].(map[string]any))
	if dom2LB["key"] != "existing" {
		t.Errorf("existing key was overwritten: %v", dom2LB["key"])
	}
	if dom2LB["secret"] != "keep" {
		t.Errorf("existing secret was overwritten: %v", dom2LB["secret"])
	}

	hasChange := false
	for _, c := range changes {
		if c != "" {
			hasChange = true
		}
	}
	if !hasChange {
		t.Error("expected at least one migration change for configs missing the new fields")
	}
}
