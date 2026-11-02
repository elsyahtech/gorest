//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
	"github.com/lib/pq"
)

func TestPostgresServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.POSTGRES)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, message, code, err := app.Database().QuerySQL(ctx, "SHOW server_version")
	if err != nil {
		t.Fatalf("query PostgreSQL server version failed (HTTP %d, %s): %v", code, message, err)
	}
	if !rows.Next() {
		rows.Close()
		t.Fatalf("PostgreSQL server version query returned no row: %v", rows.Err())
	}

	var version string
	if err := rows.Scan(&version); err != nil {
		rows.Close()
		t.Fatalf("scan PostgreSQL server version: %v", err)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("iterate PostgreSQL server version result: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close PostgreSQL server version rows: %v", err)
	}

	t.Logf("PostgreSQL server version: %s", version)
}

func TestPostgresStorageUsageEndpoint(t *testing.T) {
	internaltest.StorageUsage(t, database.POSTGRES)
}

func TestPostgresMutualTLSRequiresClientCertificate(t *testing.T) {
	const prefix = "GOREST_IT_POSTGRES_MTLS"
	if os.Getenv(prefix+"_HOST") == "" {
		t.Skipf("set %s_HOST to run PostgreSQL mTLS integration tests", prefix)
	}

	cfg := database.ConfigFromEnv(prefix)
	cfg.Driver = database.POSTGRES
	cfg.Timeout = 10 * time.Second
	cfg.TLS = database.TLSVerifyFull
	for _, name := range []string{
		"HOST", "PORT", "NAME", "USERNAME", "PASSWORD", "SSL_CA", "SSL_CERT", "SSL_KEY",
	} {
		if strings.TrimSpace(os.Getenv(prefix+"_"+name)) == "" {
			t.Fatalf("required integration-test environment variable %s_%s is not set", prefix, name)
		}
	}

	connect := func(config database.Config) (*database.Database, error) {
		ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
		defer cancel()
		db, _, err := database.New(ctx, &config, "UTC")
		if err != nil {
			return nil, err
		}
		return db, nil
	}

	clientDB, err := connect(cfg)
	if err != nil {
		t.Fatalf("connect to PostgreSQL with the configured client certificate: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	var encrypted bool
	var clientDN sql.NullString
	if err := clientDB.SQL.QueryRowContext(ctx,
		"SELECT ssl, client_dn FROM pg_stat_ssl WHERE pid = pg_backend_pid()",
	).Scan(&encrypted, &clientDN); err != nil {
		t.Fatalf("query PostgreSQL mTLS session state: %v", err)
	}
	if !encrypted {
		t.Fatal("PostgreSQL session reports TLS disabled")
	}
	if !clientDN.Valid || strings.TrimSpace(clientDN.String) == "" {
		t.Fatal("PostgreSQL session did not report the presented client certificate")
	}
	if _, _, err := clientDB.Close(ctx, &cfg); err != nil {
		t.Fatalf("close PostgreSQL mTLS connection: %v", err)
	}

	withoutClientCertificate := cfg
	withoutClientCertificate.SSLCertPath = ""
	withoutClientCertificate.SSLKeyPath = ""
	dbWithoutCertificate, err := connect(withoutClientCertificate)
	if err == nil {
		if _, _, closeErr := dbWithoutCertificate.Close(ctx, &withoutClientCertificate); closeErr != nil {
			t.Errorf("close PostgreSQL connection accepted without a client certificate: %v", closeErr)
		}
		t.Fatal("PostgreSQL accepted a connection without the required client certificate")
	}
	var postgresErr *pq.Error
	if !errors.As(err, &postgresErr) || postgresErr.Code != "28000" {
		t.Fatalf("PostgreSQL connection without a client certificate error = %v, want authentication rejection (SQLSTATE 28000)", err)
	}
}

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.POSTGRES)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.POSTGRES)
}

func TestORMParallelTransactions(t *testing.T) {
	internaltest.SQLParallelTransactions(t, database.POSTGRES)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.POSTGRES)
}
