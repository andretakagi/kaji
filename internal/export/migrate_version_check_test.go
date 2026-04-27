package export

import (
	"os"
	"regexp"
	"testing"
)

var mainGoVersionLine = regexp.MustCompile(`var version = "([^"]+)"`)

func TestMigrationsBeforeNotAheadOfMainVersion(t *testing.T) {
	data, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	match := mainGoVersionLine.FindSubmatch(data)
	if match == nil {
		t.Fatal(`could not find 'var version = "..."' in main.go`)
	}
	mainVersion := string(match[1])

	for _, m := range migrations {
		cmp, err := compareVersions(m.Before, mainVersion)
		if err != nil {
			t.Errorf("invalid migration Before %q: %v", m.Before, err)
			continue
		}
		if cmp > 0 {
			t.Errorf("migration with Before=%q targets a version greater than main.version=%q; either bump the release version or fold this migration into the current release", m.Before, mainVersion)
		}
	}
}
