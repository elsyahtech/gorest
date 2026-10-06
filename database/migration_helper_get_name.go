package database

// getMigrationNames returns list of migration filenames (for logging)
// Converts File slice to string slice containing just filenames
// Used for logging which migrations were found/executed.
func getNames(migrations []File) []string {
	names := make([]string, 0, len(migrations))

	for _, m := range migrations {
		names = append(names, m.Name)
	}

	return names
}
