package monitoring

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/server"
)

const collectionTimeout = 3 * time.Second
const collectionFailedReason = "collection_failed"
const metricUnsupportedReason = "unsupported"

var errUnsupportedDriver = errors.New("storage metrics do not support configured database driver")

type storageMetric struct {
	Driver      string `json:"driver"`
	Scope       string `json:"scope"`
	Measurement string `json:"measurement"`
	StorageUsed string `json:"storage_used"` //nolint:tagliatelle // Operational responses use snake_case JSON keys.
}

type storageResponse struct {
	Status   string         `json:"status"`
	Database *storageMetric `json:"database,omitempty"`
	Reason   string         `json:"reason,omitempty"`
}

// Register adds the database storage usage endpoint to the supplied route group.
func Register(routes *server.Server, config database.Config, databaseConn *database.Database) {
	if routes == nil {
		return
	}

	registerDatabaseMetrics(routes, "/database", config, databaseConn, nil)
	registerStorageMetrics(routes, "/storage", config, databaseConn, nil)
}

func registerStorageMetrics(
	routes *server.Server,
	path string,
	config database.Config,
	databaseConn *database.Database,
	middleware server.HandlerFunc,
) {
	if routes == nil {
		return
	}

	handler := func(requestContext *server.Context) error {
		ctx, cancel := context.WithTimeout(requestContext.Context(), collectionTimeout)
		defer cancel()

		metric, err := collectStorage(ctx, config, databaseConn)
		if err != nil {
			return requestContext.Status(http.StatusServiceUnavailable).JSON(storageResponse{
				Status: "unavailable",
				Reason: storageFailureReason(err),
			})
		}

		return requestContext.JSON(storageResponse{
			Status:   "available",
			Database: metric,
		})
	}
	registerGet(routes, path, middleware, handler)
}

func registerGet(routes *server.Server, path string, middleware, handler server.HandlerFunc) {
	if middleware == nil {
		routes.Get(path, handler)
		return
	}

	routes.Get(path, middleware, handler)
}

func collectStorage(ctx context.Context, config database.Config, databaseConn *database.Database) (*storageMetric, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errors.New("storage collection context is unavailable")
	}

	if databaseConn == nil {
		return nil, errors.New("database source is unavailable")
	}

	driver := database.NormalizeDatabaseDriver(config.Driver)
	metric := &storageMetric{
		Driver: driver,
		Scope:  "configured_database",
	}

	var (
		usedBytes int64
		err       error
	)

	switch driver {
	case database.POSTGRES, database.MYSQL, database.SQLSERVER, database.ORACLE, database.SQLITE:
		usedBytes, metric.Measurement, metric.Scope, err = querySQLStorage(ctx, driver, databaseConn)
	case database.MONGO:
		metric.Measurement = "database_storage_and_index_bytes"
		usedBytes, err = queryMongoStorage(ctx, databaseConn, config.Name)
	case database.SCYLLA:
		metric.Scope = "configured_keyspace"
		metric.Measurement = "estimated_table_bytes"
		usedBytes, err = queryScyllaStorage(ctx, databaseConn, config.Name)
	default:
		return nil, fmt.Errorf("%w: %q", errUnsupportedDriver, driver)
	}

	if err != nil {
		return nil, fmt.Errorf("collect storage usage for %s: %w", driver, err)
	}

	if usedBytes < 0 {
		return nil, errors.New("storage usage returned a negative value")
	}

	metric.StorageUsed = formatStorageSize(usedBytes)

	return metric, nil
}

func formatStorageSize(size int64) string {
	const unit = 1024

	units := [...]string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}

	if size < unit {
		return strconv.FormatInt(size, 10) + " B"
	}

	value := float64(size)

	unitIndex := 0
	for value >= unit && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}

	formatted := strconv.FormatFloat(value, 'f', 1, 64)
	if len(formatted) >= 2 && formatted[len(formatted)-2:] == ".0" {
		formatted = formatted[:len(formatted)-2]
	}

	return formatted + " " + units[unitIndex]
}

func storageFailureReason(err error) string {
	if errors.Is(err, errUnsupportedDriver) {
		return "unsupported_driver"
	}

	return collectionFailedReason
}
