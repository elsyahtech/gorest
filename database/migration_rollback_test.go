package database

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestDeleteMigrationHistoryAndStateBackendBranches(t *testing.T) {
	ctx := context.Background()
	connection, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	db := &Database{SQL: connection}

	for _, driver := range []string{MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE} {
		t.Run("history/"+driver, func(t *testing.T) {
			if err := deleteExecutionHistory(ctx, db, &Config{Driver: driver}, driver, migrationTableName, migrationColumnName, "001.sql"); err == nil {
				t.Fatal("deleteExecutionHistory() accepted a closed SQL database")
			}
			if err := deleteMigrationState(ctx, db, &Config{Driver: driver}, driver, "migration:001.sql"); err == nil {
				t.Fatal("deleteMigrationState() accepted a closed SQL database")
			}
		})
	}

	if err := deleteExecutionHistory(ctx, &Database{}, &Config{Driver: MONGO}, MONGO, migrationTableName, migrationColumnName, "001.json"); err == nil {
		t.Fatal("deleteExecutionHistory() accepted an unavailable MongoDB client")
	}
	if err := deleteMigrationState(ctx, &Database{}, &Config{Driver: MONGO}, MONGO, "migration:001.json"); err == nil {
		t.Fatal("deleteMigrationState() accepted an unavailable MongoDB client")
	}
	if err := deleteExecutionHistory(ctx, &Database{}, &Config{Driver: SCYLLA}, SCYLLA, migrationTableName, migrationColumnName, "001.cql"); err == nil {
		t.Fatal("deleteExecutionHistory() accepted an unavailable Scylla session")
	}
	if err := deleteMigrationState(ctx, &Database{}, &Config{Driver: SCYLLA}, SCYLLA, "migration:001.cql"); err == nil {
		t.Fatal("deleteMigrationState() accepted an unavailable Scylla session")
	}
	if err := deleteExecutionHistory(ctx, db, &Config{Driver: "unsupported"}, "unsupported", migrationTableName, migrationColumnName, "001"); err == nil {
		t.Fatal("deleteExecutionHistory() accepted an unsupported driver")
	}
	if err := deleteMigrationState(ctx, db, &Config{Driver: "unsupported"}, "unsupported", "001"); err == nil {
		t.Fatal("deleteMigrationState() accepted an unsupported driver")
	}
}

func TestDeleteMigrationHistoryAndStateForSQLite(t *testing.T) {
	connection, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetMaxOpenConns(1)
	ctx := context.Background()
	for _, statement := range []string{
		"CREATE TABLE gorest_migration (migration_name TEXT PRIMARY KEY)",
		"CREATE TABLE gorest_migration_state (migration_name TEXT PRIMARY KEY)",
		"INSERT INTO gorest_migration VALUES ('001.sql')",
		"INSERT INTO gorest_migration_state VALUES ('migration:001.sql')",
	} {
		if _, err := connection.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	db := &Database{SQL: connection}
	config := &Config{Driver: SQLITE}
	if err := deleteExecutionHistory(ctx, db, config, SQLITE, migrationTableName, migrationColumnName, "001.sql"); err != nil {
		t.Fatalf("deleteExecutionHistory() error = %v", err)
	}
	if err := deleteMigrationState(ctx, db, config, SQLITE, "migration:001.sql"); err != nil {
		t.Fatalf("deleteMigrationState() error = %v", err)
	}
}

func TestRollbackExecutionBatchReversesChangesAndClearsHistory(t *testing.T) {
	connection, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer connection.Close()
	connection.SetMaxOpenConns(1)

	ctx := context.Background()
	for _, statement := range []string{
		"CREATE TABLE gorest_migration (migration_name TEXT PRIMARY KEY, executed_at TIMESTAMP)",
		"CREATE TABLE gorest_migration_state (migration_name TEXT PRIMARY KEY, content_hash TEXT, status TEXT, updated_at TIMESTAMP)",
		"CREATE TABLE users (id INTEGER)",
		"CREATE TABLE posts (id INTEGER)",
		"INSERT INTO gorest_migration (migration_name) VALUES ('001_users.sql'), ('002_posts.sql')",
		"INSERT INTO gorest_migration_state (migration_name, content_hash, status) VALUES ('migration:001_users.sql', 'one', 'applied'), ('migration:002_posts.sql', 'two', 'running')",
	} {
		if _, err := connection.ExecContext(ctx, statement); err != nil {
			t.Fatalf("execute setup statement %q: %v", statement, err)
		}
	}

	db := &Database{SQL: connection}
	config := &Config{Driver: SQLITE}
	completed := File{Name: "001_users.sql", CompensationContent: "DROP TABLE users;"}
	failed := File{Name: "002_posts.sql", CompensationContent: "DROP TABLE posts;"}

	complete, err := rollbackExecutionBatch(&structRollbackExecutionBatch{
		database:             db,
		config:               config,
		driver:               SQLITE,
		execType:             typeMigration,
		failed:               failed,
		completed:            []File{completed},
		tableName:            migrationTableName,
		columnName:           migrationColumnName,
		failedFileRolledBack: false,
	})
	if err != nil {
		t.Fatalf("rollbackExecutionBatch() error = %v", err)
	}
	if !complete {
		t.Fatal("rollbackExecutionBatch() reported incomplete rollback")
	}

	var remainingTables int
	if err := connection.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('users', 'posts')",
	).Scan(&remainingTables); err != nil {
		t.Fatalf("query remaining schema: %v", err)
	}
	if remainingTables != 0 {
		t.Fatalf("remaining application tables = %d, want 0", remainingTables)
	}

	for _, tableName := range []string{migrationTableName, migrationStateTable} {
		var remaining int
		if err := connection.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tableName).Scan(&remaining); err != nil {
			t.Fatalf("query remaining records in %s: %v", tableName, err)
		}
		if remaining != 0 {
			t.Errorf("remaining records in %s = %d, want 0", tableName, remaining)
		}
	}
}

func TestExecuteSQLCompensationSuccessAndFailure(t *testing.T) {
	connection, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetMaxOpenConns(1)
	if _, err := connection.Exec("CREATE TABLE compensation_test (id INTEGER)"); err != nil {
		t.Fatal(err)
	}

	db := &Database{SQL: connection}
	config := &Config{Driver: SQLITE}
	if err := executeCompensation(context.Background(), db, config, SQLITE, typeMigration,
		File{CompensationContent: "DROP TABLE compensation_test"}); err != nil {
		t.Fatalf("executeCompensation() error = %v", err)
	}

	if err := executeCompensation(context.Background(), db, config, SQLITE, typeMigration,
		File{CompensationContent: "NOT VALID SQL"}); err == nil {
		t.Fatal("executeCompensation() accepted invalid SQL")
	}
}

func TestExecuteCompensationBackendBranches(t *testing.T) {
	ctx := context.Background()
	if err := executeCompensation(ctx, &Database{}, &Config{Driver: SCYLLA}, SCYLLA, typeMigration, File{CompensationContent: "DELETE FROM records"}); err == nil {
		t.Fatal("executeCompensation() accepted an unavailable Scylla session")
	}
	if err := executeCompensation(ctx, &Database{}, &Config{Driver: "unsupported"}, "unsupported", typeMigration, File{}); err == nil {
		t.Fatal("executeCompensation() accepted an unsupported driver")
	}
	if err := executeCompensation(ctx, &Database{}, &Config{Driver: MONGO}, MONGO, typeSeeder, File{CompensationContent: `{}`}); err == nil {
		t.Fatal("executeCompensation() accepted invalid MongoDB seeder compensation")
	}
}

func TestMongoMigrationCompensationValidatesActionsBeforeConnecting(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantMessage string
	}{
		{
			name:        "malformed JSON",
			content:     "{",
			wantMessage: "parse MongoDB migration compensation JSON",
		},
		{
			name:        "empty actions",
			content:     `{}`,
			wantMessage: "must specify dropCollections or dropIndexes",
		},
		{
			name:        "incomplete index action",
			content:     `{"dropIndexes":[{"collection":"users"}]}`,
			wantMessage: "require collection and name",
		},
		{
			name:        "empty collection name",
			content:     `{"dropCollections":[""]}`,
			wantMessage: "dropCollections entries cannot be empty",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := executeMongoMigrationCompensation(
				context.Background(),
				nil,
				nil,
				File{CompensationContent: test.content},
			)
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("error = %v, want message containing %q", err, test.wantMessage)
			}
		})
	}
}

func TestMongoSeederCompensationValidatesActionsBeforeConnecting(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantMessage string
	}{
		{
			name:        "malformed JSON",
			content:     "{",
			wantMessage: "parse MongoDB seeder compensation JSON",
		},
		{
			name:        "missing collection and filters",
			content:     `{}`,
			wantMessage: "requires collection and non-empty delete filters",
		},
		{
			name:        "empty delete filter",
			content:     `{"collection":"users","delete":[{}]}`,
			wantMessage: "delete filters cannot be empty",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := executeMongoSeederCompensation(
				context.Background(),
				nil,
				nil,
				File{CompensationContent: test.content},
			)
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("error = %v, want message containing %q", err, test.wantMessage)
			}
		})
	}
}
