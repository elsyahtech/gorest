//go:build integration

package postgres_test

import (
	"testing"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.POSTGRES)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.POSTGRES)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.POSTGRES)
}
