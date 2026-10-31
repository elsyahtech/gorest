package orm

import (
	"reflect"
	"testing"
)

func TestNormalizeStructInputMongoAppliesTagsAndSelectedColumns(t *testing.T) {
	type record struct {
		ID       int    `bson:"record_id,omitempty"`
		Name     string `gorest:"display_name"`
		Disabled bool   `gorest:"disabled"`
		Ignored  string `gorest:"-"`
		Untagged string
	}

	query := &ORM{SelectedCols: []string{"display_name, record_id"}}
	rows, err := query.normalizeStructInputMongo(reflect.ValueOf(record{
		ID: 7, Name: "Ada", Disabled: true, Ignored: "hidden", Untagged: "omit",
	}), create)
	if err != nil {
		t.Fatalf("normalizeStructInputMongo() error = %v", err)
	}
	if want := []any{map[string]any{"record_id": 7, "display_name": "Ada"}}; !reflect.DeepEqual(rows, want) {
		t.Fatalf("normalized Mongo document = %#v, want %#v", rows, want)
	}

	query.SelectedCols = nil
	rows, err = query.normalizeStructInputMongo(reflect.ValueOf([]*record{
		{ID: 8, Name: "Lin"},
		{ID: 9, Name: "Kai"},
	}), create)
	if err != nil {
		t.Fatalf("normalizeStructInputMongo(slice) error = %v", err)
	}
	if len(rows) != 2 || rows[0].(map[string]any)["record_id"] != 8 || rows[1].(map[string]any)["record_id"] != 9 {
		t.Fatalf("normalized Mongo documents = %#v", rows)
	}

	for _, invalid := range []reflect.Value{
		reflect.ValueOf(1),
		reflect.ValueOf([]*record{nil}),
		reflect.ValueOf([]int{1}),
	} {
		if _, err := query.normalizeStructInputMongo(invalid, create); err == nil {
			t.Errorf("normalizeStructInputMongo(%v) accepted invalid input", invalid.Type())
		}
	}
}

func TestValidateDataMongoChecksRequiredMetadataAndValues(t *testing.T) {
	type record struct {
		ID   int    `gorest:"id, primary_key"`
		Name string `gorest:"name, required"`
	}

	query := &ORM{}
	rows, err := query.validateDataMongo(record{ID: 3, Name: "Ada"}, fieldRequirement{
		requirePrimaryKey: true,
		validateRequired:  true,
	}, create)
	if err != nil || len(rows) != 1 {
		t.Fatalf("validateDataMongo(valid) = (%#v, %v)", rows, err)
	}

	query = &ORM{}
	if _, err := query.validateDataMongo(record{ID: 3}, fieldRequirement{
		requirePrimaryKey: true,
		validateRequired:  true,
	}, create); err == nil {
		t.Fatal("validateDataMongo() accepted an empty required field")
	}
}
