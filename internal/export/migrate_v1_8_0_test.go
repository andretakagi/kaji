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
