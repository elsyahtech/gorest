//go:build integration

package mongo_test

import (
	"context"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.MONGO)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var version string
	err := app.Database().RawQueryMongo(ctx, func(ctx context.Context, client *mongo.Client) error {
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

func TestORMCRUDWithBoundValues(t *testing.T) {
	internaltest.NoSQLCRUD(t, database.MONGO)
}
