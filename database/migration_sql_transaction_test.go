package database

import (
	"context"
	"database/sql"
	"testing"
)

func TestSQLMigrationExecutionRollsBackScriptAndHistoryOnFailure(t *testing.T) {
	connection, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer connection.Close()
	connection.SetMaxOpenConns(1)

	ctx := context.Background()
	if _, err := connection.ExecContext(ctx,
		"CREATE TABLE gorest_migration (migration_name TEXT PRIMARY KEY, executed_at TIMESTAMP)",
	); err != nil {
		t.Fatalf("create migration history table: %v", err)
	}

	db := &Database{SQL: connection}
	config := &Config{Driver: SQLITE}
	file := File{
		Name:    "001_partial.sql",
		Content: "CREATE TABLE partial_change (id INTEGER); INSERT INTO missing_table (id) VALUES (1);",
	}
	_, err = sqlDataMigrationExec(ctx, db, config, file, migrationTableName, migrationColumnName)
	if err == nil {
		t.Fatal("sqlDataMigrationExec() succeeded for invalid second statement")
	}

	var tableCount int
	if err := connection.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'partial_change'",
	).Scan(&tableCount); err != nil {
		t.Fatalf("query partial table state: %v", err)
	}
	if tableCount != 0 {
		t.Fatalf("partial table count = %d, want 0 after rollback", tableCount)
	}

	var historyCount int
	if err := connection.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM gorest_migration WHERE migration_name = ?",
		file.Name,
	).Scan(&historyCount); err != nil {
		t.Fatalf("query migration history: %v", err)
	}
	if historyCount != 0 {
		t.Fatalf("migration history count = %d, want 0 after rollback", historyCount)
	}
}
