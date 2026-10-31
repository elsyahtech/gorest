package orm

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestBuildMongoJoinPipelineBuildsLookupProjectionAndPagination(t *testing.T) {
	type joinedRow struct {
		UserID     string `gorest:"user_id"`
		UserName   string `gorest:"user_name"`
		OrderTotal int    `gorest:"order_total"`
	}

	query := &ORM{
		JoinClauses:  []string{"LEFT OUTER JOIN orders ON users.id = orders.user_id"},
		SelectedCols: []string{"users.id AS user_id, users.name AS user_name", "orders.total AS order_total"},
		OffsetVal:    2,
		LimitVal:     5,
	}
	filter := bson.M{"orders.total": bson.M{"$gt": 0}}
	sort := bson.D{{Key: "orders.total", Value: -1}}
	pipeline, err := query.buildMongoJoinPipeline("users", filter, sort, reflect.TypeFor[joinedRow]())
	if err != nil {
		t.Fatalf("buildMongoJoinPipeline() error = %v", err)
	}
	want := bson.D{
		{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "orders"},
			{Key: "localField", Value: "id"},
			{Key: "foreignField", Value: "user_id"},
			{Key: "as", Value: "orders"},
		}},
	}
	if pipeline[0][0].Key != "$lookup" || !reflect.DeepEqual(pipeline[0][0].Value, want[0].Value) {
		t.Fatalf("lookup stage = %#v, want %#v", pipeline[0], want[0])
	}
	if got := pipeline[1]; !reflect.DeepEqual(got, bson.D{{Key: "$unwind", Value: bson.D{
		{Key: "path", Value: "$orders"},
		{Key: "preserveNullAndEmptyArrays", Value: true},
	}}}) {
		t.Fatalf("LEFT JOIN unwind stage = %#v", got)
	}
	if pipeline[2][0].Key != "$match" || !reflect.DeepEqual(pipeline[2][0].Value, filter) {
		t.Fatalf("match stage = %#v", pipeline[2])
	}
	if pipeline[3][0].Key != "$sort" || !reflect.DeepEqual(pipeline[3][0].Value, sort) {
		t.Fatalf("sort stage = %#v", pipeline[3])
	}
	wantProjection := bson.D{
		{Key: "_id", Value: 0},
		{Key: "user_id", Value: "$id"},
		{Key: "user_name", Value: "$name"},
		{Key: "order_total", Value: "$orders.total"},
	}
	if pipeline[4][0].Key != "$project" || !reflect.DeepEqual(pipeline[4][0].Value, wantProjection) {
		t.Fatalf("projection stage = %#v, want %#v", pipeline[4], wantProjection)
	}
	if pipeline[5][0].Key != "$skip" || pipeline[5][0].Value != 2 ||
		pipeline[6][0].Key != "$limit" || pipeline[6][0].Value != 5 {
		t.Fatalf("pagination stages = %#v, %#v", pipeline[5], pipeline[6])
	}
}

func TestBuildMongoJoinPipelineSupportsInnerJoinAndReversedOnFields(t *testing.T) {
	type joinedRow struct {
		UserID   string `gorest:"user_id"`
		UserName string `gorest:"user_name"`
	}
	query := &ORM{
		JoinClauses:  []string{"INNER JOIN orders ON orders.user_id = users.id"},
		SelectedCols: []string{"users.id AS user_id", "orders.user_name AS user_name"},
	}
	pipeline, err := query.buildMongoJoinPipeline("users", nil, nil, reflect.TypeFor[joinedRow]())
	if err != nil {
		t.Fatalf("buildMongoJoinPipeline() error = %v", err)
	}
	if len(pipeline) != 3 {
		t.Fatalf("INNER JOIN pipeline stages = %#v, want lookup, unwind, project", pipeline)
	}
	if pipeline[0][0].Key != "$lookup" {
		t.Fatalf("first stage = %#v, want lookup", pipeline[0])
	}
	if !reflect.DeepEqual(pipeline[1], bson.D{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$orders"}}}}) {
		t.Fatalf("INNER JOIN unwind stage = %#v", pipeline[1])
	}
}

func TestMongoJoinRejectsUnsupportedClausesAndProjection(t *testing.T) {
	type joinedRow struct {
		ID   string `gorest:"id"`
		Name string `gorest:"name"`
	}
	tests := []struct {
		name  string
		query *ORM
		want  string
	}{
		{
			name:  "preload conflict",
			query: &ORM{JoinClauses: []string{"JOIN orders ON users.id = orders.user_id"}, Preloads: map[string][]any{"Owner": nil}},
			want:  "cannot be combined with Preload",
		},
		{name: "multiple joins", query: &ORM{JoinClauses: []string{"JOIN orders ON users.id = orders.user_id", "JOIN teams ON users.team_id = teams.id"}}, want: "accepts one simple JOIN"},
		{name: "invalid clause", query: &ORM{JoinClauses: []string{"JOIN orders ON users.id > orders.user_id"}}, want: "supports `JOIN collection ON"},
		{name: "wrong table references", query: &ORM{JoinClauses: []string{"JOIN orders ON teams.id = orders.user_id"}}, want: "must reference the base collection"},
		{name: "missing explicit select", query: &ORM{JoinClauses: []string{"JOIN orders ON users.id = orders.user_id"}}, want: "requires Select()"},
		{name: "wildcard selection", query: &ORM{JoinClauses: []string{"JOIN orders ON users.id = orders.user_id"}, SelectedCols: []string{"users.*"}}, want: "wildcard projection"},
		{name: "unqualified selection", query: &ORM{JoinClauses: []string{"JOIN orders ON users.id = orders.user_id"}, SelectedCols: []string{"name"}}, want: "qualify mongoDB Join() selected fields"},
		{name: "invalid alias", query: &ORM{JoinClauses: []string{"JOIN orders ON users.id = orders.user_id"}, SelectedCols: []string{"users.name AS result.name"}}, want: "aliases must be simple"},
		{name: "missing destination tag", query: &ORM{JoinClauses: []string{"JOIN orders ON users.id = orders.user_id"}, SelectedCols: []string{"users.name AS missing"}}, want: "must match a gorest tag"},
		{name: "duplicate output aliases", query: &ORM{JoinClauses: []string{"JOIN orders ON users.id = orders.user_id"}, SelectedCols: []string{"users.id AS id", "orders.id AS ID"}}, want: "output names must be unique"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.query.buildMongoJoinPipeline("users", nil, nil, reflect.TypeFor[joinedRow]())
			if err == nil || test.query.Error == nil || test.query.HTTPCode != http.StatusBadRequest {
				t.Fatalf("invalid join result = (%v, %d, %v), want HTTP 400 error", err, test.query.HTTPCode, test.query.Error)
			}
			if !strings.Contains(test.query.Message, test.want) {
				t.Fatalf("message = %q, want substring %q", test.query.Message, test.want)
			}
		})
	}
}

func TestParseMongoJoinProjectionValidatesAndParsesAliases(t *testing.T) {
	qualified, output, err := parseMongoJoinProjection("orders.total AS order_total")
	if err != nil {
		t.Fatalf("parseMongoJoinProjection() error = %v", err)
	}
	if !reflect.DeepEqual(qualified, []string{"orders", "total"}) || output != "order_total" {
		t.Fatalf("parsed projection = (%#v, %q)", qualified, output)
	}

	for _, invalid := range []string{
		"", "*", "users.*", "name", "users.name AS", "users.name DESC",
		"users.name AS result.name", "users.$field",
	} {
		t.Run(invalid, func(t *testing.T) {
			if _, _, err := parseMongoJoinProjection(invalid); err == nil {
				t.Fatalf("parseMongoJoinProjection(%q) unexpectedly succeeded", invalid)
			}
		})
	}
}
