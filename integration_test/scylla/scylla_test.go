//go:build integration

package scylla_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestScyllaMTLSExternal(t *testing.T) {
	const prefix = "GOREST_IT_SCYLLA_MTLS"
	if strings.TrimSpace(os.Getenv(prefix+"_HOST")) == "" {
		t.Skipf("set %s_HOST to run the external ScyllaDB mTLS test", prefix)
	}

	config := database.Config{
		Driver:      database.SCYLLA,
		Host:        requiredScyllaMTLSEnvironmentValue(t, prefix+"_HOST"),
		Port:        requiredScyllaMTLSEnvironmentValue(t, prefix+"_PORT"),
		Name:        requiredScyllaMTLSEnvironmentValue(t, prefix+"_NAME"),
		Username:    requiredScyllaMTLSEnvironmentValue(t, prefix+"_USERNAME"),
		Password:    requiredScyllaMTLSEnvironmentValue(t, prefix+"_PASSWORD"),
		TLS:         "enable",
		SSLCAPath:   requiredScyllaMTLSEnvironmentValue(t, prefix+"_SSL_CA"),
		SSLCertPath: requiredScyllaMTLSEnvironmentValue(t, prefix+"_SSL_CERT"),
		SSLKeyPath:  requiredScyllaMTLSEnvironmentValue(t, prefix+"_SSL_KEY"),
		Timeout:     10 * time.Second,
	}

	withoutCertificate := config
	withoutCertificate.SSLCertPath = ""
	withoutCertificate.SSLKeyPath = ""
	noCertificateCtx, cancelNoCertificate := context.WithTimeout(context.Background(), 12*time.Second)
	withoutCertificateDB, _, err := database.New(noCertificateCtx, &withoutCertificate, "UTC")
	cancelNoCertificate()
	if err == nil {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		if _, _, closeErr := withoutCertificateDB.Close(closeCtx, &withoutCertificate); closeErr != nil {
			cancelClose()
			t.Fatalf("close external ScyllaDB connection accepted without client certificate: %v", closeErr)
		}
		cancelClose()
		t.Fatal("external ScyllaDB endpoint accepted a connection without a client certificate; verify that the server requires client authentication")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, message, err := database.New(ctx, &config, "UTC")
	if err != nil {
		t.Fatalf("connect to external ScyllaDB endpoint with client certificate (TLS setup: %s): %v", message, err)
	}
	defer func() {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelClose()
		if _, _, err := db.Close(closeCtx, &config); err != nil {
			t.Errorf("close external ScyllaDB mTLS test connection: %v", err)
		}
	}()

	if message, err := db.Ping(ctx, &config); err != nil {
		t.Fatalf("ping external ScyllaDB endpoint over mTLS (%s): %v", message, err)
	}
	var version string
	if err := db.Scylla.Query("SELECT release_version FROM system.local").
		WithContext(ctx).Scan(&version); err != nil {
		t.Fatalf("query external ScyllaDB endpoint over mTLS: %v", err)
	}
	t.Logf("External ScyllaDB mTLS verified: server version %s; connection without client certificate rejected", version)
}

func requiredScyllaMTLSEnvironmentValue(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("required external ScyllaDB mTLS environment variable %s is not set", name)
	}
	return value
}

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
