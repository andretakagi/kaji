// Input validation for domains, upstreams, emails, etc.
package api

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/andretakagi/kaji/internal/caddy"
	"github.com/andretakagi/kaji/internal/config"
)

func validateDomain(domain string) string {
	if len(domain) > 253 {
		return "domain name too long"
	}
	for _, c := range domain {
		if c < 0x20 || c == 0x7f {
			return "domain contains control characters"
		}
	}
	// Allow wildcard prefix (e.g. *.example.com)
	check := domain
	if strings.HasPrefix(check, "*.") {
		check = check[2:]
	}
	if check == "" {
		return "domain is required"
	}
	labels := strings.Split(check, ".")
	if len(labels) < 2 {
		return "invalid domain address"
	}
	for _, label := range labels {
		if label == "" {
			return "domain has an empty label"
		}
		if len(label) > 63 {
			return "domain label too long (max 63 characters)"
		}
	}
	return ""
}

func validateUpstream(upstream string) string {
	if upstream == "" {
		return "upstream is required"
	}
	host, port, err := net.SplitHostPort(upstream)
	if err != nil {
		return "upstream must be host:port (e.g. 127.0.0.1:8080 or myservice:3000)"
	}
	if host == "" {
		return "upstream host is empty"
	}
	if port == "" {
		return "upstream port is empty"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return "upstream port must be a number between 1 and 65535"
	}
	return ""
}

func validateEmail(email string) string {
	if email == "" {
		return "email is required"
	}
	if len(email) > 254 {
		return "email is too long"
	}
	for _, c := range email {
		if c < 0x20 || c == 0x7f {
			return "email contains control characters"
		}
	}
	at := strings.IndexByte(email, '@')
	if at < 1 {
		return "invalid email format"
	}
	domainPart := email[at+1:]
	if !strings.Contains(domainPart, ".") || strings.HasSuffix(domainPart, ".") {
		return "invalid email format"
	}
	return ""
}

func validateCaddyAdminURL(rawURL string) string {
	if rawURL == "" {
		return "Caddy admin URL must not be empty"
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "invalid Caddy admin URL: must be http or https with a valid host"
	}
	return ""
}

func validateServerName(name string) string {
	if name == "" {
		return "server name is required"
	}
	if len(name) > 64 {
		return "server name too long (max 64 characters)"
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return "server name must contain only letters, digits, hyphens, and underscores"
		}
	}
	return ""
}

var validAutoHTTPS = map[string]bool{
	"on":                true,
	"off":               true,
	"disable_redirects": true,
}

func validateAutoHTTPS(value string) string {
	if !validAutoHTTPS[value] {
		return "auto_https must be one of: on, off, disable_redirects"
	}
	return ""
}

var validLBStrategies = map[string]bool{
	"round_robin": true,
	"first":       true,
	"least_conn":  true,
	"random":      true,
	"ip_hash":     true,
}

func validateLBStrategy(strategy string) string {
	if !validLBStrategies[strategy] {
		return "load balancing strategy must be one of: round_robin, first, least_conn, random, ip_hash"
	}
	return ""
}

func validateIPOrCIDR(value string) string {
	if strings.Contains(value, "/") {
		_, _, err := net.ParseCIDR(value)
		if err != nil {
			return fmt.Sprintf("invalid CIDR: %s", value)
		}
		return ""
	}
	if net.ParseIP(value) == nil {
		return fmt.Sprintf("invalid IP address: %s", value)
	}
	return ""
}

var validIPListTypes = map[string]bool{
	"whitelist": true,
	"blacklist": true,
}

func validateIPListType(t string) string {
	if !validIPListTypes[t] {
		return "type must be whitelist or blacklist"
	}
	return ""
}

func validateIPListName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "name is required"
	}
	if len(name) > 128 {
		return "name too long (max 128 characters)"
	}
	return ""
}

// validateIPListChildren checks that all child IDs exist, match the parent type,
// and won't create circular references. parentID should be empty for new lists.
func validateIPListChildren(children []string, parentType string, parentID string, allLists []config.IPList) string {
	for _, childID := range children {
		if childID == parentID && parentID != "" {
			return "a list cannot include itself"
		}
		childFound := false
		for _, l := range allLists {
			if l.ID == childID {
				childFound = true
				if l.Type != parentType {
					return fmt.Sprintf("child list %q is type %q, must match parent type %q", l.Name, l.Type, parentType)
				}
				break
			}
		}
		if !childFound {
			return fmt.Sprintf("child list %q not found", childID)
		}
	}

	// Check for circular references when updating an existing list
	if parentID != "" && len(children) > 0 {
		tempLists := make([]config.IPList, len(allLists))
		copy(tempLists, allLists)
		for i := range tempLists {
			if tempLists[i].ID == parentID {
				tempLists[i].Children = children
				break
			}
		}
		if _, err := caddy.ResolveIPList(parentID, tempLists); err != nil {
			return fmt.Sprintf("circular reference: %v", err)
		}
	}

	return ""
}

func validateLoadBalancing(w http.ResponseWriter, lb caddy.LoadBalancing) bool {
	if !lb.Enabled {
		return true
	}
	if msg := validateLBStrategy(lb.Strategy); msg != "" {
		writeError(w, msg, http.StatusBadRequest)
		return false
	}
	if len(lb.Upstreams) == 0 {
		writeError(w, "load balancing requires at least one additional upstream", http.StatusBadRequest)
		return false
	}
	for _, u := range lb.Upstreams {
		if msg := validateUpstream(u); msg != "" {
			writeError(w, "additional upstream: "+msg, http.StatusBadRequest)
			return false
		}
	}
	return true
}
