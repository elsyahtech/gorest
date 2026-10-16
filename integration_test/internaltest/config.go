//go:build integration

package internaltest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/orm"
)

func OpenApp(t *testing.T, driver string) *gorest.App {
	t.Helper()

	prefix := "GOREST_IT_" + strings.ToUpper(driver)
	config := database.Config{
		Driver:    driver,
		Enabled:   true,
		Timeout:   10 * time.Second,
		Migration: false,
		Seeder:    false,
	}

	if driver == database.SQLITE {
		config.Name = "integration"
		config.DatabaseDirectory = t.TempDir()
		config.MaxOpenConnections = 1
	} else {
		config.Host = configuredHost(t, prefix+"_HOST")
		config.Name = requiredEnvironmentValue(t, prefix+"_NAME")
		config.Username = os.Getenv(prefix + "_USERNAME")
		config.Password = os.Getenv(prefix + "_PASSWORD")
		config.Port = os.Getenv(prefix + "_PORT")
		config.TLS = os.Getenv(prefix + "_TLS")
		config.SSLCertPath = os.Getenv(prefix + "_SSL_CERT")
		config.SSLKeyPath = os.Getenv(prefix + "_SSL_KEY")
		config.SSLCAPath = os.Getenv(prefix + "_SSL_CA")
		config.LibDir = os.Getenv(prefix + "_LIB_DIR")
		config.TrustServerCertificate = strings.EqualFold(os.Getenv(prefix+"_TRUST_SERVER_CERTIFICATE"), "true")
	}

	normalized := database.Run(config)
	app := gorest.New()
	app.Use(normalized)
	t.Cleanup(func() {
		_, _, err := app.Close()
		if err != nil {
			t.Errorf("close integration application: %v", err)
		}
	})

	return app
}

func requiredEnvironmentValue(t *testing.T, name string) string {
	t.Helper()

	value := strings.TrimSpace(os.Getenv(name))
	if value != "" {
		return value
	}

	t.Fatalf("required integration-test environment variable %s is not set", name)
	return ""
}

func configuredHost(t *testing.T, name string) string {
	t.Helper()

	value := strings.TrimSpace(os.Getenv(name))
	if value != "" {
		return value
	}

	if strings.EqualFold(os.Getenv("GOREST_IT_REQUIRED"), "true") ||
		os.Getenv("GOREST_IT_REQUIRED") == "1" {
		t.Fatalf("required integration-test environment variable %s is not set", name)
	}

	t.Skipf("set %s to run this database integration test (or GOREST_IT_REQUIRED=1 in CI)", name)
	return ""
}

func ExecSQL(t *testing.T, app *gorest.App, query string, args ...any) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, message, code, err := app.Database().ExecSQL(ctx, query, args...)
	if err != nil {
		t.Fatalf("SQL execution failed (HTTP %d, %s): %v\nquery: %s", code, message, err, query)
	}
}

func ExecCQL(t *testing.T, app *gorest.App, query string, args ...any) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	message, code, err := app.Database().ExecCQL(ctx, query, args...)
	if err != nil {
		t.Fatalf("CQL execution failed (HTTP %d, %s): %v\nquery: %s", code, message, err, query)
	}
}

func RequireNoORMError(t *testing.T, query *orm.ORM) {
	t.Helper()
	if query.Error != nil {
		t.Fatalf("ORM operation failed (HTTP %d, %s): %v", query.HTTPCode, query.Message, query.Error)
	}
}

func SQLCRUD(t *testing.T, driver string) {
	t.Helper()

	app := OpenApp(t, driver)
	table := uniqueIdentifier(t, "g_it_")
	id := time.Now().UnixNano()
	name := "quote' /* ? */ ; -- payload"

	idType := "BIGINT"
	valueType := "INTEGER"
	if driver == database.ORACLE {
		idType = "NUMBER(19)"
		valueType = "NUMBER(19)"
	}
	textType := "VARCHAR(200)"
	if driver == database.ORACLE {
		textType = "VARCHAR2(200)"
	}

	ExecSQL(t, app, fmt.Sprintf(
		"CREATE TABLE %s (id %s NOT NULL PRIMARY KEY, name %s NOT NULL, score %s NOT NULL)",
		table, idType, textType, valueType,
	))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _, _, err := app.Database().ExecSQL(ctx, "DROP TABLE "+table)
		if err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})

	record := sqlRecord{ID: id, Name: name, Score: 10}
	RequireNoORMError(t, app.Database().Table(table).Create(&record))

	var loaded sqlRecord
	query := app.Database().Table(table).
		Where("id = ? AND name <> '?' /* ? */ -- ?\n", id).
		Find(&loaded)
	RequireNoORMError(t, query)
	if loaded.ID != id || loaded.Name != name || loaded.Score != 10 {
		t.Fatalf("Find() returned %#v, want ID=%d Name=%q Score=10", loaded, id, name)
	}

	upserted := sqlRecord{ID: id, Name: "upserted", Score: 15}
	RequireNoORMError(t, app.Database().Table(table).Upsert("id").Create(&upserted))

	var afterUpsert sqlRecord
	RequireNoORMError(t, app.Database().Table(table).Where("id = ?", id).Find(&afterUpsert))
	if afterUpsert.Name != "upserted" || afterUpsert.Score != 15 {
		t.Fatalf("Upsert() persisted %#v, want updated values", afterUpsert)
	}

	record.Name = "updated"
	record.Score = 20
	RequireNoORMError(t, app.Database().Table(table).
		Where("id = ?", id).
		Update(&record))

	var updated sqlRecord
	RequireNoORMError(t, app.Database().Table(table).Where("id = ?", id).Find(&updated))
	if updated.Name != "updated" || updated.Score != 20 {
		t.Fatalf("Update() persisted %#v, want updated values", updated)
	}

	withoutFilter := sqlRecord{Name: "must-not-update", Score: 99}
	if query := app.Database().Table(table).Update(&withoutFilter); query.Error == nil {
		t.Fatal("Update() without a WHERE clause or non-zero primary key unexpectedly succeeded")
	}
	if query := app.Database().Table(table).Delete(&sqlRecord{}); query.Error == nil {
		t.Fatal("Delete() without a WHERE clause or non-zero primary key unexpectedly succeeded")
	}
	var stillPresent sqlRecord
	RequireNoORMError(t, app.Database().Table(table).Where("id = ?", id).Find(&stillPresent))
	if stillPresent.Name != "updated" || stillPresent.Score != 20 {
		t.Fatalf("rejected unfiltered writes changed the row: %#v", stillPresent)
	}

	RequireNoORMError(t, app.Database().Table(table).
		Where("id = ?", id).
		Delete(&record))
	var remaining []sqlRecord
	deleteQuery := app.Database().Table(table).Where("id = ?", id).Find(&remaining)
	if deleteQuery.Error != nil && deleteQuery.HTTPCode != 404 {
		t.Fatalf("Find() after Delete() returned HTTP %d: %v", deleteQuery.HTTPCode, deleteQuery.Error)
	}
	if len(remaining) != 0 {
		t.Fatalf("Delete() left matching rows: %#v", remaining)
	}
}

func SQLTransactionRollback(t *testing.T, driver string) {
	t.Helper()

	app := OpenApp(t, driver)
	table := uniqueIdentifier(t, "g_tx_")
	id := time.Now().UnixNano()

	idType := "BIGINT"
	valueType := "INTEGER"
	if driver == database.ORACLE {
		idType = "NUMBER(19)"
		valueType = "NUMBER(19)"
	}
	textType := "VARCHAR(200)"
	if driver == database.ORACLE {
		textType = "VARCHAR2(200)"
	}
	ExecSQL(t, app, fmt.Sprintf(
		"CREATE TABLE %s (id %s NOT NULL PRIMARY KEY, name %s NOT NULL, score %s NOT NULL)",
		table, idType, textType, valueType,
	))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _, _, err := app.Database().ExecSQL(ctx, "DROP TABLE "+table)
		if err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})

	tx, message, code, err := app.Database().BeginTx()
	if err != nil {
		t.Fatalf("BeginTx() failed (HTTP %d, %s): %v", code, message, err)
	}
	RequireNoORMError(t, tx.Table(table).Create(&sqlRecord{ID: id, Name: "rollback", Score: 1}))
	if _, _, err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() failed: %v", err)
	}

	var loaded sqlRecord
	query := app.Database().Table(table).Where("id = ?", id).Find(&loaded)
	if query.Error == nil {
		t.Fatalf("record ID %d exists after rollback: %#v", id, loaded)
	}
	if query.HTTPCode != 404 {
		t.Fatalf("Find() after rollback returned HTTP %d, want 404 Not Found: %v", query.HTTPCode, query.Error)
	}
}

func SQLPaginationAndGrouping(t *testing.T, driver string) {
	t.Helper()

	app := OpenApp(t, driver)
	table := uniqueIdentifier(t, "g_pg_")
	idType := "BIGINT"
	valueType := "INTEGER"
	if driver == database.ORACLE {
		idType = "NUMBER(19)"
		valueType = "NUMBER(19)"
	}
	textType := "VARCHAR(200)"
	if driver == database.ORACLE {
		textType = "VARCHAR2(200)"
	}
	ExecSQL(t, app, fmt.Sprintf(
		"CREATE TABLE %s (id %s NOT NULL PRIMARY KEY, name %s NOT NULL, score %s NOT NULL)",
		table, idType, textType, valueType,
	))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _, _, err := app.Database().ExecSQL(ctx, "DROP TABLE "+table)
		if err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})

	baseID := time.Now().UnixNano()
	records := []sqlRecord{
		{ID: baseID, Name: "first", Score: 10},
		{ID: baseID + 1, Name: "second", Score: 10},
		{ID: baseID + 2, Name: "third", Score: 20},
	}
	for i := range records {
		RequireNoORMError(t, app.Database().Table(table).Create(&records[i]))
	}

	var page []sqlRecord
	RequireNoORMError(t, app.Database().Table(table).
		OrderBy("id", "ASC").
		Limit(1).
		Offset(1).
		Find(&page))
	if len(page) != 1 || page[0].ID != baseID+1 {
		t.Fatalf("paginated Find() = %#v, want only row ID %d", page, baseID+1)
	}

	var groups []groupRecord
	RequireNoORMError(t, app.Database().Table(table).
		Select("score, COUNT(*) AS entry_count").
		GroupBy("score").
		Having("COUNT(*) >= ?", 2).
		Find(&groups))
	if len(groups) != 1 || groups[0].Score != 10 || groups[0].Count != 2 {
		t.Fatalf("grouped Find() = %#v, want score 10 with count 2", groups)
	}
}

func NoSQLCRUD(t *testing.T, driver string) {
	t.Helper()

	app := OpenApp(t, driver)
	collection := uniqueIdentifier(t, "g_it_")
	id := uniqueIdentifier(t, "record_")
	mongoCollection, message, code, err := app.Database().MongoCollection(collection)
	if err != nil {
		t.Fatalf("create collection handle (HTTP %d, %s): %v", code, message, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mongoCollection.Drop(ctx); err != nil {
			t.Errorf("drop integration collection %s: %v", collection, err)
		}
	})

	record := documentRecord{ID: id, Name: "quote' ; -- payload", Score: 10}
	RequireNoORMError(t, app.Database().Table(collection).Create(&record))

	var loaded documentRecord
	RequireNoORMError(t, app.Database().Table(collection).
		Where("name = ?", record.Name).
		Find(&loaded))
	if loaded.ID != id || loaded.Name != record.Name || loaded.Score != 10 {
		t.Fatalf("Find() returned %#v, want inserted document", loaded)
	}

	upserted := documentRecord{ID: id, Name: "upserted", Score: 15}
	RequireNoORMError(t, app.Database().Table(collection).Upsert("_id").Create(&upserted))

	var afterUpsert documentRecord
	RequireNoORMError(t, app.Database().Table(collection).Where("_id = ?", id).Find(&afterUpsert))
	if afterUpsert.Name != "upserted" || afterUpsert.Score != 15 {
		t.Fatalf("Upsert() persisted %#v, want updated document", afterUpsert)
	}

	record.Name = "updated"
	record.Score = 20
	RequireNoORMError(t, app.Database().Table(collection).
		Where("_id = ?", id).
		Update(&record))

	var updated documentRecord
	RequireNoORMError(t, app.Database().Table(collection).Where("_id = ?", id).Find(&updated))
	if updated.Name != "updated" || updated.Score != 20 {
		t.Fatalf("Update() persisted %#v, want updated document", updated)
	}

	RequireNoORMError(t, app.Database().Table(collection).
		Where("_id = ?", id).
		Delete(&record))
	var remaining []documentRecord
	deleteQuery := app.Database().Table(collection).Where("_id = ?", id).Find(&remaining)
	if deleteQuery.Error != nil && deleteQuery.HTTPCode != 404 {
		t.Fatalf("Find() after Delete() returned HTTP %d: %v", deleteQuery.HTTPCode, deleteQuery.Error)
	}
	if len(remaining) != 0 {
		t.Fatalf("Delete() left matching documents: %#v", remaining)
	}
}

func ScyllaCRUD(t *testing.T) {
	t.Helper()

	app := OpenApp(t, database.SCYLLA)
	table := uniqueIdentifier(t, "g_it_")
	id := time.Now().UnixNano()

	ExecCQL(t, app, fmt.Sprintf(
		"CREATE TABLE %s (id bigint PRIMARY KEY, name text, score bigint)",
		table,
	))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		message, code, err := app.Database().ExecCQL(ctx, "DROP TABLE "+table)
		if err != nil {
			t.Errorf("drop integration table %s (HTTP %d, %s): %v", table, code, message, err)
		}
	})

	record := scyllaRecord{ID: id, Name: "integration", Score: 10}
	RequireNoORMError(t, app.Database().Table(table).Create(&record))

	var loaded scyllaRecord
	RequireNoORMError(t, app.Database().Table(table).Where("id = ?", id).Find(&loaded))
	if loaded.ID != id || loaded.Name != record.Name || loaded.Score != record.Score {
		t.Fatalf("Find() returned %#v, want inserted Scylla row", loaded)
	}

	record.Name = "updated"
	record.Score = 20
	RequireNoORMError(t, app.Database().Table(table).Upsert().Create(&record))

	var updated scyllaRecord
	RequireNoORMError(t, app.Database().Table(table).Where("id = ?", id).Find(&updated))
	if updated.Name != "updated" || updated.Score != 20 {
		t.Fatalf("Update() persisted %#v, want updated Scylla row", updated)
	}

	RequireNoORMError(t, app.Database().Table(table).Delete(&record))
	var deleted scyllaRecord
	deleteQuery := app.Database().Table(table).Where("id = ?", id).Find(&deleted)
	if deleteQuery.Error == nil || deleteQuery.HTTPCode != 404 {
		t.Fatalf("Find() after Delete() returned HTTP %d, want 404 Not Found: %v", deleteQuery.HTTPCode, deleteQuery.Error)
	}
}

type sqlRecord struct {
	ID    int64  `gorest:"id, primary_key"`
	Name  string `gorest:"name"`
	Score int64  `gorest:"score"`
}

type documentRecord struct {
	ID    string `gorest:"_id, primary_key"`
	Name  string `gorest:"name"`
	Score int64  `gorest:"score"`
}

type scyllaRecord struct {
	ID    int64  `gorest:"id, primary_key"`
	Name  string `gorest:"name"`
	Score int64  `gorest:"score"`
}

type groupRecord struct {
	Score int64 `gorest:"score"`
	Count int64 `gorest:"entry_count"`
}

func uniqueIdentifier(t *testing.T, prefix string) string {
	t.Helper()

	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("generate unique integration identifier: %v", err)
	}
	return prefix + hex.EncodeToString(suffix)
}
