package caddy

import (
	"encoding/json"
	"testing"
)

func TestBuildStatusExpression_Individual(t *testing.T) {
	got, err := buildStatusExpression("404")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "{http.error.status_code} == 404"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildStatusExpression_Range4xx(t *testing.T) {
	got, err := buildStatusExpression("4xx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "{http.error.status_code} >= 400 && {http.error.status_code} < 500"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildStatusExpression_Range5xx(t *testing.T) {
	got, err := buildStatusExpression("5xx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "{http.error.status_code} >= 500 && {http.error.status_code} < 600"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildStatusExpression_CommaSeparated(t *testing.T) {
	got, err := buildStatusExpression("400, 404, 500")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "{http.error.status_code} in [400, 404, 500]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildStatusExpression_UnsupportedRange(t *testing.T) {
	_, err := buildStatusExpression("3xx")
	if err == nil {
		t.Error("expected error for unsupported range")
	}
}

func TestParseStatusExpression_Individual(t *testing.T) {
	got := parseStatusExpression("{http.error.status_code} == 404")
	if got != "404" {
		t.Errorf("got %q, want %q", got, "404")
	}
}

func TestParseStatusExpression_Range4xx(t *testing.T) {
	got := parseStatusExpression("{http.error.status_code} >= 400 && {http.error.status_code} < 500")
	if got != "4xx" {
		t.Errorf("got %q, want %q", got, "4xx")
	}
}

func TestParseStatusExpression_Range5xx(t *testing.T) {
	got := parseStatusExpression("{http.error.status_code} >= 500 && {http.error.status_code} < 600")
	if got != "5xx" {
		t.Errorf("got %q, want %q", got, "5xx")
	}
}

func TestParseStatusExpression_CommaSeparated(t *testing.T) {
	got := parseStatusExpression("{http.error.status_code} in [400, 404, 500]")
	if got != "400, 404, 500" {
		t.Errorf("got %q, want %q", got, "400, 404, 500")
	}
}

func TestStatusExpressionRoundTrip(t *testing.T) {
	cases := []string{"404", "4xx", "5xx", "400, 404, 500"}
	for _, pattern := range cases {
		expr, err := buildStatusExpression(pattern)
		if err != nil {
			t.Fatalf("build %q: %v", pattern, err)
		}
		got := parseStatusExpression(expr)
		if got != pattern {
			t.Errorf("round-trip %q: built %q, parsed back %q", pattern, expr, got)
		}
	}
}

func TestBuildHandleErrorsRoutes_Empty(t *testing.T) {
	domains := []SyncDomain{
		{ID: "dom_1", Name: "example.com", Enabled: true, Toggles: DomainToggles{}},
	}
	result, err := BuildHandleErrorsRoutes(domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty error pages, got %s", string(result))
	}
}

func TestBuildHandleErrorsRoutes_SingleDomain(t *testing.T) {
	domains := []SyncDomain{
		{
			ID:      "dom_1",
			Name:    "example.com",
			Enabled: true,
			Toggles: DomainToggles{
				ErrorPages: []ErrorPage{
					{StatusCode: "404", Body: "<h1>Not Found</h1>", ContentType: "text/html"},
				},
			},
		},
	}

	result, err := BuildHandleErrorsRoutes(domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	var parsed struct {
		Routes []struct {
			Match []struct {
				Host       []string `json:"host"`
				Expression string   `json:"expression"`
			} `json:"match"`
			Handle []struct {
				Handler string              `json:"handler"`
				Headers map[string][]string `json:"headers"`
				Body    string              `json:"body"`
			} `json:"handle"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(parsed.Routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(parsed.Routes))
	}

	route := parsed.Routes[0]
	if route.Match[0].Host[0] != "example.com" {
		t.Errorf("host = %s, want example.com", route.Match[0].Host[0])
	}
	if route.Match[0].Expression != "{http.error.status_code} == 404" {
		t.Errorf("expression = %q", route.Match[0].Expression)
	}
	if route.Handle[0].Body != "<h1>Not Found</h1>" {
		t.Errorf("body = %q", route.Handle[0].Body)
	}
	if ct := route.Handle[0].Headers["Content-Type"]; len(ct) == 0 || ct[0] != "text/html" {
		t.Errorf("content-type = %v", ct)
	}
}

func TestBuildHandleErrorsRoutes_MultipleDomains(t *testing.T) {
	domains := []SyncDomain{
		{
			ID: "dom_1", Name: "example.com", Enabled: true,
			Toggles: DomainToggles{
				ErrorPages: []ErrorPage{
					{StatusCode: "404", Body: "not found", ContentType: "text/plain"},
				},
			},
		},
		{
			ID: "dom_2", Name: "other.com", Enabled: true,
			Toggles: DomainToggles{
				ErrorPages: []ErrorPage{
					{StatusCode: "5xx", Body: "server error", ContentType: "text/plain"},
				},
			},
		},
	}

	result, err := BuildHandleErrorsRoutes(domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Routes []json.RawMessage `json:"routes"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(parsed.Routes) != 2 {
		t.Errorf("routes = %d, want 2", len(parsed.Routes))
	}
}

func TestBuildHandleErrorsRoutes_Subdomain(t *testing.T) {
	domains := []SyncDomain{
		{
			ID: "dom_1", Name: "example.com", Enabled: true,
			Toggles: DomainToggles{},
			Subdomains: []SyncSubdomain{
				{
					ID: "sub_1", Name: "api", Enabled: true,
					Toggles: DomainToggles{
						ErrorPages: []ErrorPage{
							{StatusCode: "502", Body: `{"error":"bad gateway"}`, ContentType: "application/json"},
						},
					},
				},
			},
		},
	}

	result, err := BuildHandleErrorsRoutes(domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Routes []struct {
			Match []struct {
				Host []string `json:"host"`
			} `json:"match"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(parsed.Routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(parsed.Routes))
	}
	if host := parsed.Routes[0].Match[0].Host[0]; host != "api.example.com" {
		t.Errorf("host = %s, want api.example.com", host)
	}
}

func TestBuildHandleErrorsRoutes_DisabledSkipped(t *testing.T) {
	domains := []SyncDomain{
		{
			ID: "dom_1", Name: "disabled.com", Enabled: false,
			Toggles: DomainToggles{
				ErrorPages: []ErrorPage{
					{StatusCode: "404", Body: "nope", ContentType: "text/plain"},
				},
			},
		},
	}

	result, err := BuildHandleErrorsRoutes(domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for disabled domain, got %s", string(result))
	}
}

func TestBuildHandleErrorsRoutes_DisabledSubdomainSkipped(t *testing.T) {
	domains := []SyncDomain{
		{
			ID: "dom_1", Name: "example.com", Enabled: true,
			Subdomains: []SyncSubdomain{
				{
					ID: "sub_1", Name: "api", Enabled: false,
					Toggles: DomainToggles{
						ErrorPages: []ErrorPage{
							{StatusCode: "404", Body: "nope", ContentType: "text/plain"},
						},
					},
				},
			},
		},
	}

	result, err := BuildHandleErrorsRoutes(domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for disabled subdomain, got %s", string(result))
	}
}

func TestBuildHandleErrorsRoutes_MultipleEntriesPerDomain(t *testing.T) {
	domains := []SyncDomain{
		{
			ID: "dom_1", Name: "example.com", Enabled: true,
			Toggles: DomainToggles{
				ErrorPages: []ErrorPage{
					{StatusCode: "404", Body: "not found", ContentType: "text/html"},
					{StatusCode: "5xx", Body: "server error", ContentType: "text/html"},
				},
			},
		},
	}

	result, err := BuildHandleErrorsRoutes(domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Routes []json.RawMessage `json:"routes"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(parsed.Routes) != 2 {
		t.Errorf("routes = %d, want 2", len(parsed.Routes))
	}
}

func TestBuildHandleErrorsRoutes_EmptyBodyOmitted(t *testing.T) {
	domains := []SyncDomain{
		{
			ID: "dom_1", Name: "example.com", Enabled: true,
			Toggles: DomainToggles{
				ErrorPages: []ErrorPage{
					{StatusCode: "404", Body: "", ContentType: "text/plain"},
				},
			},
		},
	}

	result, err := BuildHandleErrorsRoutes(domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Routes []struct {
			Handle []map[string]any `json:"handle"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if _, hasBody := parsed.Routes[0].Handle[0]["body"]; hasBody {
		t.Error("expected body to be omitted when empty")
	}
}
