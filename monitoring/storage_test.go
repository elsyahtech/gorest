package monitoring

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/server"
)

func TestSQLiteStorageEndpointReportsDatabasePageBytes(t *testing.T) {
	config := database.Config{
		Driver:            database.SQLITE,
		Name:              "storage",
		DatabaseDirectory: t.TempDir(),
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
	if _, err := db.SQL.Exec(`CREATE TABLE storage_sample (value TEXT)`); err != nil {
		t.Fatalf("create SQLite sample table: %v", err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO storage_sample VALUES ('sample')`); err != nil {
		t.Fatalf("prepare SQLite database: %v", err)
	}

	httpServer, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("create HTTP server: %v", err)
	}
	routes := httpServer.Group("/gorest")
	Register(routes, config, db)

	response, err := httpServer.SRV.Test(httptest.NewRequest(http.MethodGet, "/gorest/storage", nil))
	if err != nil {
		t.Fatalf("request storage endpoint: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("storage endpoint status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read storage response: %v", err)
	}
	var result storageResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode storage response: %v", err)
	}
	if result.Status != "available" {
		t.Fatalf("storage status = %q, want available", result.Status)
	}
	if result.Database == nil {
		t.Fatalf("storage response has no database metric: %s", body)
	}
	if result.Database.Driver != database.SQLITE ||
		result.Database.Scope != "configured_database_file" ||
		result.Database.Measurement != "database_page_bytes" {
		t.Errorf("database metric = %#v; want SQLite configured file page bytes", result.Database)
	}
	if result.Database.UsedBytes <= 0 {
		t.Errorf("database used bytes = %d, want positive value", result.Database.UsedBytes)
	}
}

func TestStorageEndpointReportsUnsupportedDriverWithoutDetails(t *testing.T) {
	httpServer, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("create HTTP server: %v", err)
	}
	routes := httpServer.Group("/gorest")
	Register(routes, database.Config{Driver: "unknown"}, &database.Database{})

	response, err := httpServer.SRV.Test(httptest.NewRequest(http.MethodGet, "/gorest/storage", nil))
	if err != nil {
		t.Fatalf("request storage endpoint: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("storage endpoint status = %d, want %d", response.StatusCode, http.StatusServiceUnavailable)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read storage response: %v", err)
	}
	var result storageResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode storage response: %v", err)
	}
	if result.Status != "unavailable" || result.Reason != "unsupported_driver" {
		t.Fatalf("storage response = %s; want sanitized unsupported-driver response", body)
	}
	if strings.Contains(string(body), "unknown") {
		t.Fatalf("storage response exposed configured driver: %s", body)
	}
}
