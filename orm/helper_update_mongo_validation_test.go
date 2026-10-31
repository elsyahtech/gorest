package orm

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestScanMongoBulkUpdatePrimaryKeys(t *testing.T) {
	type record struct {
		TenantID int
		ID       int
	}
	meta := columnMetaData{
		primaryKeyColumns: []string{"tenant_id", "id"},
		primaryKeyIndex:   []int{0, 1},
	}
	cursor, err := mongo.NewCursorFromDocuments([]any{
		bson.M{"tenant_id": 1, "id": 10},
		bson.M{"tenant_id": 2, "id": 20},
	}, nil, nil)
	if err != nil {
		t.Fatalf("create Mongo cursor: %v", err)
	}
	defer cursor.Close(context.Background())

	found, err := scanMongoBulkUpdatePrimaryKeys(context.Background(), cursor, meta)
	if err != nil {
		t.Fatalf("scanMongoBulkUpdatePrimaryKeys() error = %v", err)
	}
	if !found[primaryKeyTuple([]any{1, 10})] || !found[primaryKeyTuple([]any{2, 20})] || len(found) != 2 {
		t.Fatalf("scanned Mongo keys = %#v", found)
	}

	rows := []reflect.Value{
		reflect.ValueOf(record{TenantID: 1, ID: 10}),
		reflect.ValueOf(record{TenantID: 3, ID: 30}),
		reflect.ValueOf(record{TenantID: 3, ID: 30}),
	}
	if got, want := missingBulkUpdatePrimaryKeys(rows, meta, found), []string{"3/30"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing bulk update keys = %v, want %v", got, want)
	}
	if got := buildMongoBulkUpdateFilters(rows[:1], meta); !reflect.DeepEqual(got, []bson.M{{"tenant_id": 1, "id": 10}}) {
		t.Fatalf("bulk update filters = %#v", got)
	}
	if got := buildMongoBulkUpdateProjection(meta); !reflect.DeepEqual(got, bson.M{"tenant_id": 1, "id": 1}) {
		t.Fatalf("bulk update projection = %#v", got)
	}
}

func TestScanMongoBulkUpdatePrimaryKeysReturnsIterationError(t *testing.T) {
	cursorErr := errors.New("cursor iteration failed")
	cursor, err := mongo.NewCursorFromDocuments([]any{}, cursorErr, nil)
	if err != nil {
		t.Fatalf("create erroring Mongo cursor: %v", err)
	}
	defer cursor.Close(context.Background())

	if _, err := scanMongoBulkUpdatePrimaryKeys(
		context.Background(),
		cursor,
		columnMetaData{primaryKeyColumns: []string{"id"}},
	); !errors.Is(err, cursorErr) {
		t.Fatalf("scanMongoBulkUpdatePrimaryKeys() error = %v, want wrapped cursor error", err)
	}
}
