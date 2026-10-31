package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func updateMigrationState(ctx context.Context, database *Database, config *Config, driver, name, status string) error {
	updatedAt := time.Now().UTC()

	switch driver {
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		query := "UPDATE " + migrationStateTable + " SET status = ?, updated_at = ? WHERE migration_name = ?"
		args := []any{status, updatedAt, name}

		switch driver {
		case POSTGRES:
			query = "UPDATE " + migrationStateTable + " SET status = $1, updated_at = $2 WHERE migration_name = $3"
		case SQLSERVER:
			query = "UPDATE " + migrationStateTable + " SET status = @p1, updated_at = @p2 WHERE migration_name = @p3"
		case ORACLE:
			query = "UPDATE " + migrationStateTable + " SET status = :1, updated_at = :2 WHERE migration_name = :3"
		default:
		}

		_, _, _, err := database.ExecSQL(ctx, config, query, args...)
		if err != nil {
			return fmt.Errorf("update migration state %q: %w", name, err)
		}
	case MONGO:
		collection, _, _, err := database.Collection(config, migrationStateTable)
		if err != nil {
			return fmt.Errorf("open MongoDB migration state collection: %w", err)
		}

		result, err := collection.UpdateOne(ctx, bson.M{_id: name}, bson.M{dolarSet: bson.M{"status": status, "updated_at": updatedAt}})
		if err != nil {
			return fmt.Errorf("update MongoDB migration state %q: %w", name, err)
		}

		if result.MatchedCount != 1 {
			return fmt.Errorf("update MongoDB migration state %q: state row not found", name)
		}
	case SCYLLA:
		_, _, err := database.ExecCQL(ctx, config,
			"UPDATE "+migrationStateTable+" SET status = ?, updated_at = ? WHERE migration_name = ?",
			status, updatedAt, name,
		)
		if err != nil {
			return fmt.Errorf("update ScyllaDB migration state %q: %w", name, err)
		}
	default:
		return fmt.Errorf("update migration state: unsupported driver %q", driver)
	}

	return nil
}
