package orm

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMongoUpsertKeepsPrimaryKeyOnMatch(t *testing.T) {
	type record struct {
		ID    string `gorest:"id, primary_key"`
		Email string `gorest:"email"`
		Name  string `gorest:"name"`
	}

	orm := &ORM{UpsertConflictCols: []string{"email"}}
	primaryKeyCols, err := orm.mongoPrimaryKeyColumns(record{})
	if err != nil {
		t.Fatalf("mongoPrimaryKeyColumns() error = %v", err)
	}

	op, err := orm.buildMongoUpsertOp(
		map[string]any{"id": "new-id", "email": "a@example.com", "name": "Updated"},
		nil,
		primaryKeyCols,
	)
	if err != nil {
		t.Fatalf("buildMongoUpsertOp() error = %v", err)
	}

	set := op.update["$set"].(bson.M)
	if _, ok := set["id"]; ok {
		t.Fatal("primary key id was included in $set")
	}
	if got := set["name"]; got != "Updated" {
		t.Errorf("$set name = %v, want Updated", got)
	}

	setOnInsert := op.update["$setOnInsert"].(bson.M)
	if got := setOnInsert["id"]; got != "new-id" {
		t.Errorf("$setOnInsert id = %v, want new-id", got)
	}
}

func TestMongoUpsertRejectsPrimaryKeyInUpdateColumns(t *testing.T) {
	type record struct {
		ID    string `gorest:"id, primary_key"`
		Email string `gorest:"email"`
		Name  string `gorest:"name"`
	}

	orm := &ORM{
		UpsertConflictCols: []string{"email"},
		UpsertUpdateCols:   []string{"id"},
	}
	primaryKeyCols, err := orm.mongoPrimaryKeyColumns(record{})
	if err != nil {
		t.Fatalf("mongoPrimaryKeyColumns() error = %v", err)
	}

	if _, err := orm.buildMongoUpsertOp(
		map[string]any{"id": "new-id", "email": "a@example.com", "name": "Updated"},
		nil,
		primaryKeyCols,
	); err == nil {
		t.Fatal("buildMongoUpsertOp() accepted a primary key update column")
	}
}

func TestMongoPrimaryKeyColumnUsesMongoDocumentName(t *testing.T) {
	type record struct {
		ID string `bson:"_id" gorest:"id, primary_key"`
	}

	orm := &ORM{}
	got, err := orm.mongoPrimaryKeyColumns(record{})
	if err != nil {
		t.Fatalf("mongoPrimaryKeyColumns() error = %v", err)
	}
	if len(got) != 1 || got[0] != "_id" {
		t.Fatalf("mongoPrimaryKeyColumns() = %v, want [_id]", got)
	}
}

func TestTranslateUpsertClausesToMongo(t *testing.T) {
	conditions, err := translateUpsertClausesToMongo(
		[]string{
			"email = ? AND age >= ? AND score <> ?",
			"deleted_at IS NULL AND archived_at IS NOT NULL",
		},
		[]any{"ada@example.test", 18, 100},
	)
	if err != nil {
		t.Fatalf("translateUpsertClausesToMongo() error = %v", err)
	}
	want := []bson.M{
		{"email": "ada@example.test"},
		{"age": bson.M{dolargte: 18}},
		{"score": bson.M{dolarne: 100}},
		{"deleted_at": nil},
		{"archived_at": bson.M{dolarne: nil}},
	}
	if len(conditions) != len(want) {
		t.Fatalf("conditions = %#v, want %#v", conditions, want)
	}
	for idx := range want {
		if !reflect.DeepEqual(conditions[idx], want[idx]) {
			t.Errorf("condition[%d] = %#v, want %#v", idx, conditions[idx], want[idx])
		}
	}

	for _, test := range []struct {
		name    string
		clauses []string
		args    []any
	}{
		{name: "unsupported expression", clauses: []string{"name LIKE ?"}, args: []any{"A%"}},
		{name: "missing argument", clauses: []string{"email = ?"}},
		{name: "unused argument", clauses: []string{"deleted_at IS NULL"}, args: []any{"extra"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := translateUpsertClausesToMongo(test.clauses, test.args); err == nil {
				t.Fatal("translateUpsertClausesToMongo() accepted invalid condition input")
			}
		})
	}
}

func TestBuildMongoUpsertOpsValidatesAndBuildsDocuments(t *testing.T) {
	query := &ORM{
		UpsertConflictCols: []string{"email"},
		UpsertClauses:      []string{"status = ?"},
		UpsertArgs:         []any{"active"},
	}
	ops, err := query.buildMongoUpsertOps(
		[]any{
			map[string]any{"email": "ada@example.test", "status": "active"},
			map[string]any{"email": "lin@example.test", "status": "active"},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("buildMongoUpsertOps() error = %v", err)
	}
	if len(ops) != 2 || ops[0].filter["email"] != "ada@example.test" || ops[1].filter["email"] != "lin@example.test" {
		t.Fatalf("upsert operations = %#v", ops)
	}

	if _, err := (&ORM{UpsertClauses: []string{"status LIKE ?"}, UpsertArgs: []any{"a%"}}).
		buildMongoUpsertOps([]any{map[string]any{"email": "ada@example.test"}}, nil); err == nil {
		t.Fatal("buildMongoUpsertOps() accepted an unsupported conditional clause")
	}
	if _, err := (&ORM{}).buildMongoUpsertOps([]any{"not a document"}, nil); err == nil {
		t.Fatal("buildMongoUpsertOps() accepted a non-document row")
	}
}

func TestPrepareScyllaUpsertAndBuildUpdate(t *testing.T) {
	meta := columnMetaData{
		columns:                []string{"tenant_id", "id", "name"},
		columnIndex:            []int{0, 1, 2},
		primaryKeyColumns:      []string{"tenant_id", "id"},
		primaryKeyIndex:        []int{0, 1},
		allColumnPrimaryKeyIdx: map[string]int{"tenant_id": 0, "id": 1, "name": 2},
	}
	if err := (&ORM{}).prepareScyllaUpsert(meta); err != nil {
		t.Fatalf("prepareScyllaUpsert() with default conflict behavior: %v", err)
	}

	for _, query := range []*ORM{
		{UpsertClauses: []string{"name = ?"}, UpsertArgs: []any{"Ada"}},
		{UpsertConflictCols: []string{"id"}},
		{UpsertConflictCols: []string{"tenant_id", "id"}, UpsertUpdateCols: []string{"id"}},
		{UpsertConflictCols: []string{"tenant_id", "id"}, UpsertUpdateCols: []string{"missing"}},
	} {
		if err := query.prepareScyllaUpsert(meta); err == nil {
			t.Errorf("prepareScyllaUpsert() accepted invalid configuration %#v", query)
		}
	}

	type record struct {
		TenantID int
		ID       int
		Name     string
	}
	statement, args := buildScyllaUpsertUpdate(
		"users",
		reflect.ValueOf(record{TenantID: 2, ID: 7, Name: "Ada"}),
		meta,
		[]string{"name"},
	)
	if statement != "UPDATE users SET name = ? WHERE tenant_id = ? AND id = ?" {
		t.Fatalf("Scylla upsert UPDATE = %q", statement)
	}
	if !reflect.DeepEqual(args, []any{"Ada", 2, 7}) {
		t.Fatalf("Scylla upsert arguments = %#v", args)
	}
}

func TestMongoDocumentKeyAndZeroValueHelpers(t *testing.T) {
	if key, ok := lookupDocKey(map[string]any{"Email": "ada@example.test"}, "email"); !ok || key != "Email" {
		t.Fatalf("case-insensitive lookupDocKey() = (%q, %t)", key, ok)
	}
	if key, ok := lookupDocKey(map[string]any{"email": nil}, "email"); !ok || key != "email" {
		t.Fatalf("exact lookupDocKey() = (%q, %t)", key, ok)
	}
	if key, ok := lookupDocKey(map[string]any{"name": "Ada"}, "email"); ok || key != "" {
		t.Fatalf("missing lookupDocKey() = (%q, %t), want (empty, false)", key, ok)
	}
	for _, value := range []any{nil, "", 0, false} {
		if !isZeroValue(value) {
			t.Errorf("isZeroValue(%#v) = false, want true", value)
		}
	}
	if isZeroValue("value") || isZeroValue(1) {
		t.Fatal("isZeroValue() classified non-zero values as zero")
	}
}
