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
