//go:build integration && integrationcontainers

package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	mongoImageDefault = "mongo:7.0"
	mongoDatabase     = "gorest_integration"
)

type mongoUser struct {
	ID    string `gorest:"_id, primary_key, required" json:"id"`
	Email string `gorest:"email, required" json:"email"`
	Name  string `gorest:"name, required" json:"name"`
}

type mongoPreloadDepartment struct {
	ID   string `gorest:"_id, primary_key" bson:"_id"`
	Name string `gorest:"name" bson:"name"`
}

type mongoPreloadOrder struct {
	ID     string `gorest:"_id, primary_key" bson:"_id"`
	UserID string `gorest:"user_id" bson:"user_id"`
	Status string `gorest:"status" bson:"status"`
}

type mongoPreloadUser struct {
	ID           string                  `gorest:"_id, primary_key" bson:"_id"`
	DepartmentID string                  `gorest:"department_id" bson:"department_id"`
	Department   *mongoPreloadDepartment `gorest:"-" references:"ID" bson:"-"`
	Orders       []mongoPreloadOrder     `gorest:"-" references:"UserID" bson:"-"`
}

type mongoUserHandler struct {
	db         *gorest.DBSession
	collection string
}

func TestMongoHTTPCRUDAndParameterBinding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	image := strings.TrimSpace(os.Getenv("GOREST_IT_MONGO_IMAGE"))
	if image == "" {
		image = mongoImageDefault
	}
	hostPort := availableLoopbackPort(t)
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{"27017/tcp"},
			HostConfigModifier: func(config *dockercontainer.HostConfig) {
				config.PortBindings = nat.PortMap{
					"27017/tcp": {{HostIP: "127.0.0.1", HostPort: hostPort}},
				}
			},
			WaitingFor: wait.ForListeningPort("27017/tcp").WithStartupTimeout(8 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start MongoDB test container using %s: %v", image, err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate MongoDB test container: %v", err)
		}
	})

	t.Setenv("GOREST_IT_MONGO_HOST", "127.0.0.1")
	t.Setenv("GOREST_IT_MONGO_PORT", hostPort)
	t.Setenv("GOREST_IT_MONGO_NAME", mongoDatabase)
	t.Setenv("GOREST_IT_MONGO_TLS", "false")
	t.Setenv("GOREST_IT_REQUIRED", "1")
	t.Chdir(t.TempDir()) // Keep example application logs out of the repository.
	runMigrationProcessScenarios(t, database.MONGO, container)

	migrationSeeder := internaltest.NewMigrationSeederFixture(t, database.MONGO)
	app := internaltest.OpenAppWithConfig(t, database.MONGO, migrationSeeder.Configure)
	migrationSeeder.Run(t, app)
	t.Logf("MongoDB image: %s", image)
	logMongoVersion(t, app.Database())

	collectionName := fmt.Sprintf("gorest_e2e_users_%d", time.Now().UnixNano())
	collection, message, code, err := app.Database().MongoCollection(collectionName)
	if err != nil {
		t.Fatalf("get MongoDB test collection (HTTP %d, %s): %v", code, message, err)
	}
	indexCtx, indexCancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, err = collection.Indexes().CreateOne(indexCtx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	indexCancel()
	if err != nil {
		t.Fatalf("create unique MongoDB email index: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := collection.Drop(cleanupCtx); err != nil {
			t.Errorf("drop MongoDB test collection: %v", err)
		}
	})

	runMongoRuntimeRecovery(t, app, container)
	runMongoCRUDHTTPScenario(t, app.Database(), collectionName)
	runMongoPreloadScenario(t, app.Database())
	internaltest.MongoORMCoverage(t)
}

func runMongoPreloadScenario(t *testing.T, db *gorest.DBSession) {
	t.Helper()

	const (
		usersCollection       = "gorest_preload_users"
		departmentsCollection = "departments"
		ordersCollection      = "orders"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	users, message, code, err := db.MongoCollection(usersCollection)
	if err != nil {
		t.Fatalf("open Mongo preload users collection (HTTP %d, %s): %v", code, message, err)
	}
	departments, message, code, err := db.MongoCollection(departmentsCollection)
	if err != nil {
		t.Fatalf("open Mongo preload departments collection (HTTP %d, %s): %v", code, message, err)
	}
	orders, message, code, err := db.MongoCollection(ordersCollection)
	if err != nil {
		t.Fatalf("open Mongo preload orders collection (HTTP %d, %s): %v", code, message, err)
	}
	t.Cleanup(func() {
		for _, collection := range []*mongo.Collection{users, departments, orders} {
			if err := collection.Drop(context.Background()); err != nil {
				t.Errorf("drop Mongo preload collection: %v", err)
			}
		}
	})

	if _, err := departments.InsertOne(ctx, mongoPreloadDepartment{ID: "department-1", Name: "Engineering"}); err != nil {
		t.Fatalf("insert Mongo preload department: %v", err)
	}
	if _, err := users.InsertOne(ctx, mongoPreloadUser{ID: "user-1", DepartmentID: "department-1"}); err != nil {
		t.Fatalf("insert Mongo preload user: %v", err)
	}
	if _, err := orders.InsertMany(ctx, []any{
		mongoPreloadOrder{ID: "order-1", UserID: "user-1", Status: "active"},
		mongoPreloadOrder{ID: "order-2", UserID: "user-1", Status: "inactive"},
	}); err != nil {
		t.Fatalf("insert Mongo preload orders: %v", err)
	}

	var loaded []mongoPreloadUser
	query := db.Table(usersCollection).
		Preload("Department").
		Preload("Orders", "orders.status = ?", "active").
		Find(&loaded)
	if query.Error != nil {
		t.Fatalf("Mongo belongs-to and has-many preload failed (HTTP %d): %v", query.HTTPCode, query.Error)
	}
	if len(loaded) != 1 {
		t.Fatalf("Mongo preload returned %d users, want 1", len(loaded))
	}
	if loaded[0].Department == nil || loaded[0].Department.ID != "department-1" ||
		loaded[0].Department.Name != "Engineering" {
		t.Fatalf("Mongo belongs-to preload = %#v", loaded[0].Department)
	}
	if len(loaded[0].Orders) != 1 || loaded[0].Orders[0].ID != "order-1" {
		t.Fatalf("filtered Mongo has-many preload = %#v, want only active order", loaded[0].Orders)
	}
}

func runMongoCRUDHTTPScenario(t *testing.T, db *gorest.DBSession, collection string) {
	t.Helper()
	httpServer, _, err := server.New(&server.Config{
		Limiter: &limiter.Config{Max: 1000, Expiration: time.Minute},
	})
	if err != nil {
		t.Fatalf("create GOREST HTTP server: %v", err)
	}
	handler := &mongoUserHandler{db: db, collection: collection}
	httpServer.Post("/users", handler.create)
	httpServer.Get("/users/search", handler.search)
	httpServer.Get("/users/:id", handler.getByID)
	httpServer.Put("/users/:id", handler.update)
	httpServer.Delete("/users/:id", handler.delete)
	client := httpClient{t: t, app: httpServer.SRV}

	client.request(http.MethodPost, "/users", `{"id":"user-1","email":"alice@example.test","name":"Alice"}`, http.StatusCreated)
	client.request(http.MethodPost, "/users", `{"id":"user-2","email":"bob@example.test","name":"Bob"}`, http.StatusCreated)

	injection := "alice@example.test' OR '1'='1"
	searchPath := "/users/search?email=" + url.QueryEscape(injection)
	searchResponse := client.request(http.MethodGet, searchPath, "", http.StatusOK)
	var searchResult apiResponse
	if err := json.Unmarshal(searchResponse, &searchResult); err != nil {
		t.Fatalf("decode MongoDB search response: %v", err)
	}
	encodedData, err := json.Marshal(searchResult.Data)
	if err != nil {
		t.Fatalf("encode MongoDB search data: %v", err)
	}
	var searched []mongoUser
	if err := json.Unmarshal(encodedData, &searched); err != nil {
		t.Fatalf("decode MongoDB search data: %v", err)
	}
	if len(searched) != 0 {
		t.Fatalf("injection-shaped email matched MongoDB records: %#v", searched)
	}

	client.request(http.MethodPost, "/users", `{"id":"user-3","name":"Missing email"}`, http.StatusBadRequest)
	client.request(http.MethodPost, "/users", `{"id":"user-4","email":"alice@example.test","name":"Duplicate"}`, http.StatusConflict)
	client.request(http.MethodPut, "/users/user-1", `{"email":"alice-updated@example.test","name":"Alice Updated"}`, http.StatusOK)

	getResponse := client.request(http.MethodGet, "/users/user-1", "", http.StatusOK)
	var loaded apiResponse
	if err := json.Unmarshal(getResponse, &loaded); err != nil {
		t.Fatalf("decode MongoDB GET response: %v", err)
	}
	loadedJSON, err := json.Marshal(loaded.Data)
	if err != nil {
		t.Fatalf("encode MongoDB GET data: %v", err)
	}
	var loadedUser mongoUser
	if err := json.Unmarshal(loadedJSON, &loadedUser); err != nil {
		t.Fatalf("decode MongoDB GET data: %v", err)
	}
	if loadedUser.ID != "user-1" || loadedUser.Email != "alice-updated@example.test" || loadedUser.Name != "Alice Updated" {
		t.Fatalf("updated MongoDB user = %#v", loadedUser)
	}

	client.request(http.MethodDelete, "/users/user-1", "", http.StatusOK)
	client.request(http.MethodGet, "/users/user-1", "", http.StatusNotFound)

	var remaining []mongoUser
	query := db.Table(collection).Find(&remaining)
	if query.Error != nil {
		t.Fatalf("verify persisted MongoDB documents (HTTP %d): %v", query.HTTPCode, query.Error)
	}
	if len(remaining) != 1 || remaining[0].ID != "user-2" {
		t.Fatalf("remaining MongoDB documents = %#v, want only user-2", remaining)
	}

	runConcurrentHTTPCRUDScenario(t, client)
	remaining = nil
	query = db.Table(collection).Find(&remaining)
	if query.Error != nil {
		t.Fatalf("verify MongoDB documents after concurrent HTTP requests (HTTP %d): %v", query.HTTPCode, query.Error)
	}
	if len(remaining) != 1 || remaining[0].ID != "user-2" {
		t.Fatalf("concurrent HTTP requests left unexpected MongoDB documents: %#v", remaining)
	}
}

func (handler *mongoUserHandler) create(ctx *server.Context) error {
	var payload mongoUser
	if err := ctx.Bind().Body(&payload); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(apiResponse{Error: "invalid JSON payload"})
	}
	query := handler.db.Table(handler.collection).Create(&payload)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusCreated).JSON(apiResponse{Data: payload})
}

func (handler *mongoUserHandler) search(ctx *server.Context) error {
	var users []mongoUser
	query := handler.db.Table(handler.collection).
		Select("_id, email, name").
		Where("email = ?", ctx.Query("email")).
		Find(&users)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Data: users})
}

func (handler *mongoUserHandler) getByID(ctx *server.Context) error {
	var found mongoUser
	query := handler.db.Table(handler.collection).
		Where("_id = ?", ctx.Params("id")).
		Find(&found)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Data: found})
}

func (handler *mongoUserHandler) update(ctx *server.Context) error {
	var payload mongoUser
	if err := ctx.Bind().Body(&payload); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(apiResponse{Error: "invalid JSON payload"})
	}
	payload.ID = ctx.Params("id")
	query := handler.db.Table(handler.collection).
		Where("_id = ?", payload.ID).
		Select("email, name").
		Update(&payload)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Data: payload})
}

func (handler *mongoUserHandler) delete(ctx *server.Context) error {
	id := ctx.Params("id")
	query := handler.db.Table(handler.collection).
		Where("_id = ?", id).
		Delete(&mongoUser{ID: id})
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Message: "user deleted"})
}

func logMongoVersion(t *testing.T, db *gorest.DBSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var versionInfo struct {
		Version string `bson:"version"`
	}
	_, _, err := db.RawQueryMongo(ctx, func(ctx context.Context, client *mongo.Client) error {
		return client.Database(mongoDatabase).RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&versionInfo)
	})
	if err != nil {
		t.Fatalf("read MongoDB server version: %v", err)
	}
	t.Logf("MongoDB server version: %s", versionInfo.Version)
}
