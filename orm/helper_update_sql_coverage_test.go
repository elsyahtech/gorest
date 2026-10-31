package orm

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestExtractUpdatableColumnsRejectsPrimaryKeyOnlyMetadata(t *testing.T) {
	query := &ORM{}
	meta := columnMetaData{
		columns:           []string{"id", "name"},
		columnIndex:       []int{0, 1},
		primaryKeyIndex:   []int{0},
		primaryKeyColumns: []string{"id"},
	}
	got, err := query.extractUpdatableColumns(meta)
	if err != nil {
		t.Fatalf("extractUpdatableColumns() error = %v", err)
	}
	if want := []updatableColumn{{name: "name", idx: 1}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("updatable columns = %#v, want %#v", got, want)
	}

	if _, err := query.extractUpdatableColumns(columnMetaData{
		columns: []string{"id"}, columnIndex: []int{0}, primaryKeyIndex: []int{0},
	}); err == nil {
		t.Fatal("extractUpdatableColumns() accepted metadata with no mutable columns")
	}
}

func TestExecBulkUpdateSQLHandlesEmptyInputAndDatabaseErrors(t *testing.T) {
	query := &ORM{
		Database:       &database.Database{},
		DatabaseConfig: &database.Config{Driver: database.SQLITE},
		StringBuilder:  &strings.Builder{},
	}
	emptyReq := &structExecBulkUpdateSQL{chunkSize: 1}
	affected, scanned, err := query.execBulkUpdateSQL(emptyReq)
	if err != nil || affected != 0 || len(scanned) != 0 {
		t.Fatalf("execBulkUpdateSQL(empty) = (%d, %#v, %v)", affected, scanned, err)
	}

	type record struct {
		ID   int
		Name string
	}
	rows := []reflect.Value{reflect.ValueOf(record{ID: 1, Name: "Ada"})}
	meta := columnMetaData{
		columns:           []string{"id", "name"},
		columnIndex:       []int{0, 1},
		primaryKeyIndex:   []int{0},
		primaryKeyColumns: []string{"id"},
	}
	_, _, err = query.execBulkUpdateSQL(&structExecBulkUpdateSQL{
		execCtx:       context.Background(),
		meta:          &meta,
		tableName:     "users",
		activeDriver:  database.SQLITE,
		rowsVal:       rows,
		updatableCols: []updatableColumn{{name: "name", idx: 1}},
		chunkSize:     1,
	})
	if err == nil {
		t.Fatal("execBulkUpdateSQL() did not report an unavailable SQL service")
	}

	query = &ORM{DatabaseConfig: &database.Config{Driver: database.SQLITE}}
	if err := query.updateBulkSQL(context.Background(), "users", rows, meta, database.SQLITE, nil); err == nil {
		t.Fatal("updateBulkSQL() did not report a missing SQL service")
	}
}
