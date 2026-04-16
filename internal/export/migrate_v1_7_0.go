package export

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.7.0",
		Summary: "Add route_settings for per-route metadata",
		Fn:      migrateV170,
	})
}

func migrateV170(m map[string]any) []string {
	var changes []string
	if c := setDefault(m, "route_settings", map[string]any{}); c != "" {
		changes = append(changes, c)
	}
	return changes
}
