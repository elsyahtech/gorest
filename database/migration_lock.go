package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const migrationLockTable = "gorest_migration_lock"

// acquireMigrationLock takes a database-wide, fail-closed lock for one startup
// task. The lock has no automatic expiry: after an unclean process exit, an
// operator must inspect the database and explicitly clear the stale record.
func acquireMigrationLock(
	ctx context.Context,
	db *Database,
	config *Config,
) (func(context.Context) error, string, error) {
	if ctx == nil || db == nil || config == nil {
		return nil, "Ensure the migration context and database configuration are initialized.",
			errors.New("migration lock: missing context, database, or config")
	}

	owner, err := newMigrationLockOwner()
	if err != nil {
		return nil, "Ensure the application can generate a unique migration lock owner.",
			fmt.Errorf("migration lock: generate owner: %w", err)
	}

	// One shared lock also prevents a seeder in another process from running
	// while a schema migration is in progress.
	lockName := "database_startup"
	driver := NormalizeDatabaseDriver(config.Driver)
	switch driver {
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		if err := ensureSQLMigrationLockTable(ctx, db, config, driver); err != nil {
			return nil, "Ensure the database user can create and access the GOREST migration lock table.", err
		}

		query := "INSERT INTO " + migrationLockTable + " (lock_name, owner_id, acquired_at) VALUES (?, ?, CURRENT_TIMESTAMP)"
		args := []any{lockName, owner}
		switch driver {
		case POSTGRES:
			query = "INSERT INTO " + migrationLockTable + " (lock_name, owner_id, acquired_at) VALUES ($1, $2, CURRENT_TIMESTAMP)"
		case SQLSERVER:
			query = "INSERT INTO " + migrationLockTable + " (lock_name, owner_id, acquired_at) VALUES (@p1, @p2, SYSUTCDATETIME())"
		case ORACLE:
			query = "INSERT INTO " + migrationLockTable + " (lock_name, owner_id, acquired_at) VALUES (:1, :2, CURRENT_TIMESTAMP)"
		}
		if _, _, _, err := db.ExecSQL(ctx, config, query, args...); err != nil {
			if IsDuplicateKeyError(err) {
				return nil, migrationLockHeldMessage(), fmt.Errorf("migration lock: %s lock is already held", lockName)
			}
			return nil, "Ensure the database is reachable and the migration lock table accepts writes.",
				fmt.Errorf("migration lock: acquire SQL lock: %w", err)
		}

		return func(releaseCtx context.Context) error {
			query := "DELETE FROM " + migrationLockTable + " WHERE lock_name = ? AND owner_id = ?"
			args := []any{lockName, owner}
			switch driver {
			case POSTGRES:
				query = "DELETE FROM " + migrationLockTable + " WHERE lock_name = $1 AND owner_id = $2"
			case SQLSERVER:
				query = "DELETE FROM " + migrationLockTable + " WHERE lock_name = @p1 AND owner_id = @p2"
			case ORACLE:
				query = "DELETE FROM " + migrationLockTable + " WHERE lock_name = :1 AND owner_id = :2"
			}
			_, _, _, err := db.ExecSQL(releaseCtx, config, query, args...)
			return err
		}, "", nil

	case MONGO:
		collection, _, _, err := db.Collection(config, migrationLockTable)
		if err != nil {
			return nil, "Ensure MongoDB is initialized and the migration lock collection is accessible.",
				fmt.Errorf("migration lock: open MongoDB lock collection: %w", err)
		}
		_, err = collection.InsertOne(ctx, bson.M{
			"_id":         lockName,
			"owner_id":    owner,
			"acquired_at": time.Now().UTC(),
		})
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return nil, migrationLockHeldMessage(), fmt.Errorf("migration lock: %s lock is already held", lockName)
			}
			return nil, "Ensure MongoDB is reachable and the migration lock collection accepts writes.",
				fmt.Errorf("migration lock: acquire MongoDB lock: %w", err)
		}

		return func(releaseCtx context.Context) error {
			_, err := collection.DeleteOne(releaseCtx, bson.M{"_id": lockName, "owner_id": owner})
			return err
		}, "", nil

	case SCYLLA:
		create := "CREATE TABLE IF NOT EXISTS " + migrationLockTable + " (lock_name text PRIMARY KEY, owner_id text, acquired_at timestamp)"
		if _, _, err := db.ExecCQL(ctx, config, create); err != nil {
			return nil, "Ensure the ScyllaDB keyspace allows creation and access to the migration lock table.",
				fmt.Errorf("migration lock: create ScyllaDB lock table: %w", err)
		}

		applied, _, _, err := db.MapScanCAS(ctx, config,
			"INSERT INTO "+migrationLockTable+" (lock_name, owner_id, acquired_at) VALUES (?, ?, ?) IF NOT EXISTS",
			lockName, owner, time.Now().UTC(),
		)
		if err != nil {
			return nil, "Ensure the ScyllaDB migration lock table supports conditional writes and is reachable.",
				fmt.Errorf("migration lock: acquire ScyllaDB lock: %w", err)
		}
		if !applied {
			return nil, migrationLockHeldMessage(), fmt.Errorf("migration lock: %s lock is already held", lockName)
		}

		return func(releaseCtx context.Context) error {
			_, _, err := db.ExecCQL(releaseCtx, config,
				"DELETE FROM "+migrationLockTable+" WHERE lock_name = ? IF owner_id = ?", lockName, owner)
			return err
		}, "", nil

	default:
		return nil, "Configure one of the supported GOREST database drivers before running migrations.",
			fmt.Errorf("migration lock: unsupported driver %q", config.Driver)
	}
}

func ensureSQLMigrationLockTable(ctx context.Context, db *Database, config *Config, driver string) error {
	var query string
	switch driver {
	case MYSQL, POSTGRES, SQLITE:
		query = `CREATE TABLE IF NOT EXISTS gorest_migration_lock (
			lock_name VARCHAR(255) NOT NULL PRIMARY KEY,
			owner_id VARCHAR(64) NOT NULL,
			acquired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	case SQLSERVER:
		query = `IF OBJECT_ID(N'gorest_migration_lock', N'U') IS NULL
		CREATE TABLE gorest_migration_lock (
			lock_name VARCHAR(255) NOT NULL PRIMARY KEY,
			owner_id VARCHAR(64) NOT NULL,
			acquired_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
		)`
	case ORACLE:
		query = `DECLARE
			v_count NUMBER;
		BEGIN
			SELECT COUNT(*) INTO v_count FROM user_tables WHERE table_name = 'GOREST_MIGRATION_LOCK';
			IF v_count = 0 THEN
				EXECUTE IMMEDIATE 'CREATE TABLE gorest_migration_lock (
					lock_name VARCHAR2(255) NOT NULL PRIMARY KEY,
					owner_id VARCHAR2(64) NOT NULL,
					acquired_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
				)';
			END IF;
		END;`
	default:
		return fmt.Errorf("migration lock: unsupported SQL driver %q", driver)
	}

	if _, _, _, err := db.ExecSQL(ctx, config, query); err != nil && !isMigrationLockTableExistsError(err) {
		return fmt.Errorf("migration lock: create SQL lock table: %w", err)
	}
	return nil
}

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

func migrationLockHeldMessage() string {
	return "A migration or seeder runner already holds this database lock, or a previous run stopped before releasing it. Inspect migration state before manually removing a stale lock."
}

func newMigrationLockOwner() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
