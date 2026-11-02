//go:build integration

package mongo_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMongoServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.MONGO)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var version string
	_, _, err := app.Database().RawQueryMongo(ctx, func(ctx context.Context, client *mongo.Client) error {
		var result struct {
			Version string `bson:"version"`
		}
		if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&result); err != nil {
			return err
		}
		version = result.Version
		return nil
	})
	if err != nil {
		t.Fatalf("query MongoDB server version failed: %v", err)
	}

	t.Logf("MongoDB server version: %s", version)
}

func TestMongoStorageUsageEndpoint(t *testing.T) {
	internaltest.StorageUsage(t, database.MONGO)
}

func TestORMCRUDWithBoundValues(t *testing.T) {
	internaltest.NoSQLCRUD(t, database.MONGO)
}

func TestMongoCoverage(t *testing.T) {
	app := internaltest.OpenApp(t, database.MONGO)
	collectionName := "audit_" + uniqueSuffix(t)
	collection, message, code, err := app.Database().MongoCollection(collectionName)
	if err != nil {
		t.Fatalf("create collection handle (HTTP %d, %s): %v", code, message, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := collection.Drop(ctx); err != nil {
			t.Errorf("drop integration collection %s: %v", collectionName, err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		t.Fatalf("create unique email index: %v", err)
	}

	type document struct {
		ID    string `gorest:"_id, primary_key"`
		Email string `gorest:"email"`
		Name  string `gorest:"name"`
	}
	initial := document{ID: "first", Email: "test@example.test", Name: "before"}
	internaltest.RequireNoORMError(t, app.Database().Table(collectionName).Create(&initial))

	update := document{ID: "second", Email: initial.Email, Name: "after"}
	internaltest.RequireNoORMError(t, app.Database().Table(collectionName).Upsert("email").Create(&update))

	var loaded document
	internaltest.RequireNoORMError(t, app.Database().Table(collectionName).Where("email = ?", initial.Email).Find(&loaded))
	if loaded.ID != initial.ID || loaded.Name != "after" {
		t.Fatalf("alternate-key Upsert result = %#v, want original _id with updated name", loaded)
	}

	type applicationDocument struct {
		ID    string `gorest:"id, primary_key"`
		Email string `gorest:"email"`
		Name  string `gorest:"name"`
	}
	initialApplicationDoc := applicationDocument{
		ID:    "application-id-first",
		Email: "application-id@example.test",
		Name:  "before",
	}
	internaltest.RequireNoORMError(t, app.Database().Table(collectionName).Create(&initialApplicationDoc))

	updatedApplicationDoc := applicationDocument{
		ID:    "application-id-second",
		Email: initialApplicationDoc.Email,
		Name:  "after",
	}
	internaltest.RequireNoORMError(t, app.Database().Table(collectionName).Upsert("email").Create(&updatedApplicationDoc))

	var loadedApplicationDoc applicationDocument
	internaltest.RequireNoORMError(t, app.Database().Table(collectionName).
		Where("email = ?", initialApplicationDoc.Email).
		Find(&loadedApplicationDoc))
	if loadedApplicationDoc.ID != initialApplicationDoc.ID || loadedApplicationDoc.Name != "after" {
		t.Fatalf(
			"alternate-key Upsert result = %#v, want original application id %q with updated name",
			loadedApplicationDoc,
			initialApplicationDoc.ID,
		)
	}

	if query := app.Database().Table(collectionName).Upsert("email = ? AND name = ?", initial.Email).
		Create(&document{ID: "third", Email: initial.Email, Name: "invalid"}); query.Error == nil {
		t.Fatal("Upsert() with a placeholder/argument mismatch unexpectedly succeeded")
	}

	if query := app.Database().Table(collectionName).Update(&document{Name: "unfiltered"}); query.Error == nil {
		t.Fatal("unfiltered Update() unexpectedly succeeded")
	}
	if query := app.Database().Table(collectionName).Delete(&document{}); query.Error == nil {
		t.Fatal("unfiltered Delete() unexpectedly succeeded")
	}
}

func TestMongoNullableMapping(t *testing.T) {
	app := internaltest.OpenApp(t, database.MONGO)
	collectionName := "audit_null_" + uniqueSuffix(t)
	collection, message, code, err := app.Database().MongoCollection(collectionName)
	if err != nil {
		t.Fatalf("create collection handle (HTTP %d, %s): %v", code, message, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := collection.Drop(ctx); err != nil {
			t.Errorf("drop integration collection %s: %v", collectionName, err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = collection.InsertMany(ctx, []any{
		bson.M{"_id": "null", "optional_text": nil},
		bson.M{"_id": "present", "optional_text": "available"},
	})
	if err != nil {
		t.Fatalf("insert nullable mapping documents: %v", err)
	}

	type nullableDocument struct {
		ID           string  `gorest:"_id, primary_key"`
		OptionalText *string `gorest:"optional_text"`
	}
	var records []nullableDocument
	query := app.Database().Table(collectionName).OrderBy("_id", "ASC").Find(&records)
	if query.Error != nil {
		t.Fatalf("Find() with nullable MongoDB fields failed: %v", query.Error)
	}
	if len(records) != 2 || records[0].OptionalText != nil ||
		records[1].OptionalText == nil || *records[1].OptionalText != "available" {
		t.Fatalf("nullable MongoDB mapping = %#v, want nil then %q", records, "available")
	}
}

func TestMongoCanceledContextIsPropagated(t *testing.T) {
	app := internaltest.OpenApp(t, database.MONGO)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := app.Database().RawQueryMongo(ctx, func(ctx context.Context, client *mongo.Client) error {
		return client.Ping(ctx, nil)
	})
	if err == nil {
		t.Fatal("MongoDB operation with a canceled context unexpectedly succeeded")
	}
}

func uniqueSuffix(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
