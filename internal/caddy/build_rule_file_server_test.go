package caddy

import (
	"encoding/json"
	"testing"
)

func TestBuildRuleRoute_FileServerBasic(t *testing.T) {
	cfg := mustMarshal(t, FileServerConfig{
		Root: "/var/www/html",
	})
	rule := RuleBuildParams{
		RuleID:        "rule_fs_basic",
		HandlerType:   "file_server",
		HandlerConfig: cfg,
	}

	result, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	if last["handler"] != "file_server" {
		t.Errorf("handler = %v, want file_server", last["handler"])
	}
	if last["root"] != "/var/www/html" {
		t.Errorf("root = %v, want /var/www/html", last["root"])
	}
	if _, ok := last["browse"]; ok {
		t.Error("browse should not be present when Browse is false")
	}
	if _, ok := last["index_names"]; ok {
		t.Error("index_names should not be present when empty")
	}
	if _, ok := last["hide"]; ok {
		t.Error("hide should not be present when empty")
	}
}

func TestBuildRuleRoute_FileServerBrowse(t *testing.T) {
	cfg := mustMarshal(t, FileServerConfig{
		Root:   "/var/www/html",
		Browse: true,
	})
	rule := RuleBuildParams{
		RuleID:        "rule_fs_browse",
		HandlerType:   "file_server",
		HandlerConfig: cfg,
	}

	result, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	browse, ok := last["browse"]
	if !ok {
		t.Fatal("browse key should be present when Browse is true")
	}
	browseObj, ok := browse.(map[string]any)
	if !ok {
		t.Fatalf("browse should be an object, got %T", browse)
	}
	if len(browseObj) != 0 {
		t.Errorf("browse should be empty object, got %v", browseObj)
	}
}

func TestBuildRuleRoute_FileServerCustomIndexNames(t *testing.T) {
	cfg := mustMarshal(t, FileServerConfig{
		Root:       "/var/www/html",
		IndexNames: []string{"index.html", "index.htm", "default.html"},
	})
	rule := RuleBuildParams{
		RuleID:        "rule_fs_index",
		HandlerType:   "file_server",
		HandlerConfig: cfg,
	}

	result, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	indexNames, ok := last["index_names"]
	if !ok {
		t.Fatal("index_names should be present")
	}
	names := indexNames.([]any)
	if len(names) != 3 {
		t.Fatalf("index_names count = %d, want 3", len(names))
	}
	if names[0] != "index.html" {
		t.Errorf("index_names[0] = %v, want index.html", names[0])
	}
}

func TestBuildRuleRoute_FileServerCustomHide(t *testing.T) {
	cfg := mustMarshal(t, FileServerConfig{
		Root: "/var/www/html",
		Hide: []string{".htaccess", ".git", "*.secret"},
	})
	rule := RuleBuildParams{
		RuleID:        "rule_fs_hide",
		HandlerType:   "file_server",
		HandlerConfig: cfg,
	}

	result, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)
	last := handlers[len(handlers)-1].(map[string]any)

	hide, ok := last["hide"]
	if !ok {
		t.Fatal("hide should be present")
	}
	hideList := hide.([]any)
	if len(hideList) != 3 {
		t.Fatalf("hide count = %d, want 3", len(hideList))
	}
	if hideList[0] != ".htaccess" {
		t.Errorf("hide[0] = %v, want .htaccess", hideList[0])
	}
}

func TestBuildRuleRoute_FileServerAllOptions(t *testing.T) {
	cfg := mustMarshal(t, FileServerConfig{
		Root:       "/srv/files",
		Browse:     true,
		IndexNames: []string{"index.html"},
		Hide:       []string{".env"},
	})
	rule := RuleBuildParams{
		RuleID:        "rule_fs_all",
		HandlerType:   "file_server",
		HandlerConfig: cfg,
	}
	toggles := DomainToggles{
		ForceHTTPS:  true,
		Compression: true,
	}

	result, err := BuildRuleRoute("example.com", rule, toggles, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	route := unmarshalRoute(t, result)
	handlers := route["handle"].([]any)

	if len(handlers) < 3 {
		t.Fatalf("expected at least 3 handlers (force-https, encode, file_server), got %d", len(handlers))
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
	if last["handler"] != "file_server" {
		t.Errorf("last handler = %v, want file_server", last["handler"])
	}
	if last["root"] != "/srv/files" {
		t.Errorf("root = %v, want /srv/files", last["root"])
	}
	if _, ok := last["browse"]; !ok {
		t.Error("browse should be present")
	}
	if _, ok := last["index_names"]; !ok {
		t.Error("index_names should be present")
	}
	if _, ok := last["hide"]; !ok {
		t.Error("hide should be present")
	}
}

func TestBuildRuleRoute_FileServerMissingRoot(t *testing.T) {
	cfg := mustMarshal(t, FileServerConfig{
		Root: "",
	})
	rule := RuleBuildParams{
		RuleID:        "rule_fs_noroot",
		HandlerType:   "file_server",
		HandlerConfig: cfg,
	}

	_, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for empty root")
	}
}

func TestBuildRuleRoute_FileServerInvalidJSON(t *testing.T) {
	rule := RuleBuildParams{
		RuleID:        "rule_fs_bad",
		HandlerType:   "file_server",
		HandlerConfig: json.RawMessage(`{invalid`),
	}

	_, err := BuildRuleRoute("example.com", rule, DomainToggles{}, nil, "", false)
	if err == nil {
		t.Fatal("expected error for invalid handler config JSON")
	}
}
