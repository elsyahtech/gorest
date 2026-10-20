//go:build integration && integrationcontainers

package httpapi_test

import (
	"context"
	"fmt"
	"io"
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
	sqlServerImageDefault = "mcr.microsoft.com/mssql/server:2022-latest"
	sqlServerTestPassword = "GorestE2E-SQL-2026!Q7"
)

func TestSQLServerHTTPCRUDAndParameterBinding(t *testing.T) {
	if os.Getenv("GOREST_IT_SQLSERVER_ACCEPT_EULA") != "Y" {
		t.Fatal("set GOREST_IT_SQLSERVER_ACCEPT_EULA=Y only after reviewing and accepting Microsoft's SQL Server container license terms")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	image := strings.TrimSpace(os.Getenv("GOREST_IT_SQLSERVER_IMAGE"))
	if image == "" {
		image = sqlServerImageDefault
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: image,
			Env: map[string]string{
				"ACCEPT_EULA":       "Y",
				"MSSQL_SA_PASSWORD": sqlServerTestPassword,
				"MSSQL_PID":         "Developer",
			},
			ExposedPorts: []string{"1433/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("SQL Server is now ready for client connections.").WithStartupTimeout(8*time.Minute),
				wait.ForListeningPort("1433/tcp").WithStartupTimeout(2*time.Minute),
			).WithDeadline(9 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start SQL Server test container using %s: %v", image, err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate SQL Server test container: %v", err)
		}
	})
	containerConfig, err := container.Inspect(ctx)
	if err != nil {
		t.Fatalf("inspect SQL Server test container configuration: %v", err)
	}
	containerEnv := make(map[string]string, len(containerConfig.Config.Env))
	for _, variable := range containerConfig.Config.Env {
		key, value, ok := strings.Cut(variable, "=")
		if ok {
			containerEnv[key] = value
		}
	}
	if containerEnv["MSSQL_SA_PASSWORD"] != sqlServerTestPassword || containerEnv["ACCEPT_EULA"] != "Y" {
		t.Fatalf("SQL Server container did not receive expected startup settings (EULA accepted: %t, test password configured: %t)", containerEnv["ACCEPT_EULA"] == "Y", containerEnv["MSSQL_SA_PASSWORD"] == sqlServerTestPassword)
	}
	t.Logf("SQL Server container startup settings: EULA accepted=%t, PID=%q, SA password configured=%t, data mounts=%d", containerEnv["ACCEPT_EULA"] == "Y", containerEnv["MSSQL_PID"], containerEnv["MSSQL_SA_PASSWORD"] != "", len(containerConfig.Mounts))
	for _, mount := range containerConfig.Mounts {
		t.Logf("SQL Server container mount: type=%s destination=%s", mount.Type, mount.Destination)
	}
	waitForSQLServerLogin(t, ctx, container)

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get SQL Server container host: %v", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port, err := container.MappedPort(ctx, "1433/tcp")
	if err != nil {
		t.Fatalf("get SQL Server mapped port: %v", err)
	}
	t.Setenv("GOREST_IT_SQLSERVER_HOST", host)
	t.Setenv("GOREST_IT_SQLSERVER_PORT", port.Port())
	t.Setenv("GOREST_IT_SQLSERVER_NAME", "master")
	t.Setenv("GOREST_IT_SQLSERVER_USERNAME", "sa")
	t.Setenv("GOREST_IT_SQLSERVER_PASSWORD", sqlServerTestPassword)
	t.Setenv("GOREST_IT_SQLSERVER_TLS", "disable")
	t.Setenv("GOREST_IT_SQLSERVER_TRUST_SERVER_CERTIFICATE", "true")
	t.Setenv("GOREST_IT_REQUIRED", "1")
	t.Chdir(t.TempDir()) // Keep example application logs out of the repository.
	runMigrationProcessScenarios(t, database.SQLSERVER, container)

	migrationSeeder := internaltest.NewMigrationSeederFixture(t, database.SQLSERVER)
	app := internaltest.OpenAppWithConfig(t, database.SQLSERVER, migrationSeeder.Configure)
	migrationSeeder.Run(t, app)
	t.Logf("SQL Server image: %s", image)
	logSQLServerVersion(t, app.Database())

	table := fmt.Sprintf("gorest_e2e_users_%d", time.Now().UnixNano())
	runCRUDHTTPScenario(
		t,
		app.Database(),
		table,
		fmt.Sprintf("CREATE TABLE dbo.%s (id NVARCHAR(80) NOT NULL PRIMARY KEY, email NVARCHAR(320) NOT NULL UNIQUE, name NVARCHAR(200) NOT NULL)", table),
		"DROP TABLE dbo."+table,
	)
	internaltest.SQLParallelTransactions(t, database.SQLSERVER)
}

func waitForSQLServerLogin(t *testing.T, ctx context.Context, container testcontainers.Container) {
	t.Helper()
	command := []string{
		"/opt/mssql-tools18/bin/sqlcmd",
		"-C",
		"-S", "127.0.0.1",
		"-U", "sa",
		"-P", sqlServerTestPassword,
		"-Q", "SELECT 1",
	}
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	var lastError string
	for {
		exitCode, output, err := container.Exec(ctx, command)
		if err == nil {
			result, readErr := io.ReadAll(output)
			if readErr != nil {
				lastError = fmt.Sprintf("read sqlcmd output: %v", readErr)
			} else if exitCode == 0 {
				t.Log("SQL Server accepts the configured login")
				return
			} else {
				lastError = fmt.Sprintf("sqlcmd exit %d: %s", exitCode, strings.TrimSpace(string(result)))
			}
		} else {
			lastError = fmt.Sprintf("execute sqlcmd: %v", err)
		}

		select {
		case <-ctx.Done():
			t.Fatalf("SQL Server did not accept the configured login before the test deadline: %s", lastError)
		case <-deadline.C:
			t.Fatalf("SQL Server did not accept the configured login within 2 minutes: %s", lastError)
		case <-time.After(time.Second):
		}
	}
}

func logSQLServerVersion(t *testing.T, db *gorest.DBSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, message, code, err := db.QuerySQL(ctx, "SELECT @@VERSION")
	if err != nil {
		t.Fatalf("read SQL Server version (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	var version string
	if !rows.Next() {
		t.Fatalf("SQL Server version query returned no row: %v", rows.Err())
	}
	if err := rows.Scan(&version); err != nil {
		t.Fatalf("scan SQL Server version: %v", err)
	}
	t.Logf("SQL Server server version: %s", version)
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate SQL Server version result: %v", err)
	}
}
