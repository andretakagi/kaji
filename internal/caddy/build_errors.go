package caddy

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func buildStatusExpression(pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)

	if strings.HasSuffix(pattern, "xx") {
		prefix := strings.TrimSuffix(pattern, "xx")
		switch prefix {
		case "4":
			return "{http.error.status_code} >= 400 && {http.error.status_code} < 500", nil
		case "5":
			return "{http.error.status_code} >= 500 && {http.error.status_code} < 600", nil
		default:
			return "", fmt.Errorf("unsupported range pattern: %s", pattern)
		}
	}

	if strings.Contains(pattern, ",") {
		parts := strings.Split(pattern, ",")
		codes := make([]string, 0, len(parts))
		for _, p := range parts {
			code := strings.TrimSpace(p)
			if err := validateStatusCode(code); err != nil {
				return "", err
			}
			codes = append(codes, code)
		}
		return fmt.Sprintf("{http.error.status_code} in [%s]", strings.Join(codes, ", ")), nil
	}

	if err := validateStatusCode(pattern); err != nil {
		return "", err
	}
	return fmt.Sprintf("{http.error.status_code} == %s", pattern), nil
}

func validateStatusCode(code string) error {
	n, err := strconv.Atoi(code)
	if err != nil {
		return fmt.Errorf("invalid status code %q: must be a number", code)
	}
	if n < 100 || n > 599 {
		return fmt.Errorf("invalid status code %d: must be between 100 and 599", n)
	}
	return nil
}

func parseStatusExpression(expr string) string {
	expr = strings.TrimSpace(expr)

	if strings.Contains(expr, " in [") {
		start := strings.Index(expr, "[")
		end := strings.Index(expr, "]")
		if start >= 0 && end > start {
			return expr[start+1 : end]
		}
	}

	if strings.Contains(expr, ">=") && strings.Contains(expr, "<") {
		if strings.Contains(expr, ">= 400") && strings.Contains(expr, "< 500") {
			return "4xx"
		}
		if strings.Contains(expr, ">= 500") && strings.Contains(expr, "< 600") {
			return "5xx"
		}
	}

	if strings.Contains(expr, "== ") {
		parts := strings.Split(expr, "== ")
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}

	return expr
}

func BuildHandleErrorsRoutes(domains []SyncDomain) (json.RawMessage, error) {
	var routes []map[string]any

	for _, dom := range domains {
		if !dom.Enabled {
			continue
		}
		for _, ep := range dom.Toggles.ErrorPages {
			route, err := buildErrorPageRoute(dom.Name, ep)
			if err != nil {
				return nil, fmt.Errorf("building error route for %s: %w", dom.Name, err)
			}
			routes = append(routes, route)
		}
		for _, sub := range dom.Subdomains {
			if !sub.Enabled {
				continue
			}
			subHost := sub.Name + "." + dom.Name
			for _, ep := range sub.Toggles.ErrorPages {
				route, err := buildErrorPageRoute(subHost, ep)
				if err != nil {
					return nil, fmt.Errorf("building error route for %s: %w", subHost, err)
				}
				routes = append(routes, route)
			}
		}
	}

	if len(routes) == 0 {
		return nil, nil
	}

	errorsObj := map[string]any{"routes": routes}
	data, err := json.Marshal(errorsObj)
	if err != nil {
		return nil, fmt.Errorf("marshaling handle_errors config: %w", err)
	}
	return json.RawMessage(data), nil
}

func buildErrorPageRoute(hostname string, ep ErrorPage) (map[string]any, error) {
	expr, err := buildStatusExpression(ep.StatusCode)
	if err != nil {
		return nil, err
	}

	contentType := ep.ContentType
	if contentType == "" {
		contentType = "text/html"
	}

	handler := map[string]any{
		"handler": "static_response",
		"headers": map[string][]string{
			"Content-Type": {contentType},
		},
	}
	if ep.Body != "" {
		handler["body"] = ep.Body
	}

	return map[string]any{
		"match": []map[string]any{
			{
				"host":       []string{hostname},
				"expression": expr,
			},
		},
		"handle": []any{handler},
	}, nil
}
