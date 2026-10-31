package orm

import (
	"net"
	"reflect"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"gopkg.in/inf.v0"
)

func TestScyllaPreloadPlanHelpers(t *testing.T) {
	type child struct {
		ID     int    `gorest:"id, primary_key"`
		UserID int    `gorest:"user_id"`
		Status string `gorest:"status"`
	}
	type team struct {
		ID   string `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
	type parent struct {
		ID       int     `gorest:"id, primary_key"`
		TeamID   string  `gorest:"team_id"`
		Team     *team   `gorest:"-" references:"ID"`
		Children []child `gorest:"-" references:"UserID" orderby:"ID DESC"`
	}

	if !(*scyllaPreloadPlan)(nil).isEmpty() || !(&scyllaPreloadPlan{}).isEmpty() {
		t.Fatal("empty Scylla preload plan was reported as non-empty")
	}

	query := &ORM{Preloads: map[string][]any{
		"Team":     nil,
		"Children": {"children.status = ?", "active"},
	}}
	plan, err := query.planScyllaPreloads(reflect.TypeFor[parent](), map[string][]string{
		"teams": {"name"},
	})
	if err != nil {
		t.Fatalf("planScyllaPreloads() error = %v", err)
	}
	if plan.isEmpty() || len(plan.belongsTo) != 1 || len(plan.hasMany) != 1 {
		t.Fatalf("Scylla preload plan = %#v", plan)
	}
	if got, want := plan.requiredParentColumns(), []string{"id", "team_id"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required parent columns = %v, want %v", got, want)
	}
	if got, want := plan.belongsTo[0].selectCols, []string{"name", "id"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("belongs-to select columns = %v, want %v", got, want)
	}
	if got := plan.hasMany[0].cond; got.clause != "status = ?" || !reflect.DeepEqual(got.args, []any{"active"}) {
		t.Fatalf("has-many condition = %#v", got)
	}

	if _, err := (&ORM{Preloads: map[string][]any{"Missing": nil}}).planScyllaPreloads(reflect.TypeFor[parent](), nil); err == nil {
		t.Fatal("planScyllaPreloads() accepted an unknown relation")
	}
	if _, err := (&ORM{Preloads: map[string][]any{"Children": {"status OR id = ?", "active", 1}}}).planScyllaPreloads(reflect.TypeFor[parent](), nil); err == nil {
		t.Fatal("planScyllaPreloads() accepted an unsupported condition")
	}

	if got := appendMissingColumns([]string{" Name ", "id"}, []string{"name", "ID", "team_id"}); !reflect.DeepEqual(got, []string{" Name ", "id", "team_id"}) {
		t.Fatalf("appendMissingColumns() = %v", got)
	}
}

func TestParseScyllaPreloadCondition(t *testing.T) {
	tests := []struct {
		name    string
		args    []any
		want    scyllaCond
		wantErr bool
	}{
		{name: "no condition"},
		{name: "valid and condition", args: []any{" orders.status = ? AND count >= ? ", "open", 2}, want: scyllaCond{clause: "status = ? AND count >= ?", args: []any{"open", 2}}},
		{name: "condition must be string", args: []any{42}, wantErr: true},
		{name: "condition cannot be empty", args: []any{"  "}, wantErr: true},
		{name: "or is not supported", args: []any{"status = ? OR count = ?", "open", 2}, wantErr: true},
		{name: "is null is not supported", args: []any{"deleted_at IS NULL"}, wantErr: true},
		{name: "is not null is not supported", args: []any{"deleted_at IS NOT NULL"}, wantErr: true},
		{name: "parentheses are not supported", args: []any{"(status = ?)", "open"}, wantErr: true},
		{name: "placeholder argument mismatch", args: []any{"status = ?"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseScyllaPreloadCondition("Orders", test.args, "orders")
			if (err != nil) != test.wantErr {
				t.Fatalf("parseScyllaPreloadCondition() error = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil && !reflect.DeepEqual(got, test.want) {
				t.Fatalf("condition = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestBuildChildSortKeys(t *testing.T) {
	type child struct {
		ID    int    `gorest:"id"`
		Name  string `gorest:"name"`
		Score int    `gorest:"score"`
	}

	got, err := buildChildSortKeys(" name DESC, ID ", reflect.TypeFor[child]())
	if err != nil {
		t.Fatalf("buildChildSortKeys() error = %v", err)
	}
	if want := []childSortKey{{fieldIdx: 1, desc: true}, {fieldIdx: 0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sort keys = %#v, want %#v", got, want)
	}
	if got, err := buildChildSortKeys(" ", reflect.TypeFor[child]()); err != nil || got != nil {
		t.Fatalf("empty sort keys = %#v, %v", got, err)
	}

	for _, raw := range []string{"unknown", "name SIDEWAYS", "name DESC extra", ", name"} {
		if _, err := buildChildSortKeys(raw, reflect.TypeFor[child]()); err == nil {
			t.Errorf("buildChildSortKeys(%q) accepted invalid input", raw)
		}
	}
}

func TestScyllaSortComparisonsAndChildSorting(t *testing.T) {
	earlier := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Hour)
	for _, test := range []struct {
		name string
		a    any
		b    any
		want int
	}{
		{name: "time before", a: earlier, b: later, want: -1},
		{name: "time equal", a: earlier, b: earlier},
		{name: "time after", a: later, b: earlier, want: 1},
		{name: "string", a: "a", b: "b", want: -1},
		{name: "integer", a: 2, b: 1, want: 1},
		{name: "different kinds", a: "1", b: 1},
		{name: "nil before value", a: (*int)(nil), b: 1, want: -1},
		{name: "unknown kinds equal", a: []int{1}, b: []int{2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := compareSortValues(reflect.ValueOf(test.a), reflect.ValueOf(test.b)); got != test.want {
				t.Fatalf("compareSortValues() = %d, want %d", got, test.want)
			}
		})
	}

	type child struct {
		Name string
		Rank int
	}
	items := []reflect.Value{
		reflect.ValueOf(child{Name: "B", Rank: 1}),
		reflect.ValueOf(child{Name: "A", Rank: 1}),
		reflect.ValueOf(child{Name: "A", Rank: 2}),
	}
	sortChildren(items, []childSortKey{{fieldIdx: 1, desc: true}, {fieldIdx: 0}})
	if got, want := []child{items[0].Interface().(child), items[1].Interface().(child), items[2].Interface().(child)}, []child{
		{Name: "A", Rank: 2},
		{Name: "A", Rank: 1},
		{Name: "B", Rank: 1},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted children = %#v, want %#v", got, want)
	}
}

func TestMapScyllaRowAndBuildInQuery(t *testing.T) {
	type item struct {
		ID   int    `gorest:"id"`
		Name string `gorest:"name"`
	}
	query := &ORM{}
	lookup := buildStructFieldMapByTag(reflect.TypeFor[item]())

	mapped, err := query.mapScyllaRow(map[string]any{"ID": int32(7), "unknown": "ignored"}, reflect.TypeFor[item](), lookup)
	if err != nil || mapped.Elem().Interface().(item) != (item{ID: 7}) {
		t.Fatalf("mapScyllaRow() = %#v, %v", mapped, err)
	}
	if _, err := query.mapScyllaRow(map[string]any{"id": "not-an-integer"}, reflect.TypeFor[item](), lookup); err == nil {
		t.Fatal("mapScyllaRow() accepted an incompatible field value")
	}

	query.AllowFilteringFlag = true
	cql, args := query.buildScyllaInQuery("orders", "user_id", "id, status", []any{1, 2}, scyllaCond{
		clause: "status = ?",
		args:   []any{"active"},
	})
	if want := "SELECT id, status FROM orders WHERE user_id IN (?,?) AND status = ? ALLOW FILTERING"; cql != want {
		t.Fatalf("CQL = %q, want %q", cql, want)
	}
	if want := []any{1, 2, "active"}; !reflect.DeepEqual(args, want) {
		t.Fatalf("CQL args = %v, want %v", args, want)
	}
	cql, _ = (&ORM{}).buildScyllaInQuery("users", "id", "", []any{1}, scyllaCond{})
	if want := "SELECT * FROM users WHERE id IN (?)"; cql != want {
		t.Fatalf("default CQL = %q, want %q", cql, want)
	}
}

func TestNormalizeScyllaValues(t *testing.T) {
	now := time.Now()
	decimal := inf.NewDec(12345, 2)
	uuid := gocql.UUID{1}
	tests := []struct {
		name string
		in   any
		want any
	}{
		{name: "nil UUID", in: gocql.UUID{}, want: nil},
		{name: "UUID", in: uuid, want: uuid.String()},
		{name: "zero time", in: time.Time{}, want: nil},
		{name: "time normalized to UTC", in: now, want: now.UTC()},
		{name: "IP", in: net.IP{127, 0, 0, 1}, want: "127.0.0.1"},
		{name: "decimal", in: decimal, want: 123.45},
		{name: "nil decimal", in: (*inf.Dec)(nil), want: float64(0)},
		{name: "default value", in: int32(3), want: int32(3)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeScyllaValue(test.in)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("normalizeScyllaValue() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestLoadParentScyllaHasManyAndEmptyPlan(t *testing.T) {
	type child struct {
		ID int
	}
	type parent struct {
		Children []child
	}

	value := reflect.ValueOf(&parent{}).Elem()
	loadParentScyllaHasMany(
		map[string][]reflect.Value{"1": {reflect.ValueOf(child{ID: 2})}},
		"1",
		[]reflect.Value{value},
		&hasManyRelation{fieldIndex: 0, sliceType: reflect.TypeFor[[]child]()},
	)
	if got, want := value.Interface().(parent).Children, []child{{ID: 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded child rows = %#v, want %#v", got, want)
	}

	if err := (&ORM{}).loadScyllaPreloads(t.Context(), &scyllaPreloadPlan{}, nil); err != nil {
		t.Fatalf("loadScyllaPreloads() with empty plan = %v", err)
	}
}

func TestScyllaCreateStatementAndResultBuilders(t *testing.T) {
	type record struct {
		ID   int
		Name string
	}
	rows := []reflect.Value{
		reflect.ValueOf(record{ID: 1, Name: "Ada"}),
		reflect.ValueOf(record{ID: 2, Name: "Lin"}),
	}
	meta := columnMetaData{
		columns:     []string{"id", "name"},
		columnIndex: []int{0, 1},
	}
	query := &ORM{}
	buildRow, err := query.buildUpsertCreateScylla(meta, "users")
	if err != nil {
		t.Fatalf("buildUpsertCreateScylla() error = %v", err)
	}

	statement, args := buildRow(rows[0])
	if want := "INSERT INTO users (id, name) VALUES (?, ?)"; statement != want {
		t.Fatalf("single-row CQL = %q, want %q", statement, want)
	}
	if want := []any{1, "Ada"}; !reflect.DeepEqual(args, want) {
		t.Fatalf("single-row args = %#v, want %#v", args, want)
	}

	batch := buildQueryStatement(rows, buildRow)
	if want := "BEGIN BATCH\nINSERT INTO users (id, name) VALUES (?, ?);\nINSERT INTO users (id, name) VALUES (?, ?);\nAPPLY BATCH;"; batch.queryStr != want {
		t.Fatalf("batch CQL = %q, want %q", batch.queryStr, want)
	}
	if want := []any{1, "Ada", 2, "Lin"}; !reflect.DeepEqual(batch.valueArgs, want) {
		t.Fatalf("batch args = %#v, want %#v", batch.valueArgs, want)
	}

	query.extractResultCreateScylla(rows, []int{0})
	if query.RowsAffected != 2 || query.LastInsertId != "1, 2" {
		t.Fatalf("create result metadata = (%d, %q)", query.RowsAffected, query.LastInsertId)
	}
	result, ok := query.Result.(map[string]any)
	if !ok || result[lastInsertID] != "1, 2" || result[rowsAffected] != int64(2) {
		t.Fatalf("create result = %#v", query.Result)
	}

	query.IsUpsert = true
	query.extractResultCreateScylla(rows, nil)
	result, ok = query.Result.(map[string]any)
	if !ok || query.LastInsertId != "" || result[upsertedID] != "" || result[rowsAffected] != int64(2) {
		t.Fatalf("upsert result = %#v, LastInsertId = %q", query.Result, query.LastInsertId)
	}
}

func TestScyllaPreloadLoadersHandleMissingDatabase(t *testing.T) {
	type child struct {
		ID     int `gorest:"id"`
		UserID int `gorest:"user_id"`
	}
	type team struct {
		ID int `gorest:"id"`
	}
	type parent struct {
		ID     int `gorest:"id"`
		TeamID int `gorest:"team_id"`
		Orders []child
		Team   *team
	}

	ctx := t.Context()
	parentValue := reflect.ValueOf(parent{ID: 1, TeamID: 2})
	parents := []reflect.Value{parentValue}
	hasMany := &scyllaHasMany{
		rel: &hasManyRelation{
			fieldIndex:   2,
			sliceType:    reflect.TypeFor[[]child](),
			elemType:     reflect.TypeFor[child](),
			parentKeyIdx: 0,
			childFKIdx:   1,
			childFKCol:   "user_id",
			table:        "orders",
		},
	}
	belongsTo := &scyllaBelongsTo{
		core: &mongoBelongsTo{
			relatedType: reflect.TypeFor[team](),
			collection:  "teams",
			targetCol:   "id",
			targetIdx:   0,
			parentFKIdx: 1,
			fieldIndex:  3,
		},
	}

	query := &ORM{}
	if err := query.eachScyllaRow(ctx, "SELECT id FROM users", nil, reflect.TypeFor[parent](), nil, func(map[string]any) error {
		return nil
	}); err == nil {
		t.Fatal("eachScyllaRow() did not report a missing database service")
	}
	if err := query.loadScyllaHasMany(ctx, parents, hasMany); err == nil {
		t.Fatal("loadScyllaHasMany() did not propagate unavailable database error")
	}
	query = &ORM{}
	if err := query.loadScyllaBelongsTo(ctx, parents, belongsTo); err == nil {
		t.Fatal("loadScyllaBelongsTo() did not propagate unavailable database error")
	}

	query = &ORM{}
	if err := query.loadScyllaPreloads(ctx, &scyllaPreloadPlan{hasMany: []*scyllaHasMany{hasMany}}, parents); err == nil {
		t.Fatal("loadScyllaPreloads() did not propagate has-many loader error")
	}
	query = &ORM{}
	if err := query.loadScyllaPreloads(ctx, &scyllaPreloadPlan{belongsTo: []*scyllaBelongsTo{belongsTo}}, parents); err == nil {
		t.Fatal("loadScyllaPreloads() did not propagate belongs-to loader error")
	}

	query = &ORM{}
	if err := query.loadQueryScyllaHasMany(&structLoadQueryScyllaHasMany{
		ctx:         ctx,
		rel:         hasMany.rel,
		hashMany:    hasMany,
		childLookup: buildStructFieldMapByTag(reflect.TypeFor[child]()),
		grouped:     map[string][]reflect.Value{},
		keys:        []any{1},
	}); err == nil {
		t.Fatal("loadQueryScyllaHasMany() did not propagate unavailable database error")
	}

	query = &ORM{}
	if err := query.loadQueryScyllaBelongsTo(&structLoadQueryScyllaBelongsTo{
		ctx:           ctx,
		core:          belongsTo.core,
		belong:        belongsTo,
		found:         map[string]reflect.Value{},
		relatedLookup: buildStructFieldMapByTag(reflect.TypeFor[team]()),
		keys:          []any{2},
	}); err == nil {
		t.Fatal("loadQueryScyllaBelongsTo() did not propagate unavailable database error")
	}
}

func TestLoadFindScyllaPreloadDataSkipsEmptyPlan(t *testing.T) {
	value := reflect.ValueOf(&struct{ ID int }{ID: 1}).Elem()
	if err := (&ORM{}).loadFindScyllaPreloadData(
		t.Context(),
		&scyllaPreloadPlan{},
		&value,
		false,
	); err != nil {
		t.Fatalf("loadFindScyllaPreloadData(empty) = %v", err)
	}
}
