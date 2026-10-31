package orm

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestUpdateBulkMongoRejectsDuplicateUniqueValuesBeforeDatabaseAccess(t *testing.T) {
	type record struct {
		ID    int    `gorest:"id,primary_key"`
		Email string `gorest:"email,unique"`
	}

	query := &ORM{DatabaseConfig: &database.Config{Driver: database.MONGO}}
	meta := columnMetaData{
		columns:           []string{"id", "email"},
		columnIndex:       []int{0, 1},
		primaryKeyColumns: []string{"id"},
		primaryKeyIndex:   []int{0},
	}
	rows := []reflect.Value{
		reflect.ValueOf(record{ID: 1, Email: "duplicate@example.test"}),
		reflect.ValueOf(record{ID: 2, Email: "duplicate@example.test"}),
	}

	err := query.updateBulkMongo(context.Background(), nil, rows, meta)
	if err == nil || !errors.Is(err, query.Error) || query.HTTPCode != 409 {
		t.Fatalf("updateBulkMongo() error = %v, ORM.Error = %v, HTTPCode = %d; want unique-value conflict", err, query.Error, query.HTTPCode)
	}
	if !strings.Contains(query.Message, "email=duplicate@example.test") {
		t.Fatalf("updateBulkMongo() message = %q, want conflicting unique field", query.Message)
	}
}

func TestOracleUpdateReturnRejectsInvalidUpdateAndOutputTypes(t *testing.T) {
	type inputRecord struct {
		ID   int    `gorest:"id,primary_key"`
		Name string `gorest:"name"`
	}
	meta := columnMetaData{
		columns:                []string{"id", "name"},
		columnIndex:            []int{0, 1},
		primaryKeyColumns:      []string{"id"},
		primaryKeyIndex:        []int{0},
		allColumnPrimaryKeyIdx: map[string]int{"id": 0, "name": 1},
	}
	row := reflect.ValueOf(inputRecord{ID: 1, Name: "updated"})

	t.Run("no update fields", func(t *testing.T) {
		query := &ORM{}
		err := query.updateOracleWithReturn(
			context.Background(),
			"records",
			[]reflect.Value{reflect.ValueOf(struct {
				ID int `gorest:"id,primary_key"`
			}{ID: 1})},
			columnMetaData{columns: []string{"id"}, columnIndex: []int{0}, primaryKeyColumns: []string{"id"}, primaryKeyIndex: []int{0}},
			&returnPlan{destType: reflect.TypeOf(struct{}{}), queryCols: []string{"id"}},
		)
		if err == nil || query.HTTPCode != 500 {
			t.Fatalf("updateOracleWithReturn(no update fields) error = %v, HTTPCode = %d", err, query.HTTPCode)
		}
	})

	t.Run("unsupported RETURN output", func(t *testing.T) {
		query := &ORM{StringBuilder: &strings.Builder{}}
		err := query.updateOracleWithReturn(
			context.Background(),
			"records",
			[]reflect.Value{row},
			meta,
			&returnPlan{
				destType: reflect.TypeOf(struct {
					Values map[string]string `gorest:"values"`
				}{}),
				queryCols: []string{"values"},
			},
		)
		if err == nil || query.HTTPCode != 400 || !strings.Contains(err.Error(), "unsupported output type") {
			t.Fatalf("updateOracleWithReturn(unsupported output) error = %v, HTTPCode = %d", err, query.HTTPCode)
		}
	})
}

func TestExecOracleUpdateReturningRejectsInvalidBindPlanBeforeDatabaseAccess(t *testing.T) {
	query := &ORM{Database: &database.Database{}, DatabaseConfig: &database.Config{Driver: database.ORACLE}}
	_, _, err := query.execOracleUpdateReturning(&structExecOracleUpdateReturning{
		plan: &returnPlan{
			destType: reflect.TypeOf(struct {
				Values map[string]string `gorest:"values"`
			}{}),
			queryCols: []string{"values"},
		},
		inputRow: reflect.ValueOf(struct{ ID int }{ID: 1}),
		meta:     columnMetaData{allColumnPrimaryKeyIdx: map[string]int{"id": 0}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported output type") {
		t.Fatalf("execOracleUpdateReturning() error = %v, want invalid Oracle output bind error", err)
	}
}
