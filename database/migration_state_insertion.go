package database

import (
	"context"
	"fmt"
	"time"
)

func insertMigrationState(ctx context.Context, database *Database, config *Config, driver, name, checksum, status string) error {
	updatedAt := time.Now().UTC()

	switch driver {
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		query := "INSERT INTO " + migrationStateTable + " (migration_name, content_hash, status, updated_at) VALUES (?, ?, ?, ?)"

		switch driver {
		case POSTGRES:
			query = "INSERT INTO " + migrationStateTable + " (migration_name, content_hash, status, updated_at) VALUES ($1, $2, $3, $4)"
		case SQLSERVER:
			query = "INSERT INTO " + migrationStateTable + " (migration_name, content_hash, status, updated_at) VALUES (@p1, @p2, @p3, @p4)"
		case ORACLE:
			query = "INSERT INTO " + migrationStateTable + " (migration_name, content_hash, status, updated_at) VALUES (:1, :2, :3, :4)"
		default:
		}

		_, _, _, err := database.ExecSQL(ctx, config, query, name, checksum, status, updatedAt)
		if err != nil {
			return fmt.Errorf("insert migration state %q: %w", name, err)
		}
	case MONGO:
		collection, _, _, err := database.Collection(config, migrationStateTable)
		if err != nil {
			return fmt.Errorf("open MongoDB migration state collection: %w", err)
		}

		_, err = collection.InsertOne(ctx, mongoMigrationState{ID: name, Checksum: checksum, Status: status, Updated: updatedAt})
		if err != nil {
			return fmt.Errorf("insert MongoDB migration state %q: %w", name, err)
		}
	case SCYLLA:
		_, _, err := database.ExecCQL(ctx, config,
			"INSERT INTO "+migrationStateTable+" (migration_name, content_hash, status, updated_at) VALUES (?, ?, ?, ?)",
			name, checksum, status, updatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert ScyllaDB migration state %q: %w", name, err)
		}
	default:
		return fmt.Errorf("insert migration state: unsupported driver %q", driver)
	}

	return nil
}
