package orm

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestBuildUpdatePayloadMongoExcludesPrimaryKey(t *testing.T) {
	type record struct {
		ID   int
		Name string
		Age  int
	}
	row := reflect.ValueOf(record{ID: 5, Name: "Ada", Age: 36})
	got := buildUpdatePayloadMongo(row, columnMetaData{
		columns:         []string{"id", "name", "age"},
		columnIndex:     []int{0, 1, 2},
		primaryKeyIndex: []int{0},
	})
	want := bson.M{"name": "Ada", "age": 36}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Mongo update payload = %#v, want %#v", got, want)
	}
}

func TestBuildMongoDeletePayloadVariants(t *testing.T) {
	type singleKey struct {
		Name string
		ID   int
	}
	singleRows := []reflect.Value{
		reflect.ValueOf(singleKey{Name: "first", ID: 11}),
		reflect.ValueOf(singleKey{Name: "second", ID: 12}),
	}
	singleMeta := columnMetaData{
		columns:           []string{"id"},
		columnIndex:       []int{1},
		primaryKeyIndex:   []int{1},
		primaryKeyColumns: []string{"id"},
	}

	payload, err := (&ORM{}).buildDeletePayloadMongo(singleRows[:1], singleMeta)
	if err != nil || !reflect.DeepEqual(bson.M(payload), bson.M{"id": 11}) {
		t.Fatalf("single-key delete payload = %#v, %v", payload, err)
	}
	payload, err = (&ORM{}).buildDeletePayloadMongo(singleRows, singleMeta)
	if err != nil || !reflect.DeepEqual(bson.M(payload), bson.M{"id": bson.M{"$in": []any{11, 12}}}) {
		t.Fatalf("batch single-key delete payload = %#v, %v", payload, err)
	}

	type compositeKey struct {
		Name string
		Org  int
		ID   int
	}
	compositeRows := []reflect.Value{
		reflect.ValueOf(compositeKey{Name: "first", Org: 2, ID: 21}),
		reflect.ValueOf(compositeKey{Name: "second", Org: 3, ID: 22}),
	}
	compositeMeta := columnMetaData{
		columns:           []string{"org_id", "id"},
		columnIndex:       []int{1, 2},
		primaryKeyIndex:   []int{1, 2},
		primaryKeyColumns: []string{"org_id", "id"},
	}
	payload, err = (&ORM{}).buildDeletePayloadMongo(compositeRows, compositeMeta)
	if err != nil || !reflect.DeepEqual(bson.M(payload), bson.M{"$or": []bson.M{
		{"org_id": 2, "id": 21},
		{"org_id": 3, "id": 22},
	}}) {
		t.Fatalf("composite-key delete payload = %#v, %v", payload, err)
	}

	wherePayload, err := (&ORM{
		WhereClauses: []string{"status = ?"},
		WhereArgs:    []any{"inactive"},
	}).buildDeletePayloadMongo(singleRows[:1], singleMeta)
	if err != nil || !reflect.DeepEqual(bson.M(wherePayload), bson.M{"status": "inactive"}) {
		t.Fatalf("explicit where delete payload = %#v, %v", wherePayload, err)
	}
}

func TestBuildMongoDeletePayloadRejectsMissingAndZeroKeys(t *testing.T) {
	type record struct {
		ID int
	}
	row := reflect.ValueOf(record{})
	rows := []reflect.Value{row, row}
	noPK := columnMetaData{columns: []string{"id"}}
	if _, err := (&ORM{}).buildDeletePayloadMongo(rows, noPK); err == nil {
		t.Fatal("batch Mongo delete accepted metadata without a primary key")
	}

	meta := columnMetaData{columns: []string{"id"}, columnIndex: []int{0}, primaryKeyIndex: []int{0}}
	if _, err := (&ORM{}).buildDeletePayloadMongo([]reflect.Value{row}, meta); err == nil {
		t.Fatal("single Mongo delete accepted a zero-valued primary key")
	}
	if _, err := (&ORM{}).buildDeletePayloadMongo(rows, meta); err == nil {
		t.Fatal("batch Mongo delete accepted a zero-valued primary key")
	}

	type composite struct {
		Org int
		ID  int
	}
	compositeMeta := columnMetaData{
		columns:           []string{"org_id", "id"},
		primaryKeyIndex:   []int{0, 1},
		primaryKeyColumns: []string{"org_id", "id"},
	}
	invalidComposite := []reflect.Value{
		reflect.ValueOf(composite{Org: 1, ID: 2}),
		reflect.ValueOf(composite{Org: 2}),
	}
	if _, err := (&ORM{}).buildDeletePayloadMongo(invalidComposite, compositeMeta); err == nil {
		t.Fatal("composite Mongo delete accepted an unset key")
	}
}
