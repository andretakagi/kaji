package caddy

import (
	"encoding/json"
	"testing"
)

func TestBuildRuleRoute_StaticResponseClose(t *testing.T) {
	srCfg := mustMarshal(t, StaticResponseConfig{Close: true})
	rule := RuleBuildParams{
		RuleID:        "rule_sr_close",
		HandlerType:   "static_response",
		HandlerConfig: srCfg,
	}

	result, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	if last["handler"] != "static_response" {
		t.Errorf("handler = %v, want static_response", last["handler"])
	}
	if last["close"] != true {
		t.Errorf("close = %v, want true", last["close"])
	}
	if _, ok := last["status_code"]; ok {
		t.Error("close=true should not include status_code")
	}
	if _, ok := last["body"]; ok {
		t.Error("close=true should not include body")
	}
	if _, ok := last["headers"]; ok {
		t.Error("close=true should not include headers")
	}
}

func TestBuildRuleRoute_StaticResponseFull(t *testing.T) {
	srCfg := mustMarshal(t, StaticResponseConfig{
		StatusCode: "200",
		Body:       "Hello, world!",
		Headers: map[string][]string{
			"Content-Type": {"text/plain"},
		},
	})
	rule := RuleBuildParams{
		RuleID:        "rule_sr_full",
		HandlerType:   "static_response",
		HandlerConfig: srCfg,
	}

	result, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	if last["handler"] != "static_response" {
		t.Errorf("handler = %v, want static_response", last["handler"])
	}
	if last["status_code"] != "200" {
		t.Errorf("status_code = %v, want 200", last["status_code"])
	}
	if last["body"] != "Hello, world!" {
		t.Errorf("body = %v, want Hello, world!", last["body"])
	}
	headers := last["headers"].(map[string]any)
	ct := headers["Content-Type"].([]any)
	if ct[0] != "text/plain" {
		t.Errorf("Content-Type = %v, want text/plain", ct[0])
	}
}

func TestBuildRuleRoute_StaticResponseMinimal(t *testing.T) {
	srCfg := mustMarshal(t, StaticResponseConfig{
		StatusCode: "204",
	})
	rule := RuleBuildParams{
		RuleID:        "rule_sr_min",
		HandlerType:   "static_response",
		HandlerConfig: srCfg,
	}

	result, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	if last["handler"] != "static_response" {
		t.Errorf("handler = %v, want static_response", last["handler"])
	}
	if last["status_code"] != "204" {
		t.Errorf("status_code = %v, want 204", last["status_code"])
	}
	if _, ok := last["body"]; ok {
		t.Error("empty body should be omitted")
	}
	if _, ok := last["headers"]; ok {
		t.Error("empty headers should be omitted")
	}
}

func TestBuildRuleRoute_StaticResponseWithToggles(t *testing.T) {
	srCfg := mustMarshal(t, StaticResponseConfig{
		StatusCode: "403",
		Body:       "Forbidden",
	})
	rule := RuleBuildParams{
		RuleID:        "rule_sr_toggles",
		HandlerType:   "static_response",
		HandlerConfig: srCfg,
	}
	toggles := DomainToggles{
		ForceHTTPS:  true,
		Compression: true,
	}

	result, err := BuildRuleRoute("example.com", rule, toggles, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)

	if len(handlers) < 3 {
		t.Fatalf("expected at least 3 handlers (force-https, encode, static_response), got %d", len(handlers))
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

func TestBuildRuleRoute_StaticResponseInvalidJSON(t *testing.T) {
	rule := RuleBuildParams{
		RuleID:        "rule_sr_bad",
		HandlerType:   "static_response",
		HandlerConfig: json.RawMessage(`{invalid`),
	}

	_, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "")
	if err == nil {
		t.Fatal("expected error for invalid handler config JSON")
	}
}
