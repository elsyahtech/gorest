package database

// filterPendingSQLSeeders returns only seeders not yet executed.
func filterPendingMigrationExec(files []File, executed []string) []File {
	executedMap := make(map[string]bool)
	for _, name := range executed {
		executedMap[name] = true
	}

	var pending []File

	for _, file := range files {
		if !executedMap[file.Name] {
			pending = append(pending, file)
		}
	}

	return pending
}
