package export

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.8.0",
		Summary: "Add log_skip_rules for per-sink skip conditions",
		Fn:      migrateV180,
	})
}

func migrateV180(m map[string]any) []string {
	var changes []string
	if c := setDefault(m, "log_skip_rules", map[string]any{}); c != "" {
		changes = append(changes, c)
	}
	return changes
}
