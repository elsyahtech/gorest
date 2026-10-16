//go:build integration

package mysql_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestMySQLServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.MYSQL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, message, code, err := app.Database().QuerySQL(ctx, "SELECT VERSION(), @@character_set_connection")
	if err != nil {
		t.Fatalf("query MySQL server version failed (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("MySQL server version query returned no row: %v", rows.Err())
	}

	var version, charset string
	if err := rows.Scan(&version, &charset); err != nil {
		t.Fatalf("scan MySQL server version and charset: %v", err)
	}
	if !strings.EqualFold(charset, database.ConfigDefault.Charset) {
		t.Errorf("connection charset = %q, want %q", charset, database.ConfigDefault.Charset)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate MySQL server version result: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close MySQL server version rows: %v", err)
	}

	t.Logf("MySQL server version: %s, connection charset: %s", version, charset)
}

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.MYSQL)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.MYSQL)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.MYSQL)
}

func TestORMCoverage(t *testing.T) {
	t.Run("joins and relation mapping", func(t *testing.T) {
		internaltest.SQLJoinAndPreload(t, database.MYSQL)
	})
	t.Run("nullable mapping", func(t *testing.T) {
		internaltest.SQLNullableMapping(t, database.MYSQL)
	})
	t.Run("pagination boundaries", func(t *testing.T) {
		internaltest.SQLPaginationBoundaries(t, database.MYSQL)
	})
	t.Run("batch failure atomicity", func(t *testing.T) {
		internaltest.SQLBatchFailureAtomicity(t, database.MYSQL)
	})
	t.Run("live query cancellation", func(t *testing.T) {
		internaltest.SQLContextCancellation(t, database.MYSQL)
	})
	t.Run("upsert behavior", func(t *testing.T) {
		internaltest.SQLUpsertBehavior(t, database.MYSQL)
	})
	t.Run("invalid ORM inputs", func(t *testing.T) {
		internaltest.SQLInvalidInputs(t, database.MYSQL)
	})
}
