package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestMigrationExecAppliesAndSkipsPreviouslyAppliedSQLFiles(t *testing.T) {
	connection, err := sql.Open(SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	migrationDirectory := t.TempDir()
	migration := "CREATE TABLE migration_exec_users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);" +
		"INSERT INTO migration_exec_users (id, name) VALUES (1, 'Ada');"
	if err := os.WriteFile(filepath.Join(migrationDirectory, "001_users.sql"), []byte(migration), 0o600); err != nil {
		t.Fatalf("write migration file: %v", err)
	}

	config := &Config{
		Driver:             SQLITE,
		Migration:          true,
		MigrationDirectory: migrationDirectory,
		Timeout:            DefaultTimeout,
	}
	db := &Database{SQL: connection}
	ctx := context.Background()

	message, err := db.Migration(ctx, config, zap.NewNop())
	if err != nil {
		t.Fatalf("Migration() failed: %s: %v", message, err)
	}

	var name string
	if err := connection.QueryRow("SELECT name FROM migration_exec_users WHERE id = 1").Scan(&name); err != nil {
		t.Fatalf("read migrated row: %v", err)
	}
	if name != "Ada" {
		t.Fatalf("migrated name = %q, want Ada", name)
	}

	message, err = db.Migration(ctx, config, zap.NewNop())
	if err != nil {
		t.Fatalf("repeated Migration() failed: %s: %v", message, err)
	}

	var rowCount int
	if err := connection.QueryRow("SELECT COUNT(*) FROM migration_exec_users").Scan(&rowCount); err != nil {
		t.Fatalf("count migrated rows: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("repeated migration left %d rows, want 1", rowCount)
	}
}

func TestMigrationExecWithNoFilesInitializesStateAndCompletes(t *testing.T) {
	connection, err := sql.Open(SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	config := &Config{
		Driver:             SQLITE,
		Migration:          true,
		MigrationDirectory: t.TempDir(),
		Timeout:            DefaultTimeout,
	}
	message, err := (&Database{SQL: connection}).Migration(context.Background(), config, zap.NewNop())
	if err != nil {
		t.Fatalf("Migration() with no files failed: %s: %v", message, err)
	}

	var tableCount int
	if err := connection.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		migrationStateTable,
	).Scan(&tableCount); err != nil {
		t.Fatalf("check migration state table: %v", err)
	}
	if tableCount != 1 {
		t.Fatalf("migration state table count = %d, want 1", tableCount)
	}
}

func TestMigrationRollbackFailureReportsCompleteAndIncompleteOutcomes(t *testing.T) {
	connection, err := sql.Open(SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})
	for _, statement := range []string{
		"CREATE TABLE gorest_migration (migration_name TEXT PRIMARY KEY, executed_at TIMESTAMP)",
		"CREATE TABLE gorest_migration_state (migration_name TEXT PRIMARY KEY, content_hash TEXT, status TEXT, updated_at TIMESTAMP)",
	} {
		if _, err := connection.Exec(statement); err != nil {
			t.Fatalf("create rollback metadata table: %v", err)
		}
	}

	newExecution := func() (*structMigrationExecution, *bool) {
		releaseLockAfterRollback := false
		return &structMigrationExecution{
			database:                 &Database{SQL: connection},
			config:                   &Config{Driver: SQLITE},
			log:                      zap.NewNop(),
			driver:                   SQLITE,
			execType:                 typeMigration,
			tableName:                migrationTableName,
			columnName:               migrationColumnName,
			releaseLockAfterRollback: &releaseLockAfterRollback,
		}, &releaseLockAfterRollback
	}

	file := File{Name: "001_failure.sql", CompensationContent: "SELECT 1;"}
	execution, releaseLock := newExecution()
	message, err := execution.rollbackFailure(file, "migration failed", errors.New("statement error"))
	if err == nil || !*releaseLock || !strings.Contains(message, "rolled back all changes") {
		t.Fatalf("complete rollback = (%q, %v, release lock %t)", message, err, *releaseLock)
	}
	fields := execution.rollbackLogFields(file)
	if len(fields) != 3 || fields[0].Key != "driver" || fields[1].Key != "execution_type" ||
		fields[2].Key != "failed_file" {
		t.Fatalf("rollback log fields = %#v, want driver, execution_type, and failed_file", fields)
	}

	execution, releaseLock = newExecution()
	file.CompensationContent = "INVALID SQL;"
	message, err = execution.rollbackFailure(file, "migration failed", errors.New("statement error"))
	if err == nil || *releaseLock || !strings.Contains(message, "could not fully roll back") ||
		!strings.Contains(err.Error(), "rollback incomplete") {
		t.Fatalf("incomplete rollback = (%q, %v, release lock %t)", message, err, *releaseLock)
	}
}
