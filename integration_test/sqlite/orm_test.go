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
