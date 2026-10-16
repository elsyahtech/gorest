//go:build integration

package sqlserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestSQLServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.SQLSERVER)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, message, code, err := app.Database().QuerySQL(ctx, "SELECT @@VERSION")
	if err != nil {
		t.Fatalf("query SQL Server version failed (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("SQL Server version query returned no row: %v", rows.Err())
	}

	var version string
	if err := rows.Scan(&version); err != nil {
		t.Fatalf("scan SQL Server version: %v", err)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate SQL Server version result: %v", err)
	}

	t.Logf("SQL Server version: %s", version)
}

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.SQLSERVER)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.SQLSERVER)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.SQLSERVER)
}

func TestORMCoverage(t *testing.T) {
	t.Run("joins and relation mapping", func(t *testing.T) {
		internaltest.SQLJoinAndPreload(t, database.SQLSERVER)
	})
	t.Run("nullable mapping", func(t *testing.T) {
		internaltest.SQLNullableMapping(t, database.SQLSERVER)
	})
	t.Run("pagination boundaries", func(t *testing.T) {
		internaltest.SQLPaginationBoundaries(t, database.SQLSERVER)
	})
	t.Run("batch failure atomicity", func(t *testing.T) {
		internaltest.SQLBatchFailureAtomicity(t, database.SQLSERVER)
	})
	t.Run("live query cancellation", func(t *testing.T) {
		internaltest.SQLContextCancellation(t, database.SQLSERVER)
	})
	t.Run("upsert behavior", func(t *testing.T) {
		internaltest.SQLUpsertBehavior(t, database.SQLSERVER)
	})
	t.Run("invalid ORM inputs", func(t *testing.T) {
		internaltest.SQLInvalidInputs(t, database.SQLSERVER)
	})
}
