//go:build integration

package sqlserver_test

import (
	"testing"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.SQLSERVER)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.SQLSERVER)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.SQLSERVER)
}
