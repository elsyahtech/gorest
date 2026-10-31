package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const migrationStateTable = "gorest_migration_state"

type migrationState struct {
	Checksum string
	Status   string
}

type mongoMigrationState struct {
	Updated  time.Time `bson:"updated_at"`
	ID       string    `bson:"_id"`
	Checksum string    `bson:"content_hash"`
	Status   string    `bson:"status"`
}

func loadMigrationStates(ctx context.Context, database *Database, config *Config, driver string) (map[string]migrationState, error) {
	states := make(map[string]migrationState)

	if err := ensureMigrationStateTable(ctx, database, config, driver); err != nil {
		return nil, err
	}

	switch driver {
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		return sqlCaseLoadMigrationStates(ctx, database, config, states)
	case MONGO:
		return mongoCaseLoadMigrationStates(ctx, database, config, states)
	case SCYLLA:
		return scyllaCaseLoadMigrationStates(ctx, database, config, states)
	default:
		return nil, fmt.Errorf("read migration state: unsupported driver %q", driver)
	}
}

func sqlCaseLoadMigrationStates(
	ctx context.Context,
	database *Database,
	config *Config,
	states map[string]migrationState,
) (map[string]migrationState, error) {
	rows, _, _, err := database.QuerySQL(ctx, config, "SELECT migration_name, content_hash, status FROM "+migrationStateTable)
	if err != nil {
		return nil, fmt.Errorf("read migration state: %w", err)
	}

	defer func() {
		if err = rows.Close(); err != nil {
			log.Print(err)
		}
	}()

	for rows.Next() {
		var name, checksum, status string

		if err := rows.Scan(&name, &checksum, &status); err != nil {
			return nil, fmt.Errorf("scan migration state: %w", err)
		}

		states[name] = migrationState{Checksum: checksum, Status: status}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration state: %w", err)
	}

	return states, nil
}

func mongoCaseLoadMigrationStates(
	ctx context.Context,
	database *Database,
	config *Config,
	states map[string]migrationState,
) (map[string]migrationState, error) {
	collection, _, _, err := database.Collection(config, migrationStateTable)
	if err != nil {
		return nil, fmt.Errorf("open MongoDB migration state collection: %w", err)
	}

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("query MongoDB migration state: %w", err)
	}

	defer func() {
		if err = cursor.Close(ctx); err != nil {
			log.Print(err)
		}
	}()

	var records []mongoMigrationState

	if err := cursor.All(ctx, &records); err != nil {
		return nil, fmt.Errorf("decode MongoDB migration state: %w", err)
	}

	for _, record := range records {
		states[record.ID] = migrationState{Checksum: record.Checksum, Status: record.Status}
	}

	return states, nil
}

func scyllaCaseLoadMigrationStates(
	ctx context.Context,
	database *Database,
	config *Config,
	states map[string]migrationState,
) (map[string]migrationState, error) {
	iter, _, _, err := database.QueryCQL(ctx, config, "SELECT migration_name, content_hash, status FROM "+migrationStateTable)
	if err != nil {
		return nil, fmt.Errorf("query ScyllaDB migration state: %w", err)
	}

	var scanErr error

	for row := make(map[string]any); iter.MapScan(row); row = make(map[string]any) {
		name, nameOK := row["migration_name"].(string)
		checksum, checksumOK := row["content_hash"].(string)
		status, statusOK := row["status"].(string)

		if !nameOK || !checksumOK || !statusOK {
			scanErr = fmt.Errorf("invalid ScyllaDB migration state row for %q", name)

			break
		}

		states[name] = migrationState{Checksum: checksum, Status: status}
	}

	if closeErr := iter.Close(); closeErr != nil {
		return nil, fmt.Errorf("close ScyllaDB migration state iterator: %w", closeErr)
	}

	if scanErr != nil {
		return nil, scanErr
	}

	return states, nil
}
