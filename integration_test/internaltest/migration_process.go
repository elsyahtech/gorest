//go:build integration

package internaltest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// MigrationProcessFixture creates two ordered migration files so process tests
// can pause after the first has committed and the second is marked running.
type MigrationProcessFixture struct {
	Driver       string
	Directory    string
	FirstName    string
	SecondName   string
	FirstObject  string
	SecondObject string
}

func NewMigrationProcessFixture(t *testing.T, driver string) *MigrationProcessFixture {
	t.Helper()
	token := uniqueIdentifier(t, "")[:8]
	fixture := &MigrationProcessFixture{
		Driver:       driver,
		Directory:    t.TempDir(),
		FirstName:    "001_" + token + "_first.sql",
		SecondName:   "002_" + token + "_second.sql",
		FirstObject:  "g_it_p_" + token + "_a",
		SecondObject: "g_it_p_" + token + "_b",
	}
	if driver == database.MONGO {
		fixture.FirstName = "001_" + token + "_first.json"
		fixture.SecondName = "002_" + token + "_second.json"
		fixture.FirstObject = "g_it_p_" + token + "_a"
		fixture.SecondObject = "g_it_p_" + token + "_b"
	}
	if driver == database.SCYLLA {
		fixture.FirstName = "001_" + token + "_first.cql"
		fixture.SecondName = "002_" + token + "_second.cql"
	}
	dir := filepath.Join(fixture.Directory, "migrations", driver)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create process migration directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fixture.FirstName), []byte(fixture.content(fixture.FirstObject)), 0o600); err != nil {
		t.Fatalf("write first process migration: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fixture.SecondName), []byte(fixture.content(fixture.SecondObject)), 0o600); err != nil {
		t.Fatalf("write second process migration: %v", err)
	}
	return fixture
}

func (fixture *MigrationProcessFixture) Configure(config *database.Config) {
	config.DatabaseDirectory = fixture.Directory
	config.MigrationDirectory = "migrations"
	config.SeederDirectory = "seeders"
	config.Migration = true
	config.Seeder = false
}

// SetSecondMigrationContent replaces the pending test migration body. Tests
// use this to install a deliberately long-running statement, then restore the
// normal schema statement before operator recovery retries the file.
func (fixture *MigrationProcessFixture) SetSecondMigrationContent(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(fixture.Directory, "migrations", fixture.Driver, fixture.SecondName), []byte(content), 0o600); err != nil {
		t.Fatalf("write second %s migration content: %v", fixture.Driver, err)
	}
}

func (fixture *MigrationProcessFixture) DefaultSecondMigrationContent() string {
	return fixture.content(fixture.SecondObject)
}

func (fixture *MigrationProcessFixture) RequireInterruptedState(t *testing.T, app *gorest.App) {
	t.Helper()
	fixture.requireHistory(t, app, fixture.FirstName, true)
	fixture.requireHistory(t, app, fixture.SecondName, false)
	fixture.requireState(t, app, "migration:"+fixture.FirstName, "applied")
	fixture.requireState(t, app, "migration:"+fixture.SecondName, "running")
}

func (fixture *MigrationProcessFixture) RequireCompleted(t *testing.T, app *gorest.App) {
	t.Helper()
	for _, name := range []string{fixture.FirstName, fixture.SecondName} {
		fixture.requireHistory(t, app, name, true)
		fixture.requireState(t, app, "migration:"+name, "applied")
	}
}

// WaitForDatabase polls a simple native operation after a test container has
// restarted. It verifies that the driver can reach the recovered service
// before state assertions begin.
func (fixture *MigrationProcessFixture) WaitForDatabase(t *testing.T, app *gorest.App) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Minute)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = nil
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		switch fixture.Driver {
		case database.MONGO:
			collection, message, code, err := app.Database().MongoCollection("gorest_migration_state")
			if err == nil {
				_, err = collection.CountDocuments(ctx, bson.M{})
			}
			if err != nil {
				lastErr = fmt.Errorf("MongoDB probe (HTTP %d, %s): %w", code, message, err)
			}
		case database.SCYLLA:
			iter, message, code, err := app.Database().QueryCQL(ctx, "SELECT cluster_name FROM system.local")
			if err == nil {
				err = iter.Close()
			} else {
				lastErr = fmt.Errorf("ScyllaDB probe (HTTP %d, %s): %w", code, message, err)
			}
			if err != nil && lastErr == nil {
				lastErr = err
			}
		default:
			rows, message, code, err := app.Database().QuerySQL(ctx, "SELECT 1")
			if err == nil {
				err = rows.Close()
			} else {
				lastErr = fmt.Errorf("%s probe (HTTP %d, %s): %w", fixture.Driver, code, message, err)
			}
			if err != nil && lastErr == nil {
				lastErr = err
			}
		}
		cancel()
		if lastErr == nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("%s database did not become reachable after container restart: %v", fixture.Driver, lastErr)
}

// Recover removes only this fixture's unfinished state and the startup lock
// after the caller has verified that the interrupted process is gone.
func (fixture *MigrationProcessFixture) Recover(t *testing.T, app *gorest.App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db := app.Database()
	switch fixture.Driver {
	case database.MONGO:
		for collectionName, filter := range map[string]bson.M{
			"gorest_migration_state": {"_id": "migration:" + fixture.SecondName},
			"gorest_migration_lock":  {"_id": "database_startup"},
		} {
			collection, message, code, err := db.MongoCollection(collectionName)
			if err != nil {
				t.Fatalf("open MongoDB recovery collection (HTTP %d, %s): %v", code, message, err)
			}
			if _, err := collection.DeleteOne(ctx, filter); err != nil {
				t.Fatalf("remove MongoDB recovery record from %s: %v", collectionName, err)
			}
		}
	case database.SCYLLA:
		for _, item := range []struct {
			query string
			args  []any
		}{
			{"DELETE FROM gorest_migration_state WHERE migration_name = ?", []any{"migration:" + fixture.SecondName}},
			{"DELETE FROM gorest_migration_lock WHERE lock_name = ?", []any{"database_startup"}},
		} {
			if message, code, err := db.ExecCQL(ctx, item.query, item.args...); err != nil {
				t.Fatalf("clear ScyllaDB recovery record (HTTP %d, %s): %v", code, message, err)
			}
		}
	default:
		stateQuery := fmt.Sprintf("DELETE FROM gorest_migration_state WHERE migration_name = %s", fixture.placeholder(1))
		lockQuery := "DELETE FROM gorest_migration_lock WHERE lock_name = " + fixture.placeholder(1)
		if _, message, code, err := db.ExecSQL(ctx, stateQuery, "migration:"+fixture.SecondName); err != nil {
			t.Fatalf("clear %s unfinished state (HTTP %d, %s): %v", fixture.Driver, code, message, err)
		}
		if _, message, code, err := db.ExecSQL(ctx, lockQuery, "database_startup"); err != nil {
			t.Fatalf("clear %s stale lock (HTTP %d, %s): %v", fixture.Driver, code, message, err)
		}
	}
}

func (fixture *MigrationProcessFixture) Cleanup(t *testing.T, app *gorest.App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db := app.Database()
	if fixture.Driver == database.MONGO {
		for _, name := range []string{fixture.FirstObject, fixture.SecondObject} {
			collection, message, code, err := db.MongoCollection(name)
			if err == nil {
				err = collection.Drop(ctx)
			}
			if err != nil {
				t.Errorf("drop MongoDB migration test collection (HTTP %d, %s): %v", code, message, err)
			}
		}
		for collectionName, filter := range map[string]bson.M{
			"gorest_migration":       {"migration_name": bson.M{"$in": []string{fixture.FirstName, fixture.SecondName}}},
			"gorest_migration_state": {"_id": bson.M{"$in": []string{"migration:" + fixture.FirstName, "migration:" + fixture.SecondName}}},
		} {
			collection, message, code, err := db.MongoCollection(collectionName)
			if err == nil {
				_, err = collection.DeleteMany(ctx, filter)
			}
			if err != nil {
				t.Errorf("clean MongoDB process migration records (HTTP %d, %s): %v", code, message, err)
			}
		}
		return
	}
	if fixture.Driver == database.SCYLLA {
		for _, name := range []string{fixture.SecondObject, fixture.FirstObject} {
			if message, code, err := db.ExecCQL(ctx, "DROP TABLE "+name); err != nil {
				t.Errorf("drop ScyllaDB process migration table (HTTP %d, %s): %v", code, message, err)
			}
		}
		return
	}
	for _, name := range []string{fixture.FirstName, fixture.SecondName} {
		key := "migration:" + name
		if fixture.Driver == database.SCYLLA {
			for _, item := range []struct {
				query string
				args  []any
			}{
				{"DELETE FROM gorest_migration WHERE migration_name = ?", []any{name}},
				{"DELETE FROM gorest_migration_state WHERE migration_name = ?", []any{key}},
			} {
				if message, code, err := db.ExecCQL(ctx, item.query, item.args...); err != nil {
					t.Errorf("clean ScyllaDB process migration record (HTTP %d, %s): %v", code, message, err)
				}
			}
			continue
		}
		for _, item := range []struct {
			query string
			value string
		}{
			{fmt.Sprintf("DELETE FROM gorest_migration WHERE migration_name = %s", fixture.placeholder(1)), name},
			{fmt.Sprintf("DELETE FROM gorest_migration_state WHERE migration_name = %s", fixture.placeholder(1)), key},
		} {
			if _, message, code, err := db.ExecSQL(ctx, item.query, item.value); err != nil {
				t.Errorf("clean %s process migration record (HTTP %d, %s): %v", fixture.Driver, code, message, err)
			}
		}
	}
	for _, object := range []string{fixture.SecondObject, fixture.FirstObject} {
		drop := "DROP TABLE IF EXISTS " + object
		if fixture.Driver == database.ORACLE {
			drop = "DROP TABLE " + object
		}
		if _, message, code, err := db.ExecSQL(ctx, drop); err != nil && fixture.Driver != database.ORACLE {
			t.Errorf("drop %s process migration object (HTTP %d, %s): %v", fixture.Driver, code, message, err)
		}
	}
}

func (fixture *MigrationProcessFixture) requireHistory(t *testing.T, app *gorest.App, name string, want bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if fixture.Driver == database.MONGO {
		collection, message, code, err := app.Database().MongoCollection("gorest_migration")
		if err != nil {
			t.Fatalf("open MongoDB migration history (HTTP %d, %s): %v", code, message, err)
		}
		count, err := collection.CountDocuments(ctx, bson.M{"migration_name": name})
		if err != nil || (count == 1) != want {
			t.Fatalf("MongoDB migration history %q count=%d err=%v, want present=%v", name, count, err, want)
		}
		return
	}
	query := fmt.Sprintf("SELECT COUNT(*) FROM gorest_migration WHERE migration_name = %s", fixture.placeholder(1))
	if fixture.Driver == database.SCYLLA {
		iter, message, code, err := app.Database().QueryCQL(ctx, "SELECT migration_name FROM gorest_migration WHERE migration_name = ?", name)
		if err != nil {
			t.Fatalf("query ScyllaDB migration history (HTTP %d, %s): %v", code, message, err)
		}
		found := iter.MapScan(map[string]any{})
		if err := iter.Close(); err != nil {
			t.Fatalf("close ScyllaDB history iterator: %v", err)
		}
		if found != want {
			t.Fatalf("ScyllaDB migration history %q present=%v, want %v", name, found, want)
		}
		return
	}
	rows, message, code, err := app.Database().QuerySQL(ctx, query, name)
	if err != nil {
		t.Fatalf("query %s migration history (HTTP %d, %s): %v", fixture.Driver, code, message, err)
	}
	defer rows.Close()
	var count int64
	if !rows.Next() || rows.Scan(&count) != nil || (count == 1) != want {
		t.Fatalf("%s migration history %q count=%d, want present=%v", fixture.Driver, name, count, want)
	}
}

func (fixture *MigrationProcessFixture) requireState(t *testing.T, app *gorest.App, key, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if fixture.Driver == database.MONGO {
		collection, message, code, err := app.Database().MongoCollection("gorest_migration_state")
		if err != nil {
			t.Fatalf("open MongoDB migration state (HTTP %d, %s): %v", code, message, err)
		}
		var row struct {
			Status string `bson:"status"`
		}
		if err := collection.FindOne(ctx, bson.M{"_id": key}).Decode(&row); err != nil || row.Status != want {
			t.Fatalf("MongoDB state %q status=%q err=%v, want %q", key, row.Status, err, want)
		}
		return
	}
	if fixture.Driver == database.SCYLLA {
		iter, message, code, err := app.Database().QueryCQL(ctx, "SELECT status FROM gorest_migration_state WHERE migration_name = ?", key)
		if err != nil {
			t.Fatalf("query ScyllaDB migration state (HTTP %d, %s): %v", code, message, err)
		}
		row := map[string]any{}
		found := iter.MapScan(row)
		closeErr := iter.Close()
		status, _ := row["status"].(string)
		if !found || closeErr != nil || status != want {
			t.Fatalf("ScyllaDB state %q status=%q found=%v closeErr=%v, want %q", key, status, found, closeErr, want)
		}
		return
	}
	query := fmt.Sprintf("SELECT status FROM gorest_migration_state WHERE migration_name = %s", fixture.placeholder(1))
	rows, message, code, err := app.Database().QuerySQL(ctx, query, key)
	if err != nil {
		t.Fatalf("query %s migration state (HTTP %d, %s): %v", fixture.Driver, code, message, err)
	}
	defer rows.Close()
	var status string
	if !rows.Next() || rows.Scan(&status) != nil || status != want {
		t.Fatalf("%s state %q status=%q, want %q", fixture.Driver, key, status, want)
	}
}

func (fixture *MigrationProcessFixture) content(object string) string {
	switch fixture.Driver {
	case database.MONGO:
		payload, _ := json.Marshal(map[string]any{"collection": object})
		return string(payload)
	case database.SCYLLA:
		return fmt.Sprintf("CREATE TABLE %s (id text PRIMARY KEY, value_text text);", object)
	case database.ORACLE:
		return fmt.Sprintf("CREATE TABLE %s (id VARCHAR2(64) NOT NULL PRIMARY KEY, value_text VARCHAR2(100) NOT NULL)", object)
	default:
		return fmt.Sprintf("CREATE TABLE %s (id VARCHAR(64) NOT NULL PRIMARY KEY, value_text VARCHAR(100) NOT NULL)", object)
	}
}

func (fixture *MigrationProcessFixture) placeholder(index int) string {
	switch fixture.Driver {
	case database.POSTGRES:
		return fmt.Sprintf("$%d", index)
	case database.SQLSERVER:
		return fmt.Sprintf("@p%d", index)
	case database.ORACLE:
		return fmt.Sprintf(":%d", index)
	default:
		return "?"
	}
}
