package orm

import (
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
