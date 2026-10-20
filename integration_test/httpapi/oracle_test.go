//go:build integration && integrationcontainers

package httpapi_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	oracleImageDefault = "gvenzl/oracle-free:23.26.3-slim-faststart"
	oracleService      = "FREEPDB1"
	oracleAppUser      = "gorest_test"
	oracleAppPassword  = "gorest_test_only"
	oracleSysPassword  = "gorest_sys_test_only"
)

func TestOracleHTTPCRUDAndParameterBinding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	image := strings.TrimSpace(os.Getenv("GOREST_IT_ORACLE_IMAGE"))
	if image == "" {
		image = oracleImageDefault
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: image,
			Env: map[string]string{
				"ORACLE_PASSWORD":   oracleSysPassword,
				"APP_USER":          oracleAppUser,
				"APP_USER_PASSWORD": oracleAppPassword,
			},
			ExposedPorts: []string{"1521/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("DATABASE IS READY TO USE!").WithStartupTimeout(12*time.Minute),
				wait.ForListeningPort("1521/tcp").WithStartupTimeout(2*time.Minute),
			).WithDeadline(14 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start Oracle test container using %s: %v", image, err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate Oracle test container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get Oracle test container host: %v", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port, err := container.MappedPort(ctx, "1521/tcp")
	if err != nil {
		t.Fatalf("get Oracle mapped port: %v", err)
	}

	libDir := discoverOracleClientDir()
	if libDir == "" {
		t.Fatal("Oracle Instant Client is required by GOREST's godror driver; set GOREST_IT_ORACLE_LIB_DIR to its directory")
	}
	t.Setenv("GOREST_IT_ORACLE_HOST", host)
	t.Setenv("GOREST_IT_ORACLE_PORT", port.Port())
	t.Setenv("GOREST_IT_ORACLE_NAME", oracleService)
	t.Setenv("GOREST_IT_ORACLE_USERNAME", oracleAppUser)
	t.Setenv("GOREST_IT_ORACLE_PASSWORD", oracleAppPassword)
	t.Setenv("GOREST_IT_ORACLE_TLS", "disable")
	t.Setenv("GOREST_IT_ORACLE_LIB_DIR", libDir)
	t.Setenv("GOREST_IT_REQUIRED", "1")
	t.Chdir(t.TempDir()) // Keep example application logs out of the repository.
	runMigrationProcessScenarios(t, database.ORACLE, container)

	migrationSeeder := internaltest.NewMigrationSeederFixture(t, database.ORACLE)
	app := internaltest.OpenAppWithConfig(t, database.ORACLE, migrationSeeder.Configure)
	migrationSeeder.Run(t, app)
	t.Logf("Oracle image: %s", image)
	t.Logf("Oracle Instant Client configured: true")
	logOracleVersion(t, app.Database())

	table := fmt.Sprintf("GOREST_E2E_USERS_%d", time.Now().UnixNano())
	runCRUDHTTPScenario(
		t,
		app.Database(),
		table,
		fmt.Sprintf("CREATE TABLE %s (id VARCHAR2(80) PRIMARY KEY, email VARCHAR2(320) NOT NULL UNIQUE, name VARCHAR2(200) NOT NULL)", table),
		"DROP TABLE "+table+" PURGE",
	)
	internaltest.SQLParallelTransactions(t, database.ORACLE)
}

func discoverOracleClientDir() string {
	candidates := []string{strings.TrimSpace(os.Getenv("GOREST_IT_ORACLE_LIB_DIR"))}
	if oracleHome := strings.TrimSpace(os.Getenv("ORACLE_HOME")); oracleHome != "" {
		candidates = append(candidates, oracleHome, filepath.Join(oracleHome, "lib"))
	}
	for _, variable := range []string{"DYLD_LIBRARY_PATH", "LD_LIBRARY_PATH"} {
		for _, dir := range filepath.SplitList(os.Getenv(variable)) {
			candidates = append(candidates, dir)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		matches, _ := filepath.Glob(filepath.Join(home, "oracle", "instantclient*"))
		candidates = append(candidates, matches...)
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/opt/homebrew/lib", "/usr/local/lib")
	} else if runtime.GOOS == "linux" {
		candidates = append(candidates, "/usr/lib", "/usr/lib64", "/usr/local/lib")
	}

	library := "libclntsh.so"
	if runtime.GOOS == "darwin" {
		library = "libclntsh.dylib"
	} else if runtime.GOOS == "windows" {
		library = "oci.dll"
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, library)); err == nil && !info.IsDir() {
			return dir
		}
		// Oracle Instant Client archives often include a versioned library and
		// a matching unversioned symlink; accept the versioned file if needed.
		matches, _ := filepath.Glob(filepath.Join(dir, library+".*"))
		if len(matches) > 0 {
			return dir
		}
	}
	return ""
}

func logOracleVersion(t *testing.T, db *gorest.DBSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, message, code, err := db.QuerySQL(ctx, "SELECT banner FROM v$version WHERE ROWNUM = 1")
	if err != nil {
		t.Fatalf("read Oracle version (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	var version string
	if !rows.Next() {
		t.Fatalf("Oracle version query returned no row: %v", rows.Err())
	}
	if err := rows.Scan(&version); err != nil {
		t.Fatalf("scan Oracle version: %v", err)
	}
	t.Logf("Oracle server version: %s", version)
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate Oracle version result: %v", err)
	}
}
