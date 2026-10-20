//go:build integration && integrationcontainers

package httpapi_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	mysqlImageDefault = "mysql:8.4"
	mysqlDatabase     = "gorest_integration"
	mysqlUsername     = "gorest_test"
	mysqlPassword     = "gorest_test_only"
	mysqlRootPassword = "gorest_root_test_only"
)

func TestMySQLHTTPCRUDAndParameterBinding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	image := strings.TrimSpace(os.Getenv("GOREST_IT_MYSQL_IMAGE"))
	if image == "" {
		image = mysqlImageDefault
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: image,
			Env: map[string]string{
				"MYSQL_DATABASE":      mysqlDatabase,
				"MYSQL_USER":          mysqlUsername,
				"MYSQL_PASSWORD":      mysqlPassword,
				"MYSQL_ROOT_PASSWORD": mysqlRootPassword,
			},
			ExposedPorts: []string{"3306/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("/usr/sbin/mysqld: ready for connections").WithOccurrence(2).WithStartupTimeout(8*time.Minute),
				wait.ForListeningPort("3306/tcp").WithStartupTimeout(2*time.Minute),
			).WithDeadline(9 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start MySQL test container using %s: %v", image, err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate MySQL test container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get MySQL container host: %v", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		t.Fatalf("get MySQL mapped port: %v", err)
	}
	t.Setenv("GOREST_IT_MYSQL_HOST", host)
	t.Setenv("GOREST_IT_MYSQL_PORT", port.Port())
	t.Setenv("GOREST_IT_MYSQL_NAME", mysqlDatabase)
	t.Setenv("GOREST_IT_MYSQL_USERNAME", mysqlUsername)
	t.Setenv("GOREST_IT_MYSQL_PASSWORD", mysqlPassword)
	t.Setenv("GOREST_IT_MYSQL_TLS", database.TLSDisabled)
	t.Setenv("GOREST_IT_REQUIRED", "1")
	t.Chdir(t.TempDir()) // Keep example application logs out of the repository.
	runMigrationProcessScenarios(t, database.MYSQL, container)

	migrationSeeder := internaltest.NewMigrationSeederFixture(t, database.MYSQL)
	app := internaltest.OpenAppWithConfig(t, database.MYSQL, migrationSeeder.Configure)
	migrationSeeder.Run(t, app)
	t.Logf("MySQL image: %s", image)
	logMySQLVersion(t, app.Database())

	table := fmt.Sprintf("gorest_e2e_users_%d", time.Now().UnixNano())
	runCRUDHTTPScenario(
		t,
		app.Database(),
		table,
		fmt.Sprintf("CREATE TABLE %s (id VARCHAR(80) NOT NULL PRIMARY KEY, email VARCHAR(320) NOT NULL UNIQUE, name VARCHAR(200) NOT NULL)", table),
		"DROP TABLE IF EXISTS "+table,
	)
	internaltest.SQLParallelTransactions(t, database.MYSQL)
}

func logMySQLVersion(t *testing.T, db *gorest.DBSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, message, code, err := db.QuerySQL(ctx, "SELECT VERSION()")
	if err != nil {
		t.Fatalf("read MySQL version (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	var version string
	if !rows.Next() {
		t.Fatalf("MySQL version query returned no row: %v", rows.Err())
	}
	if err := rows.Scan(&version); err != nil {
		t.Fatalf("scan MySQL version: %v", err)
	}
	t.Logf("MySQL server version: %s", version)
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate MySQL version result: %v", err)
	}
}
