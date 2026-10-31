package orm

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestDeleteSQLBulkDeletesMatchingSingleAndCompositeKeys(t *testing.T) {
	connection, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	execute := func(statement string) {
		t.Helper()
		if _, err := connection.Exec(statement); err != nil {
			t.Fatalf("execute setup statement %q: %v", statement, err)
		}
	}
	execute("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
	execute("INSERT INTO users (id, name) VALUES (1, 'one'), (2, 'two'), (3, 'keep')")

	type user struct {
		ID   int    `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
	table := "users"
	rowsToDelete := []user{{ID: 1}, {ID: 2}}
	query := (&ORM{
		Database:       &database.Database{SQL: connection},
		DatabaseConfig: &database.Config{Driver: database.SQLITE},
		StringBuilder:  &strings.Builder{},
		Table:          &table,
	}).Delete(&rowsToDelete)

	if query.Error != nil {
		t.Fatalf("bulk Delete() failed (HTTP %d): %v", query.HTTPCode, query.Error)
	}
	if query.HTTPCode != http.StatusOK || query.RowsAffected != 2 {
		t.Fatalf("bulk Delete() = HTTP %d, rows affected %d; want HTTP 200 and 2 rows",
			query.HTTPCode, query.RowsAffected)
	}

	var remainingID int
	var remainingName string
	if err := connection.QueryRow("SELECT id, name FROM users").Scan(&remainingID, &remainingName); err != nil {
		t.Fatalf("query remaining user: %v", err)
	}
	if remainingID != 3 || remainingName != "keep" {
		t.Fatalf("remaining row = (%d, %q), want (3, keep)", remainingID, remainingName)
	}

	execute("CREATE TABLE tenant_items (tenant_id TEXT NOT NULL, item_id TEXT NOT NULL, name TEXT NOT NULL, PRIMARY KEY (tenant_id, item_id))")
	execute("INSERT INTO tenant_items (tenant_id, item_id, name) VALUES ('tenant', 'one', 'one'), ('tenant', 'two', 'two'), ('other', 'keep', 'preserve')")

	type tenantItem struct {
		TenantID string `gorest:"tenant_id, primary_key"`
		ItemID   string `gorest:"item_id, primary_key"`
		Name     string `gorest:"name"`
	}
	compositeTable := "tenant_items"
	compositeRows := []tenantItem{{TenantID: "tenant", ItemID: "one"}, {TenantID: "tenant", ItemID: "two"}}
	query = (&ORM{
		Database:       &database.Database{SQL: connection},
		DatabaseConfig: &database.Config{Driver: database.SQLITE},
		StringBuilder:  &strings.Builder{},
		Table:          &compositeTable,
	}).Delete(&compositeRows)

	if query.Error != nil {
		t.Fatalf("composite-key bulk Delete() failed (HTTP %d): %v", query.HTTPCode, query.Error)
	}
	if query.HTTPCode != http.StatusOK || query.RowsAffected != 2 {
		t.Fatalf("composite-key bulk Delete() = HTTP %d, rows affected %d; want HTTP 200 and 2 rows",
			query.HTTPCode, query.RowsAffected)
	}

	var tenantID, itemID, name string
	if err := connection.QueryRow("SELECT tenant_id, item_id, name FROM tenant_items").Scan(&tenantID, &itemID, &name); err != nil {
		t.Fatalf("query remaining composite row: %v", err)
	}
	if tenantID != "other" || itemID != "keep" || name != "preserve" {
		t.Fatalf("remaining composite row = (%q, %q, %q), want (other, keep, preserve)",
			tenantID, itemID, name)
	}
}
