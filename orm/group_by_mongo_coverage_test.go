package orm

import (
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type mongoGroupFixture struct {
	Status string  `gorest:"status"`
	Amount float64 `gorest:"amount"`
}

func TestBuildMongoGroupPipelineProjectsGroupAndAggregateFields(t *testing.T) {
	query := &ORM{
		GroupByClauses: []string{"status"},
		SelectedCols:   []string{"status", "COUNT(*) AS total", "SUM(amount) AS gross"},
		HavingClauses:  []string{"total > ?"},
		HavingArgs:     []any{1},
		OffsetVal:      2,
		LimitVal:       5,
	}
	filter := bson.M{"active": true}
	sort := bson.D{{Key: "total", Value: -1}}

	pipeline, err := query.buildMongoGroupPipeline(
		"orders",
		filter,
		sort,
		reflect.TypeFor[mongoGroupFixture](),
	)
	if err != nil {
		t.Fatalf("buildMongoGroupPipeline() error = %v", err)
	}
	if len(pipeline) != 7 {
		t.Fatalf("pipeline has %d stages, want match, group, having, project, sort, skip, limit stages", len(pipeline))
	}
	if got := pipeline[0][0].Key; got != dolarMatch {
		t.Fatalf("first stage = %q, want $match", got)
	}
	group, ok := pipeline[1][0].Value.(bson.M)
	if !ok {
		t.Fatalf("group stage type = %T, want bson.M", pipeline[1][0].Value)
	}
	id, ok := group[_id].(bson.M)
	if !ok || id["status"] != "$status" {
		t.Fatalf("group ID = %#v, want status field", group[_id])
	}
	if _, ok := group["__gorest_agg_0"]; !ok {
		t.Fatalf("count accumulator missing from group stage: %#v", group)
	}
	if _, ok := group["__gorest_agg_1"]; !ok {
		t.Fatalf("sum accumulator missing from group stage: %#v", group)
	}
	if got := pipeline[2][0].Key; got != "$match" {
		t.Fatalf("post-group having stage = %q, want $match", got)
	}
	project, ok := pipeline[3][0].Value.(bson.M)
	if !ok {
		t.Fatalf("project stage type = %T, want bson.M", pipeline[3][0].Value)
	}
	if project["status"] != "$_id.status" || project["total"] != "$__gorest_agg_0" ||
		project["gross"] != "$__gorest_agg_1" {
		t.Fatalf("projection = %#v", project)
	}
	if pipeline[4][0].Key != dolarSort {
		t.Fatalf("sort stage = %#v", pipeline[4])
	}
	if pipeline[5][0].Key != dolarSkip {
		t.Fatalf("skip stage = %#v", pipeline[5])
	}
	if pipeline[6][0].Key != dolarLimit || pipeline[6][0].Value != int64(5) {
		t.Fatalf("limit stage = %#v", pipeline[6])
	}
}

func TestBuildMongoSingleGroupPipelineAppliesDefaultLimit(t *testing.T) {
	query := &ORM{GroupByClauses: []string{"status"}, SelectedCols: []string{"status"}}
	pipeline, err := query.buildMongoSingleGroupPipeline("orders", nil, nil, reflect.TypeFor[mongoGroupFixture]())
	if err != nil {
		t.Fatalf("buildMongoSingleGroupPipeline() error = %v", err)
	}
	if got := pipeline[len(pipeline)-1]; len(got) != 1 || got[0].Key != dolarLimit || got[0].Value != int64(1) {
		t.Fatalf("single-result final stage = %#v, want default limit 1", got)
	}

	explicit := mongo.Pipeline{bson.D{{Key: dolarMatch, Value: bson.M{}}}}
	if got := applyMongoSingleResultDefaultLimit(explicit, 4); len(got) != 1 {
		t.Fatalf("explicit single-result limit changed pipeline: %#v", got)
	}
	if got := applyMongoSingleResultDefaultLimit(explicit, 0); len(got) != 2 || got[1][0].Value != int64(1) {
		t.Fatalf("default single-result limit pipeline = %#v", got)
	}
}

func TestBuildMongoGroupPipelineRejectsUnsupportedSelectionsAndHaving(t *testing.T) {
	fixtureType := reflect.TypeFor[mongoGroupFixture]()
	tests := []struct {
		name  string
		query *ORM
		want  string
	}{
		{name: "preload", query: &ORM{GroupByClauses: []string{"status"}, Preloads: map[string][]any{"Owner": nil}}, want: "preload"},
		{name: "missing group columns", query: &ORM{}, want: "group columns"},
		{name: "unknown group column", query: &ORM{GroupByClauses: []string{"missing"}}, want: "unknown MongoDB group column"},
		{name: "wildcard select", query: &ORM{GroupByClauses: []string{"status"}, SelectedCols: []string{"*"}}, want: "wildcard projection"},
		{name: "ungrouped select", query: &ORM{GroupByClauses: []string{"status"}, SelectedCols: []string{"amount"}}, want: "ungrouped selected field"},
		{name: "missing aggregate alias", query: &ORM{GroupByClauses: []string{"status"}, SelectedCols: []string{"COUNT(*)"}}, want: "invalid aggregate selection"},
		{name: "unknown aggregate field", query: &ORM{GroupByClauses: []string{"status"}, SelectedCols: []string{"SUM(missing) AS total"}}, want: "invalid aggregate selection"},
		{
			name: "having argument mismatch",
			query: &ORM{
				GroupByClauses: []string{"status"},
				HavingClauses:  []string{"COUNT(*) > ?"},
			},
			want: "unsupported having expression",
		},
		{
			name: "unknown having field",
			query: &ORM{
				GroupByClauses: []string{"status"},
				HavingClauses:  []string{"amount > ?"},
				HavingArgs:     []any{1},
			},
			want: "unknown having field",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.query.buildMongoGroupPipeline("orders", nil, nil, fixtureType)
			if err == nil {
				t.Fatal("unsupported grouped query unexpectedly succeeded")
			}
			if test.query.Error == nil {
				t.Fatal("grouped query error was not stored on ORM")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want it to contain %q", err, test.want)
			}
		})
	}
}

func TestAddMongoGroupAccumulatorsAndSplitColumns(t *testing.T) {
	fixtureType := reflect.TypeFor[mongoGroupFixture]()
	known := map[string]string{"status": "Status", "amount": "Amount"}
	group := bson.M{}
	project := bson.M{}
	accumulators := map[string]mongoGroupAccumulator{}
	aliases := map[string]string{}

	for _, test := range []struct {
		expression string
		alias      string
	}{
		{expression: "COUNT(*) AS total", alias: "total"},
		{expression: "SUM(amount) AS sum_amount", alias: "sum_amount"},
		{expression: "AVG(amount) AS avg_amount", alias: "avg_amount"},
		{expression: "MIN(amount) AS min_amount", alias: "min_amount"},
		{expression: "MAX(amount) AS max_amount", alias: "max_amount"},
	} {
		req := &structAddAccumulator{
			structType: fixtureType, knownColumns: known, accumulators: accumulators,
			groupDoc: group, aliases: aliases, project: project,
			expression: test.expression, alias: test.alias, collection: "orders",
		}
		if _, err := addAccumulator(req); err != nil {
			t.Errorf("addAccumulator(%q) error = %v", test.expression, err)
		}
	}
	if len(accumulators) != 5 || len(group) != 5 || len(aliases) != 5 || len(project) != 5 {
		t.Fatalf("aggregate state sizes = accumulators %d, group %d, aliases %d, project %d",
			len(accumulators), len(group), len(aliases), len(project))
	}

	acc := mongoGroupAccumulator{key: "existing", operation: "SUM", field: "Amount"}
	accumulators["sum(amount)"] = acc
	req := &structAddAccumulator{
		structType: fixtureType, knownColumns: known, accumulators: accumulators,
		groupDoc: group, aliases: aliases, project: project,
		expression: "SUM(Amount) AS repeated", alias: "repeated", collection: "orders",
	}
	if got, err := addAccumulator(req); err != nil || got.key != acc.key {
		t.Fatalf("reused accumulator = %#v, %v; want key %q", got, err, acc.key)
	}

	if got := splitMongoSelectColumns([]string{" status, ", "COUNT(*) AS total", ""}); !reflect.DeepEqual(got, []string{"status", "COUNT(*) AS total"}) {
		t.Fatalf("splitMongoSelectColumns() = %#v", got)
	}
}

func TestMongoHavingTargetResolvesAliasesAndRejectsUnknownColumns(t *testing.T) {
	req := &structPipelinepOnbuildMongoGroupPipeline{
		collection:   "orders",
		knownColumns: map[string]string{"status": "Status"},
		aliases:      map[string]string{"total": "__gorest_agg_0"},
		groupKeys:    map[string]string{"status": "Status"},
		accumulators: map[string]mongoGroupAccumulator{},
		groupDoc:     bson.M{},
		project:      bson.M{},
		structType:   reflect.TypeFor[mongoGroupFixture](),
	}
	if target, aggregate, err := mongoHavingTarget("total", 0, req); err != nil || aggregate || target != "__gorest_agg_0" {
		t.Fatalf("aggregate alias target = %q, %t, %v", target, aggregate, err)
	}
	if target, aggregate, err := mongoHavingTarget("orders.status", 0, req); err != nil || aggregate || target != "_id.Status" {
		t.Fatalf("group key target = %q, %t, %v", target, aggregate, err)
	}
	if target, aggregate, err := mongoHavingTarget("unknown", 0, req); err == nil || aggregate || target != "" {
		t.Fatalf("unknown target = %q, %t, %v; want error", target, aggregate, err)
	}

	target, err := mongoAggregateHavingTarget("COUNT(*)", 0, "", req)
	if err != nil || target == "" {
		t.Fatalf("unaliased aggregate target = %q, %v", target, err)
	}
}
