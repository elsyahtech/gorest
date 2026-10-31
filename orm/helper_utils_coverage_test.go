package orm

import (
	"reflect"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestBuildChunkSizeRespectsDriverLimitsAndClamps(t *testing.T) {
	tests := []struct {
		name        string
		driver      string
		columns     int
		primaryKeys int
		want        int
	}{
		{name: "sql server", driver: database.SQLSERVER, columns: 10, primaryKeys: 1, want: 99},
		{name: "postgres", driver: database.POSTGRES, columns: 10, primaryKeys: 1, want: 1000},
		{name: "mysql", driver: database.MYSQL, columns: 10, primaryKeys: 1, want: 1000},
		{name: "default driver", driver: database.SQLITE, columns: 10, primaryKeys: 1, want: 47},
		{name: "minimum one row", driver: database.SQLITE, columns: 1000, primaryKeys: 1, want: 1},
		{name: "maximum batch cap", driver: database.POSTGRES, columns: 0, primaryKeys: 0, want: 1000},
		{name: "non-positive parameter count", driver: database.SQLITE, columns: -1, primaryKeys: 0, want: 990},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := buildChunkSize(test.driver, test.columns, test.primaryKeys); got != test.want {
				t.Fatalf("buildChunkSize() = %d, want %d", got, test.want)
			}
		})
	}

	limits := map[string]int{
		database.SQLSERVER: 2100,
		database.POSTGRES:  65535,
		database.MYSQL:     65535,
		database.SQLITE:   1000,
		database.ORACLE:   1000,
	}
	for driver, want := range limits {
		if got := getDriverParamLimit(driver); got != want {
			t.Errorf("getDriverParamLimit(%q) = %d, want %d", driver, got, want)
		}
	}
}

func TestContainsStringRequiresExactMatch(t *testing.T) {
	values := []string{"user_id", "email"}
	if !containsString(values, "email") {
		t.Fatal("containsString() did not find an exact match")
	}
	if containsString(values, "user") || containsString(nil, "email") {
		t.Fatal("containsString() matched a partial value or a nil slice")
	}
}

func TestInferUniqueColumnsUsesCaseInsensitiveColumnNames(t *testing.T) {
	meta := columnMetaData{columns: []string{"user_id", "email", "created_at"}}
	got := inferUniqueColumns("idx_USER_ID_EMAIL", meta)
	want := []string{"user_id", "email"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inferUniqueColumns() = %#v, want %#v", got, want)
	}
	if got := inferUniqueColumns("idx_unknown", meta); len(got) != 0 {
		t.Fatalf("inferUniqueColumns(unknown) = %#v, want no columns", got)
	}
}

func TestPluralizeAndNormalizeCollectionNames(t *testing.T) {
	plurals := map[string]string{
		"fish":       "fish",
		"person":     "people",
		"child":      "children",
		"city":       "cities",
		"day":        "days",
		"box":        "boxes",
		"church":     "churches",
		"leaf":       "leaves",
		"knife":      "knives",
		"tomato":     "tomatoes",
		"stereo":     "stereos",
		"book":       "books",
		"status":     "statuses",
	}
	for singular, want := range plurals {
		if got := pluralizeForm(singular); got != want {
			t.Errorf("pluralizeForm(%q) = %q, want %q", singular, got, want)
		}
	}

	singulars := map[string]string{
		"FISH":       "fish",
		"people":     "person",
		"children":   "child",
		"cities":     "city",
		"days":       "day",
		"boxes":      "box",
		"churches":   "church",
		"leaves":     "leaf",
		"knives":     "knife",
		"tomatoes":   "tomato",
		"stereos":    "stereo",
		"books":      "book",
	}
	for plural, want := range singulars {
		if got := pluralismNormalization(plural); got != want {
			t.Errorf("pluralismNormalization(%q) = %q, want %q", plural, got, want)
		}
	}
}

func TestReflectValueConversionHelpers(t *testing.T) {
	var text string
	if err := assignReflectValue(reflect.ValueOf(&text).Elem(), []byte("value")); err != nil || text != "value" {
		t.Fatalf("byte-to-string assignment = %q, %v", text, err)
	}

	var flag bool
	if err := assignReflectValue(reflect.ValueOf(&flag).Elem(), int8(1)); err != nil || !flag {
		t.Fatalf("integer-to-bool assignment = %v, %v", flag, err)
	}

	var number int64
	if err := assignReflectValue(reflect.ValueOf(&number).Elem(), int32(7)); err != nil || number != 7 {
		t.Fatalf("convertible integer assignment = %d, %v", number, err)
	}

	var pointer *int64
	if err := assignReflectValue(reflect.ValueOf(&pointer).Elem(), int32(9)); err != nil || pointer == nil || *pointer != 9 {
		t.Fatalf("pointer assignment = %v, %v", pointer, err)
	}

	var boolPointer *bool
	if err := assignReflectValue(reflect.ValueOf(&boolPointer).Elem(), 1); err != nil || boolPointer == nil || !*boolPointer {
		t.Fatalf("pointer-to-bool assignment = %v, %v", boolPointer, err)
	}

	present := int64(3)
	pointer = &present
	if err := assignReflectValue(reflect.ValueOf(&pointer).Elem(), (*int)(nil)); err != nil || pointer != nil {
		t.Fatalf("nil source pointer assignment = %v, %v", pointer, err)
	}

	initial := "unchanged"
	if err := assignReflectValue(reflect.ValueOf(&initial).Elem(), nil); err != nil || initial != "unchanged" {
		t.Fatalf("nil source assignment = %q, %v", initial, err)
	}
	if err := assignReflectValue(reflect.ValueOf(&initial), "value"); err == nil {
		t.Fatal("assignReflectValue() accepted a non-settable target")
	}
	if err := assignReflectValue(reflect.Value{}, "value"); err == nil {
		t.Fatal("assignReflectValue() accepted an invalid target")
	}
	if err := assignReflectValue(reflect.ValueOf(&initial).Elem(), struct{}{}); err == nil {
		t.Fatal("assignReflectValue() accepted an incompatible value")
	}

	if got, ok := toString([]byte("bytes")); !ok || got != "bytes" {
		t.Fatalf("toString([]byte) = %q, %v", got, ok)
	}
	if _, ok := toString(42); ok {
		t.Fatal("toString() accepted an unsupported value")
	}
	if got, ok := toInt64(uint16(12)); !ok || got != 12 {
		t.Fatalf("toInt64(uint16) = %d, %v", got, ok)
	}
	if _, ok := toInt64(float64(1)); ok {
		t.Fatal("toInt64() accepted a floating-point value")
	}
	if got, ok := toBool(int64(0)); !ok || got {
		t.Fatalf("toBool(0) = %v, %v", got, ok)
	}
	if _, ok := toBool("true"); ok {
		t.Fatal("toBool() accepted a string")
	}
}

func TestReturnDestinationAndSQLClauseHelpers(t *testing.T) {
	type record struct {
		ID int
	}
	var item record
	var items []record
	var pointers []*record
	var numbers []int
	var scalar int

	for _, destination := range []any{&item, &items, &pointers} {
		if !isReturnDest(destination) {
			t.Errorf("isReturnDest(%T) = false, want true", destination)
		}
	}
	for _, destination := range []any{nil, item, &scalar, &numbers} {
		if isReturnDest(destination) {
			t.Errorf("isReturnDest(%T) = true, want false", destination)
		}
	}

	plan := &returnPlan{queryCols: []string{"id", "display_name"}}
	if got, want := plan.returningClause(), " RETURNING id, display_name"; got != want {
		t.Errorf("returningClause() = %q, want %q", got, want)
	}
	if got, want := plan.outputClause(), "OUTPUT INSERTED.id, INSERTED.display_name"; got != want {
		t.Errorf("outputClause() = %q, want %q", got, want)
	}
}
