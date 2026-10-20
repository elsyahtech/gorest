//go:build integration && integrationcontainers

package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	scyllaImageDefault = "scylladb/scylla:2025.1"
	scyllaKeyspace     = "gorest_integration"
)

type scyllaUser struct {
	ID    string `gorest:"id, primary_key, required" json:"id"`
	Email string `gorest:"email, required" json:"email"`
	Name  string `gorest:"name, required" json:"name"`
}

type scyllaUserHandler struct {
	db    *gorest.DBSession
	table string
}

func TestScyllaHTTPCRUDAndParameterBinding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	image := strings.TrimSpace(os.Getenv("GOREST_IT_SCYLLA_IMAGE"))
	if image == "" {
		image = scyllaImageDefault
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			Cmd:          []string{"--smp", "1", "--memory", "1G", "--overprovisioned", "1"},
			ExposedPorts: []string{"9042/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("Starting listening for CQL clients").WithStartupTimeout(12*time.Minute),
				wait.ForListeningPort("9042/tcp").WithStartupTimeout(2*time.Minute),
			).WithDeadline(13 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start ScyllaDB test container using %s: %v", image, err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate ScyllaDB test container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get ScyllaDB container host: %v", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port, err := container.MappedPort(ctx, "9042/tcp")
	if err != nil {
		t.Fatalf("get ScyllaDB mapped port: %v", err)
	}
	createScyllaKeyspace(t, ctx, container)
	t.Setenv("GOREST_IT_SCYLLA_HOST", host)
	t.Setenv("GOREST_IT_SCYLLA_PORT", port.Port())
	t.Setenv("GOREST_IT_SCYLLA_NAME", scyllaKeyspace)
	t.Setenv("GOREST_IT_REQUIRED", "1")
	t.Chdir(t.TempDir()) // Keep example application logs out of the repository.
	runMigrationProcessScenarios(t, database.SCYLLA, container)

	migrationSeeder := internaltest.NewMigrationSeederFixture(t, database.SCYLLA)
	app := internaltest.OpenAppWithConfig(t, database.SCYLLA, migrationSeeder.Configure)
	migrationSeeder.Run(t, app)
	t.Logf("ScyllaDB image: %s", image)
	logScyllaVersion(t, app.Database())

	table := fmt.Sprintf("gorest_e2e_users_%d", time.Now().UnixNano())
	ctxSchema, cancelSchema := context.WithTimeout(context.Background(), 15*time.Second)
	message, code, err := app.Database().ExecCQL(ctxSchema,
		"CREATE TABLE "+table+" (id text PRIMARY KEY, email text, name text)")
	cancelSchema()
	if err != nil {
		t.Fatalf("create ScyllaDB test table (HTTP %d, %s): %v", code, message, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, _, err := app.Database().ExecCQL(cleanupCtx, "DROP TABLE "+table); err != nil {
			t.Errorf("drop ScyllaDB test table: %v", err)
		}
	})

	runScyllaCRUDHTTPScenario(t, app.Database(), table)
}

func createScyllaKeyspace(t *testing.T, ctx context.Context, container testcontainers.Container) {
	t.Helper()
	query := "CREATE KEYSPACE " + scyllaKeyspace + " WITH replication = {'class':'SimpleStrategy','replication_factor':1}"
	exitCode, output, err := container.Exec(ctx, []string{"cqlsh", "-e", query})
	if err != nil {
		t.Fatalf("create temporary ScyllaDB keyspace: %v", err)
	}
	if exitCode != 0 {
		body, _ := io.ReadAll(output)
		t.Fatalf("create temporary ScyllaDB keyspace exited %d: %s", exitCode, body)
	}
}

func runScyllaCRUDHTTPScenario(t *testing.T, db *gorest.DBSession, table string) {
	t.Helper()
	httpServer, _, err := server.New(&server.Config{
		Limiter: &limiter.Config{Max: 1000, Expiration: time.Minute},
	})
	if err != nil {
		t.Fatalf("create GOREST HTTP server: %v", err)
	}
	handler := &scyllaUserHandler{db: db, table: table}
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
		t.Fatalf("decode ScyllaDB search response: %v", err)
	}
	encodedData, err := json.Marshal(searchResult.Data)
	if err != nil {
		t.Fatalf("encode ScyllaDB search data: %v", err)
	}
	var searched []scyllaUser
	if err := json.Unmarshal(encodedData, &searched); err != nil {
		t.Fatalf("decode ScyllaDB search data: %v", err)
	}
	if len(searched) != 0 {
		t.Fatalf("injection-shaped email matched ScyllaDB records: %#v", searched)
	}

	client.request(http.MethodPost, "/users", `{"id":"user-3","name":"Missing email"}`, http.StatusBadRequest)
	// ScyllaDB Create uses INSERT IF NOT EXISTS against the full primary key.
	client.request(http.MethodPost, "/users", `{"id":"user-1","email":"alice2@example.test","name":"Duplicate ID"}`, http.StatusConflict)
	client.request(http.MethodPut, "/users/user-1", `{"email":"alice-updated@example.test","name":"Alice Updated"}`, http.StatusOK)

	getResponse := client.request(http.MethodGet, "/users/user-1", "", http.StatusOK)
	var loaded apiResponse
	if err := json.Unmarshal(getResponse, &loaded); err != nil {
		t.Fatalf("decode ScyllaDB GET response: %v", err)
	}
	loadedJSON, err := json.Marshal(loaded.Data)
	if err != nil {
		t.Fatalf("encode ScyllaDB GET data: %v", err)
	}
	var loadedUser scyllaUser
	if err := json.Unmarshal(loadedJSON, &loadedUser); err != nil {
		t.Fatalf("decode ScyllaDB GET data: %v", err)
	}
	if loadedUser.ID != "user-1" || loadedUser.Email != "alice-updated@example.test" || loadedUser.Name != "Alice Updated" {
		t.Fatalf("updated ScyllaDB user = %#v", loadedUser)
	}

	client.request(http.MethodDelete, "/users/user-1", "", http.StatusOK)
	client.request(http.MethodGet, "/users/user-1", "", http.StatusNotFound)

	var remaining []scyllaUser
	query := db.Table(table).Find(&remaining)
	if query.Error != nil {
		t.Fatalf("verify persisted ScyllaDB rows (HTTP %d): %v", query.HTTPCode, query.Error)
	}
	if len(remaining) != 1 || remaining[0].ID != "user-2" {
		t.Fatalf("remaining ScyllaDB rows = %#v, want only user-2", remaining)
	}

	runConcurrentHTTPCRUDScenario(t, client)
	remaining = nil
	query = db.Table(table).Find(&remaining)
	if query.Error != nil {
		t.Fatalf("verify ScyllaDB rows after concurrent HTTP requests (HTTP %d): %v", query.HTTPCode, query.Error)
	}
	if len(remaining) != 1 || remaining[0].ID != "user-2" {
		t.Fatalf("concurrent HTTP requests left unexpected ScyllaDB rows: %#v", remaining)
	}
}

func (handler *scyllaUserHandler) create(ctx *server.Context) error {
	var payload scyllaUser
	if err := ctx.Bind().Body(&payload); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(apiResponse{Error: "invalid JSON payload"})
	}
	query := handler.db.Table(handler.table).Create(&payload)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusCreated).JSON(apiResponse{Data: payload})
}

func (handler *scyllaUserHandler) search(ctx *server.Context) error {
	var users []scyllaUser
	query := handler.db.Table(handler.table).
		Select("id, email, name").
		Where("email = ?", ctx.Query("email")).
		AllowFiltering().
		Find(&users)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Data: users})
}

func (handler *scyllaUserHandler) getByID(ctx *server.Context) error {
	var found scyllaUser
	query := handler.db.Table(handler.table).
		Where("id = ?", ctx.Params("id")).
		Find(&found)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Data: found})
}

func (handler *scyllaUserHandler) update(ctx *server.Context) error {
	var payload scyllaUser
	if err := ctx.Bind().Body(&payload); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(apiResponse{Error: "invalid JSON payload"})
	}
	payload.ID = ctx.Params("id")
	query := handler.db.Table(handler.table).
		Where("id = ?", payload.ID).
		Select("email, name").
		Update(&payload)
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Data: payload})
}

func (handler *scyllaUserHandler) delete(ctx *server.Context) error {
	id := ctx.Params("id")
	query := handler.db.Table(handler.table).
		Where("id = ?", id).
		Delete(&scyllaUser{ID: id})
	if query.Error != nil {
		return writeORMError(ctx, query)
	}
	return ctx.Status(http.StatusOK).JSON(apiResponse{Message: "user deleted"})
}

func logScyllaVersion(t *testing.T, db *gorest.DBSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var version string
	_, _, err := db.RawQueryScylla(ctx, func(ctx context.Context, session *gocql.Session) error {
		return session.Query("SELECT release_version FROM system.local").WithContext(ctx).Scan(&version)
	})
	if err != nil {
		t.Fatalf("read ScyllaDB server version: %v", err)
	}
	t.Logf("ScyllaDB server version: %s", version)
}
