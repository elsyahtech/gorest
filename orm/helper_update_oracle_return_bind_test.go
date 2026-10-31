package orm

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildOracleReturnBindsMapsSupportedDestinationTypes(t *testing.T) {
	type oracleRecord struct {
		ID        int       `gorest:"id,primary_key"`
		Name      *string   `gorest:"name"`
		CreatedAt time.Time `gorest:"created_at"`
		Payload   []byte    `gorest:"payload"`
		Enabled   bool      `gorest:"enabled"`
	}
	plan := &returnPlan{
		destType:  reflect.TypeFor[oracleRecord](),
		queryCols: []string{"id", "name", "created_at", "payload", "enabled"},
	}
	meta := columnMetaData{allColumnPrimaryKeyIdx: map[string]int{"id": 0}}
	binds, err := buildOracleReturnBinds(plan, reflect.ValueOf(oracleRecord{}), meta)
	if err != nil {
		t.Fatalf("buildOracleReturnBinds() error = %v", err)
	}
	if len(binds) != len(plan.queryCols) {
		t.Fatalf("bind count = %d, want %d", len(binds), len(plan.queryCols))
	}
	wantTypes := []reflect.Type{
		reflect.TypeFor[int](),
		reflect.TypeFor[string](),
		reflect.TypeFor[time.Time](),
		reflect.TypeFor[[]byte](),
		reflect.TypeFor[bool](),
	}
	for idx, bind := range binds {
		if bind.column != plan.queryCols[idx] || bind.destination != idx || bind.holder.Type() != wantTypes[idx] {
			t.Errorf("bind[%d] = (%q, destination %d, holder %s), want (%q, %d, %s)",
				idx, bind.column, bind.destination, bind.holder.Type(), plan.queryCols[idx], idx, wantTypes[idx])
		}
	}
}

func TestOracleReturnBindHelpersResolveInputAndRejectUnsupportedTypes(t *testing.T) {
	type inputRecord struct {
		ID     int
		Values map[string]string
	}
	plan := &returnPlan{
		destType:  reflect.TypeOf(struct{}{}),
		queryCols: []string{"ID"},
	}
	meta := columnMetaData{allColumnPrimaryKeyIdx: map[string]int{"id": 0, "values": 1}}
	binds, err := buildOracleReturnBinds(plan, reflect.ValueOf(inputRecord{}), meta)
	if err != nil {
		t.Fatalf("buildOracleReturnBinds() did not find input field by case-insensitive name: %v", err)
	}
	if len(binds) != 1 || binds[0].destination != -1 || binds[0].holder.Type() != reflect.TypeFor[int]() {
		t.Fatalf("input-field bind = %#v", binds)
	}

	if _, err := oracleReturnFieldType(plan, reflect.TypeFor[inputRecord](), meta, "unknown", -1); err == nil {
		t.Fatal("oracleReturnFieldType() accepted a column absent from both destination and input")
	}
	if _, err := buildOracleReturnBinds(
		&returnPlan{destType: reflect.TypeFor[inputRecord]()},
		reflect.ValueOf(inputRecord{}),
		meta,
	); err == nil {
		t.Fatal("buildOracleReturnBinds() accepted an empty return-column list")
	}
	unsupported := &returnPlan{
		destType:  reflect.TypeOf(struct{ Values map[string]string }{}),
		queryCols: []string{"values"},
	}
	if _, err := buildOracleReturnBinds(unsupported, reflect.ValueOf(inputRecord{}), meta); err == nil ||
		!strings.Contains(err.Error(), "unsupported output type") {
		t.Fatalf("unsupported Oracle output type error = %v", err)
	}

	if got, ok := oracleInputColumnIndex(meta.allColumnPrimaryKeyIdx, "id"); !ok || got != 0 {
		t.Fatalf("exact input column lookup = (%d, %t)", got, ok)
	}
	if got, ok := oracleInputColumnIndex(meta.allColumnPrimaryKeyIdx, "ID"); !ok || got != 0 {
		t.Fatalf("case-insensitive input column lookup = (%d, %t)", got, ok)
	}
	if _, ok := oracleInputColumnIndex(meta.allColumnPrimaryKeyIdx, "missing"); ok {
		t.Fatal("oracleInputColumnIndex() found a missing field")
	}

	if got := dereferenceOracleOutputType(reflect.TypeFor[**int]()); got != reflect.TypeFor[int]() {
		t.Fatalf("dereferenced output type = %s, want int", got)
	}
	for _, supported := range []reflect.Type{
		reflect.TypeFor[time.Time](),
		reflect.TypeFor[string](),
		reflect.TypeFor[bool](),
		reflect.TypeFor[int8](),
		reflect.TypeFor[uint64](),
		reflect.TypeFor[float32](),
		reflect.TypeFor[[]byte](),
	} {
		if !isOracleOutputBindType(supported) {
			t.Errorf("isOracleOutputBindType(%s) = false, want true", supported)
		}
	}
	for _, unsupportedType := range []reflect.Type{
		reflect.TypeFor[any](),
		reflect.TypeFor[map[string]string](),
		reflect.TypeFor[[]int](),
		reflect.TypeFor[complex64](),
		reflect.TypeFor[struct{}](),
	} {
		if isOracleOutputBindType(unsupportedType) {
			t.Errorf("isOracleOutputBindType(%s) = true, want false", unsupportedType)
		}
	}
}
