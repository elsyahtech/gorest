package orm

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestReturnConfiguresDestinationAndRejectsInvalidArguments(t *testing.T) {
	type record struct {
		ID   int
		Name string
	}

	var dest record
	query := (&ORM{}).Return(&dest, "id, name, id", " ")
	if !query.IsReturn || query.ReturnDest != &dest ||
		!reflect.DeepEqual(query.ReturnCols, []string{"id", "name"}) || query.Error != nil {
		t.Fatalf("Return() configuration = %#v", query)
	}

	for _, invalid := range []any{record{}, 42, (*record)(nil)} {
		query := (&ORM{}).Return(invalid)
		if query.Error == nil || !strings.Contains(query.Message, "destination") {
			t.Errorf("Return(%T) did not reject invalid destination: %#v", invalid, query)
		}
	}

	query = (&ORM{}).Return(&dest, &dest)
	if query.Error == nil || query.Message != "Return() accepts at most one destination" {
		t.Fatalf("Return() accepted multiple destinations: %#v", query)
	}
}

func TestMySQLReturnFinishingUpdatesStateAndDestinations(t *testing.T) {
	type record struct {
		ID   int    `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
	plan := &returnPlan{
		destType:    reflect.TypeFor[record](),
		sameAsInput: true,
		fillIdx:     []int{1},
		queryCols:   []string{"name"},
	}
	meta := columnMetaData{primaryKeyIndex: []int{0}}

	single := record{ID: 8, Name: "old"}
	singleRows := []reflect.Value{reflect.ValueOf(&single).Elem()}
	scanned := []reflect.Value{reflect.ValueOf(record{ID: 8, Name: "new"})}
	query := &ORM{}
	if err := query.finishMySQLSingleReturn(plan, scanned, singleRows, meta); err != nil {
		t.Fatalf("finishMySQLSingleReturn() error = %v", err)
	}
	if single.Name != "new" || query.LastInsertId != "8" ||
		query.Message != "Data updated successfully" || query.HTTPCode != 200 || query.RowsAffected != 1 {
		t.Fatalf("single RETURN result = %#v, ORM state: %#v", single, query)
	}

	bulk := []record{{ID: 1, Name: "old-1"}, {ID: 2, Name: "old-2"}}
	bulkRows := []reflect.Value{reflect.ValueOf(&bulk[0]).Elem(), reflect.ValueOf(&bulk[1]).Elem()}
	scannedRows := []reflect.Value{
		reflect.ValueOf(record{ID: 1, Name: "new-1"}),
		reflect.ValueOf(record{ID: 2, Name: "new-2"}),
	}
	bulkPlan := *plan
	bulkPlan.inputIsSlice = true
	query = &ORM{}
	if err := query.finishMySQLBulkReturn(&bulkPlan, scannedRows, bulkRows, meta); err != nil {
		t.Fatalf("finishMySQLBulkReturn() error = %v", err)
	}
	if !reflect.DeepEqual(bulk, []record{{ID: 1, Name: "new-1"}, {ID: 2, Name: "new-2"}}) ||
		query.RowsAffected != 2 || query.LastInsertId != "1" ||
		query.Message != "Data updated successfully" || query.HTTPCode != 200 {
		t.Fatalf("bulk RETURN result = %#v, ORM state: %#v", bulk, query)
	}
}

func TestMySQLReturnOperationsRejectIncompleteMetadataBeforeDatabaseAccess(t *testing.T) {
	query := &ORM{DatabaseConfig: &database.Config{Driver: database.MYSQL}}
	row := reflect.ValueOf(struct{ ID int }{ID: 1})
	err := query.updateMySQLSingleWithReturn(&structUpdateMySQLSingleWithReturn{
		field:       &resultBuildUpdatePayload{},
		whereClause: &resBuildSingleWhereClause{},
		inputRows:   []reflect.Value{row},
	})
	if err == nil {
		t.Fatal("single MySQL Return accepted incomplete primary-key metadata")
	}

	query = &ORM{}
	err = query.updateMySQLBulkWithReturn(
		context.Background(),
		"users",
		nil,
		columnMetaData{},
		nil,
		&returnPlan{},
	)
	if err == nil {
		t.Fatal("bulk MySQL Return accepted incomplete primary-key metadata")
	}
}

func TestScyllaWriteOrderTerms(t *testing.T) {
	query := &ORM{StringBuilder: &strings.Builder{}}
	query.writeOrderTerms(nil)
	if query.StringBuilder.Len() != 0 {
		t.Fatal("writeOrderTerms() wrote an ORDER BY for no terms")
	}

	query.writeOrderTerms([]orderByTerm{
		{Column: "created_at", Direction: "DESC"},
		{Column: "id"},
	})
	if got, want := query.StringBuilder.String(), " ORDER BY created_at DESC, id"; got != want {
		t.Fatalf("ORDER BY clause = %q, want %q", got, want)
	}
}
