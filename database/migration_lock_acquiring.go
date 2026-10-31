package database

func sqlCaseAcquireMigrationLock(driver string) string {
	switch driver {
	case POSTGRES:
		return "INSERT INTO " + migrationLockTable + " (lock_name, owner_id, acquired_at) VALUES ($1, $2, CURRENT_TIMESTAMP)"
	case SQLSERVER:
		return "INSERT INTO " + migrationLockTable + " (lock_name, owner_id, acquired_at) VALUES (@p1, @p2, SYSUTCDATETIME())"
	case ORACLE:
		return "INSERT INTO " + migrationLockTable + " (lock_name, owner_id, acquired_at) VALUES (:1, :2, CURRENT_TIMESTAMP)"
	default:
		return "INSERT INTO " + migrationLockTable + " (lock_name, owner_id, acquired_at) VALUES (?, ?, CURRENT_TIMESTAMP)"
	}
}
