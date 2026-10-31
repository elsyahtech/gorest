package orm

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoFindValueAndProjectionHelpers(t *testing.T) {
	objectID := bson.NewObjectID()
	objectIDColumns := map[string]bool{"_id": true, "owner_id": true}
	if got := mongoValue("_id", objectID.Hex(), objectIDColumns); got != objectID {
		t.Fatalf("mongoValue(ObjectID hex) = %#v, want %#v", got, objectID)
	}
	if got := mongoValue("_id", "invalid", objectIDColumns); got != "invalid" {
		t.Fatalf("mongoValue(invalid ObjectID) = %#v, want unchanged string", got)
	}
	if got := mongoValue("name", objectID.Hex(), objectIDColumns); got != objectID.Hex() {
		t.Fatalf("mongoValue(non-ObjectID field) = %#v, want unchanged string", got)
	}
	if got := mongoFieldName("users.owner_id", "users"); got != "owner_id" {
		t.Fatalf("mongoFieldName() = %q, want owner_id", got)
	}

	got := flattenMongoArgs("_id", []any{[]string{objectID.Hex(), "invalid"}, [1]string{objectID.Hex()}, []byte{1, 2}}, objectIDColumns)
	want := []any{objectID, "invalid", objectID, []byte{1, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flattenMongoArgs() = %#v, want %#v", got, want)
	}

	if got := combineMongo("$and", []bson.M{{"a": 1}}); !reflect.DeepEqual(got, bson.M{"a": 1}) {
		t.Fatalf("combineMongo(single) = %#v", got)
	}
	if got := combineMongo("$or", []bson.M{{"a": 1}, {"b": 2}}); !reflect.DeepEqual(got, bson.M{"$or": []bson.M{{"a": 1}, {"b": 2}}}) {
		t.Fatalf("combineMongo(multiple) = %#v", got)
	}
	if got, want := likeToRegex("user_%"), "^user..*$"; got != want {
		t.Fatalf("likeToRegex() = %q, want %q", got, want)
	}
}

func TestBuildMongoSortAndProjectionValidateAndNormalizeFields(t *testing.T) {
	type record struct {
		ID   string `gorest:"_id, primary_key"`
		Name string `gorest:"display_name"`
		Age  int    `gorest:"age"`
	}
	query := &ORM{}
	sort, err := query.buildMongoSort([]string{"users.display_name DESC", "age"}, "users", reflect.TypeFor[record]())
	if err != nil {
		t.Fatalf("buildMongoSort() error = %v", err)
	}
	if !reflect.DeepEqual(sort, bson.D{{Key: "display_name", Value: -1}, {Key: "age", Value: 1}}) {
		t.Fatalf("Mongo sort = %#v", sort)
	}

	if _, err := query.buildMongoSort([]string{"unknown"}, "users", reflect.TypeFor[record]()); err == nil {
		t.Fatal("buildMongoSort() accepted an untagged field")
	}
	if _, err := query.buildMongoSort([]string{"age"}, "", reflect.TypeFor[record]()); err == nil {
		t.Fatal("buildMongoSort() accepted an empty collection")
	}

	if got, want := buildMongoProjection([]string{"users.display_name, age"}, "users"),
		(bson.M{"display_name": 1, "age": 1}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Mongo projection = %#v, want %#v", got, want)
	}
	if got := buildMongoProjection(nil, "users"); got != nil {
		t.Fatalf("empty Mongo projection = %#v, want nil", got)
	}
	if got := buildMongoProjection([]string{"users.*"}, "users"); got != nil {
		t.Fatalf("wildcard Mongo projection = %#v, want nil", got)
	}
}

func TestBuildMongoDistinctPipelineMaintainsTupleShapeAndPagination(t *testing.T) {
	query := &ORM{
		DistinctCols: []string{"users.team_id", "users.status"},
		OffsetVal:    3,
		LimitVal:     4,
	}
	filter := bson.M{"active": true}
	sort := bson.D{{Key: "team_id", Value: 1}}
	pipeline, err := query.buildMongoDistinctPipeline(filter, sort, "users")
	if err != nil {
		t.Fatalf("buildMongoDistinctPipeline() error = %v", err)
	}
	if len(pipeline) != 6 {
		t.Fatalf("distinct pipeline stages = %#v", pipeline)
	}
	if pipeline[0][0].Key != dolarMatch || pipeline[1][0].Key != dolarGroup ||
		pipeline[2][0].Key != dolarProject || pipeline[3][0].Key != dolarSort ||
		pipeline[4][0].Key != dolarSkip || pipeline[5][0].Key != dolarLimit {
		t.Fatalf("distinct pipeline stage order = %#v", pipeline)
	}

	query.LimitVal = 0
	query.OffsetVal = 0
	pipeline, err = query.buildMongoDistinctPipeline(nil, nil, "users")
	if err != nil {
		t.Fatalf("distinct pipeline without limit error = %v", err)
	}
	if len(pipeline) != 2 {
		t.Fatalf("distinct pipeline without sort/limit = %#v", pipeline)
	}
}

func TestBuildMongoDistinctPipelineRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name  string
		query *ORM
		want  string
	}{
		{name: "preload", query: &ORM{DistinctCols: []string{"team"}, Preloads: map[string][]any{"Owner": nil}}, want: "Preload"},
		{name: "missing fields", query: &ORM{}, want: "requires at least one field"},
		{name: "invalid field syntax", query: &ORM{DistinctCols: []string{"COUNT(*)"}}, want: "invalid MongoDB distinct field"},
		{name: "duplicate output names", query: &ORM{DistinctCols: []string{"users.team", "teams.team"}}, want: "duplicate MongoDB distinct output"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.query.buildMongoDistinctPipeline(nil, nil, "users")
			if err == nil || test.query.Error == nil {
				t.Fatal("invalid distinct request unexpectedly succeeded")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want substring %q", err, test.want)
			}
		})
	}
}

func TestNormalizeBSONValueRecursively(t *testing.T) {
	objectID := bson.NewObjectID()
	date := bson.DateTime(time.Date(2025, time.January, 2, 3, 4, 5, 0, time.FixedZone("test", 3600)).UnixMilli())
	binary := bson.Binary{Subtype: 0, Data: []byte{1, 2}}
	input := bson.D{
		{Key: "id", Value: objectID},
		{Key: "created", Value: date},
		{Key: "data", Value: binary},
		{Key: "items", Value: bson.A{objectID, bson.M{"nested": date}}},
	}
	got, ok := normalizeBSONValue(input).(map[string]any)
	if !ok {
		t.Fatalf("normalized document type = %T, want map[string]any", normalizeBSONValue(input))
	}
	if got["id"] != objectID.Hex() {
		t.Fatalf("normalized ObjectID = %#v", got["id"])
	}
	if got["created"] != date.Time().UTC() {
		t.Fatalf("normalized date = %#v, want %v", got["created"], date.Time().UTC())
	}
	if !reflect.DeepEqual(got["data"], []byte{1, 2}) {
		t.Fatalf("normalized binary = %#v", got["data"])
	}
	if _, ok := got["items"].([]any); !ok {
		t.Fatalf("normalized array type = %T, want []any", got["items"])
	}
	if got := normalizeBSONValue("unchanged"); got != "unchanged" {
		t.Fatalf("normalization changed scalar value: %#v", got)
	}
}

func TestMongoOptionBuilderRequiresCollectionAndPreloadPlan(t *testing.T) {
	if options, err := (&ORM{}).buildMongoOptionFindMongo("users", nil, nil, true); err == nil || options != nil {
		t.Fatalf("nil preload options = %#v, %v; want error", options, err)
	}
	if options, err := (&ORM{}).buildMongoOptionFindMongo("", &mongoPreloadPlan{}, nil, true); err == nil || options != nil {
		t.Fatalf("empty collection options = %#v, %v; want error", options, err)
	}

	preload := &mongoPreloadPlan{}
	query := &ORM{LimitVal: 3, OffsetVal: 2, limitSet: true}
	options, err := query.buildMongoOptionFindMongo("users", preload, bson.D{{Key: "age", Value: 1}}, true)
	if err != nil || options == nil {
		t.Fatalf("valid Find options = %#v, %v", options, err)
	}
}

func TestMongoSingleResultDefaultLimitDoesNotMutateInput(t *testing.T) {
	pipeline := mongo.Pipeline{bson.D{{Key: dolarMatch, Value: bson.M{"active": true}}}}
	got := applyMongoSingleResultDefaultLimit(pipeline, 0)
	if len(pipeline) != 1 || len(got) != 2 {
		t.Fatalf("default limit changed input pipeline or not appended: input %#v, output %#v", pipeline, got)
	}
}
