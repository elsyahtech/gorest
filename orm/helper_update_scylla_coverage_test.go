package orm

import (
	"context"
	"reflect"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestScyllaUpdateHelpersValidateDataBeforeDatabaseOperations(t *testing.T) {
	type record struct {
		ID   int
		Name string
	}
	row := reflect.ValueOf(record{ID: 1, Name: "updated"})
	meta := columnMetaData{
		columns:           []string{"id", "name"},
		columnIndex:       []int{0, 1},
		primaryKeyColumns: []string{"id"},
		primaryKeyIndex:   []int{0},
	}
	ctx := context.Background()

	query := &ORM{}
	err := query.updateSingleScylla(ctx, "users", []reflect.Value{row}, columnMetaData{
		columns: []string{"id"}, columnIndex: []int{0}, primaryKeyColumns: []string{"id"}, primaryKeyIndex: []int{0},
	}, database.SCYLLA)
	if err == nil {
		t.Fatal("updateSingleScylla() accepted a primary-key-only update")
	}

	query = &ORM{
		DatabaseConfig: &database.Config{Driver: database.SCYLLA},
	}
	if err := query.updateBulkScylla(ctx, "users", []reflect.Value{row}, meta, database.SCYLLA); err == nil {
		t.Fatal("updateBulkScylla() did not report an unavailable Scylla service")
	}

	query = &ORM{DatabaseConfig: &database.Config{Driver: database.SCYLLA}}
	if err := query.probeScyllaRecordsExist(ctx, []reflect.Value{row}, meta, "users"); err == nil {
		t.Fatal("probeScyllaRecordsExist() did not propagate a database error")
	}
}
