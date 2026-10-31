package database

import (
	"context"
	"errors"
	"fmt"
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
	database *Database,
	config *Config,
) (func(context.Context) error, string, error) {
	if ctx == nil || database == nil || config == nil {
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
		return sqlAcquireMigrationLock(ctx, database, config, driver, lockName, owner)
	case MONGO:
		return mongoCaseAcquireMigrationLock(ctx, database, config, lockName, owner)
	case SCYLLA:
		return scyllaCaseAcquireMigrationLock(ctx, database, config, lockName, owner)
	default:
		return nil, "Configure one of the supported GOREST database drivers before running migrations.",
			fmt.Errorf("migration lock: unsupported driver %q", config.Driver)
	}
}

func sqlAcquireMigrationLock(
	ctx context.Context,
	database *Database,
	config *Config,
	driver string,
	lockName string,
	owner string,
) (func(context.Context) error, string, error) {
	if err := ensureSQLMigrationLockTable(ctx, database, config, driver); err != nil {
		return nil, "Ensure the database user can create and access the GOREST migration lock table.", err
	}

	query := sqlCaseAcquireMigrationLock(driver)
	args := []any{lockName, owner}

	_, _, _, err := database.ExecSQL(ctx, config, query, args...)
	if err != nil {
		if IsDuplicateKeyError(err) {
			return nil, migrationLockHeldMessage(), fmt.Errorf("migration lock: %s lock is already held", lockName)
		}

		return nil, "Ensure the database is reachable and the migration lock table accepts writes.",
			fmt.Errorf("migration lock: acquire SQL lock: %w", err)
	}

	return func(releaseCtx context.Context) error {
		return releaseSQLMigrationLock(releaseCtx, database, config, driver, lockName, owner)
	}, "", nil
}

func releaseSQLMigrationLock(
	ctx context.Context,
	database *Database,
	config *Config,
	driver string,
	lockName string,
	owner string,
) error {
	query := "DELETE FROM " + migrationLockTable + " WHERE lock_name = ? AND owner_id = ?"

	//nolint:revive
	switch driver {
	case POSTGRES:
		query = "DELETE FROM " + migrationLockTable + " WHERE lock_name = $1 AND owner_id = $2"
	case SQLSERVER:
		query = "DELETE FROM " + migrationLockTable + " WHERE lock_name = @p1 AND owner_id = @p2"
	case ORACLE:
		query = "DELETE FROM " + migrationLockTable + " WHERE lock_name = :1 AND owner_id = :2"
	}

	_, _, _, err := database.ExecSQL(ctx, config, query, lockName, owner)

	return err
}

func mongoCaseAcquireMigrationLock(
	ctx context.Context,
	database *Database,
	config *Config,
	lockName string,
	owner string,
) (func(context.Context) error, string, error) {
	collection, _, _, err := database.Collection(config, migrationLockTable)
	if err != nil {
		const message = "Ensure MongoDB is initialized and the migration lock collection is accessible."

		return nil, message,
			fmt.Errorf("migration lock: open MongoDB lock collection: %w", err)
	}

	const (
		ownerId    = "owner_id"
		acquiredAt = "acquired_at"
	)

	_, err = collection.InsertOne(ctx, bson.M{
		_id:        lockName,
		ownerId:    owner,
		acquiredAt: time.Now().UTC(),
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, migrationLockHeldMessage(), fmt.Errorf("migration lock: %s lock is already held", lockName)
		}

		const message = "Ensure MongoDB is reachable and the migration lock collection accepts writes."

		return nil, message,
			fmt.Errorf("migration lock: acquire MongoDB lock: %w", err)
	}

	return func(releaseCtx context.Context) error {
		_, err := collection.DeleteOne(releaseCtx, bson.M{"_id": lockName, "owner_id": owner})
		if err != nil {
			return fmt.Errorf("migration lock: release MongoDB lock: %w", err)
		}

		return nil
	}, "", nil
}

func scyllaCaseAcquireMigrationLock(
	ctx context.Context,
	database *Database,
	config *Config,
	lockName string,
	owner string,
) (func(context.Context) error, string, error) {
	create := "CREATE TABLE IF NOT EXISTS " + migrationLockTable + " (lock_name text PRIMARY KEY, owner_id text, acquired_at timestamp)"
	if _, _, err := database.ExecCQL(ctx, config, create); err != nil {
		const message = "Ensure the ScyllaDB keyspace allows creation and access to the migration lock table."

		return nil, message,
			fmt.Errorf("migration lock: create ScyllaDB lock table: %w", err)
	}

	applied, _, _, err := database.MapScanCAS(ctx, config,
		"INSERT INTO "+migrationLockTable+" (lock_name, owner_id, acquired_at) VALUES (?, ?, ?) IF NOT EXISTS",
		lockName, owner, time.Now().UTC(),
	)
	if err != nil {
		const message = "Ensure the ScyllaDB migration lock table supports conditional writes and is reachable."

		return nil, message,
			fmt.Errorf("migration lock: acquire ScyllaDB lock: %w", err)
	}

	if !applied {
		return nil, migrationLockHeldMessage(), fmt.Errorf("migration lock: %s lock is already held", lockName)
	}

	return func(releaseCtx context.Context) error {
		_, _, err := database.ExecCQL(releaseCtx, config,
			"DELETE FROM "+migrationLockTable+" WHERE lock_name = ? IF owner_id = ?", lockName, owner)

		return err
	}, "", nil
}

func ensureSQLMigrationLockTable(ctx context.Context, database *Database, config *Config, driver string) error {
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

	_, _, _, err := database.ExecSQL(ctx, config, query)
	if err != nil && !isMigrationLockTableExistsError(err) {
		return fmt.Errorf("migration lock: create SQL lock table: %w", err)
	}

	return nil
}

func migrationLockHeldMessage() string {
	const message = "A migration or seeder runner already holds this database lock, " +
		"or a previous run stopped before releasing it. Inspect migration state before manually removing a stale lock."

	return message
}
