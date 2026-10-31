package orm

import (
	"reflect"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

type returnTestRecord struct {
	ID      int    `gorest:"id,primary_key"`
	Name    string `gorest:"name"`
	Email   string `gorest:"email"`
	Ignored string `gorest:"-"`
}

func returnTestMeta() columnMetaData {
	return columnMetaData{
		columns:                []string{"name", "email"},
		primaryKeyColumns:      []string{"id"},
		columnIndex:            []int{1, 2},
		primaryKeyIndex:        []int{0},
		allColumnPrimaryKeyIdx: map[string]int{"id": 0, "name": 1, "email": 2},
	}
}

func TestPlanReturnBuildsProjectionAndDestinationMetadata(t *testing.T) {
	input := returnTestRecord{ID: 8, Name: "old", Email: "old@example.test"}
	rows := []reflect.Value{reflect.ValueOf(&input).Elem()}
	query := &ORM{ReturnCols: []string{"users.name"}}
	plan, err := query.planReturn(&input, rows, returnTestMeta(), database.POSTGRES)
	if err != nil {
		t.Fatalf("planReturn() error = %v", err)
	}
	if !reflect.DeepEqual(plan.queryCols, []string{"name", "id"}) ||
		!reflect.DeepEqual(plan.fillIdx, []int{1}) ||
		!reflect.DeepEqual(plan.pkIdx, []int{0}) ||
		!plan.sameAsInput || !plan.projection {
		t.Fatalf("return plan = %#v", plan)
	}

	query = &ORM{}
	plan, err = query.planReturn(&input, rows, returnTestMeta(), database.SQLITE)
	if err != nil {
		t.Fatalf("planReturn(all tagged fields) error = %v", err)
	}
	if !reflect.DeepEqual(plan.queryCols, []string{"id", "name", "email"}) ||
		!reflect.DeepEqual(plan.fillIdx, []int{0, 1, 2}) {
		t.Fatalf("default return columns = %#v, fill indexes %#v", plan.queryCols, plan.fillIdx)
	}

	var destination []*returnTestRecord
	query = &ORM{ReturnDest: &destination}
	plan, err = query.planReturn(&input, rows, returnTestMeta(), database.SQLSERVER)
	if err != nil {
		t.Fatalf("planReturn(pointer slice destination) error = %v", err)
	}
	if !plan.destIsSlice || !plan.elemIsPtr || plan.sameAsInput || plan.destType != reflect.TypeFor[returnTestRecord]() {
		t.Fatalf("pointer slice destination plan = %#v", plan)
	}
}

func TestPlanReturnRejectsUnsupportedDriversAndInvalidDestinations(t *testing.T) {
	input := returnTestRecord{ID: 1}
	rows := []reflect.Value{reflect.ValueOf(&input).Elem()}

	query := &ORM{}
	if _, err := query.planReturn(&input, rows, returnTestMeta(), database.MONGO); err == nil || query.Error == nil {
		t.Fatal("planReturn() accepted an unsupported driver")
	}

	query = &ORM{ReturnCols: []string{"missing"}}
	if _, err := query.planReturn(&input, rows, returnTestMeta(), database.SQLITE); err == nil || query.Error == nil {
		t.Fatal("planReturn() accepted a return column without a matching gorest tag")
	}

	query = &ORM{ReturnDest: new(struct{ Value string })}
	if _, err := query.planReturn(&input, rows, returnTestMeta(), database.SQLITE); err == nil || query.Error == nil {
		t.Fatal("planReturn() accepted a destination with no tagged columns")
	}

	query = &ORM{}
	if _, err := query.planReturn(input, []reflect.Value{reflect.ValueOf(input)}, returnTestMeta(), database.SQLITE); err == nil || query.Error == nil {
		t.Fatal("planReturn() accepted a non-addressable input destination")
	}
}

func TestTaggedReturnColumnsAndPointerDereferencing(t *testing.T) {
	type tagged struct {
		ID        int    `gorest:"ID,primary_key"`
		Duplicate string `gorest:"id"`
		Name      string `gorest:" name , immutable"`
		Empty     string `gorest:" ,option"`
		Skipped   string `gorest:"-"`
		Untagged  string
	}
	columns, indexes := taggedColumns(reflect.TypeFor[tagged]())
	if !reflect.DeepEqual(columns, []string{"ID", "name"}) ||
		!reflect.DeepEqual(indexes, map[string]int{"id": 0, "name": 2}) {
		t.Fatalf("taggedColumns() = (%#v, %#v)", columns, indexes)
	}

	value := 3
	if got := derefValue(reflect.ValueOf(&value)); got.Kind() != reflect.Int || got.Int() != 3 {
		t.Fatalf("derefValue(pointer) = %#v", got)
	}
	var missing *int
	if got := derefValue(reflect.ValueOf(missing)); got.Kind() != reflect.Pointer || !got.IsNil() {
		t.Fatalf("derefValue(nil pointer) = %#v", got)
	}
}

func TestReturnColumnsAllowAllAndDeduplicateCaseInsensitively(t *testing.T) {
	query := &ORM{ReturnCols: []string{"name", "users.NAME", "*"}}
	plan := &returnPlan{destType: reflect.TypeFor[returnTestRecord]()}
	var columns []string
	err := query.buildColInPlanReturn(
		func(column string) { columns = append(columns, column) },
		[]string{"id", "name", "email"},
		map[string]int{"id": 0, "name": 1, "email": 2},
		plan,
	)
	if err != nil {
		t.Fatalf("buildColInPlanReturn() error = %v", err)
	}
	if !reflect.DeepEqual(columns, []string{"id", "name", "email"}) ||
		!reflect.DeepEqual(plan.fillIdx, []int{0, 1, 2}) {
		t.Fatalf("selected columns = %#v, fill indexes %#v", columns, plan.fillIdx)
	}

	invalid := &ORM{ReturnCols: []string{"unknown"}}
	if err := invalid.buildColInPlanReturn(
		func(string) {},
		[]string{"id", "name"},
		map[string]int{"id": 0, "name": 1},
		&returnPlan{destType: reflect.TypeFor[returnTestRecord]()},
	); err == nil || invalid.Error == nil {
		t.Fatal("buildColInPlanReturn() accepted an unknown output field")
	}
}

func TestReturnMatchingUsesPrimaryKeysOrUpsertConflictColumns(t *testing.T) {
	input := []returnTestRecord{{ID: 1}, {ID: 2}}
	rows := []reflect.Value{reflect.ValueOf(&input[0]).Elem(), reflect.ValueOf(&input[1]).Elem()}
	meta := returnTestMeta()
	query := &ORM{}
	plan := &returnPlan{inputIsSlice: true}

	query.compareColToPKInPlanReturn(func(string) {}, meta, plan, rows, map[string]int{"id": 0})
	if !reflect.DeepEqual(plan.matchIdx, []int{0}) ||
		!reflect.DeepEqual(query.matchColumnsForReturn(rows, meta), []string{"id"}) {
		t.Fatalf("primary key return matching = %#v", plan.matchIdx)
	}

	query = &ORM{IsUpsert: true, UpsertConflictCols: []string{"email"}}
	if got := query.matchColumnsForReturn(rows, meta); !reflect.DeepEqual(got, []string{"email"}) {
		t.Fatalf("upsert match columns = %#v", got)
	}

	input[1].ID = 0
	if got := (&ORM{}).matchColumnsForReturn(rows, meta); got != nil {
		t.Fatalf("incomplete primary key match columns = %#v, want nil", got)
	}
}

func TestApplyReturnRowsMatchesInputSliceAndBuildsPointerDestinations(t *testing.T) {
	input := []returnTestRecord{
		{ID: 1, Name: "first"},
		{ID: 2, Name: "second"},
	}
	rows := []reflect.Value{reflect.ValueOf(&input[0]).Elem(), reflect.ValueOf(&input[1]).Elem()}
	scanned := []reflect.Value{
		reflect.ValueOf(returnTestRecord{ID: 2, Name: "updated second"}),
		reflect.ValueOf(returnTestRecord{ID: 1, Name: "updated first"}),
	}
	query := &ORM{}
	plan := &returnPlan{sameAsInput: true, inputIsSlice: true, matchIdx: []int{0}, fillIdx: []int{1}}
	if err := query.applyReturnRows(plan, scanned, rows); err != nil {
		t.Fatalf("applyReturnRows(input match) error = %v", err)
	}
	if input[0].Name != "updated first" || input[1].Name != "updated second" {
		t.Fatalf("input rows were not matched by key: %#v", input)
	}

	var pointers []*returnTestRecord
	plan = &returnPlan{
		destVal:     reflect.ValueOf(&pointers).Elem(),
		destType:    reflect.TypeFor[returnTestRecord](),
		destIsSlice: true,
		elemIsPtr:   true,
		fillIdx:     []int{0, 1, 2},
	}
	if err := query.applyReturnRows(plan, scanned, nil); err != nil {
		t.Fatalf("applyReturnRows(pointer slice) error = %v", err)
	}
	if len(pointers) != 2 || pointers[0] == nil || pointers[0].Name != "updated second" {
		t.Fatalf("pointer slice return destination = %#v", pointers)
	}

	single := returnTestRecord{ID: 7, Name: "original"}
	plan = &returnPlan{
		destVal:    reflect.ValueOf(&single).Elem(),
		destType:   reflect.TypeFor[returnTestRecord](),
		fillIdx:    []int{1},
		projection: true,
	}
	if err := query.applyReturnRows(plan, scanned[:1], nil); err != nil {
		t.Fatalf("applyReturnRows(single destination) error = %v", err)
	}
	if single.Name != "updated second" || single.ID != 0 || single.Email != "" {
		t.Fatalf("projected single destination = %#v", single)
	}

	if err := query.applyReturnRows(plan, nil, nil); err != nil {
		t.Fatalf("empty return rows should leave the destination unchanged: %v", err)
	}

	plan = &returnPlan{inputIsSlice: true}
	if err := query.applyReturnToInput(plan, scanned[:1], rows); err == nil || query.Error == nil {
		t.Fatal("applyReturnToInput() accepted ambiguous row counts without a matching key")
	}
}

func TestReturnKeyAndUpdateStatementBuilders(t *testing.T) {
	type keyRow struct {
		ID    *int
		Shard string
	}
	id := 12
	row := reflect.ValueOf(keyRow{ID: &id, Shard: "a"})
	if got, want := returnKey(row, []int{0, 1}), "12\x00a"; got != want {
		t.Fatalf("returnKey() = %q, want %q", got, want)
	}
	if got, want := returnKey(reflect.ValueOf(keyRow{}), []int{0}), "<nil>"; got != want {
		t.Fatalf("returnKey(nil) = %q, want %q", got, want)
	}

	plan := &returnPlan{queryCols: []string{"id"}}
	if got, want := buildUpdateStatement("users", "name = ?", "id = ?", nil, database.MYSQL),
		"UPDATE users SET name = ? WHERE id = ?"; got != want {
		t.Fatalf("plain update = %q, want %q", got, want)
	}
	if got, want := buildUpdateStatement("users", "name = ?", "id = ?", plan, database.SQLSERVER),
		"UPDATE users SET name = ? OUTPUT INSERTED.id WHERE id = ?"; got != want {
		t.Fatalf("SQL Server update = %q, want %q", got, want)
	}
	if got, want := buildUpdateStatement("users", "name = ?", "id = ?", plan, database.POSTGRES),
		"UPDATE users SET name = ? WHERE id = ? RETURNING id"; got != want {
		t.Fatalf("RETURNING update = %q, want %q", got, want)
	}

	plan.sameAsInput = true
	if got := lastInsertIDFromReturn(plan, []reflect.Value{reflect.ValueOf(returnTestRecord{ID: 9})},
		[]reflect.Value{reflect.ValueOf(returnTestRecord{ID: 8})}, returnTestMeta()); got != "9" {
		t.Fatalf("lastInsertIDFromReturn() = %q, want returned primary key", got)
	}
	if got := lastInsertIDFromReturn(plan, nil, nil, columnMetaData{}); got != "" {
		t.Fatalf("lastInsertIDFromReturn(without PK) = %q, want empty", got)
	}

	plan.sameAsInput = false
	if got := lastInsertIDFromReturn(plan, nil, []reflect.Value{reflect.ValueOf(returnTestRecord{ID: 8})}, returnTestMeta()); got != "8" {
		t.Fatalf("lastInsertIDFromReturn(fallback) = %q, want input primary key", got)
	}
	if err := (&ORM{}).finishReturn(plan, nil, nil); err != nil {
		t.Fatalf("finishReturn(empty) error = %v", err)
	}
}
