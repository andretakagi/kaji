package caddy

import (
	"encoding/json"
	"testing"
)

func TestBuildRuleRoute_RedirectBasic(t *testing.T) {
	cfg := mustMarshal(t, RedirectConfig{
		TargetURL:    "https://example.com",
		StatusCode:   "301",
		PreservePath: false,
	})
	rule := RuleBuildParams{
		RuleID:        "rule_rd_basic",
		HandlerType:   "redirect",
		HandlerConfig: cfg,
	}

	result, err := BuildRuleRoute("old.example.com", rule, DomainToggles{}, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	if last["handler"] != "static_response" {
		t.Errorf("handler = %v, want static_response", last["handler"])
	}
	if last["status_code"] != "301" {
		t.Errorf("status_code = %v, want 301", last["status_code"])
	}
	headers := last["headers"].(map[string]any)
	loc := headers["Location"].([]any)
	if loc[0] != "https://example.com" {
		t.Errorf("Location = %v, want https://example.com", loc[0])
	}
}

func TestBuildRuleRoute_RedirectPreservePath(t *testing.T) {
	cfg := mustMarshal(t, RedirectConfig{
		TargetURL:    "https://new.example.com",
		StatusCode:   "302",
		PreservePath: true,
	})
	rule := RuleBuildParams{
		RuleID:        "rule_rd_path",
		HandlerType:   "redirect",
		HandlerConfig: cfg,
	}

	result, err := BuildRuleRoute("old.example.com", rule, DomainToggles{}, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	headers := last["headers"].(map[string]any)
	loc := headers["Location"].([]any)
	want := "https://new.example.com{http.request.uri}"
	if loc[0] != want {
		t.Errorf("Location = %v, want %v", loc[0], want)
	}
}

func TestBuildRuleRoute_RedirectPreservePathTrailingSlash(t *testing.T) {
	cfg := mustMarshal(t, RedirectConfig{
		TargetURL:    "https://new.example.com/",
		StatusCode:   "301",
		PreservePath: true,
	})
	rule := RuleBuildParams{
		RuleID:        "rule_rd_slash",
		HandlerType:   "redirect",
		HandlerConfig: cfg,
	}

	result, err := BuildRuleRoute("old.example.com", rule, DomainToggles{}, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	headers := last["headers"].(map[string]any)
	loc := headers["Location"].([]any)
	want := "https://new.example.com{http.request.uri}"
	if loc[0] != want {
		t.Errorf("Location = %v, want %v (trailing slash should be stripped)", loc[0], want)
	}
}

func TestBuildRuleRoute_RedirectPreservePathWithSubpath(t *testing.T) {
	cfg := mustMarshal(t, RedirectConfig{
		TargetURL:    "https://new.example.com/docs",
		StatusCode:   "301",
		PreservePath: true,
	})
	rule := RuleBuildParams{
		RuleID:        "rule_rd_subpath",
		HandlerType:   "redirect",
		HandlerConfig: cfg,
	}

	result, err := BuildRuleRoute("old.example.com", rule, DomainToggles{}, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	headers := last["headers"].(map[string]any)
	loc := headers["Location"].([]any)
	want := "https://new.example.com/docs{http.request.uri}"
	if loc[0] != want {
		t.Errorf("Location = %v, want %v", loc[0], want)
	}
}

func TestBuildRuleRoute_RedirectWithToggles(t *testing.T) {
	cfg := mustMarshal(t, RedirectConfig{
		TargetURL:    "https://example.com",
		StatusCode:   "301",
		PreservePath: false,
	})
	rule := RuleBuildParams{
		RuleID:        "rule_rd_toggles",
		HandlerType:   "redirect",
		HandlerConfig: cfg,
	}
	toggles := DomainToggles{
		ForceHTTPS:  true,
		Compression: true,
	}

	result, err := BuildRuleRoute("old.example.com", rule, toggles, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)

	if len(handlers) < 3 {
		t.Fatalf("expected at least 3 handlers (force-https, encode, redirect), got %d", len(handlers))
	}

	first := handlers[0].(map[string]any)
	if first["handler"] != "subroute" {
		t.Errorf("first handler = %v, want subroute (force HTTPS)", first["handler"])
	}

	second := handlers[1].(map[string]any)
	if second["handler"] != "encode" {
		t.Errorf("second handler = %v, want encode (compression)", second["handler"])
	}

	last := handlers[len(handlers)-1].(map[string]any)
	if last["handler"] != "static_response" {
		t.Errorf("last handler = %v, want static_response", last["handler"])
	}
}

func TestBuildRuleRoute_RedirectEmptyTarget(t *testing.T) {
	cfg := mustMarshal(t, RedirectConfig{
		TargetURL:  "",
		StatusCode: "301",
	})
	rule := RuleBuildParams{
		RuleID:        "rule_rd_empty",
		HandlerType:   "redirect",
		HandlerConfig: cfg,
	}

	_, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "")
	if err == nil {
		t.Fatal("expected error for empty target URL")
	}
}

func TestBuildRuleRoute_RedirectInvalidJSON(t *testing.T) {
	rule := RuleBuildParams{
		RuleID:        "rule_rd_bad",
		HandlerType:   "redirect",
		HandlerConfig: json.RawMessage(`{invalid`),
	}

	_, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "")
	if err == nil {
		t.Fatal("expected error for invalid handler config JSON")
	}
}
