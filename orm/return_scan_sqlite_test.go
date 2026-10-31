package orm

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestReturnSQLHelpersScanRowsAndPopulateDestination(t *testing.T) {
	connection, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	defer connection.Close()

	db := &database.Database{SQL: connection}
	config := &database.Config{Driver: database.SQLITE}
	plan := &returnPlan{
		destType: reflect.TypeFor[returnTestRecord](),
		destVal:  reflect.ValueOf(&returnTestRecord{}).Elem(),
		fillIdx:  []int{0, 1},
		queryCols: []string{
			"id", "name",
		},
	}
	query := &ORM{Database: db, DatabaseConfig: config}

	rows, err := query.queryReturnRows(context.Background(),
		"SELECT 7 AS id, 'Ada' AS name",
		nil,
		plan,
		"Create",
	)
	if err != nil {
		t.Fatalf("queryReturnRows() error = %v", err)
	}
	if len(rows) != 1 || rows[0].Interface().(returnTestRecord).Name != "Ada" {
		t.Fatalf("scanned RETURN rows = %#v", rows)
	}

	plan.destVal = reflect.ValueOf(&returnTestRecord{}).Elem()
	if err := query.execReturn(
		context.Background(),
		"SELECT 8 AS id, 'Lin' AS name",
		nil,
		plan,
		[]reflect.Value{reflect.ValueOf(returnTestRecord{ID: 8})},
		columnMetaData{primaryKeyIndex: []int{0}},
	); err != nil {
		t.Fatalf("execReturn() error = %v", err)
	}
	if got := plan.destVal.Interface().(returnTestRecord); got.ID != 8 || got.Name != "Lin" {
		t.Fatalf("RETURN destination = %#v", got)
	}
	if query.LastInsertId != "8" || query.Message != "Data created successfully" || query.HTTPCode != 200 {
		t.Fatalf("execReturn() ORM status = (%q, %q, %d)", query.LastInsertId, query.Message, query.HTTPCode)
	}
}

func TestReturnSQLHelpersPropagateDatabaseAndScanErrors(t *testing.T) {
	query := &ORM{DatabaseConfig: &database.Config{Driver: database.SQLITE}}
	plan := &returnPlan{
		destType:  reflect.TypeFor[returnTestRecord](),
		queryCols: []string{"id", "name"},
	}
	if _, err := query.queryReturnRows(context.Background(), "SELECT 1", nil, plan, "Create"); err == nil {
		t.Fatal("queryReturnRows() suppressed an unavailable database error")
	}
	if _, err := query.queryReturnRowsWithDatabase(
		context.Background(),
		&database.Database{},
		"SELECT 1",
		nil,
		plan,
		"Create",
	); err == nil {
		t.Fatal("queryReturnRowsWithDatabase() suppressed an unavailable database error")
	}

	connection, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	defer connection.Close()
	rows, err := connection.Query("SELECT 'not-an-integer' AS id, 'Ada' AS name")
	if err != nil {
		t.Fatalf("query invalid RETURN row: %v", err)
	}
	defer rows.Close()
	query = &ORM{}
	if _, err := query.scanReturnRows(rows, plan); err == nil {
		t.Fatal("scanReturnRows() accepted an incompatible SQL column type")
	}
}
