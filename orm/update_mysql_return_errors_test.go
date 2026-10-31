package orm

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestMySQLReturnKeyAndUpdateHelpersPropagateDatabaseErrors(t *testing.T) {
	type record struct {
		ID   int
		Name string
	}
	query := &ORM{
		Database:       &database.Database{},
		DatabaseConfig: &database.Config{Driver: database.MYSQL},
		StringBuilder:  &strings.Builder{},
	}
	meta := columnMetaData{
		columns:           []string{"id", "name"},
		columnIndex:       []int{0, 1},
		primaryKeyColumns: []string{"id"},
		primaryKeyIndex:   []int{0},
	}
	row := reflect.ValueOf(record{ID: 1, Name: "updated"})

	if _, err := query.lockMySQLSingleReturnKeys(
		context.Background(),
		query.Database,
		"users",
		"id = ?",
		[]any{1},
		meta,
		reflect.TypeFor[record](),
	); err == nil {
		t.Fatal("lockMySQLSingleReturnKeys() suppressed a database error")
	}

	if err := query.applyMySQLSingleReturnUpdate(
		context.Background(),
		query.Database,
		"users",
		"name = ?",
		"id = ?",
		[]any{"updated", 1},
	); err == nil {
		t.Fatal("applyMySQLSingleReturnUpdate() suppressed a database error")
	}

	req := &structUpdateMySQLBulkChunk{
		execCtx:       context.Background(),
		execDB:        query.Database,
		tableName:     "users",
		rowsVal:       []reflect.Value{row},
		meta:          meta,
		updatableCols: []updatableColumn{{name: "name", idx: 1}},
		start:         0,
		end:           1,
	}
	if _, err := query.updateMySQLBulkChunk(req); err == nil {
		t.Fatal("updateMySQLBulkChunk() suppressed a key-query database error")
	}

	if _, err := query.queryMySQLBulkReturnKeys(
		context.Background(),
		query.Database,
		"users",
		"id = ?",
		[]any{1},
		meta,
		reflect.TypeFor[record](),
	); err == nil {
		t.Fatal("queryMySQLBulkReturnKeys() suppressed a database error")
	}
}
