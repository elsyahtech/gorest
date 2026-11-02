//go:build integration

package oracle_test

import (
	"context"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestOracleServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.ORACLE)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, message, code, err := app.Database().QuerySQL(
		ctx,
		"SELECT banner FROM v$version WHERE ROWNUM = 1",
	)
	if err != nil {
		t.Fatalf("query Oracle server version failed (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("Oracle server version query returned no row: %v", rows.Err())
	}

	var version string
	if err := rows.Scan(&version); err != nil {
		t.Fatalf("scan Oracle server version: %v", err)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate Oracle server version result: %v", err)
	}

	t.Logf("Oracle server version: %s", version)
}

func TestOracleStorageUsageEndpoint(t *testing.T) {
	internaltest.StorageUsage(t, database.ORACLE)
}

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.ORACLE)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.ORACLE)
}

func TestORMParallelTransactions(t *testing.T) {
	internaltest.SQLParallelTransactions(t, database.ORACLE)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.ORACLE)
}

func TestORMCoverage(t *testing.T) {
	t.Run("joins and relation mapping", func(t *testing.T) {
		internaltest.SQLJoinAndPreload(t, database.ORACLE)
	})
	t.Run("nullable mapping", func(t *testing.T) {
		internaltest.SQLNullableMapping(t, database.ORACLE)
	})
	t.Run("pagination boundaries", func(t *testing.T) {
		internaltest.SQLPaginationBoundaries(t, database.ORACLE)
	})
	t.Run("batch failure atomicity", func(t *testing.T) {
		internaltest.SQLBatchFailureAtomicity(t, database.ORACLE)
	})
	t.Run("live query cancellation", func(t *testing.T) {
		internaltest.SQLContextCancellation(t, database.ORACLE)
	})
	t.Run("upsert behavior", func(t *testing.T) {
		internaltest.SQLUpsertBehavior(t, database.ORACLE)
	})
	t.Run("invalid ORM inputs", func(t *testing.T) {
		internaltest.SQLInvalidInputs(t, database.ORACLE)
	})
}
