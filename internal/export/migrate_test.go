package export

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input              string
		major, minor, patch int
		wantErr            bool
	}{
		{"1.2.3", 1, 2, 3, false},
		{"v1.2.3", 1, 2, 3, false},
		{"0.0.0", 0, 0, 0, false},
		{"10.20.30", 10, 20, 30, false},
		{"1.2", 0, 0, 0, true},
		{"1.2.3.4", 0, 0, 0, true},
		{"abc", 0, 0, 0, true},
		{"1.x.3", 0, 0, 0, true},
		{"1.2.x", 0, 0, 0, true},
		{"x.2.3", 0, 0, 0, true},
		{"", 0, 0, 0, true},
	}
	for _, tt := range tests {
		major, minor, patch, err := parseVersion(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseVersion(%q) expected error, got %d.%d.%d", tt.input, major, minor, patch)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseVersion(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if major != tt.major || minor != tt.minor || patch != tt.patch {
			t.Errorf("parseVersion(%q) = %d.%d.%d, want %d.%d.%d",
				tt.input, major, minor, patch, tt.major, tt.minor, tt.patch)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.1.0", "1.2.0", -1},
		{"1.2.0", "1.1.0", 1},
		{"1.0.1", "1.0.2", -1},
		{"1.0.2", "1.0.1", 1},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		{"1.9.0", "1.10.0", -1},
	}
	for _, tt := range tests {
		got, err := compareVersions(tt.a, tt.b)
		if err != nil {
			t.Errorf("compareVersions(%q, %q) error: %v", tt.a, tt.b, err)
			continue
		}
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompareVersionsErrors(t *testing.T) {
	_, err := compareVersions("bad", "1.0.0")
	if err == nil {
		t.Error("expected error for invalid first version")
	}
	_, err = compareVersions("1.0.0", "bad")
	if err == nil {
		t.Error("expected error for invalid second version")
	}
}

func TestCheckVersion(t *testing.T) {
	tests := []struct {
		export, running string
		wantErr         bool
	}{
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.1.0", false},
		{"1.0.0", "2.0.0", false},
		{"2.0.0", "1.0.0", true},
		{"1.5.0", "1.4.0", true},
		{"", "1.0.0", true},
	}
	for _, tt := range tests {
		err := CheckVersion(tt.export, tt.running)
		if tt.wantErr && err == nil {
			t.Errorf("CheckVersion(%q, %q) expected error", tt.export, tt.running)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("CheckVersion(%q, %q) unexpected error: %v", tt.export, tt.running, err)
		}
	}
}

func TestRunMigrationsSkipsOlderMigrations(t *testing.T) {
	origMigrations := migrations
	defer func() { migrations = origMigrations }()

	migrations = []Migration{
		{
			Before:  "1.1.0",
			Summary: "add foo",
			Fn: func(m map[string]any) []string {
				return []string{setDefault(m, "foo", "bar")}
			},
		},
		{
			Before:  "1.2.0",
			Summary: "add baz",
			Fn: func(m map[string]any) []string {
				return []string{setDefault(m, "baz", 42)}
			},
		},
	}

	configMap := map[string]any{"existing": true}
	changes, err := RunMigrations(configMap, "1.0.0")
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %v", len(changes), changes)
	}
	if configMap["foo"] != "bar" {
		t.Errorf("foo = %v, want bar", configMap["foo"])
	}
	if configMap["baz"] != 42 {
		t.Errorf("baz = %v, want 42", configMap["baz"])
	}

	configMap2 := map[string]any{"existing": true}
	changes2, err := RunMigrations(configMap2, "1.1.0")
	if err != nil {
		t.Fatalf("RunMigrations from 1.1.0: %v", err)
	}
	if len(changes2) != 1 {
		t.Fatalf("expected 1 change from 1.1.0, got %d: %v", len(changes2), changes2)
	}
	if _, ok := configMap2["foo"]; ok {
		t.Error("foo should not be set when migrating from 1.1.0")
	}
	if configMap2["baz"] != 42 {
		t.Errorf("baz = %v, want 42", configMap2["baz"])
	}

	configMap3 := map[string]any{"existing": true}
	changes3, err := RunMigrations(configMap3, "1.2.0")
	if err != nil {
		t.Fatalf("RunMigrations from 1.2.0: %v", err)
	}
	if len(changes3) != 0 {
		t.Errorf("expected 0 changes from 1.2.0, got %d: %v", len(changes3), changes3)
	}
}

func TestSetDefault(t *testing.T) {
	m := map[string]any{}
	msg := setDefault(m, "key", "val")
	if m["key"] != "val" {
		t.Errorf("key = %v, want val", m["key"])
	}
	if msg == "" {
		t.Error("expected non-empty message for new key")
	}

	msg = setDefault(m, "key", "other")
	if m["key"] != "val" {
		t.Errorf("key = %v, want val (should not overwrite)", m["key"])
	}
	if msg != "" {
		t.Errorf("expected empty message for existing key, got %q", msg)
	}
}

func TestRemoveField(t *testing.T) {
	m := map[string]any{"a": 1, "b": 2}
	msg := removeField(m, "a")
	if _, ok := m["a"]; ok {
		t.Error("a should be removed")
	}
	if msg == "" {
		t.Error("expected non-empty message for removed key")
	}

	msg = removeField(m, "nonexistent")
	if msg != "" {
		t.Errorf("expected empty message for missing key, got %q", msg)
	}
}

func TestRenameField(t *testing.T) {
	m := map[string]any{"old": "value"}
	msg := renameField(m, "old", "new")
	if _, ok := m["old"]; ok {
		t.Error("old key should be removed")
	}
	if m["new"] != "value" {
		t.Errorf("new = %v, want value", m["new"])
	}
	if msg == "" {
		t.Error("expected non-empty message for rename")
	}

	msg = renameField(m, "nonexistent", "other")
	if msg != "" {
		t.Errorf("expected empty message for missing key, got %q", msg)
	}
}

func TestSetNestedDefault(t *testing.T) {
	m := map[string]any{}

	msg := setNestedDefault(m, []string{"a", "b", "c"}, 99)
	if msg == "" {
		t.Error("expected non-empty message for new nested key")
	}
	a, ok := m["a"].(map[string]any)
	if !ok {
		t.Fatal("a should be a map")
	}
	b, ok := a["b"].(map[string]any)
	if !ok {
		t.Fatal("a.b should be a map")
	}
	if b["c"] != 99 {
		t.Errorf("a.b.c = %v, want 99", b["c"])
	}

	msg = setNestedDefault(m, []string{"a", "b", "c"}, 100)
	if msg != "" {
		t.Errorf("expected empty message for existing nested key, got %q", msg)
	}
	if b["c"] != 99 {
		t.Errorf("a.b.c = %v, want 99 (should not overwrite)", b["c"])
	}

	msg = setNestedDefault(m, []string{}, "nope")
	if msg != "" {
		t.Errorf("expected empty message for empty path, got %q", msg)
	}
}

func TestSetNestedDefaultExistingNonMap(t *testing.T) {
	m := map[string]any{"x": "string_value"}
	msg := setNestedDefault(m, []string{"x", "y"}, 1)
	if msg != "" {
		t.Errorf("expected empty message when intermediate is not a map, got %q", msg)
	}
}
