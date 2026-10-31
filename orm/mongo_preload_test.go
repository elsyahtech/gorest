package orm

import (
	"context"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestExecParseMongoConditionRejectsInvalidArguments(t *testing.T) {
	orm := &ORM{}
	for _, test := range []struct {
		name     string
		args     []any
		argIndex *int
		count    int
	}{
		{name: "nil index", args: []any{1}, argIndex: nil, count: 1},
		{name: "negative index", args: []any{1}, argIndex: intPointer(-1), count: 1},
		{name: "insufficient arguments", args: []any{1}, argIndex: intPointer(0), count: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := orm.execParseMongoCondition(test.args, test.argIndex, test.count, "=", "id", nil); err == nil {
				t.Fatal("execParseMongoCondition() error = nil, want invalid argument error")
			}
		})
	}
}

func TestExecParseMongoConditionOperators(t *testing.T) {
	orm := &ORM{}
	tests := []struct {
		bundle string
		arg    any
	}{
		{bundle: "=", arg: "one"}, {bundle: "!=", arg: "one"}, {bundle: "<>", arg: "one"},
		{bundle: ">", arg: 1}, {bundle: ">=", arg: 1}, {bundle: "<", arg: 1}, {bundle: "<=", arg: 1},
		{bundle: "LIKE", arg: "ali%"}, {bundle: "NOT LIKE", arg: "ali%"},
		{bundle: "IN", arg: []any{"a", "b"}}, {bundle: "NOT IN", arg: []any{"a", "b"}},
	}
	for _, test := range tests {
		t.Run(test.bundle, func(t *testing.T) {
			args := []any{test.arg}
			index := 0
			count := 1
			if test.bundle == "IN" || test.bundle == "NOT IN" {
				args = test.arg.([]any)
				count = 2
			}
			if _, err := orm.execParseMongoCondition(args, &index, count, test.bundle, "name", nil); err != nil {
				t.Fatalf("execParseMongoCondition(%q) error = %v", test.bundle, err)
			}
		})
	}

	index := 0
	if _, err := orm.execParseMongoCondition([]any{1}, &index, 1, "LIKE", "name", nil); err == nil {
		t.Fatal("LIKE accepted an integer argument")
	}
}

func TestValMongoPrimaryKeyColumnsHandlesInvalidValues(t *testing.T) {
	invalid := reflect.Value{}
	if valMongoPrimaryKeyColumns(nil) != invalid || valMongoPrimaryKeyColumns(&invalid).IsValid() {
		t.Fatal("invalid primary-key value was accepted")
	}

	empty := reflect.ValueOf([]struct{ ID string }{})
	if valMongoPrimaryKeyColumns(&empty).IsValid() {
		t.Fatal("empty slice was accepted as primary-key value")
	}

	pointer := reflect.ValueOf((*struct{ ID string })(nil))
	if valMongoPrimaryKeyColumns(&pointer).IsValid() {
		t.Fatal("nil pointer was accepted as primary-key value")
	}

	number := reflect.ValueOf(1)
	if valMongoPrimaryKeyColumns(&number).IsValid() {
		t.Fatal("scalar was accepted as primary-key struct")
	}
}

func intPointer(value int) *int { return &value }

func TestMongoPreloadPlanHelpers(t *testing.T) {
	empty := &mongoPreloadPlan{}
	if !empty.isEmpty() || !(*mongoPreloadPlan)(nil).isEmpty() {
		t.Fatal("empty Mongo preload plan was reported as non-empty")
	}

	type department struct {
		ID   string `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
	type order struct {
		ID     string `gorest:"id, primary_key"`
		UserID int    `gorest:"user_ref"`
	}
	type user struct {
		ID           int         `gorest:"id, primary_key"`
		DepartmentID string      `gorest:"department_id"`
		Department   *department `gorest:"-" references:"ID"`
		Orders       []order     `gorest:"-" references:"UserID"`
	}

	query := &ORM{
		Preloads: map[string][]any{
			"Department": nil,
			"Orders":     nil,
		},
	}
	plan, err := query.planMongoPreloads(reflect.TypeFor[user]())
	if err != nil {
		t.Fatalf("planMongoPreloads() error = %v", err)
	}
	if plan.isEmpty() || len(plan.belongsTo) != 1 || len(plan.hasMany) != 1 {
		t.Fatalf("Mongo preload plan = %#v", plan)
	}
	if got, want := plan.requiredParentColumns(), []string{"id", "department_id"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required Mongo parent columns = %v, want %v", got, want)
	}

	if _, err := (&ORM{Preloads: map[string][]any{"Unknown": nil}}).planMongoPreloads(reflect.TypeFor[user]()); err == nil {
		t.Fatal("planMongoPreloads() accepted an unknown relation")
	}
}

func TestParseMongoPreloadFilterAndBuildFilter(t *testing.T) {
	query := &ORM{}
	filter, err := query.parseMongoPreloadFilter(
		"Orders",
		[]any{"orders.status = ? AND orders.count >= ?", "active", 2},
		"orders",
		nil,
	)
	if err != nil {
		t.Fatalf("parseMongoPreloadFilter() error = %v", err)
	}
	if !reflect.DeepEqual(filter, bson.M{"$and": []bson.M{
		bson.M{"status": "active"},
		bson.M{"count": bson.M{dolarGte: 2}},
	}}) {
		t.Fatalf("parsed Mongo preload filter = %#v", filter)
	}

	original := bson.M{"status": "active"}
	got, err := query.parseMongoPreloadFilter("Orders", []any{original}, "orders", nil)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("bson.M preload filter = %#v, %v", got, err)
	}
	if _, err := query.parseMongoPreloadFilter("Orders", []any{bson.D{{Key: "status", Value: "active"}}}, "orders", nil); err != nil {
		t.Fatalf("bson.D preload filter: %v", err)
	}

	for _, args := range [][]any{
		{""},
		{bson.M{"status": "active"}, "extra"},
		{bson.D{{Key: "status", Value: "active"}}, "extra"},
		{42},
	} {
		if _, err := query.parseMongoPreloadFilter("Orders", args, "orders", nil); err == nil {
			t.Errorf("parseMongoPreloadFilter() accepted invalid args %#v", args)
		}
	}

	if _, err := query.buildMongoFilter([]string{"status = ?"}, nil, "users", nil); err == nil {
		t.Fatal("buildMongoFilter() accepted a missing argument")
	}
	if _, err := query.buildMongoFilter([]string{"status = ?"}, []any{"active", "extra"}, "users", nil); err == nil {
		t.Fatal("buildMongoFilter() accepted an unused argument")
	}
	if _, err := query.buildMongoFilter(nil, []any{"extra"}, "users", nil); err == nil {
		t.Fatal("buildMongoFilter() accepted arguments without a clause")
	}
}

func TestMongoPreloadConditionAndFilterCompositionHelpers(t *testing.T) {
	query := &ORM{}
	index := 0
	submatch := mongoCondExpr.FindStringSubmatch("orders.age >= ?")
	field, bundle, count, err := query.buildFieldAndBundleInparseMongoCondition(
		submatch,
		"orders",
		"orders.age >= ?",
		&index,
		[]any{18},
	)
	if err != nil || field != "age" || bundle != ">=" || count != 1 {
		t.Fatalf("condition parse metadata = (%q, %q, %d, %v)", field, bundle, count, err)
	}

	index = 0
	parsed, err := query.parseMongoCondition("orders.age >= ?", []any{18}, &index, "orders", nil)
	if err != nil || !reflect.DeepEqual(parsed, bson.M{"age": bson.M{dolarGte: 18}}) {
		t.Fatalf("parseMongoCondition() = %#v, %v", parsed, err)
	}
	for _, condition := range []string{"", "age IS NULL"} {
		if _, err := query.parseMongoCondition(condition, nil, new(int), "orders", nil); err == nil {
			t.Errorf("parseMongoCondition(%q) accepted invalid syntax", condition)
		}
	}

	if _, _, _, err := query.buildFieldAndBundleInparseMongoCondition(
		submatch,
		"orders",
		"orders.age >= ?",
		new(int),
		nil,
	); err == nil {
		t.Fatal("condition parser accepted missing values")
	}
	if got := mongoKeyVariants("507f1f77bcf86cd799439011"); len(got) != 2 {
		t.Fatalf("ObjectID key variants = %#v, want string and ObjectID", got)
	}
	if got := mongoKeyVariants("not-an-object-id"); len(got) != 1 || got[0] != "not-an-object-id" {
		t.Fatalf("ordinary key variants = %#v", got)
	}

	base := bson.M{"user_id": 7}
	if got := andMongoFilter(base, nil); !reflect.DeepEqual(got, base) {
		t.Fatalf("filter without extra condition = %#v", got)
	}
	if got := andMongoFilter(base, bson.M{"status": "active"}); !reflect.DeepEqual(got, bson.M{
		"$and": []any{base, bson.M{"status": "active"}},
	}) {
		t.Fatalf("combined Mongo filter = %#v", got)
	}
}

func TestGroupMongoParentsAndMapMongoDoc(t *testing.T) {
	type parent struct {
		ID *int `gorest:"id"`
	}
	firstKey, secondKey := 4, 9
	parents := []reflect.Value{
		reflect.ValueOf(parent{ID: &firstKey}),
		reflect.ValueOf(parent{ID: &firstKey}),
		reflect.ValueOf(parent{}),
		reflect.ValueOf(parent{ID: &secondKey}),
	}
	grouped, keys := groupMongoParents(parents, 0)
	if !reflect.DeepEqual(keys, []any{4, 9}) || len(grouped["4"]) != 2 || len(grouped["9"]) != 1 {
		t.Fatalf("Mongo parents grouped by key = %#v / %#v", keys, grouped)
	}

	type user struct {
		ID   int    `gorest:"id"`
		Name string `gorest:"name"`
	}
	mapped, err := mapMongoDoc(
		bson.M{"id": int32(7), "name": "Ada", "unknown": "ignored"},
		reflect.TypeFor[user](),
		buildStructFieldMapByTag(reflect.TypeFor[user]()),
	)
	if err != nil {
		t.Fatalf("mapMongoDoc() error = %v", err)
	}
	if got := mapped.Elem().Interface().(user); got != (user{ID: 7, Name: "Ada"}) {
		t.Fatalf("mapped Mongo document = %#v", got)
	}
	if _, err := mapMongoDoc(
		bson.M{"id": "not-an-integer"},
		reflect.TypeFor[user](),
		buildStructFieldMapByTag(reflect.TypeFor[user]()),
	); err == nil {
		t.Fatal("mapMongoDoc() accepted a value that cannot be assigned")
	}
}

func TestMongoPreloadLoadersHandleEmptyKeysAndUnavailableDatabase(t *testing.T) {
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
		Team   *team
		Orders []child
	}

	ctx := context.Background()
	query := &ORM{}
	if err := query.loadMongoPreloads(ctx, &mongoPreloadPlan{}, nil); err != nil {
		t.Fatalf("loadMongoPreloads(empty) = %v", err)
	}
	if err := query.eachMongoDoc(ctx, "orders", bson.M{}, nil, func(bson.M) error { return nil }); err == nil {
		t.Fatal("eachMongoDoc() did not report a missing database service")
	}

	parentValue := reflect.ValueOf(&parent{ID: 1, TeamID: 2}).Elem()
	parents := []reflect.Value{parentValue}
	hasMany := &mongoHasMany{rel: &hasManyRelation{
		fieldIndex:   3,
		sliceType:    reflect.TypeFor[[]child](),
		elemType:     reflect.TypeFor[child](),
		parentKeyIdx: 0,
		childFKIdx:   1,
		childFKCol:   "user_id",
		table:        "orders",
	}}
	belongsTo := &mongoBelongsTo{
		relatedType: reflect.TypeFor[team](),
		collection:  "teams",
		targetCol:   "id",
		parentFKIdx: 1,
		fieldIndex:  2,
	}
	if err := query.loadMongoHasMany(ctx, parents, hasMany); err == nil {
		t.Fatal("loadMongoHasMany() did not propagate unavailable database error")
	}
	if err := query.loadMongoBelongsTo(ctx, parents, belongsTo); err == nil {
		t.Fatal("loadMongoBelongsTo() did not propagate unavailable database error")
	}

	query = &ORM{}
	if err := query.loadMongoPreloads(ctx, &mongoPreloadPlan{
		hasMany: []*mongoHasMany{hasMany},
	}, parents); err == nil {
		t.Fatal("loadMongoPreloads() did not propagate has-many loader error")
	}

	query = &ORM{}
	if err := query.loadMongoPreloads(ctx, &mongoPreloadPlan{
		belongsTo: []*mongoBelongsTo{belongsTo},
	}, parents); err == nil {
		t.Fatal("loadMongoPreloads() did not propagate belongs-to loader error")
	}

	query = &ORM{}
	_, err := query.loadGroupedMongoHasMany(
		ctx, 0, []any{1},
		hasMany.rel, hasMany, nil,
		buildStructFieldMapByTag(reflect.TypeFor[child]()),
		map[string][]reflect.Value{},
	)
	if err == nil {
		t.Fatal("loadGroupedMongoHasMany() did not report unavailable database")
	}

	query = &ORM{}
	err = query.loadFilterMongoBelongsTo(
		ctx, 0, []any{2}, belongsTo,
		buildStructFieldMapByTag(reflect.TypeFor[team]()),
		map[string]reflect.Value{},
	)
	if err == nil {
		t.Fatal("loadFilterMongoBelongsTo() did not report unavailable database")
	}
}

func TestLoadParentMongoHasManyMapsValuesAndPointers(t *testing.T) {
	type child struct {
		ID int
	}
	type parent struct {
		Children []child
		Pointers []*child
	}
	value := reflect.ValueOf(&parent{}).Elem()
	item := reflect.ValueOf(child{ID: 3})

	loadParentMongoHasMany(
		map[string][]reflect.Value{"1": {item}},
		"1",
		[]reflect.Value{value},
		&hasManyRelation{fieldIndex: 0, sliceType: reflect.TypeFor[[]child]()},
	)
	loadParentMongoHasMany(
		map[string][]reflect.Value{"1": {reflect.ValueOf(&child{ID: 3})}},
		"1",
		[]reflect.Value{value},
		&hasManyRelation{fieldIndex: 1, sliceType: reflect.TypeFor[[]*child]()},
	)
	got := value.Interface().(parent)
	if !reflect.DeepEqual(got.Children, []child{{ID: 3}}) ||
		len(got.Pointers) != 1 || got.Pointers[0] == nil || *got.Pointers[0] != (child{ID: 3}) {
		t.Fatalf("loaded Mongo child relations = %#v", got)
	}
}
