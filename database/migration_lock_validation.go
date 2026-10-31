package database

import "strings"

func isMigrationLockTableExistsError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())

	return strings.Contains(message, "ora-00955") ||
		strings.Contains(message, "already an object named") ||
		strings.Contains(message, "already exists") ||
		strings.Contains(message, "relation \"gorest_migration_lock\" already exists")
}
