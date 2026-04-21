package export

func init() {
	migrations = append(migrations, Migration{
		Before:  "1.9.0",
		Summary: "Rename route_ip_lists to domain_ip_lists",
		Fn:      migrateV190,
	})
}

func migrateV190(m map[string]any) []string {
	var changes []string
	if c := renameField(m, "route_ip_lists", "domain_ip_lists"); c != "" {
		changes = append(changes, c)
	}
	return changes
}
