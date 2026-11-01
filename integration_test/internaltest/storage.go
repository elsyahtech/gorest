//go:build integration

package internaltest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/monitoring"
	"github.com/elsyahtech/gorest/server"
)

func StorageUsage(t *testing.T, driver string) {
	t.Helper()

	config := database.Run(DatabaseConfig(t, driver))

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	databaseConn, message, err := database.New(ctx, &config, "UTC")
	if err != nil {
		t.Fatalf("connect to %s integration database (%s): %v", driver, message, err)
	}

	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()

		if _, _, err := databaseConn.Close(closeCtx, &config); err != nil {
			t.Errorf("close %s storage integration database: %v", driver, err)
		}
	})

	assertStorageUsageEndpoint(ctx, t, driver, config, databaseConn)
}

func assertStorageUsageEndpoint(
	ctx context.Context,
	t *testing.T,
	driver string,
	config database.Config,
	databaseConn *database.Database,
) {
	t.Helper()

	httpServer, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("create HTTP server for %s storage integration test: %v", driver, err)
	}

	monitoring.Register(httpServer.Group("/gorest"), config, databaseConn)

	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/gorest/storage", nil)

	response, err := httpServer.SRV.Test(request)
	if err != nil {
		t.Fatalf("request %s storage endpoint: %v", driver, err)
	}

	body, err := io.ReadAll(response.Body)
	closeErr := response.Body.Close()

	if err != nil {
		t.Fatalf("read %s storage endpoint response: %v", driver, err)
	}

	if closeErr != nil {
		t.Fatalf("close %s storage endpoint response: %v", driver, closeErr)
	}

	assertStorageUsageResponse(ctx, t, driver, databaseConn, config.Name, response.StatusCode, body)
}

type storageEndpointResponse struct {
	Status   string          `json:"status"`
	Reason   string          `json:"reason"`
	Database json.RawMessage `json:"database"`
}

func assertStorageUsageResponse(
	ctx context.Context,
	t *testing.T,
	driver string,
	databaseConn *database.Database,
	keyspace string,
	statusCode int,
	body []byte,
) {
	t.Helper()

	var result storageEndpointResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode %s storage endpoint response: %v", driver, err)
	}

	wantStatus := http.StatusOK
	wantResultStatus := "available"

	if driver == database.SCYLLA && !scyllaStorageEstimatesAvailable(ctx, t, databaseConn, keyspace) {
		wantStatus = http.StatusServiceUnavailable
		wantResultStatus = "unavailable"
	}

	if statusCode != wantStatus {
		t.Fatalf("%s storage endpoint status = %d, want %d; response: %s",
			driver, statusCode, wantStatus, body)
	}

	if result.Status != wantResultStatus {
		t.Fatalf("%s storage status = %q, want %q; response: %s",
			driver, result.Status, wantResultStatus, body)
	}

	if wantResultStatus == "unavailable" {
		if result.Reason != "collection_failed" {
			t.Fatalf("%s unavailable storage reason = %q, want collection_failed; response: %s",
				driver, result.Reason, body)
		}

		t.Logf("%s storage estimates are not available for the configured keyspace", driver)

		return
	}

	validateStorageMetric(t, driver, result.Database, body)
}

func validateStorageMetric(t *testing.T, driver string, rawDatabase json.RawMessage, body []byte) {
	t.Helper()

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawDatabase, &fields); err != nil {
		t.Fatalf("decode %s storage metric: %v; response: %s", driver, err, body)
	}

	var (
		metricDriver      string
		metricScope       string
		metricMeasurement string
		metricUsedBytes   int64
	)

	decodeStorageField(t, driver, fields, "driver", &metricDriver)
	decodeStorageField(t, driver, fields, "scope", &metricScope)
	decodeStorageField(t, driver, fields, "measurement", &metricMeasurement)
	decodeStorageField(t, driver, fields, "used_bytes", &metricUsedBytes)

	if metricDriver != driver {
		t.Errorf("%s storage metric driver = %q", driver, metricDriver)
	}

	if metricMeasurement != storageMeasurement(driver) {
		t.Errorf("%s storage measurement = %q, want %q",
			driver, metricMeasurement, storageMeasurement(driver))
	}

	if metricScope != storageScope(driver) {
		t.Errorf("%s storage scope = %q, want %q", driver, metricScope, storageScope(driver))
	}

	if metricUsedBytes < 0 {
		t.Errorf("%s storage used_bytes = %d, want a non-negative estimate", driver, metricUsedBytes)
	}

	t.Logf("%s storage metric: %d bytes (%s)", driver, metricUsedBytes, metricMeasurement)
}

func decodeStorageField(t *testing.T, driver string, fields map[string]json.RawMessage, name string, value any) {
	t.Helper()

	if err := json.Unmarshal(fields[name], value); err != nil {
		t.Fatalf("decode %s storage metric field %q: %v", driver, name, err)
	}
}

func scyllaStorageEstimatesAvailable(
	ctx context.Context,
	t *testing.T,
	databaseConn *database.Database,
	keyspace string,
) bool {
	t.Helper()

	iter := databaseConn.Scylla.Query(
		"SELECT mean_partition_size, partitions_count FROM system.size_estimates WHERE keyspace_name = ?",
		keyspace,
	).IterContext(ctx)

	var (
		meanPartitionSize int64
		partitionsCount   int64
		hasEstimate       bool
	)
	for iter.Scan(&meanPartitionSize, &partitionsCount) {
		hasEstimate = true
	}

	if err := iter.Close(); err != nil {
		t.Fatalf("query ScyllaDB storage estimates: %v", err)
	}

	return hasEstimate
}

func storageMeasurement(driver string) string {
	switch driver {
	case database.POSTGRES:
		return "database_size"
	case database.MYSQL:
		return "table_data_and_index_size"
	case database.SQLSERVER:
		return "allocated_data_file_bytes"
	case database.ORACLE:
		return "schema_segment_bytes"
	case database.MONGO:
		return "database_storage_and_index_bytes"
	case database.SCYLLA:
		return "estimated_table_bytes"
	default:
		return fmt.Sprintf("unsupported_%s", driver)
	}
}

func storageScope(driver string) string {
	if driver == database.ORACLE {
		return "configured_schema"
	}

	if driver == database.SCYLLA {
		return "configured_keyspace"
	}

	return "configured_database"
}
