//go:build integration

package scylla_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestScyllaServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.SCYLLA)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var version string
	_, _, err := app.Database().RawQueryScylla(ctx, func(ctx context.Context, session *gocql.Session) error {
		return session.Query("SELECT release_version FROM system.local").WithContext(ctx).Scan(&version)
	})
	if err != nil {
		t.Fatalf("query ScyllaDB server version failed: %v", err)
	}

	t.Logf("ScyllaDB server version: %s", version)
}

func TestORMCRUD(t *testing.T) {
	internaltest.ScyllaCRUD(t)
}

func TestScyllaCoverage(t *testing.T) {
	app := internaltest.OpenApp(t, database.SCYLLA)
	table := "audit_scylla_" + fmt.Sprintf("%x", time.Now().UnixNano())[:8]
	internaltest.ExecCQL(t, app, "CREATE TABLE "+table+
		" (tenant_id text, item_id text, name text, optional_text text, PRIMARY KEY (tenant_id, item_id))")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		message, code, err := app.Database().ExecCQL(ctx, "DROP TABLE "+table)
		if err != nil {
			t.Errorf("drop integration table %s (HTTP %d, %s): %v", table, code, message, err)
		}
	})

	type record struct {
		TenantID     string  `gorest:"tenant_id, primary_key"`
		ItemID       string  `gorest:"item_id, primary_key"`
		Name         string  `gorest:"name"`
		OptionalText *string `gorest:"optional_text"`
	}
	original := record{TenantID: "tenant", ItemID: "item", Name: "before"}
	internaltest.RequireNoORMError(t, app.Database().Table(table).Create(&original))

	if query := app.Database().Table(table).Upsert("tenant_id").Create(&record{
		TenantID: "tenant", ItemID: "item", Name: "invalid conflict target",
	}); query.Error == nil {
		t.Fatal("Upsert() with only part of the composite key unexpectedly succeeded")
	}
	if query := app.Database().Table(table).Upsert("tenant_id = ?", "tenant").Create(&record{
		TenantID: "tenant", ItemID: "item", Name: "conditional",
	}); query.Error == nil {
		t.Fatal("conditional ScyllaDB Upsert() unexpectedly succeeded")
	}

	updated := record{TenantID: "tenant", ItemID: "item", Name: "after"}
	internaltest.RequireNoORMError(t, app.Database().Table(table).Upsert("tenant_id", "item_id").Create(&updated))

	var loaded record
	internaltest.RequireNoORMError(t, app.Database().Table(table).
		Where("tenant_id = ? AND item_id = ?", updated.TenantID, updated.ItemID).Find(&loaded))
	if loaded.Name != "after" || loaded.OptionalText != nil {
		t.Fatalf("ScyllaDB composite-key Upsert/nullable mapping = %#v", loaded)
	}

	if query := app.Database().Table(table).Update(&record{Name: "unfiltered"}); query.Error == nil {
		t.Fatal("unfiltered Update() unexpectedly succeeded")
	}
	if query := app.Database().Table(table).Delete(&record{}); query.Error == nil {
		t.Fatal("unfiltered Delete() unexpectedly succeeded")
	}
}

func TestScyllaCanceledContextIsPropagated(t *testing.T) {
	app := internaltest.OpenApp(t, database.SCYLLA)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := app.Database().RawQueryScylla(ctx, func(ctx context.Context, session *gocql.Session) error {
		var version string
		return session.Query("SELECT release_version FROM system.local").WithContext(ctx).Scan(&version)
	})
	if err == nil {
		t.Fatal("ScyllaDB query with a canceled context unexpectedly succeeded")
	}
}
