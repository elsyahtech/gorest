package orm

import (
	"reflect"
	"testing"
)

func TestSQLPreloadScanMapsEmbeddedRelationValues(t *testing.T) {
	type department struct {
		ID   int    `gorest:"id"`
		Name string `gorest:"name"`
	}
	type user struct {
		ID         int `gorest:"id"`
		Department department
		Team       *department
	}

	var departmentID any = 9
	var departmentName any = "engineering"
	var teamID any = 12
	var teamName any = "operations"
	columns := []string{
		"departments_id", "departments_name",
		"teams_id", "teams_name",
	}
	scanArgs := []any{
		&departmentID, &departmentName,
		&teamID, &teamName,
	}
	value := reflect.ValueOf(&user{}).Elem()
	query := &ORM{Preloads: map[string][]any{
		"Department": nil,
		"Team":       nil,
	}}

	query.preloadScanFindSQL(&structPreloadScanFindSQL{
		hasManyNames: map[string]struct{}{},
		structType:   value.Type(),
		newStructVal: value,
		preloadReferences: map[string]string{
			"department": "id",
			"team":       "id",
		},
		cols:     columns,
		scanArgs: scanArgs,
	})

	got := value.Interface().(user)
	if got.Department != (department{ID: 9, Name: "engineering"}) {
		t.Fatalf("mapped department = %#v", got.Department)
	}
	if got.Team == nil || *got.Team != (department{ID: 12, Name: "operations"}) {
		t.Fatalf("mapped team = %#v", got.Team)
	}
}

func TestSQLPreloadScanClearsRelationsWithoutReferenceValue(t *testing.T) {
	type department struct {
		ID int `gorest:"id"`
	}
	type user struct {
		Department department
	}

	value := reflect.ValueOf(&user{Department: department{ID: 5}}).Elem()
	query := &ORM{Preloads: map[string][]any{"Department": nil}}
	query.preloadScanFindSQL(&structPreloadScanFindSQL{
		hasManyNames:      map[string]struct{}{},
		structType:        value.Type(),
		newStructVal:      value,
		preloadReferences: map[string]string{"department": "id"},
		cols:              []string{"name"},
		scanArgs:          []any{new(any)},
	})
	if got := value.Interface().(user).Department; got != (department{}) {
		t.Fatalf("relation without foreign reference was not cleared: %#v", got)
	}
}
