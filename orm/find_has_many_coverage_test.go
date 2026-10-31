package orm

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestLoadHasManyValidatesKeySelectionAndHandlesEmptyParents(t *testing.T) {
	type child struct {
		ID     int `gorest:"id"`
		UserID int `gorest:"user_id"`
	}
	type parent struct {
		ID       int `gorest:"id, primary_key"`
		Children []child
	}
	rel := &hasManyRelation{
		fieldIndex:   1,
		sliceType:    reflect.TypeFor[[]child](),
		elemType:     reflect.TypeFor[child](),
		parentKeyCol: "id",
		parentKeyIdx: 0,
	}
	query := &ORM{}
	if err := query.loadHasMany(context.Background(), database.SQLITE, nil, []string{"name"}, rel); err == nil {
		t.Fatal("loadHasMany() accepted selected columns without the parent key")
	}

	query = &ORM{}
	if err := query.loadHasMany(context.Background(), database.SQLITE, nil, []string{"id"}, rel); err != nil {
		t.Fatalf("loadHasMany() with no parent keys = %v", err)
	}

	query = &ORM{DatabaseConfig: &database.Config{Driver: database.SQLITE}}
	parentValue := reflect.ValueOf(parent{ID: 5})
	if err := query.loadHasMany(context.Background(), database.SQLITE, []reflect.Value{parentValue}, []string{"id"}, rel); err == nil {
		t.Fatal("loadHasMany() did not report a missing database service")
	}
}

func TestRowsNextInExecLoadHasManyScansMappedAndNullRows(t *testing.T) {
	connection, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	defer connection.Close()

	rows, err := connection.Query(
		"SELECT 4 AS user_id, 'Ada' AS name UNION ALL SELECT NULL AS user_id, NULL AS name",
	)
	if err != nil {
		t.Fatalf("query child test rows: %v", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatalf("read child columns: %v", err)
	}

	type child struct {
		UserID *int    `gorest:"user_id"`
		Name   *string `gorest:"name"`
	}
	rel := &hasManyRelation{
		elemType:   reflect.TypeFor[child](),
		childFKIdx: 0,
		elemIsPtr:  true,
	}
	grouped := map[string][]reflect.Value{}
	query := &ORM{}
	err = query.rowsNextInExecLoadHasMany(
		rows,
		rel,
		columns,
		buildStructFieldMapByTag(reflect.TypeFor[child]()),
		grouped,
	)
	if err != nil {
		t.Fatalf("rowsNextInExecLoadHasMany() error = %v", err)
	}
	if len(grouped) != 1 || len(grouped["4"]) != 1 {
		t.Fatalf("grouped child rows = %#v, want only the row with non-null foreign key", grouped)
	}
	got := grouped["4"][0].Elem().Interface().(child)
	if got.UserID == nil || *got.UserID != 4 || got.Name == nil || *got.Name != "Ada" {
		t.Fatalf("scanned child = %#v", got)
	}
}
