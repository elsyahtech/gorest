//go:build integration

package internaltest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// MigrationSeederFixture creates isolated, driver-specific migration and
// seeder files for the managed database integration runners.
type MigrationSeederFixture struct {
	driver         string
	databaseDir    string
	table          string
	recordID       string
	migrationName  string
	seederName     string
	collectionName string
}

func NewMigrationSeederFixture(t *testing.T, driver string) *MigrationSeederFixture {
	t.Helper()

	token := uniqueIdentifier(t, "")[:8]
	fixture := &MigrationSeederFixture{
		driver:         driver,
		databaseDir:    t.TempDir(),
		table:          "g_it_m_" + token,
		collectionName: "g_it_m_" + token,
		recordID:       "migration-seed-" + token,
		migrationName:  "001_" + token + "_create.sql",
		seederName:     "001_" + token + "_insert.sql",
	}

	if driver == database.MONGO {
		fixture.migrationName = "001_" + token + "_create.json"
		fixture.seederName = "001_" + token + "_insert.json"
	}
	if driver == database.SCYLLA {
		fixture.migrationName = "001_" + token + "_create.cql"
		fixture.seederName = "001_" + token + "_insert.cql"
	}

	migrationDir := filepath.Join(fixture.databaseDir, "migrations", driver)
	seederDir := filepath.Join(fixture.databaseDir, "seeders", driver)
	if err := os.MkdirAll(migrationDir, 0o700); err != nil {
		t.Fatalf("create test migration directory: %v", err)
	}
	if err := os.MkdirAll(seederDir, 0o700); err != nil {
		t.Fatalf("create test seeder directory: %v", err)
	}
	migrationPath := filepath.Join(migrationDir, fixture.migrationName)
	seederPath := filepath.Join(seederDir, fixture.seederName)
	if err := os.WriteFile(migrationPath, []byte(fixture.migrationContent()), 0o600); err != nil {
		t.Fatalf("write test migration file: %v", err)
	}
	if err := os.WriteFile(seederPath, []byte(fixture.seederContent()), 0o600); err != nil {
		t.Fatalf("write test seeder file: %v", err)
	}

	return fixture
}

func (fixture *MigrationSeederFixture) Configure(config *database.Config) {
	config.DatabaseDirectory = fixture.databaseDir
	config.MigrationDirectory = "migrations"
	config.SeederDirectory = "seeders"
	config.Migration = true
	config.Seeder = true
}

// Run checks initial execution, persisted seed data, repeat-run idempotence,
// and lock release through the public application startup-task API.
func (fixture *MigrationSeederFixture) Run(t *testing.T, app *gorest.App) {
	t.Helper()
	if err := app.RunDatabaseStartupTasks(); err != nil {
		t.Fatalf("run %s migration and seeder: %v", fixture.driver, err)
	}
	fixture.requireSeededValue(t, app)
	fixture.requireExecutionRecords(t, app)

	// A second successful run demonstrates that recorded migration/seeder files
	// are skipped and that the shared startup lock was released.
	if err := app.RunDatabaseStartupTasks(); err != nil {
		t.Fatalf("rerun %s migration and seeder: %v", fixture.driver, err)
	}
	fixture.requireSeededValue(t, app)
	fixture.requireExecutionRecords(t, app)
	fixture.requireLockContentionAndRecovery(t, app)
	t.Logf("%s migration/seeder execution, history/state recording, repeat-run, concurrent lock contention, stale-lock rejection, and manual recovery passed", fixture.driver)

	t.Cleanup(func() { fixture.cleanup(t, app) })
}

func (fixture *MigrationSeederFixture) requireLockContentionAndRecovery(t *testing.T, app *gorest.App) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type contenderResult struct {
		owner    string
		acquired bool
		err      error
	}
	start := make(chan struct{})
	ready := make(chan struct{}, 2)
	results := make(chan contenderResult, 2)
	owners := []string{"integration-runner-a-" + fixture.recordID, "integration-runner-b-" + fixture.recordID}
	var waitGroup sync.WaitGroup
	for _, owner := range owners {
		owner := owner
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			ready <- struct{}{}
			<-start
			acquired, err := fixture.tryAcquireLock(ctx, app, owner)
			results <- contenderResult{owner: owner, acquired: acquired, err: err}
		}()
	}
	<-ready
	<-ready
	close(start)
	waitGroup.Wait()
	close(results)

	var winner string
	acquiredCount := 0
	conflictCount := 0
	for result := range results {
		if result.err != nil {
			if !database.IsDuplicateKeyError(result.err) && !mongo.IsDuplicateKeyError(result.err) {
				t.Fatalf("%s lock contender %s failed unexpectedly: %v", fixture.driver, result.owner, result.err)
			}
			conflictCount++
			continue
		}
		if result.acquired {
			winner = result.owner
			acquiredCount++
		} else {
			conflictCount++
		}
	}
	if acquiredCount != 1 || conflictCount != 1 {
		if winner != "" {
			_ = fixture.releaseTestLock(ctx, app, winner)
		}
		t.Fatalf("%s concurrent lock claims: acquired=%d conflicted=%d, want exactly one each", fixture.driver, acquiredCount, conflictCount)
	}

	lockReleased := false
	defer func() {
		if !lockReleased {
			if err := fixture.releaseTestLock(ctx, app, winner); err != nil {
				t.Errorf("release %s simulated stale startup lock after test failure: %v", fixture.driver, err)
			}
		}
	}()

	// The persisted winner lock represents the record left by a crashed or
	// competing runner. Startup must refuse to continue while it is present.
	if err := app.RunDatabaseStartupTasks(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "lock") {
		t.Fatalf("%s startup with a held migration lock returned %v, want a lock error", fixture.driver, err)
	}

	if err := fixture.releaseTestLock(ctx, app, winner); err != nil {
		t.Fatalf("manually clear %s simulated stale startup lock: %v", fixture.driver, err)
	}
	lockReleased = true
	if err := app.RunDatabaseStartupTasks(); err != nil {
		t.Fatalf("rerun %s startup tasks after lock recovery: %v", fixture.driver, err)
	}
	fixture.requireSeededValue(t, app)
	fixture.requireExecutionRecords(t, app)
}

func (fixture *MigrationSeederFixture) tryAcquireLock(ctx context.Context, app *gorest.App, owner string) (bool, error) {
	const lockName = "database_startup"
	db := app.Database()
	switch fixture.driver {
	case database.MONGO:
		collection, message, code, err := db.MongoCollection("gorest_migration_lock")
		if err != nil {
			return false, fmt.Errorf("open MongoDB migration lock collection (HTTP %d, %s): %w", code, message, err)
		}
		_, err = collection.InsertOne(ctx, bson.M{
			"_id":         lockName,
			"owner_id":    owner,
			"acquired_at": time.Now().UTC(),
		})
		return err == nil, err
	case database.SCYLLA:
		applied, message, code, err := db.MapScanCAS(ctx,
			"INSERT INTO gorest_migration_lock (lock_name, owner_id, acquired_at) VALUES (?, ?, ?) IF NOT EXISTS",
			lockName, owner, time.Now().UTC())
		if err != nil {
			return false, fmt.Errorf("acquire ScyllaDB migration lock (HTTP %d, %s): %w", code, message, err)
		}
		return applied, nil
	default:
		placeholder := fixture.sqlPlaceholder(1)
		timestamp := "CURRENT_TIMESTAMP"
		if fixture.driver == database.SQLSERVER {
			timestamp = "SYSUTCDATETIME()"
		}
		query := fmt.Sprintf("INSERT INTO gorest_migration_lock (lock_name, owner_id, acquired_at) VALUES (%s, %s, %s)",
			placeholder, fixture.sqlPlaceholder(2), timestamp)
		_, message, code, err := db.ExecSQL(ctx, query, lockName, owner)
		if err != nil {
			return false, fmt.Errorf("acquire %s migration lock (HTTP %d, %s): %w", fixture.driver, code, message, err)
		}
		return true, nil
	}
}

func (fixture *MigrationSeederFixture) releaseTestLock(ctx context.Context, app *gorest.App, owner string) error {
	const lockName = "database_startup"
	db := app.Database()
	switch fixture.driver {
	case database.MONGO:
		collection, message, code, err := db.MongoCollection("gorest_migration_lock")
		if err != nil {
			return fmt.Errorf("open MongoDB migration lock collection (HTTP %d, %s): %w", code, message, err)
		}
		result, err := collection.DeleteOne(ctx, bson.M{"_id": lockName, "owner_id": owner})
		if err != nil {
			return fmt.Errorf("delete MongoDB migration lock: %w", err)
		}
		if result.DeletedCount != 1 {
			return fmt.Errorf("delete MongoDB migration lock: deleted %d records, want 1", result.DeletedCount)
		}
		return nil
	case database.SCYLLA:
		applied, message, code, err := db.MapScanCAS(ctx,
			"DELETE FROM gorest_migration_lock WHERE lock_name = ? IF owner_id = ?", lockName, owner)
		if err != nil {
			return fmt.Errorf("delete ScyllaDB migration lock (HTTP %d, %s): %w", code, message, err)
		}
		if !applied {
			return errors.New("delete ScyllaDB migration lock: owner did not match")
		}
		return nil
	default:
		query := fmt.Sprintf("DELETE FROM gorest_migration_lock WHERE lock_name = %s AND owner_id = %s",
			fixture.sqlPlaceholder(1), fixture.sqlPlaceholder(2))
		_, message, code, err := db.ExecSQL(ctx, query, lockName, owner)
		if err != nil {
			return fmt.Errorf("delete %s migration lock (HTTP %d, %s): %w", fixture.driver, code, message, err)
		}
		return nil
	}
}

func (fixture *MigrationSeederFixture) requireExecutionRecords(t *testing.T, app *gorest.App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	records := []struct {
		table  string
		column string
		value  string
	}{
		{table: "gorest_migration", column: "migration_name", value: fixture.migrationName},
		{table: "gorest_seeder", column: "seeder_name", value: fixture.seederName},
		{table: "gorest_migration_state", column: "migration_name", value: "migration:" + fixture.migrationName},
		{table: "gorest_migration_state", column: "migration_name", value: "seeder:" + fixture.seederName},
	}
	for _, record := range records {
		switch fixture.driver {
		case database.MONGO:
			collection, message, code, err := app.Database().MongoCollection(record.table)
			if err != nil {
				t.Fatalf("open MongoDB execution record collection (HTTP %d, %s): %v", code, message, err)
			}
			field := record.column
			if record.table == "gorest_migration_state" {
				field = "_id"
			}
			count, err := collection.CountDocuments(ctx, bson.M{field: record.value})
			if err != nil {
				t.Fatalf("count MongoDB execution records in %s: %v", record.table, err)
			}
			if count != 1 {
				t.Fatalf("MongoDB %s record count for %q = %d, want 1", record.table, record.value, count)
			}
		case database.SCYLLA:
			iter, message, code, err := app.Database().QueryCQL(ctx,
				fmt.Sprintf("SELECT %s FROM %s WHERE %s = ?", record.column, record.table, record.column), record.value)
			if err != nil {
				t.Fatalf("query ScyllaDB execution record (HTTP %d, %s): %v", code, message, err)
			}
			row := map[string]any{}
			if !iter.MapScan(row) {
				closeErr := iter.Close()
				t.Fatalf("ScyllaDB %s record %q missing (close error: %v)", record.table, record.value, closeErr)
			}
			if err := iter.Close(); err != nil {
				t.Fatalf("close ScyllaDB execution-record iterator: %v", err)
			}
		default:
			placeholder := fixture.sqlPlaceholder(1)
			rows, message, code, err := app.Database().QuerySQL(ctx,
				fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = %s", record.table, record.column, placeholder), record.value)
			if err != nil {
				t.Fatalf("query %s execution record (HTTP %d, %s): %v", fixture.driver, code, message, err)
			}
			var count int64
			if !rows.Next() {
				rows.Close()
				t.Fatalf("%s %s count query returned no row: %v", fixture.driver, record.table, rows.Err())
			}
			if err := rows.Scan(&count); err != nil {
				rows.Close()
				t.Fatalf("scan %s execution record count: %v", fixture.driver, err)
			}
			if err := rows.Close(); err != nil {
				t.Fatalf("close %s execution record rows: %v", fixture.driver, err)
			}
			if count != 1 {
				t.Fatalf("%s %s record count for %q = %d, want 1", fixture.driver, record.table, record.value, count)
			}
		}
	}
}

func (fixture *MigrationSeederFixture) sqlPlaceholder(index int) string {
	switch fixture.driver {
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

func (fixture *MigrationSeederFixture) migrationContent() string {
	switch fixture.driver {
	case database.MONGO:
		payload, _ := json.Marshal(map[string]any{"collection": fixture.collectionName})
		return string(payload)
	case database.SCYLLA:
		return fmt.Sprintf("CREATE TABLE %s (id text PRIMARY KEY, value_text text);", fixture.table)
	case database.ORACLE:
		return fmt.Sprintf("CREATE TABLE %s (id VARCHAR2(64) NOT NULL PRIMARY KEY, value_text VARCHAR2(100) NOT NULL)", fixture.table)
	default:
		return fmt.Sprintf("CREATE TABLE %s (id VARCHAR(64) NOT NULL PRIMARY KEY, value_text VARCHAR(100) NOT NULL)", fixture.table)
	}
}

func (fixture *MigrationSeederFixture) seederContent() string {
	switch fixture.driver {
	case database.MONGO:
		payload, _ := json.Marshal(map[string]any{
			"collection": fixture.collectionName,
			"data":       []map[string]string{{"_id": fixture.recordID, "value_text": "seeded"}},
		})
		return string(payload)
	case database.SCYLLA:
		return fmt.Sprintf("INSERT INTO %s (id, value_text) VALUES ('%s', 'seeded');", fixture.table, fixture.recordID)
	default:
		return fmt.Sprintf("INSERT INTO %s (id, value_text) VALUES ('%s', 'seeded')", fixture.table, fixture.recordID)
	}
}

func (fixture *MigrationSeederFixture) requireSeededValue(t *testing.T, app *gorest.App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch fixture.driver {
	case database.MONGO:
		collection, message, code, err := app.Database().MongoCollection(fixture.collectionName)
		if err != nil {
			t.Fatalf("open migrated MongoDB collection (HTTP %d, %s): %v", code, message, err)
		}
		var document struct {
			ID    string `bson:"_id"`
			Value string `bson:"value_text"`
		}
		if err := collection.FindOne(ctx, bson.M{"_id": fixture.recordID}).Decode(&document); err != nil {
			t.Fatalf("read MongoDB seeded document: %v", err)
		}
		if document.Value != "seeded" {
			t.Fatalf("MongoDB seeded value = %q, want seeded", document.Value)
		}
	case database.SCYLLA:
		iter, message, code, err := app.Database().QueryCQL(ctx,
			fmt.Sprintf("SELECT value_text FROM %s WHERE id = ?", fixture.table), fixture.recordID)
		if err != nil {
			t.Fatalf("query ScyllaDB seeded row (HTTP %d, %s): %v", code, message, err)
		}
		row := map[string]any{}
		if !iter.MapScan(row) {
			closeErr := iter.Close()
			t.Fatalf("ScyllaDB seeded row was not found (close error: %v)", closeErr)
		}
		if value, ok := row["value_text"].(string); !ok || value != "seeded" {
			t.Fatalf("ScyllaDB seeded value = %#v, want seeded", row["value_text"])
		}
		if err := iter.Close(); err != nil {
			t.Fatalf("close ScyllaDB seeded-row iterator: %v", err)
		}
	default:
		placeholder := fixture.sqlPlaceholder(1)
		rows, message, code, err := app.Database().QuerySQL(ctx,
			fmt.Sprintf("SELECT value_text FROM %s WHERE id = %s", fixture.table, placeholder), fixture.recordID)
		if err != nil {
			t.Fatalf("query %s seeded row (HTTP %d, %s): %v", fixture.driver, code, message, err)
		}
		defer rows.Close()
		if !rows.Next() {
			t.Fatalf("%s seeded row was not found: %v", fixture.driver, rows.Err())
		}
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan %s seeded value: %v", fixture.driver, err)
		}
		if value != "seeded" {
			t.Fatalf("%s seeded value = %q, want seeded", fixture.driver, value)
		}
		if rows.Next() {
			t.Fatalf("%s seeder inserted duplicate rows", fixture.driver)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate %s seeded rows: %v", fixture.driver, err)
		}
	}
}

func (fixture *MigrationSeederFixture) cleanup(t *testing.T, app *gorest.App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db := app.Database()

	if fixture.driver == database.MONGO {
		for _, name := range []string{fixture.collectionName, "gorest_migration", "gorest_seeder", "gorest_migration_state"} {
			collection, message, code, err := db.MongoCollection(name)
			if err != nil {
				t.Errorf("open MongoDB cleanup collection %s (HTTP %d, %s): %v", name, code, message, err)
				continue
			}
			filter := bson.M{}
			switch name {
			case "gorest_migration":
				filter = bson.M{"migration_name": fixture.migrationName}
			case "gorest_seeder":
				filter = bson.M{"seeder_name": fixture.seederName}
			case "gorest_migration_state":
				if _, err := collection.DeleteOne(ctx, bson.M{"_id": "migration:" + fixture.migrationName}); err != nil {
					t.Errorf("delete MongoDB migration state record: %v", err)
				}
				_, err = collection.DeleteOne(ctx, bson.M{"_id": "seeder:" + fixture.seederName})
				if err != nil {
					t.Errorf("delete MongoDB migration state: %v", err)
				}
				continue
			}
			if name == fixture.collectionName {
				err = collection.Drop(ctx)
			} else {
				_, err = collection.DeleteOne(ctx, filter)
			}
			if err != nil {
				t.Errorf("clean MongoDB migration fixture %s: %v", name, err)
			}
		}
		return
	}

	if fixture.driver == database.SCYLLA {
		for _, item := range []struct {
			query string
			args  []any
		}{
			{"DELETE FROM gorest_migration WHERE migration_name = ?", []any{fixture.migrationName}},
			{"DELETE FROM gorest_seeder WHERE seeder_name = ?", []any{fixture.seederName}},
			{"DELETE FROM gorest_migration_state WHERE migration_name = ?", []any{"migration:" + fixture.migrationName}},
			{"DELETE FROM gorest_migration_state WHERE migration_name = ?", []any{"seeder:" + fixture.seederName}},
			{"DROP TABLE IF EXISTS " + fixture.table, nil},
		} {
			if message, code, err := db.ExecCQL(ctx, item.query, item.args...); err != nil {
				t.Errorf("clean ScyllaDB migration fixture (HTTP %d, %s): %v", code, message, err)
			}
		}
		return
	}

	placeholder := fixture.sqlPlaceholder(1)
	for _, item := range []struct {
		query string
		args  []any
	}{
		{fmt.Sprintf("DELETE FROM gorest_migration WHERE migration_name = %s", placeholder), []any{fixture.migrationName}},
		{fmt.Sprintf("DELETE FROM gorest_seeder WHERE seeder_name = %s", placeholder), []any{fixture.seederName}},
		{fmt.Sprintf("DELETE FROM gorest_migration_state WHERE migration_name = %s", placeholder), []any{"migration:" + fixture.migrationName}},
		{fmt.Sprintf("DELETE FROM gorest_migration_state WHERE migration_name = %s", placeholder), []any{"seeder:" + fixture.seederName}},
	} {
		if _, message, code, err := db.ExecSQL(ctx, item.query, item.args...); err != nil {
			t.Errorf("clean %s migration history (HTTP %d, %s): %v", fixture.driver, code, message, err)
		}
	}
	drop := "DROP TABLE IF EXISTS " + fixture.table
	if fixture.driver == database.ORACLE {
		drop = "DROP TABLE " + fixture.table
	}
	if _, message, code, err := db.ExecSQL(ctx, drop); err != nil {
		t.Errorf("drop %s migration test table (HTTP %d, %s): %v", fixture.driver, code, message, err)
	}
}
