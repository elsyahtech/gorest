//go:build integration

package oracle_test

import (
	"testing"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.ORACLE)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.ORACLE)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.ORACLE)
}
