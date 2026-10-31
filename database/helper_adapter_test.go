package database

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoCursorStructType(t *testing.T) {
	type document struct{}

	tests := []struct {
		name        string
		destination reflect.Value
		wantPointer bool
		wantError   bool
	}{
		{
			name:        "single struct",
			destination: reflect.ValueOf(document{}),
		},
		{
			name:        "slice of structs",
			destination: reflect.ValueOf([]document{}),
		},
		{
			name:        "slice of struct pointers",
			destination: reflect.ValueOf([]*document{}),
			wantPointer: true,
		},
		{
			name:        "slice of non-structs",
			destination: reflect.ValueOf([]string{}),
			wantError:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotType, gotPointer, err := mongoCursorStructType(test.destination)
			if (err != nil) != test.wantError {
				t.Fatalf("mongoCursorStructType() error = %v, wantError %t", err, test.wantError)
			}
			if err != nil {
				return
			}
			if gotType != reflect.TypeOf(document{}) {
				t.Errorf("struct type = %v, want %v", gotType, reflect.TypeOf(document{}))
			}
			if gotPointer != test.wantPointer {
				t.Errorf("pointer elements = %t, want %t", gotPointer, test.wantPointer)
			}
		})
	}
}

type mongoAdapterDocument struct {
	ID         string    `gorest:"_id,primary_key"`
	Name       string    `gorest:"name"`
	Age        int       `gorest:"age"`
	Active     bool      `gorest:"active"`
	CreatedAt  time.Time `gorest:"created_at"`
	Payload    []byte    `gorest:"payload"`
	Nickname   *string   `gorest:"nickname"`
	Ignored    string    `gorest:"-"`
	unexported string
}

func TestEncodeMongoCollectionUsesGorestTagsAndHandlesValidation(t *testing.T) {
	database := &Database{}
	nickname := "Ada"
	input := &mongoAdapterDocument{
		ID:       "id-1",
		Name:     "Ada",
		Nickname: &nickname,
		Ignored:  "not persisted",
	}
	document, message, status, err := database.EncodeMongoCollection(input)
	if err != nil {
		t.Fatalf("EncodeMongoCollection() error = %v", err)
	}
	if status != http.StatusOK || message != "" {
		t.Fatalf("EncodeMongoCollection() response = (%q, %d), want empty message and 200", message, status)
	}
	if !reflect.DeepEqual(document, bson.M{
		"_id": "id-1", "name": "Ada", "age": 0, "active": false,
		"created_at": time.Time{}, "payload": []byte(nil), "nickname": &nickname,
	}) {
		t.Fatalf("encoded document = %#v", document)
	}

	var nilDocument *mongoAdapterDocument
	tests := []struct {
		name string
		dest any
	}{
		{name: "nil", dest: nil},
		{name: "nil pointer", dest: nilDocument},
		{name: "non-struct", dest: "not a document"},
		{name: "no tagged fields", dest: struct{ Name string }{Name: "Ada"}},
		{name: "duplicate tags", dest: struct {
			First  string `gorest:"name"`
			Second string `gorest:"name"`
		}{First: "Ada", Second: "Grace"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, message, status, err := database.EncodeMongoCollection(test.dest)
			if err == nil || got != nil || status != http.StatusBadRequest || message == "" {
				t.Fatalf("EncodeMongoCollection(%T) = (%#v, %q, %d, %v), want a bad-request validation error", test.dest, got, message, status, err)
			}
		})
	}
}

func TestDecodeMongoCursorIntoStructAndSlices(t *testing.T) {
	database := &Database{}
	objectID := bson.NewObjectID()
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	documents := []any{
		bson.M{
			"_id": objectID, "name": "Ada", "age": int32(37), "active": true,
			"created_at": bson.DateTime(createdAt.UnixMilli()),
			"payload":    bson.Binary{Subtype: 0, Data: []byte{1, 2}},
			"nickname":   "Countess", "unknown": "ignored",
		},
		bson.M{
			"_id": bson.NewObjectID(), "name": "Grace", "age": int32(40),
			"active": false, "nickname": nil,
		},
	}

	cursor, err := mongo.NewCursorFromDocuments(documents[:1], nil, nil)
	if err != nil {
		t.Fatalf("NewCursorFromDocuments() error = %v", err)
	}
	var single mongoAdapterDocument
	message, status, err := database.DecodeMongoCursor(context.Background(), cursor, &single)
	if err != nil {
		t.Fatalf("DecodeMongoCursor(struct) error = %v", err)
	}
	if status != http.StatusOK || message != "" || single.ID != objectID.Hex() ||
		single.Name != "Ada" || single.Age != 37 || !single.Active ||
		!single.CreatedAt.Equal(createdAt) || !reflect.DeepEqual(single.Payload, []byte{1, 2}) ||
		single.Nickname == nil || *single.Nickname != "Countess" || single.Ignored != "" {
		t.Fatalf("decoded struct = %#v, message %q, status %d", single, message, status)
	}
	_ = cursor.Close(context.Background())

	cursor, err = mongo.NewCursorFromDocuments(documents, nil, nil)
	if err != nil {
		t.Fatalf("NewCursorFromDocuments() error = %v", err)
	}
	var rows []mongoAdapterDocument
	message, status, err = database.DecodeMongoCursor(context.Background(), cursor, &rows)
	if err != nil {
		t.Fatalf("DecodeMongoCursor([]struct) error = %v", err)
	}
	if status != http.StatusOK || message != "" || len(rows) != 2 || rows[0].Name != "Ada" ||
		rows[1].Name != "Grace" || rows[1].Nickname != nil {
		t.Fatalf("decoded struct slice = %#v, message %q, status %d", rows, message, status)
	}
	_ = cursor.Close(context.Background())

	cursor, err = mongo.NewCursorFromDocuments(documents[:1], nil, nil)
	if err != nil {
		t.Fatalf("NewCursorFromDocuments() error = %v", err)
	}
	var pointerRows []*mongoAdapterDocument
	message, status, err = database.DecodeMongoCursor(context.Background(), cursor, &pointerRows)
	if err != nil {
		t.Fatalf("DecodeMongoCursor([]*struct) error = %v", err)
	}
	if status != http.StatusOK || len(pointerRows) != 1 || pointerRows[0] == nil || pointerRows[0].ID != objectID.Hex() {
		t.Fatalf("decoded pointer slice = %#v, message %q, status %d", pointerRows, message, status)
	}
	_ = cursor.Close(context.Background())

	cursor, err = mongo.NewCursorFromDocuments(nil, nil, nil)
	if err != nil {
		t.Fatalf("NewCursorFromDocuments(empty) error = %v", err)
	}
	rows = nil
	message, status, err = database.DecodeMongoCursor(context.Background(), cursor, &rows)
	if err != nil || message != "" || status != http.StatusOK || rows == nil || len(rows) != 0 {
		t.Fatalf("empty cursor decode = (%#v, %q, %d, %v)", rows, message, status, err)
	}
	_ = cursor.Close(context.Background())
}

func TestDecodeMongoCursorRejectsInvalidInputsAndReportsFailures(t *testing.T) {
	database := &Database{}
	validCursor := func(t *testing.T, docs []any, cursorErr error) *mongo.Cursor {
		t.Helper()
		cursor, err := mongo.NewCursorFromDocuments(docs, cursorErr, nil)
		if err != nil {
			t.Fatalf("NewCursorFromDocuments() error = %v", err)
		}
		t.Cleanup(func() { _ = cursor.Close(context.Background()) })
		return cursor
	}

	var destination mongoAdapterDocument
	for _, test := range []struct {
		name   string
		ctx    context.Context
		cursor *mongo.Cursor
		dest   any
	}{
		{name: "nil context", cursor: validCursor(t, nil, nil), dest: &destination},
		{name: "nil cursor", ctx: context.Background(), dest: &destination},
		{name: "nil destination", ctx: context.Background(), cursor: validCursor(t, nil, nil)},
		{name: "nil pointer destination", ctx: context.Background(), cursor: validCursor(t, nil, nil), dest: (*mongoAdapterDocument)(nil)},
		{name: "non-pointer destination", ctx: context.Background(), cursor: validCursor(t, nil, nil), dest: destination},
		{name: "unsupported destination", ctx: context.Background(), cursor: validCursor(t, nil, nil), dest: new(int)},
		{name: "unsupported slice element", ctx: context.Background(), cursor: validCursor(t, nil, nil), dest: new([]string)},
	} {
		t.Run(test.name, func(t *testing.T) {
			message, status, err := database.DecodeMongoCursor(test.ctx, test.cursor, test.dest)
			if err == nil || message == "" || status != http.StatusBadRequest {
				t.Fatalf("DecodeMongoCursor() = (%q, %d, %v), want bad-request validation error", message, status, err)
			}
		})
	}

	message, status, err := database.DecodeMongoCursor(
		context.Background(),
		validCursor(t, nil, nil),
		&destination,
	)
	if err == nil || message == "" || status != http.StatusNotFound {
		t.Fatalf("empty struct cursor = (%q, %d, %v), want not-found error", message, status, err)
	}

	message, status, err = database.DecodeMongoCursor(
		context.Background(),
		validCursor(t, []any{bson.M{"age": "not an integer"}}, nil),
		&destination,
	)
	if err == nil || message == "" || status != http.StatusInternalServerError {
		t.Fatalf("type mismatch decode = (%q, %d, %v), want server error", message, status, err)
	}

	cursorFailure := errors.New("cursor iteration failed")
	message, status, err = database.DecodeMongoCursor(
		context.Background(),
		validCursor(t, nil, cursorFailure),
		new([]mongoAdapterDocument),
	)
	if err == nil || message == "" || status != http.StatusInternalServerError ||
		!errors.Is(err, cursorFailure) {
		t.Fatalf("cursor failure = (%q, %d, %v), want wrapped iteration error", message, status, err)
	}
}
