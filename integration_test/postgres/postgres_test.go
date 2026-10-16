//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestPostgresServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.POSTGRES)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, message, code, err := app.Database().QuerySQL(ctx, "SHOW server_version")
	if err != nil {
		t.Fatalf("query PostgreSQL server version failed (HTTP %d, %s): %v", code, message, err)
	}
	if !rows.Next() {
		rows.Close()
		t.Fatalf("PostgreSQL server version query returned no row: %v", rows.Err())
	}

	var version string
	if err := rows.Scan(&version); err != nil {
		rows.Close()
		t.Fatalf("scan PostgreSQL server version: %v", err)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("iterate PostgreSQL server version result: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close PostgreSQL server version rows: %v", err)
	}

	t.Logf("PostgreSQL server version: %s", version)
}

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.POSTGRES)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.POSTGRES)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.POSTGRES)
}
