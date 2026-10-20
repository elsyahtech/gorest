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
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresImage = "postgres:16-alpine"

func TestPostgresHandlerCRUDAndParameterBinding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := postgres.Run(
		ctx,
		postgresImage,
		postgres.WithDatabase("gorest_integration"),
		postgres.WithUsername("gorest_test"),
		postgres.WithPassword("gorest_test_only"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL test container: %v", err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate PostgreSQL test container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get PostgreSQL container host: %v", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("get PostgreSQL mapped port: %v", err)
	}
	t.Setenv("GOREST_IT_POSTGRES_HOST", host)
	t.Setenv("GOREST_IT_POSTGRES_PORT", port.Port())
	t.Setenv("GOREST_IT_POSTGRES_NAME", "gorest_integration")
	t.Setenv("GOREST_IT_POSTGRES_USERNAME", "gorest_test")
	t.Setenv("GOREST_IT_POSTGRES_PASSWORD", "gorest_test_only")
	t.Setenv("GOREST_IT_POSTGRES_TLS", "disable")
	t.Setenv("GOREST_IT_REQUIRED", "1")
	t.Chdir(t.TempDir()) // Keep example application logs out of the repository.

	app := internaltest.OpenApp(t, database.POSTGRES)
	t.Logf("PostgreSQL image: %s", postgresImage)
	logPostgresVersion(t, app.Database())

	table := fmt.Sprintf("gorest_e2e_users_%d", time.Now().UnixNano())
	runCRUDHTTPScenario(
		t,
		app.Database(),
		table,
		fmt.Sprintf("CREATE TABLE %s (id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, name TEXT NOT NULL)", table),
		"DROP TABLE IF EXISTS "+table,
	)
	internaltest.SQLParallelTransactions(t, database.POSTGRES)
}

func logPostgresVersion(t *testing.T, db *gorest.DBSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, message, code, err := db.QuerySQL(ctx, "SELECT version()")
	if err != nil {
		t.Fatalf("read PostgreSQL version (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	var version string
	if !rows.Next() {
		t.Fatalf("PostgreSQL version query returned no row: %v", rows.Err())
	}
	if err := rows.Scan(&version); err != nil {
		t.Fatalf("scan PostgreSQL version: %v", err)
	}
	t.Logf("PostgreSQL server version: %s", version)
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate PostgreSQL version result: %v", err)
	}
}
