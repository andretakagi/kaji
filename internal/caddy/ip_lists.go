package caddy

import (
	"fmt"

	"github.com/andretakagi/kaji/internal/config"
)

// ResolveIPList flattens a list and all its children into a deduplicated slice of IPs/CIDRs.
func ResolveIPList(listID string, allLists []config.IPList) ([]string, error) {
	seen := make(map[string]bool)
	return resolveIPListInner(listID, allLists, seen)
}

func resolveIPListInner(listID string, allLists []config.IPList, visited map[string]bool) ([]string, error) {
	if visited[listID] {
		return nil, fmt.Errorf("circular reference detected: list %q", listID)
	}
	visited[listID] = true

	var list *config.IPList
	for i := range allLists {
		if allLists[i].ID == listID {
			list = &allLists[i]
			break
		}
	}
	if list == nil {
		return nil, fmt.Errorf("IP list %q not found", listID)
	}

	unique := make(map[string]bool)
	for _, ip := range list.IPs {
		unique[ip] = true
	}

	for _, childID := range list.Children {
		childIPs, err := resolveIPListInner(childID, allLists, visited)
		if err != nil {
			return nil, err
		}
		for _, ip := range childIPs {
			unique[ip] = true
		}
	}

	result := make([]string, 0, len(unique))
	for ip := range unique {
		result = append(result, ip)
	}
	return result, nil
}
