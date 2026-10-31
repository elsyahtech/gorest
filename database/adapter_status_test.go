package database

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
)

func TestNativeAdaptersReportUnavailableServicesAs503(t *testing.T) {
	db := &Database{}
	config := &Config{Driver: SQLITE}
	cases := []struct {
		call func() (int, error)
		name string
	}{
		{name: "ExecSQL", call: func() (int, error) {
			_, _, code, err := db.ExecSQL(context.Background(), config, "SELECT 1")
			return code, err
		}},
		{name: "QuerySQL", call: func() (int, error) {
			_, _, code, err := db.QuerySQL(context.Background(), config, "SELECT 1")
			return code, err
		}},
		{name: "BeginTxContext", call: func() (int, error) {
			_, _, code, err := db.BeginTxContext(context.Background(), config)
			return code, err
		}},
		{name: "MongoCollection", call: func() (int, error) { _, _, code, err := db.Collection(config, "users"); return code, err }},
		{name: "CreateCollection", call: func() (int, error) {
			_, code, err := db.CreateCollection(context.Background(), config, "users")
			return code, err
		}},
		{name: "ExecCQL", call: func() (int, error) {
			_, code, err := db.ExecCQL(context.Background(), config, "SELECT * FROM users")
			return code, err
		}},
		{name: "QueryCQL", call: func() (int, error) {
			_, _, code, err := db.QueryCQL(context.Background(), config, "SELECT * FROM users")
			return code, err
		}},
		{name: "ExecBatchCQL", call: func() (int, error) {
			_, code, err := db.ExecBatchCQL(context.Background(), config, nil, nil)
			return code, err
		}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			code, err := test.call()
			if err == nil || code != http.StatusServiceUnavailable {
				t.Fatalf("got HTTP %d, error %v; want HTTP 503 and an error", code, err)
			}
		})
	}
}

func TestScyllaAdaptersRejectMissingConfigWithoutPanic(t *testing.T) {
	db := &Database{}
	if _, _, _, err := db.QueryCQL(context.Background(), nil, "SELECT * FROM users"); err == nil {
		t.Fatal("QueryCQL with nil config succeeded")
	}
	if _, _, err := db.ExecBatchCQL(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("ExecBatchCQL with nil config succeeded")
	}
}

func TestBeginTxStartsTransaction(t *testing.T) {
	sqlDB, err := sql.Open(SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	db := &Database{SQL: sqlDB}
	txDB, message, status, err := db.BeginTx(&Config{Driver: SQLITE})
	if err != nil {
		t.Fatalf("BeginTx() failed (HTTP %d, %s): %v", status, message, err)
	}
	if txDB == nil || txDB.Tx == nil || status != http.StatusOK {
		t.Fatalf("BeginTx() = (%#v, %q, %d), want an active transaction and HTTP 200", txDB, message, status)
	}
	if err := txDB.Tx.Rollback(); err != nil {
		t.Fatalf("rollback test transaction: %v", err)
	}
}

func TestQuerySQLReturnsNoRowsWithoutErrorWhenIDIsMissing(t *testing.T) {
	sqlDB, err := sql.Open(SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	if _, err := sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create users table: %v", err)
	}
	if _, err := sqlDB.Exec("INSERT INTO users (id) VALUES (?)", 1); err != nil {
		t.Fatalf("insert existing user: %v", err)
	}

	rows, message, status, err := (&Database{SQL: sqlDB}).QuerySQL(
		context.Background(),
		&Config{Driver: SQLITE},
		"SELECT id FROM users WHERE id = ?",
		404,
	)
	if err != nil {
		t.Fatalf("QuerySQL() failed (HTTP %d, %s): %v", status, message, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close query rows: %v", err)
		}
	}()

	if status != http.StatusOK {
		t.Fatalf("QuerySQL() status = %d, want %d", status, http.StatusOK)
	}
	if rows.Next() {
		t.Fatal("QuerySQL() returned a row for an ID that does not exist")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate query rows: %v", err)
	}
}

func TestQuerySQLMapsDuplicateKeyFromReturningQueryToConflict(t *testing.T) {
	sqlDB, err := sql.Open(SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	if _, err := sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create users table: %v", err)
	}
	if _, err := sqlDB.Exec("INSERT INTO users (id) VALUES (?)", 1); err != nil {
		t.Fatalf("insert existing user: %v", err)
	}

	rows, message, status, err := (&Database{SQL: sqlDB}).QuerySQL(
		context.Background(),
		&Config{Driver: SQLITE},
		"INSERT INTO users (id) VALUES (?) RETURNING id",
		1,
	)
	if err == nil {
		if rows != nil {
			if closeErr := rows.Close(); closeErr != nil {
				t.Errorf("close returned rows: %v", closeErr)
			}
		}
		t.Fatal("QuerySQL() with a duplicate key unexpectedly succeeded")
	}
	if status != http.StatusConflict {
		t.Fatalf("QuerySQL() status = %d (%s), want %d; error: %v", status, message, http.StatusConflict, err)
	}
	if !IsDuplicateKeyError(err) {
		t.Fatalf("QuerySQL() error %v is not recognized as a duplicate-key error", err)
	}
}
