package orm

import (
	"errors"
	"reflect"
	"testing"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestLoadParentBelongsToCopiesRelatedValue(t *testing.T) {
	type related struct {
		ID   int
		Name string
	}
	type parent struct {
		Related *related
		Value   related
	}

	source := reflect.ValueOf(&related{ID: 4, Name: "team"})
	parentValues := []reflect.Value{
		reflect.ValueOf(&parent{}).Elem(),
		reflect.ValueOf(&parent{}).Elem(),
	}
	core := &mongoBelongsTo{fieldIndex: 0, relatedType: reflect.TypeFor[related](), fieldIsPtr: true}
	loadParentBelongsTo(map[string]reflect.Value{"4": source}, "4", parentValues, core)

	first := parentValues[0].Field(0).Interface().(*related)
	second := parentValues[1].Field(0).Interface().(*related)
	if first == second || *first != *second || first.Name != "team" {
		t.Fatalf("belongs-to values are not independent copies: first=%#v second=%#v", first, second)
	}

	valueParent := reflect.ValueOf(&parent{}).Elem()
	core.fieldIndex, core.fieldIsPtr = 1, false
	loadParentBelongsTo(map[string]reflect.Value{"4": source}, "4", []reflect.Value{valueParent}, core)
	if got := valueParent.Interface().(parent).Value; got != (related{ID: 4, Name: "team"}) {
		t.Fatalf("belongs-to value field = %#v", got)
	}

	missing := reflect.ValueOf(&parent{}).Elem()
	loadParentBelongsTo(nil, "missing", []reflect.Value{missing}, core)
	if got := missing.Interface().(parent); got != (parent{}) {
		t.Fatalf("missing belongs-to relation changed parent: %#v", got)
	}
}

func TestBuildSelectedColumnsByTableAndValidatePrefixes(t *testing.T) {
	selected := buildSelectedColumnsByTable([]string{"id, name", "orders.status", "teams.*", " "}, "users")
	if !selected.isSelect {
		t.Fatal("wildcard select was not detected")
	}
	if want := map[string][]string{"users": {"id", "name"}, "orders": {"status"}}; !reflect.DeepEqual(selected.table, want) {
		t.Fatalf("selected columns = %#v, want %#v", selected.table, want)
	}

	plan := &scyllaPreloadPlan{
		hasMany: []*scyllaHasMany{{rel: &hasManyRelation{table: "orders"}}},
		belongsTo: []*scyllaBelongsTo{{
			core: &mongoBelongsTo{collection: "teams"},
		}},
	}
	query := &ORM{}
	for _, table := range []string{"users", "orders", "teams"} {
		if err := query.validateSelectTablePrefixes(map[string][]string{table: {"id"}}, "users", plan); err != nil {
			t.Errorf("validateSelectTablePrefixes(%q) error = %v", table, err)
		}
	}
	if err := query.validateSelectTablePrefixes(nil, "users", plan); err != nil {
		t.Fatalf("empty selected columns: %v", err)
	}
	if err := query.validateSelectTablePrefixes(map[string][]string{"unknown": {"id"}}, "users", plan); err == nil {
		t.Fatal("validateSelectTablePrefixes() accepted an unknown table")
	}
}

func TestValidateUpsertConditionAndWriteErrorHelpers(t *testing.T) {
	if err := (&ORM{}).validateUpsertCondition("status = ?"); err != nil {
		t.Fatalf("valid upsert condition rejected: %v", err)
	}
	query := &ORM{}
	if err := query.validateUpsertCondition("status = ? OR"); err == nil {
		t.Fatal("validateUpsertCondition() accepted an invalid condition")
	}
	if query.Error == nil {
		t.Fatal("invalid upsert condition did not set ORM.Error")
	}

	backendErr := errors.New("backend failure")
	for _, operation := range []func(*ORM, string, error) error{
		(*ORM).setMongoWriteError,
		(*ORM).setScyllaWriteError,
	} {
		query := &ORM{}
		got := operation(query, "Create", backendErr)
		if !errors.Is(got, backendErr) || !errors.Is(query.Error, backendErr) {
			t.Fatalf("write error helper = %v, ORM.Error = %v", got, query.Error)
		}
	}
}

func TestBuildMongoQuerySelectsDistinctAndFindPaths(t *testing.T) {
	req := &structBuildMongoQuery{
		filter:    bson.M{"status": "active"},
		preload:   &mongoPreloadPlan{},
		tableName: "users",
		sort:      bson.D{{Key: "name", Value: 1}},
		isSlice:   true,
	}

	distinct := &ORM{IsDistinct: true, DistinctCols: []string{"status"}}
	pipeline, opts, err := distinct.buildMongoQuery(req)
	if err != nil || opts != nil || len(pipeline) == 0 {
		t.Fatalf("distinct buildMongoQuery() = (%v, %v, %v)", pipeline, opts, err)
	}

	ordinary := &ORM{}
	pipeline, opts, err = ordinary.buildMongoQuery(req)
	if err != nil || len(pipeline) != 0 || opts == nil {
		t.Fatalf("find buildMongoQuery() = (%v, %v, %v)", pipeline, opts, err)
	}

	type mongoQueryRecord struct {
		ID   string `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
	req.structInfo = &structInfo{structType: reflect.TypeOf(mongoQueryRecord{})}
	req.grouped = true
	groupedQuery := &ORM{GroupByClauses: []string{"name"}, SelectedCols: []string{"name"}}
	pipeline, opts, err = groupedQuery.buildMongoQuery(req)
	if err != nil || opts != nil || pipeline == nil {
		t.Fatalf("grouped buildMongoQuery() = (%v, %v, %v)", pipeline, opts, err)
	}

	req.grouped = false
	req.joining = true
	type joinedMongoQueryRecord struct {
		UserID     string `gorest:"user_id"`
		OrderTotal int    `gorest:"order_total"`
	}
	req.structInfo = &structInfo{structType: reflect.TypeOf(joinedMongoQueryRecord{})}
	joinedQuery := &ORM{
		JoinClauses:  []string{"LEFT OUTER JOIN orders ON users.id = orders.user_id"},
		SelectedCols: []string{"users.id AS user_id", "orders.total AS order_total"},
	}
	pipeline, opts, err = joinedQuery.buildMongoQuery(req)
	if err != nil || opts != nil || pipeline == nil {
		t.Fatalf("joined buildMongoQuery() = (%v, %v, %v)", pipeline, opts, err)
	}
}

func TestScyllaRestrictionsAndScanRow(t *testing.T) {
	query := &ORM{}
	for _, configure := range []func(*ORM){
		func(query *ORM) { query.HavingClauses = []string{"count > ?"} },
		func(query *ORM) { query.JoinClauses = []string{"JOIN teams"} },
		func(query *ORM) { query.OffsetVal = 1 },
	} {
		query := &ORM{}
		configure(query)
		if err := query.bannedJoinAndOffsetFindScylla(); err == nil {
			t.Error("bannedJoinAndOffsetFindScylla() accepted an unsupported query clause")
		}
	}
	if err := query.bannedJoinAndOffsetFindScylla(); err != nil {
		t.Fatalf("bannedJoinAndOffsetFindScylla() rejected a supported query: %v", err)
	}

	type record struct {
		ID   int     `gorest:"id"`
		Name *string `gorest:"name"`
	}
	lookup := buildStructFieldMapByTag(reflect.TypeFor[record]())
	row := scyllaScanRow(
		[]gocql.ColumnInfo{{Name: "id"}, {Name: "name"}, {Name: "unknown"}},
		reflect.TypeFor[record](),
		lookup,
	)
	if _, ok := row["id"]; ok {
		t.Fatalf("non-pointer field unexpectedly allocated: %#v", row)
	}
	name, ok := row["name"]
	if !ok || reflect.TypeOf(name).Kind() != reflect.Pointer {
		t.Fatalf("pointer field scan target = %#v", row)
	}
	if len(row) != 1 {
		t.Fatalf("scan target included unmappable columns: %#v", row)
	}
}

func TestValidateSelectedColumnsAndRequiredFieldError(t *testing.T) {
	request := &strucTextractColumnMetaData{normalizedSelectedCols: []string{"id", "name"}}
	if _, found := validateSelectedColsExtractColumnMetaData(request, "name"); !found {
		t.Fatal("selected column was not found")
	}
	if _, found := validateSelectedColsExtractColumnMetaData(request, "email"); found {
		t.Fatal("unselected column was found")
	}

	query := &ORM{}
	err := query.requiredFieldError("display_name, required", 2, "Create")
	if err == nil || query.HTTPCode != 400 || query.Message != `Field "display_name" is required and must not be empty (record 2).` {
		t.Fatalf("required field error = (%v, %d, %q)", err, query.HTTPCode, query.Message)
	}
}
