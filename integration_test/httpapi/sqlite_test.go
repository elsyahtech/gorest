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
)

func TestSQLiteHTTPCRUDAndParameterBinding(t *testing.T) {
	t.Chdir(t.TempDir()) // Keep example application logs and SQLite files out of the repository.
	app := internaltest.OpenApp(t, database.SQLITE)
	logSQLiteVersion(t, app.Database())

	table := fmt.Sprintf("gorest_e2e_users_%d", time.Now().UnixNano())
	runCRUDHTTPScenario(
		t,
		app.Database(),
		table,
		fmt.Sprintf("CREATE TABLE %s (id VARCHAR(80) NOT NULL PRIMARY KEY, email VARCHAR(320) NOT NULL UNIQUE, name VARCHAR(200) NOT NULL)", table),
		"DROP TABLE IF EXISTS "+table,
	)
	internaltest.SQLParallelTransactions(t, database.SQLITE)
}

func logSQLiteVersion(t *testing.T, db *gorest.DBSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, message, code, err := db.QuerySQL(ctx, "SELECT sqlite_version()")
	if err != nil {
		t.Fatalf("read SQLite version (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	var version string
	if !rows.Next() {
		t.Fatalf("SQLite version query returned no row: %v", rows.Err())
	}
	if err := rows.Scan(&version); err != nil {
		t.Fatalf("scan SQLite version: %v", err)
	}
	t.Logf("SQLite server version: %s", version)
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate SQLite version result: %v", err)
	}
}
