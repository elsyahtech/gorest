package orm

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestValidateGroupByColumnsKeepsRequestedCanonicalColumns(t *testing.T) {
	type testRow struct {
		Email      string `gorest:"email"`
		Department string `gorest:"department"`
		ID         int    `gorest:"id"`
	}

	orm := &ORM{GroupByClauses: []string{"USERS.Email", " DEPARTMENT "}}

	if err := orm.validateGroupByColumns(reflect.TypeOf(testRow{}), "users"); err != nil {
		t.Fatalf("validateGroupByColumns() error = %v", err)
	}

	want := []string{"email", "department"}
	if !reflect.DeepEqual(orm.GroupByClauses, want) {
		t.Fatalf("GroupByClauses = %#v, want %#v", orm.GroupByClauses, want)
	}
}

func TestAddMongoGroupAccumulatorUsesAverageOperator(t *testing.T) {
	type testRow struct {
		Score float64 `gorest:"score"`
	}

	groupDoc := bson.M{}
	_, err := addAccumulator(&structAddAccumulator{
		expression:   "AVG(score) AS average",
		collection:   "records",
		knownColumns: map[string]string{"score": "score"},
		structType:   reflect.TypeOf(testRow{}),
		accumulators: map[string]mongoGroupAccumulator{},
		groupDoc:     groupDoc,
		aliases:      map[string]string{},
		project:      bson.M{},
	})
	if err != nil {
		t.Fatalf("addAccumulator() error = %v", err)
	}

	got, ok := groupDoc["__gorest_agg_0"].(bson.M)
	if !ok {
		t.Fatalf("group accumulator type = %T, want bson.M", groupDoc["__gorest_agg_0"])
	}

	if got[dolarAvg] != "$score" {
		t.Fatalf("AVG accumulator = %#v, want %q", got, "$score")
	}
}

func TestMongoGroupPipelineAppliesDefaultLimitByResultShape(t *testing.T) {
	tests := []struct {
		build       func(*ORM, mongo.Pipeline, mongo.Pipeline, bson.M, bson.D) mongo.Pipeline
		name        string
		limit       int
		wantLimitTo int64
		wantLimit   bool
	}{
		{
			name: "slice has no default limit",
			build: func(orm *ORM, pipeline, having mongo.Pipeline, project bson.M, sort bson.D) mongo.Pipeline {
				return orm.postGroupSlicePipeline(pipeline, having, project, sort)
			},
		},
		{
			name: "single result defaults to one",
			build: func(orm *ORM, pipeline, having mongo.Pipeline, project bson.M, sort bson.D) mongo.Pipeline {
				return applyMongoSingleResultDefaultLimit(
					orm.postGroupSlicePipeline(pipeline, having, project, sort),
					orm.LimitVal,
				)
			},
			wantLimit:   true,
			wantLimitTo: 1,
		},
		{
			name:  "explicit limit is preserved",
			limit: 5,
			build: func(orm *ORM, pipeline, having mongo.Pipeline, project bson.M, sort bson.D) mongo.Pipeline {
				return orm.postGroupSlicePipeline(pipeline, having, project, sort)
			},
			wantLimit:   true,
			wantLimitTo: 5,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			orm := &ORM{LimitVal: test.limit}
			pipeline := test.build(
				orm,
				mongo.Pipeline{bson.D{{Key: "$group", Value: bson.M{"_id": "$department"}}}},
				nil,
				bson.M{"department": "$_id"},
				nil,
			)

			var gotLimit int64
			for _, stage := range pipeline {
				if value, ok := stage[0].Value.(int64); stage[0].Key == dolarLimit && ok {
					gotLimit = value
				}
			}

			if test.wantLimit && gotLimit != test.wantLimitTo {
				t.Fatalf("pipeline limit = %d, want %d", gotLimit, test.wantLimitTo)
			}
			if !test.wantLimit && gotLimit != 0 {
				t.Fatalf("pipeline limit = %d, want no limit", gotLimit)
			}
		})
	}
}
