package orm

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestFindByMissingIDReturnsEmptyResultWithoutError(t *testing.T) {
	sqlDB, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	if _, err := sqlDB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create users table: %v", err)
	}
	if _, err := sqlDB.Exec("INSERT INTO users (id) VALUES (?)", 1); err != nil {
		t.Fatalf("insert existing user: %v", err)
	}

	type user struct {
		ID int `gorest:"id, primary_key"`
	}

	table := "users"
	var found []user
	query := (&ORM{
		Database:       &database.Database{SQL: sqlDB},
		DatabaseConfig: &database.Config{Driver: database.SQLITE},
		StringBuilder:  &strings.Builder{},
		Table:          &table,
	}).Where("id = ?", 404).Find(&found)

	if query.Error != nil {
		t.Fatalf("Find() for missing ID returned HTTP %d and error: %v", query.HTTPCode, query.Error)
	}
	if query.HTTPCode != http.StatusOK {
		t.Fatalf("Find() HTTP status = %d, want %d", query.HTTPCode, http.StatusOK)
	}
	if len(found) != 0 {
		t.Fatalf("Find() returned %#v for an ID that does not exist, want an empty result", found)
	}
}
