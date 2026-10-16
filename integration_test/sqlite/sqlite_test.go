//go:build integration

package sqlite_test

import (
	"testing"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.SQLITE)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.SQLITE)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.SQLITE)
}

func TestORMCoverage(t *testing.T) {
	t.Run("joins and relation mapping", func(t *testing.T) {
		internaltest.SQLJoinAndPreload(t, database.SQLITE)
	})
	t.Run("nullable mapping", func(t *testing.T) {
		internaltest.SQLNullableMapping(t, database.SQLITE)
	})
	t.Run("pagination boundaries", func(t *testing.T) {
		internaltest.SQLPaginationBoundaries(t, database.SQLITE)
	})
	t.Run("batch failure atomicity", func(t *testing.T) {
		internaltest.SQLBatchFailureAtomicity(t, database.SQLITE)
	})
	t.Run("live query cancellation", func(t *testing.T) {
		internaltest.SQLContextCancellation(t, database.SQLITE)
	})
	t.Run("upsert behavior", func(t *testing.T) {
		internaltest.SQLUpsertBehavior(t, database.SQLITE)
	})
	t.Run("invalid ORM inputs", func(t *testing.T) {
		internaltest.SQLInvalidInputs(t, database.SQLITE)
	})
}
