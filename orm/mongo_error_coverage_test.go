package orm

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoCaseOnIsInvalidDatabaseQueryError(t *testing.T) {
	for _, code := range []int32{2, 9, 14, 168, 40324} {
		err := mongo.CommandError{Code: code, Message: "invalid query"}
		if !mongoCaseOnIsInvalidDatabaseQueryError(err) {
			t.Fatalf("MongoDB error code %d was not classified as an invalid query", code)
		}
	}

	if mongoCaseOnIsInvalidDatabaseQueryError(errors.New("ordinary error")) {
		t.Fatal("ordinary error was classified as MongoDB invalid query")
	}
	if mongoCaseOnIsInvalidDatabaseQueryError(mongo.CommandError{Code: 999, Message: "other"}) {
		t.Fatal("unrelated MongoDB error code was classified as invalid query")
	}
}
