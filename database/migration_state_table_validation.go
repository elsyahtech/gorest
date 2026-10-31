package database

import (
	"context"
	"fmt"
)

func ensureMigrationStateTable(ctx context.Context, database *Database, config *Config, driver string) error {
	switch driver {
	case MYSQL, POSTGRES, SQLITE, SQLSERVER, ORACLE:
		var query string

		switch driver {
		case MYSQL, POSTGRES, SQLITE:
			query = `CREATE TABLE IF NOT EXISTS gorest_migration_state (
				migration_name VARCHAR(255) NOT NULL PRIMARY KEY,
				content_hash VARCHAR(64) NOT NULL,
				status VARCHAR(20) NOT NULL,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`
		case SQLSERVER:
			query = `IF OBJECT_ID(N'gorest_migration_state', N'U') IS NULL
			CREATE TABLE gorest_migration_state (
				migration_name VARCHAR(255) NOT NULL PRIMARY KEY,
				content_hash VARCHAR(64) NOT NULL,
				status VARCHAR(20) NOT NULL,
				updated_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
			)`
		case ORACLE:
			query = `DECLARE
				v_count NUMBER;
			BEGIN
				SELECT COUNT(*) INTO v_count FROM user_tables WHERE table_name = 'GOREST_MIGRATION_STATE';
				IF v_count = 0 THEN
					EXECUTE IMMEDIATE 'CREATE TABLE gorest_migration_state (
						migration_name VARCHAR2(255) NOT NULL PRIMARY KEY,
					content_hash VARCHAR2(64) NOT NULL,
						status VARCHAR2(20) NOT NULL,
						updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
					)';
				END IF;
			END;`
		default:
		}

		_, _, _, err := database.ExecSQL(ctx, config, query)
		if err != nil && !isMigrationLockTableExistsError(err) {
			return fmt.Errorf("create migration state table: %w", err)
		}
	case MONGO:
		// MongoDB creates this collection on the first state insert. The _id
		// field is the state key and has a built-in unique index.
	case SCYLLA:
		query := "CREATE TABLE IF NOT EXISTS " + migrationStateTable +
			" (migration_name text PRIMARY KEY, content_hash text, status text, updated_at timestamp)"

		if _, _, err := database.ExecCQL(ctx, config, query); err != nil {
			return fmt.Errorf("create ScyllaDB migration state table: %w", err)
		}
	default:
		return fmt.Errorf("create migration state table: unsupported driver %q", driver)
	}

	return nil
}
