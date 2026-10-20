package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const migrationStateTable = "gorest_migration_state"

type migrationState struct {
	Checksum string
	Status   string
}

type mongoMigrationState struct {
	ID       string    `bson:"_id"`
	Checksum string    `bson:"content_hash"`
	Status   string    `bson:"status"`
	Updated  time.Time `bson:"updated_at"`
}

func loadMigrationStates(ctx context.Context, db *Database, config *Config, driver string) (map[string]migrationState, error) {
	states := make(map[string]migrationState)
	if err := ensureMigrationStateTable(ctx, db, config, driver); err != nil {
		return nil, err
	}

	switch driver {
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		rows, _, _, err := db.QuerySQL(ctx, config,
			"SELECT migration_name, content_hash, status FROM "+migrationStateTable)
		if err != nil {
			return nil, fmt.Errorf("read migration state: %w", err)
		}
		defer func() { _ = rows.Close() }()
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
	case MONGO:
		collection, _, _, err := db.Collection(config, migrationStateTable)
		if err != nil {
			return nil, fmt.Errorf("open MongoDB migration state collection: %w", err)
		}
		cursor, err := collection.Find(ctx, bson.M{})
		if err != nil {
			return nil, fmt.Errorf("query MongoDB migration state: %w", err)
		}
		defer func() { _ = cursor.Close(ctx) }()
		var records []mongoMigrationState
		if err := cursor.All(ctx, &records); err != nil {
			return nil, fmt.Errorf("decode MongoDB migration state: %w", err)
		}
		for _, record := range records {
			states[record.ID] = migrationState{Checksum: record.Checksum, Status: record.Status}
		}
	case SCYLLA:
		iter, _, _, err := db.QueryCQL(ctx, config,
			"SELECT migration_name, content_hash, status FROM "+migrationStateTable)
		if err != nil {
			return nil, fmt.Errorf("query ScyllaDB migration state: %w", err)
		}
		var scanErr error
		for row := map[string]any{}; iter.MapScan(row); row = map[string]any{} {
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
	default:
		return nil, fmt.Errorf("read migration state: unsupported driver %q", driver)
	}
	return states, nil
}

func ensureMigrationStateTable(ctx context.Context, db *Database, config *Config, driver string) error {
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
		}
		if _, _, _, err := db.ExecSQL(ctx, config, query); err != nil && !isMigrationLockTableExistsError(err) {
			return fmt.Errorf("create migration state table: %w", err)
		}
	case MONGO:
		// MongoDB creates this collection on the first state insert. The _id
		// field is the state key and has a built-in unique index.
	case SCYLLA:
		query := "CREATE TABLE IF NOT EXISTS " + migrationStateTable +
			" (migration_name text PRIMARY KEY, content_hash text, status text, updated_at timestamp)"
		if _, _, err := db.ExecCQL(ctx, config, query); err != nil {
			return fmt.Errorf("create ScyllaDB migration state table: %w", err)
		}
	default:
		return fmt.Errorf("create migration state table: unsupported driver %q", driver)
	}
	return nil
}

func insertMigrationState(ctx context.Context, db *Database, config *Config, driver, name, checksum, status string) error {
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
		}
		if _, _, _, err := db.ExecSQL(ctx, config, query, name, checksum, status, updatedAt); err != nil {
			return fmt.Errorf("insert migration state %q: %w", name, err)
		}
	case MONGO:
		collection, _, _, err := db.Collection(config, migrationStateTable)
		if err != nil {
			return fmt.Errorf("open MongoDB migration state collection: %w", err)
		}
		_, err = collection.InsertOne(ctx, mongoMigrationState{ID: name, Checksum: checksum, Status: status, Updated: updatedAt})
		if err != nil {
			return fmt.Errorf("insert MongoDB migration state %q: %w", name, err)
		}
	case SCYLLA:
		if _, _, err := db.ExecCQL(ctx, config,
			"INSERT INTO "+migrationStateTable+" (migration_name, content_hash, status, updated_at) VALUES (?, ?, ?, ?)",
			name, checksum, status, updatedAt,
		); err != nil {
			return fmt.Errorf("insert ScyllaDB migration state %q: %w", name, err)
		}
	default:
		return fmt.Errorf("insert migration state: unsupported driver %q", driver)
	}
	return nil
}

func updateMigrationState(ctx context.Context, db *Database, config *Config, driver, name, status string) error {
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
		}
		if _, _, _, err := db.ExecSQL(ctx, config, query, args...); err != nil {
			return fmt.Errorf("update migration state %q: %w", name, err)
		}
	case MONGO:
		collection, _, _, err := db.Collection(config, migrationStateTable)
		if err != nil {
			return fmt.Errorf("open MongoDB migration state collection: %w", err)
		}
		result, err := collection.UpdateOne(ctx, bson.M{"_id": name}, bson.M{"$set": bson.M{"status": status, "updated_at": updatedAt}})
		if err != nil {
			return fmt.Errorf("update MongoDB migration state %q: %w", name, err)
		}
		if result.MatchedCount != 1 {
			return fmt.Errorf("update MongoDB migration state %q: state row not found", name)
		}
	case SCYLLA:
		if _, _, err := db.ExecCQL(ctx, config,
			"UPDATE "+migrationStateTable+" SET status = ?, updated_at = ? WHERE migration_name = ?",
			status, updatedAt, name,
		); err != nil {
			return fmt.Errorf("update ScyllaDB migration state %q: %w", name, err)
		}
	default:
		return fmt.Errorf("update migration state: unsupported driver %q", driver)
	}
	return nil
}
