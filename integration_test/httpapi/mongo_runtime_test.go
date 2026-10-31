//go:build integration && integrationcontainers

package httpapi_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
	"github.com/testcontainers/testcontainers-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type mongoRuntimeRecord struct {
	ID    string `gorest:"_id, primary_key" bson:"_id"`
	Email string `gorest:"email" bson:"email"`
	Name  string `gorest:"name" bson:"name"`
}

func runMongoRuntimeRecovery(t *testing.T, app *gorest.App, container testcontainers.Container) {
	t.Helper()

	collectionName := fmt.Sprintf("gorest_runtime_recovery_%d", time.Now().UnixNano())
	db := app.Database()
	collection, message, code, err := db.MongoCollection(collectionName)
	if err != nil {
		t.Fatalf("get MongoDB runtime recovery collection (HTTP %d, %s): %v", code, message, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelCleanup()
		if err := collection.Drop(cleanupCtx); err != nil {
			t.Errorf("drop MongoDB runtime recovery collection: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	beforeRestart := mongoRuntimeRecord{
		ID:    fmt.Sprintf("before-%d", time.Now().UnixNano()),
		Email: fmt.Sprintf("before-%d@example.test", time.Now().UnixNano()),
		Name:  "Before restart",
	}
	if _, err := collection.InsertOne(ctx, beforeRestart); err != nil {
		t.Fatalf("insert MongoDB runtime recovery record before outage: %v", err)
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 30*time.Second)
	gracePeriod := 10 * time.Second
	err = container.Stop(stopCtx, &gracePeriod)
	cancelStop()
	if err != nil {
		t.Fatalf("stop MongoDB during runtime recovery test: %v", err)
	}

	pingCtx, cancelPing := context.WithTimeout(context.Background(), 2*time.Second)
	_, _, err = db.RawQueryMongo(pingCtx, func(ctx context.Context, client *mongo.Client) error {
		return client.Ping(ctx, nil)
	})
	cancelPing()
	if err == nil {
		t.Fatal("MongoDB health check succeeded while the database container was stopped")
	}

	outageCtx, cancelOutage := context.WithTimeout(context.Background(), 2*time.Second)
	outageStarted := time.Now()
	_, err = collection.InsertOne(outageCtx, mongoRuntimeRecord{
		ID: "during-outage-" + beforeRestart.ID,
	})
	cancelOutage()
	if err == nil {
		t.Fatal("MongoDB write succeeded while the database container was stopped")
	}
	if elapsed := time.Since(outageStarted); elapsed > 3*time.Second {
		t.Fatalf("MongoDB write took %s to fail during outage; want it bounded by its 2-second context", elapsed)
	}

	startCtx, cancelStart := context.WithTimeout(context.Background(), 3*time.Minute)
	err = container.Start(startCtx)
	cancelStart()
	if err != nil {
		t.Fatalf("restart MongoDB during runtime recovery test: %v", err)
	}
	waitForContainerPort(t, database.MONGO, container)
	waitForMongoDatabase(t, db)

	recoveryCtx, cancelRecovery := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelRecovery()
	var persisted mongoRuntimeRecord
	_, _, err = db.RawQueryMongo(recoveryCtx, func(ctx context.Context, client *mongo.Client) error {
		return client.Database(mongoDatabase).Collection(collectionName).
			FindOne(ctx, bson.M{"_id": beforeRestart.ID}).Decode(&persisted)
	})
	if err != nil {
		t.Fatalf("read MongoDB record after runtime recovery: %v", err)
	}
	if persisted.Email != beforeRestart.Email || persisted.Name != beforeRestart.Name {
		t.Fatalf("MongoDB record after runtime recovery = %#v, want %#v", persisted, beforeRestart)
	}

	afterRestart := mongoRuntimeRecord{
		ID:    "after-" + beforeRestart.ID,
		Email: "after-" + beforeRestart.Email,
		Name:  "After restart",
	}
	internaltest.RequireNoORMError(t, db.Table(collectionName).Create(&afterRestart))
	var created mongoRuntimeRecord
	_, _, err = db.RawQueryMongo(recoveryCtx, func(ctx context.Context, client *mongo.Client) error {
		return client.Database(mongoDatabase).Collection(collectionName).
			FindOne(ctx, bson.M{"_id": afterRestart.ID}).Decode(&created)
	})
	if err != nil {
		t.Fatalf("read ORM-created MongoDB record after recovery: %v", err)
	}
	for _, id := range []string{beforeRestart.ID, afterRestart.ID} {
		_, _, err = db.RawQueryMongo(recoveryCtx, func(ctx context.Context, client *mongo.Client) error {
			_, err := client.Database(mongoDatabase).Collection(collectionName).
				DeleteOne(ctx, bson.M{"_id": id})
			return err
		})
		if err != nil {
			t.Errorf("remove MongoDB runtime recovery record %s: %v", id, err)
		}
	}
	t.Log("MongoDB runtime recovery: health check and bounded write failed while stopped; persisted data and new ORM writes succeeded after restart using the existing GOREST session")
}

func waitForMongoDatabase(t *testing.T, appDB *gorest.DBSession) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var lastErr error
	for {
		pingCtx, cancelPing := context.WithTimeout(ctx, 3*time.Second)
		_, _, lastErr = appDB.RawQueryMongo(pingCtx, func(ctx context.Context, client *mongo.Client) error {
			return client.Ping(ctx, nil)
		})
		cancelPing()
		if lastErr == nil {
			return
		}

		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatalf("MongoDB did not recover within the runtime test deadline: %v (last ping error: %v)", ctx.Err(), lastErr)
		case <-timer.C:
		}
	}
}
