package export

import (
	"fmt"
	"strconv"
	"strings"
)

type Migration struct {
	Before  string
	Summary string
	Fn      func(m map[string]any) []string
}

var migrations []Migration

func CheckVersion(exportVersion, runningVersion string) error {
	if exportVersion == "" {
		return fmt.Errorf("backup is missing kaji_version and cannot be migrated")
	}
	cmp, err := compareVersions(exportVersion, runningVersion)
	if err != nil {
		return fmt.Errorf("invalid version in backup: %w", err)
	}
	if cmp > 0 {
		return fmt.Errorf("backup is from Kaji %s but this is %s - upgrade before importing", exportVersion, runningVersion)
	}
	return nil
}

func RunMigrations(configMap map[string]any, fromVersion string) ([]string, error) {
	var allChanges []string
	for _, m := range migrations {
		if m.Before == "" {
			continue
		}
		cmp, err := compareVersions(fromVersion, m.Before)
		if err != nil {
			return nil, fmt.Errorf("comparing versions: %w", err)
		}
		if cmp < 0 {
			for _, c := range m.Fn(configMap) {
				if c != "" {
					allChanges = append(allChanges, c)
				}
			}
		}
	}
	return allChanges, nil
}

func setDefault(m map[string]any, key string, value any) string {
	if _, exists := m[key]; exists {
		return ""
	}
	m[key] = value
	return fmt.Sprintf("added %s (default: %v)", key, value)
}

func removeField(m map[string]any, key string) string {
	if _, exists := m[key]; !exists {
		return ""
	}
	delete(m, key)
	return fmt.Sprintf("removed %s", key)
}

func renameField(m map[string]any, oldKey, newKey string) string {
	val, exists := m[oldKey]
	if !exists {
		return ""
	}
	m[newKey] = val
	delete(m, oldKey)
	return fmt.Sprintf("renamed %s to %s", oldKey, newKey)
}

func setNestedDefault(m map[string]any, path []string, value any) string {
	if len(path) == 0 {
		return ""
	}
	current := m
	for _, key := range path[:len(path)-1] {
		child, ok := current[key]
		if !ok {
			nested := make(map[string]any)
			current[key] = nested
			current = nested
			continue
		}
		nested, ok := child.(map[string]any)
		if !ok {
			return ""
		}
		current = nested
	}
	finalKey := path[len(path)-1]
	if _, exists := current[finalKey]; exists {
		return ""
	}
	current[finalKey] = value
	return fmt.Sprintf("added %s (default: %v)", strings.Join(path, "."), value)
}

func parseVersion(s string) (int, int, int, error) {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid version %q: expected major.minor.patch", s)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version %q", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version %q", parts[1])
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch version %q", parts[2])
	}
	return major, minor, patch, nil
}

func CompareVersions(a, b string) (int, error) {
	return compareVersions(a, b)
}

func compareVersions(a, b string) (int, error) {
	aMaj, aMin, aPat, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	bMaj, bMin, bPat, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	switch {
	case aMaj != bMaj:
		return intCmp(aMaj, bMaj), nil
	case aMin != bMin:
		return intCmp(aMin, bMin), nil
	default:
		return intCmp(aPat, bPat), nil
	}
}

func intCmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
