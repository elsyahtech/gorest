package orm

import (
	"context"
	"reflect"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestFindHasManyRelationBuildsMetadataAndRejectsBrokenReferences(t *testing.T) {
	type child struct {
		ID     int64  `gorest:"id, primary_key"`
		UserID int64  `gorest:"user_ref"`
		Title  string `gorest:"title"`
	}
	type parent struct {
		ID     int64   `gorest:"parent_id, primary_key"`
		Orders []child `gorest:"-" references:"UserID" orderby:"Title DESC, ID"`
	}

	query := &ORM{}
	relation, err := query.findHasManyRelation(reflect.TypeFor[parent](), "Orders")
	if err != nil {
		t.Fatalf("findHasManyRelation() error = %v", err)
	}
	if relation == nil {
		t.Fatal("findHasManyRelation() returned no relation")
	}
	if relation.table != "orders" || relation.childFKCol != "user_ref" ||
		relation.parentKeyCol != "parent_id" || relation.parentKeyIdx != 0 ||
		relation.childFKIdx != 1 || relation.elemIsPtr {
		t.Fatalf("has-many metadata = %#v", relation)
	}
	if want := "orders.title DESC, orders.id ASC"; relation.orderBy != want {
		t.Fatalf("child order = %q, want %q", relation.orderBy, want)
	}

	type noReference struct {
		ID     int64   `gorest:"id, primary_key"`
		Orders []child `gorest:"-"`
	}
	if _, err := (&ORM{}).findHasManyRelation(reflect.TypeFor[noReference](), "Orders"); err == nil {
		t.Fatal("has-many relation without references unexpectedly succeeded")
	}

	type noParentKey struct {
		Orders []child `gorest:"-" references:"UserID"`
	}
	if _, err := (&ORM{}).findHasManyRelation(reflect.TypeFor[noParentKey](), "Orders"); err == nil {
		t.Fatal("has-many relation without a parent primary key unexpectedly succeeded")
	}

	type invalidOrder struct {
		ID     int64   `gorest:"id, primary_key"`
		Orders []child `gorest:"-" references:"UserID" orderby:"missing ASC"`
	}
	if _, err := (&ORM{}).findHasManyRelation(reflect.TypeFor[invalidOrder](), "Orders"); err == nil {
		t.Fatal("has-many relation with unknown order column unexpectedly succeeded")
	}
}

func TestFindHasManyRelationSupportsPointerChildrenAndNoMatch(t *testing.T) {
	type child struct {
		ID     int64 `gorest:"id, primary_key"`
		UserID int64 `gorest:"user_id"`
	}
	type parent struct {
		ID     int64    `gorest:"id, primary_key"`
		Orders []*child `gorest:"-" references:"UserID"`
	}

	relation, err := (&ORM{}).findHasManyRelation(reflect.TypeFor[parent](), "Orders")
	if err != nil {
		t.Fatalf("pointer child relation error = %v", err)
	}
	if relation == nil || !relation.elemIsPtr || relation.elemType != reflect.TypeFor[child]() {
		t.Fatalf("pointer child metadata = %#v", relation)
	}

	if relation, err := (&ORM{}).findHasManyRelation(reflect.TypeFor[parent](), "Missing"); err != nil || relation != nil {
		t.Fatalf("missing relation = %#v, %v; want nil, nil", relation, err)
	}
}

func TestHasManyKeyAndParentHelpers(t *testing.T) {
	type parent struct {
		ID int64 `gorest:"parent_id, primary_key"`
	}
	parentType := reflect.TypeFor[parent]()
	relation := &hasManyRelation{parentKeyCol: "parent_id", parentKeyIdx: 0}
	parents := []reflect.Value{
		reflect.ValueOf(parent{ID: 10}),
		reflect.ValueOf(parent{ID: 10}),
		reflect.ValueOf(parent{ID: 20}),
		reflect.ValueOf(parent{ID: 0}),
	}
	keys, byKey := buildKeysInLoadHasMany(parents, relation)
	if !reflect.DeepEqual(keys, []any{int64(10), int64(20)}) {
		t.Fatalf("parent keys = %#v", keys)
	}
	if len(byKey["10"]) != 2 || len(byKey["20"]) != 1 {
		t.Fatalf("parent grouping = %#v", byKey)
	}

	if got := primaryKeyColumn(parentType); got != "parent_id" {
		t.Fatalf("primaryKeyColumn() = %q, want parent_id", got)
	}
	if _, ok := derefKey(reflect.ValueOf((*int64)(nil))); ok {
		t.Fatal("derefKey(nil pointer) unexpectedly succeeded")
	}
	if _, ok := derefKey(reflect.ValueOf(int64(0))); ok {
		t.Fatal("derefKey(zero value) unexpectedly succeeded")
	}
	if got, ok := derefKey(reflect.ValueOf(int64(7))); !ok || got.Int() != 7 {
		t.Fatalf("derefKey(non-zero value) = %v, %t", got, ok)
	}

	query := &ORM{}
	if err := query.isHasKeyColInLoadHasMany([]string{"books.title"}, relation); err == nil {
		t.Fatal("preload without selected parent key unexpectedly succeeded")
	}
	if err := (&ORM{}).isHasKeyColInLoadHasMany([]string{"books.parent_id"}, relation); err != nil {
		t.Fatalf("qualified parent key was rejected: %v", err)
	}
}

func TestCollectParentsHandlesStructSlicesAndNilPointers(t *testing.T) {
	type parent struct{ ID int }
	query := &ORM{}
	single := reflect.ValueOf(parent{ID: 1})
	parents, err := query.collectParents(single, false)
	if err != nil || len(parents) != 1 || parents[0].Field(0).Int() != 1 {
		t.Fatalf("single parent = %#v, %v", parents, err)
	}

	values := []parent{{ID: 2}, {ID: 3}}
	parents, err = query.collectParents(reflect.ValueOf(values), true)
	if err != nil || len(parents) != 2 {
		t.Fatalf("value slice parents = %#v, %v", parents, err)
	}

	pointers := []*parent{{ID: 4}, nil, {ID: 5}}
	parents, err = query.collectParents(reflect.ValueOf(pointers), true)
	if err != nil || len(parents) != 2 || parents[1].Field(0).Int() != 5 {
		t.Fatalf("pointer slice parents = %#v, %v", parents, err)
	}
}

func TestBuildChildOrderByUsesSafeTaggedColumns(t *testing.T) {
	type child struct {
		ID    int64  `gorest:"id, primary_key"`
		Title string `gorest:"display_title"`
	}
	lookup := buildStructFieldMap(reflect.TypeFor[child]())
	tests := []struct {
		name string
		raw  string
		want string
		bad  bool
	}{
		{name: "default primary key", want: "entries.id ASC"},
		{name: "explicit multiple columns", raw: "Title DESC, ID", want: "entries.display_title DESC, entries.id ASC"},
		{name: "unknown column", raw: "missing", bad: true},
		{name: "unsafe direction", raw: "ID RANDOM", bad: true},
		{name: "too many terms", raw: "ID ASC DESC", bad: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := buildChildOrderBy(test.raw, "entries", reflect.TypeFor[child](), lookup)
			if (err != nil) != test.bad {
				t.Fatalf("buildChildOrderBy() error = %v, want bad %t", err, test.bad)
			}
			if err == nil && got != test.want {
				t.Fatalf("child order = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParsePreloadConditionValidatesArgumentsAndDriverPlaceholders(t *testing.T) {
	tests := []struct {
		name   string
		args   []any
		driver string
		want   string
		values []any
		bad    bool
	}{
		{name: "no condition"},
		{
			name:   "postgres condition",
			args:   []any{"status = ? AND archived IS NULL", "active"},
			driver: database.POSTGRES,
			want:   "status = ? AND archived IS NULL",
			values: []any{"active"},
		},
		{name: "non-string condition", args: []any{42}, bad: true},
		{name: "empty condition", args: []any{"  "}, bad: true},
		{name: "unsafe condition", args: []any{"status = 'active'"}, bad: true},
		{name: "argument mismatch", args: []any{"status = ?"}, bad: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &ORM{}
			got, values, err := query.parsePreloadCondition("Orders", test.args, test.driver)
			if (err != nil) != test.bad {
				t.Fatalf("parsePreloadCondition() error = %v, want bad %t", err, test.bad)
			}
			if err == nil && (got != test.want || !reflect.DeepEqual(values, test.values)) {
				t.Fatalf("condition/arguments = %q / %#v, want %q / %#v", got, values, test.want, test.values)
			}
		})
	}
}

func TestHasManyChunkBuilderUsesDriverPlaceholders(t *testing.T) {
	type child struct {
		ID     int64 `gorest:"id, primary_key"`
		UserID int64 `gorest:"user_id"`
	}
	rel := &hasManyRelation{
		relationName: "Orders",
		table:        "orders",
		childFKCol:   "user_id",
		orderBy:      "orders.id ASC",
		elemType:     reflect.TypeFor[child](),
		childFKIdx:   1,
	}

	query := &ORM{
		Database:       &database.Database{},
		DatabaseConfig: &database.Config{Driver: database.POSTGRES},
	}
	grouped, err := query.buildGroupInloadHasMany(
		context.Background(),
		rel,
		[]any{int64(3), int64(4)},
		"status = ?",
		[]any{"active"},
		database.POSTGRES,
	)
	if err == nil {
		t.Fatal("query builder without a live database unexpectedly succeeded")
	}
	if grouped != nil {
		t.Fatalf("failed query returned grouped rows %#v", grouped)
	}
	if query.Error == nil {
		t.Fatal("database query failure did not propagate into ORM.Error")
	}
}
