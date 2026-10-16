//go:build integration

package mongo_test

import (
	"testing"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestORMCRUDWithBoundValues(t *testing.T) {
	internaltest.NoSQLCRUD(t, database.MONGO)
}
