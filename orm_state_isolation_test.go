package gorest

import (
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestTableCreatesIsolatedORMQueryState(t *testing.T) {
	session := &DBSession{
		database: &database.Database{},
		config:   &database.Config{Driver: database.POSTGRES},
	}

	first := session.Table("users").Where("status = ?", "active").Select("id")
	second := session.Table("orders")

	if len(second.WhereClauses) != 0 || len(second.WhereArgs) != 0 || len(second.SelectedCols) != 0 {
		t.Fatalf(
			"second query inherited first query state: clauses=%#v args=%#v selected=%#v",
			second.WhereClauses,
			second.WhereArgs,
			second.SelectedCols,
		)
	}
	if first.StringBuilder == second.StringBuilder {
		t.Fatal("separate Table() calls share a SQL builder")
	}
}
