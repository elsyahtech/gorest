package orm

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/elsyahtech/gorest/database"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestBuildRowsScanFindMongoMapsSliceAndReportsCursorErrors(t *testing.T) {
	type record struct {
		ID   int    `gorest:"id"`
		Name string `gorest:"name"`
	}
	query := &ORM{}
	cursor, err := mongo.NewCursorFromDocuments([]any{
		bson.M{"id": 1, "name": "Ada", "unknown": "ignored"},
		bson.M{"id": 2, "name": "Lin"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("create Mongo cursor: %v", err)
	}
	defer cursor.Close(context.Background())

	result := []record{}
	value := reflect.ValueOf(&result).Elem()
	slice := reflect.MakeSlice(value.Type(), 0, 0)
	req := &structBuildRowsScanFindMongo{
		execCtx:        context.Background(),
		structType:     reflect.TypeFor[record](),
		cursor:         cursor,
		fieldLookupMap: buildStructFieldMapByTag(reflect.TypeFor[record]()),
		valElem:        &value,
		sliceVal:       &slice,
		isSlice:        true,
	}
	if _, err := query.buildRowsScanFindMongo(req); err != nil {
		t.Fatalf("buildRowsScanFindMongo() error = %v", err)
	}
	if want := []record{{ID: 1, Name: "Ada"}, {ID: 2, Name: "Lin"}}; !reflect.DeepEqual(result, want) ||
		query.RowsAffected != 2 {
		t.Fatalf("Mongo scan result = %#v, affected = %d", result, query.RowsAffected)
	}

	cursorErr := errors.New("Mongo cursor failed")
	errorCursor, err := mongo.NewCursorFromDocuments([]any{}, cursorErr, nil)
	if err != nil {
		t.Fatalf("create erroring Mongo cursor: %v", err)
	}
	defer errorCursor.Close(context.Background())
	req.cursor = errorCursor
	if _, err := query.buildRowsScanFindMongo(req); !errors.Is(err, cursorErr) {
		t.Fatalf("buildRowsScanFindMongo() error = %v, want wrapped cursor error", err)
	}
}

func TestMongoCreateAndUpdateHelpersHandleValidationAndServiceErrors(t *testing.T) {
	ctx := context.Background()
	query := &ORM{DatabaseConfig: &database.Config{Driver: database.MONGO}}
	if err := query.execCreateMongo(ctx, []any{map[string]any{"id": 1}}, "users", []string{"id"}); err == nil {
		t.Fatal("execCreateMongo() suppressed unavailable database error")
	}

	if err := (&ORM{}).execCreateMongoNormal(ctx, nil, nil); err != nil {
		t.Fatalf("execCreateMongoNormal(empty) = %v", err)
	}
	upsert := &ORM{}
	if err := upsert.execCreateMongoUpsert(ctx, []any{42}, nil, []string{"id"}); err == nil {
		t.Fatal("execCreateMongoUpsert() accepted an invalid document type")
	}

	type record struct {
		ID int
	}
	row := reflect.ValueOf(record{ID: 1})
	if err := (&ORM{}).updateSingleMongo(ctx, nil, []reflect.Value{row}, columnMetaData{
		columns: []string{"id"}, columnIndex: []int{0}, primaryKeyIndex: []int{0},
	}); err == nil {
		t.Fatal("updateSingleMongo() accepted a primary-key-only update")
	}
}

func TestLoadFindMongoPreloadDataHandlesDistinctAndEmptyPlans(t *testing.T) {
	value := reflect.ValueOf(&struct{ ID int }{ID: 1}).Elem()
	if err := (&ORM{IsDistinct: true}).loadFindMogoPreloadData(
		context.Background(),
		&mongoPreloadPlan{hasMany: []*mongoHasMany{{}}},
		&value,
		false,
	); err != nil {
		t.Fatalf("distinct loadFindMogoPreloadData() = %v", err)
	}
	if err := (&ORM{}).loadFindMogoPreloadData(context.Background(), &mongoPreloadPlan{}, &value, false); err != nil {
		t.Fatalf("empty loadFindMogoPreloadData() = %v", err)
	}
}
