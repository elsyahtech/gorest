package monitoring

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/server"
)

func TestDatabaseMetricsEndpointReportsApplicationSQLMetrics(t *testing.T) {
	config := database.Config{
		Driver:             database.SQLITE,
		Name:               "database_metrics",
		DatabaseDirectory:  t.TempDir(),
		MaxOpenConnections: 4,
	}
	db, _, err := database.New(context.Background(), &config, "UTC")
	if err != nil {
		t.Fatalf("initialize SQLite database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.SQL.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	if _, err := db.SQL.Exec(`CREATE TABLE metrics_sample (value TEXT)`); err != nil {
		t.Fatalf("create sample table: %v", err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO metrics_sample VALUES ('sample')`); err != nil {
		t.Fatalf("insert sample row: %v", err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO missing_metrics_table VALUES ('failure')`); err == nil {
		t.Fatal("invalid SQL statement unexpectedly succeeded")
	}
	tx, err := db.SQL.Begin()
	if err != nil {
		t.Fatalf("begin SQL transaction: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit SQL transaction: %v", err)
	}

	httpServer, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("create HTTP server: %v", err)
	}
	routes := httpServer.Group("/gorest")
	Register(routes, config, db)

	response, err := httpServer.SRV.Test(httptest.NewRequest(http.MethodGet, "/gorest/database", nil))
	if err != nil {
		t.Fatalf("request database metrics endpoint: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("database metrics status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read database metrics response: %v", err)
	}
	var result databaseMetricsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode database metrics response: %v", err)
	}
	metric := result.Database
	if result.Status != "partial" {
		t.Errorf("database metrics status = %q, want partial for unsupported SQLite row estimate", result.Status)
	}
	if metric == nil || metric.Driver != database.SQLITE || metric.Scope != "application_database_client" {
		t.Fatalf("database metric = %#v; want SQLite application metrics", metric)
	}
	if metric.CounterPeriod != "since_client_init" {
		t.Errorf("counter_period = %q, want since_client_init", metric.CounterPeriod)
	}
	if metric.StorageUsed == nil || *metric.StorageUsed == "" {
		t.Errorf("storage_used = %v, want formatted storage size", metric.StorageUsed)
	}
	if metric.TableCount == nil || *metric.TableCount != 1 {
		t.Errorf("table_count = %v, want 1", metric.TableCount)
	}
	if metric.RowCount != nil || metric.UnavailableMetrics["row_count"] != metricUnsupportedReason {
		t.Errorf("row_count = %v and unavailable = %q; want unavailable for SQLite", metric.RowCount, metric.UnavailableMetrics["row_count"])
	}
	if metric.ConnectionActive == nil || metric.ConnectionIdle == nil || metric.ConnectionMax == nil ||
		metric.ConnectionWaitCount == nil || metric.ConnectionWaitDuration == nil || metric.PoolUsagePercent == nil {
		t.Errorf("SQL pool metrics are incomplete: %#v", metric)
	}
	if metric.QueryTotal == nil || *metric.QueryTotal < 3 {
		t.Errorf("query_total = %v, want at least 3 application SQL operations", metric.QueryTotal)
	}
	if metric.QueryErrorTotal == nil || *metric.QueryErrorTotal != 1 {
		t.Errorf("query_error_total = %v, want 1", metric.QueryErrorTotal)
	}
	if metric.TransactionCommitTotal == nil || *metric.TransactionCommitTotal != 1 {
		t.Errorf("transaction_commit_total = %v, want 1", metric.TransactionCommitTotal)
	}
	if metric.TransactionRollbackTotal == nil || *metric.TransactionRollbackTotal != 0 {
		t.Errorf("transaction_rollback_total = %v, want 0", metric.TransactionRollbackTotal)
	}
}
