package orm

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestBuildBulkMongoFilterCombinesPrimaryKeyAndWhere(t *testing.T) {
	type record struct {
		ID     int
		Tenant int
	}
	row := reflect.ValueOf(record{ID: 17, Tenant: 4})
	meta := &columnMetaData{
		primaryKeyColumns: []string{"id"},
		primaryKeyIndex:   []int{0},
	}

	filter, err := (&ORM{}).buildBulkMongoFilter(row, meta)
	if err != nil {
		t.Fatalf("buildBulkMongoFilter(primary key) error = %v", err)
	}
	if !reflect.DeepEqual(filter, bson.M{"id": 17}) {
		t.Fatalf("primary key filter = %#v", filter)
	}

	query := &ORM{WhereClauses: []string{"tenant = ?"}, WhereArgs: []any{4}}
	filter, err = query.buildBulkMongoFilter(row, meta)
	if err != nil {
		t.Fatalf("buildBulkMongoFilter(where) error = %v", err)
	}
	want := bson.M{"$and": []bson.M{{"id": 17}, {"tenant": 4}}}
	if !reflect.DeepEqual(filter, want) {
		t.Fatalf("bulk update filter = %#v, want %#v", filter, want)
	}

	invalid := &ORM{WhereClauses: []string{"tenant = ?"}}
	if _, err := invalid.buildBulkMongoFilter(row, meta); err == nil || invalid.Error == nil {
		t.Fatal("buildBulkMongoFilter() accepted a missing WHERE argument")
	}
	if got := invalid.HTTPCode; got == 0 {
		t.Fatal("buildBulkMongoFilter() did not report an HTTP error status")
	}

	withoutPrimaryKey := &ORM{}
	if _, err := withoutPrimaryKey.buildBulkMongoFilter(row, &columnMetaData{}); err == nil {
		t.Fatal("buildBulkMongoFilter() accepted a missing primary key")
	}
}

func TestBuildSingleMongoFilterUsesWhereBeforePrimaryKey(t *testing.T) {
	type record struct {
		ID int
	}
	row := reflect.ValueOf(record{ID: 17})
	meta := &columnMetaData{
		primaryKeyColumns: []string{"id"},
		primaryKeyIndex:   []int{0},
	}

	filter, err := (&ORM{}).buildSingleMongoFilter(row, meta)
	if err != nil || !reflect.DeepEqual(filter, bson.M{"id": 17}) {
		t.Fatalf("primary key filter = %#v, %v", filter, err)
	}

	query := &ORM{WhereClauses: []string{"active = ?"}, WhereArgs: []any{true}}
	filter, err = query.buildSingleMongoFilter(row, meta)
	if err != nil || !reflect.DeepEqual(filter, bson.M{"active": true}) {
		t.Fatalf("where filter = %#v, %v", filter, err)
	}

	invalid := &ORM{WhereClauses: []string{"active = ?"}}
	if _, err := invalid.buildSingleMongoFilter(row, meta); err == nil || invalid.Error == nil {
		t.Fatal("buildSingleMongoFilter() accepted a missing WHERE argument")
	}

	withoutKey := &ORM{}
	if _, err := withoutKey.buildSingleMongoFilter(row, &columnMetaData{}); err == nil {
		t.Fatal("buildSingleMongoFilter() accepted neither a WHERE clause nor primary key")
	}
}
