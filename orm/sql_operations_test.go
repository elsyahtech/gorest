package orm

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestSQLiteORMCreateFindUpdateAndDeleteLifecycle(t *testing.T) {
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
	if _, err := connection.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)"); err != nil {
		t.Fatalf("create users table: %v", err)
	}

	type user struct {
		ID   int    `gorest:"id, primary_key"`
		Name string `gorest:"name, required"`
	}
	table := "users"
	newORM := func() *ORM {
		return &ORM{
			Database:       &database.Database{SQL: connection},
			DatabaseConfig: &database.Config{Driver: database.SQLITE},
			StringBuilder:  &strings.Builder{},
			Table:          &table,
		}
	}

	created := user{ID: 1, Name: "initial"}
	query := newORM().Create(&created)
	if query.Error != nil || query.HTTPCode != http.StatusOK {
		t.Fatalf("Create() = HTTP %d, error %v", query.HTTPCode, query.Error)
	}

	var found user
	query = newORM().Where("id = ?", 1).Find(&found)
	if query.Error != nil || query.HTTPCode != http.StatusOK {
		t.Fatalf("Find() = HTTP %d, error %v", query.HTTPCode, query.Error)
	}
	if found != created {
		t.Fatalf("Find() result = %#v, want %#v", found, created)
	}

	for _, operation := range []struct {
		name string
		run  func() *ORM
	}{
		{
			name: "Find",
			run: func() *ORM {
				var result user
				return newORM().Where("id = ?", 1, "extra").Find(&result)
			},
		},
		{
			name: "Update",
			run: func() *ORM {
				value := user{ID: 1, Name: "must-not-write"}
				return newORM().Where("id = ?", 1, "extra").Update(&value)
			},
		},
		{
			name: "Delete",
			run: func() *ORM {
				return newORM().Where("id = ?", 1, "extra").Delete(&created)
			},
		},
	} {
		t.Run(operation.name+" rejects surplus WHERE arguments", func(t *testing.T) {
			query := operation.run()
			if query.Error == nil || query.HTTPCode != http.StatusBadRequest {
				t.Fatalf("operation result = (HTTP %d, %v), want HTTP 400 validation error",
					query.HTTPCode, query.Error)
			}
		})
	}

	found = user{}
	query = newORM().Where("id = ?", 1).Find(&found)
	if query.Error != nil || found != created {
		t.Fatalf("valid Find() after rejected argument counts = (%#v, %v), want %#v",
			found, query.Error, created)
	}

	updated := user{ID: 1, Name: "updated"}
	query = newORM().Update(&updated)
	if query.Error != nil || query.HTTPCode != http.StatusOK || query.RowsAffected != 1 {
		t.Fatalf("Update() = HTTP %d, rows affected %d, error %v; want HTTP 200 and one row",
			query.HTTPCode, query.RowsAffected, query.Error)
	}

	found = user{}
	query = newORM().Where("id = ?", 1).Find(&found)
	if query.Error != nil || found != updated {
		t.Fatalf("Find() after Update() = (%#v, %v), want %#v", found, query.Error, updated)
	}

	query = newORM().Delete(&updated)
	if query.Error != nil || query.HTTPCode != http.StatusOK || query.RowsAffected != 1 {
		t.Fatalf("Delete() = HTTP %d, rows affected %d, error %v; want HTTP 200 and one row",
			query.HTTPCode, query.RowsAffected, query.Error)
	}

	query = newORM().Delete(&updated)
	if query.Error == nil || query.HTTPCode != http.StatusNotFound {
		t.Fatalf("Delete() for a missing ID = HTTP %d, error %v; want HTTP 404",
			query.HTTPCode, query.Error)
	}
}
