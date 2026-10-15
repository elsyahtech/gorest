package gorest

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
)

func newSQLiteDBSession(t *testing.T, timeout time.Duration) *DBSession {
	t.Helper()

	sqlDB, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	return &DBSession{
		database: &database.Database{SQL: sqlDB},
		config:   &database.Config{Driver: database.SQLITE, Timeout: timeout},
	}
}

func TestBeginTxUsesConfiguredTimeout(t *testing.T) {
	dbSession := newSQLiteDBSession(t, 30*time.Millisecond)

	txSession, _, _, err := dbSession.BeginTx()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	_, _, _, err = txSession.ExecSQL(context.Background(), "CREATE TABLE timeout_test (id INTEGER)")
	if err != nil {
		t.Fatalf("create table in transaction: %v", err)
	}

	time.Sleep(60 * time.Millisecond)
	if _, _, err := txSession.Commit(); err == nil {
		t.Fatal("commit succeeded after the configured transaction timeout")
	}

	var tableCount int
	if err := dbSession.database.SQL.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'timeout_test'",
	).Scan(&tableCount); err != nil {
		t.Fatalf("check table after transaction timeout: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("transaction changes remained after configured timeout")
	}
}

func TestBeginTxContextRespectsCallerCancellation(t *testing.T) {
	dbSession := newSQLiteDBSession(t, time.Second)
	ctx, cancel := context.WithCancel(context.Background())

	txSession, _, _, err := dbSession.BeginTxContext(ctx)
	if err != nil {
		cancel()
		t.Fatalf("begin transaction: %v", err)
	}

	_, _, _, err = txSession.ExecSQL(context.Background(), "CREATE TABLE canceled_test (id INTEGER)")
	if err != nil {
		t.Fatalf("create table in transaction: %v", err)
	}
	cancel()
	time.Sleep(20 * time.Millisecond)
	if _, _, err := txSession.Commit(); err == nil {
		t.Fatal("commit succeeded after the caller context was canceled")
	}

	var tableCount int
	if err := dbSession.database.SQL.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'canceled_test'",
	).Scan(&tableCount); err != nil {
		t.Fatalf("check table after caller cancellation: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("transaction changes remained after caller cancellation")
	}
}

func TestRawQuerySQLRejectsActiveTransaction(t *testing.T) {
	dbSession := newSQLiteDBSession(t, time.Second)
	txSession, _, _, err := dbSession.BeginTx()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	called := false
	err = txSession.RawQuerySQL(context.Background(), func(context.Context, *sql.DB) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("RawQuerySQL succeeded with an active transaction")
	}
	if called {
		t.Fatal("RawQuerySQL callback ran with an active transaction")
	}

	if _, _, rollbackErr := txSession.Rollback(); rollbackErr != nil {
		t.Fatalf("rollback transaction: %v", rollbackErr)
	}
}

func TestRawQuerySQLWithTxUsesTransaction(t *testing.T) {
	dbSession := newSQLiteDBSession(t, time.Second)
	if _, err := dbSession.database.SQL.Exec(
		"CREATE TABLE raw_query_transaction_test (value TEXT)",
	); err != nil {
		t.Fatalf("create test table: %v", err)
	}

	txSession, _, _, err := dbSession.BeginTx()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	err = txSession.RawQuerySQLWithTx(context.Background(), func(ctx context.Context, executor SQLExecutor) error {
		_, err := executor.ExecContext(
			ctx,
			"INSERT INTO raw_query_transaction_test (value) VALUES (?)",
			"uncommitted",
		)
		return err
	})
	if err != nil {
		t.Fatalf("execute raw SQL in transaction: %v", err)
	}

	if _, _, err := txSession.Rollback(); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}

	var count int
	if err := dbSession.database.SQL.QueryRow(
		"SELECT count(*) FROM raw_query_transaction_test WHERE value = ?",
		"uncommitted",
	).Scan(&count); err != nil {
		t.Fatalf("check rolled-back row: %v", err)
	}
	if count != 0 {
		t.Fatalf("found %d rows after rollback, want 0", count)
	}
}
