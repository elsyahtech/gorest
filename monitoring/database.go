package monitoring

import (
	"context"
	"errors"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/server"
)

//nolint:govet // Keep status first to match other operational endpoint responses.
type databaseMetricsResponse struct {
	Status   string           `json:"status"`
	Database *databaseMetrics `json:"database"`
}

//nolint:govet,tagliatelle // Preserve the established snake_case API and field order.
type databaseMetrics struct {
	Driver                   string            `json:"driver"`
	Scope                    string            `json:"scope"`
	CounterPeriod            string            `json:"counter_period"`
	SlowQueryThreshold       string            `json:"slow_query_threshold"`
	StorageUsed              *string           `json:"storage_used"`
	StorageScope             *string           `json:"storage_scope"`
	StorageMeasurement       *string           `json:"storage_measurement"`
	TableCount               *int64            `json:"table_count"`
	RowCount                 *int64            `json:"row_count"`
	ConnectionActive         *int64            `json:"connection_active"`
	ConnectionIdle           *int64            `json:"connection_idle"`
	ConnectionMax            *int64            `json:"connection_max"`
	ConnectionWaitCount      *int64            `json:"connection_wait_count"`
	ConnectionWaitDuration   *string           `json:"connection_wait_duration"`
	QueryTotal               *int64            `json:"query_total"`
	QuerySlowTotal           *int64            `json:"query_slow_total"`
	QueryErrorTotal          *int64            `json:"query_error_total"`
	TransactionCommitTotal   *int64            `json:"transaction_commit_total"`
	TransactionRollbackTotal *int64            `json:"transaction_rollback_total"`
	PoolUsagePercent         *float64          `json:"pool_usage_percent"`
	UnavailableMetrics       map[string]string `json:"unavailable_metrics"`
}

func registerDatabaseMetrics(routes *server.Server, config database.Config, databaseConn *database.Database) {
	if routes == nil {
		return
	}

	routes.Get("/database", func(requestContext *server.Context) error {
		ctx, cancel := context.WithTimeout(requestContext.Context(), collectionTimeout)
		defer cancel()

		metric := collectDatabaseMetrics(ctx, config, databaseConn)
		if ctx.Err() != nil {
			markUnavailable(metric, "database_metrics", "collection_timeout")
		}

		status := "available"

		for _, reason := range metric.UnavailableMetrics {
			if reason != "" {
				status = "partial"
				break
			}
		}

		return requestContext.JSON(databaseMetricsResponse{Status: status, Database: metric})
	})
}

func collectDatabaseMetrics(ctx context.Context, config database.Config, databaseConn *database.Database) *databaseMetrics {
	driver := database.NormalizeDatabaseDriver(config.Driver)
	metric := &databaseMetrics{
		Driver:             driver,
		Scope:              "application_database_client",
		CounterPeriod:      "since_client_init",
		SlowQueryThreshold: time.Second.String(),
		UnavailableMetrics: make(map[string]string),
	}

	if databaseConn == nil {
		markAllUnavailable(metric, "connection_unavailable")
		return metric
	}

	if ctx == nil || ctx.Err() != nil {
		markAllUnavailable(metric, "collection_timeout")
		return metric
	}

	collectDatabaseStorage(ctx, config, databaseConn, metric)
	collectDatabaseCounts(ctx, driver, config.Name, databaseConn, metric)
	collectClientMetrics(driver, databaseConn, metric)

	return metric
}

func collectDatabaseStorage(
	ctx context.Context,
	config database.Config,
	databaseConn *database.Database,
	metric *databaseMetrics,
) {
	storage, err := collectStorage(ctx, config, databaseConn)
	if err != nil {
		reason := collectionFailedReason
		if errors.Is(err, errUnsupportedDriver) {
			reason = metricUnsupportedReason
		}

		markUnavailable(metric, "storage_used", reason)
		markUnavailable(metric, "storage_scope", reason)
		markUnavailable(metric, "storage_measurement", reason)
	} else {
		metric.StorageUsed = &storage.StorageUsed
		metric.StorageScope = &storage.Scope
		metric.StorageMeasurement = &storage.Measurement
	}
}

//nolint:wsl_v5 // Keep per-driver availability and failure handling together.
func collectDatabaseCounts(
	ctx context.Context,
	driver string,
	databaseName string,
	databaseConn *database.Database,
	metric *databaseMetrics,
) {
	var err error

	switch driver {
	case database.POSTGRES, database.MYSQL, database.SQLSERVER, database.ORACLE, database.SQLITE:
		metric.TableCount, metric.RowCount, err = collectSQLTableCounts(ctx, driver, databaseConn)
	case database.MONGO:
		metric.TableCount, metric.RowCount, err = collectMongoCounts(ctx, databaseConn, databaseName)
	case database.SCYLLA:
		metric.TableCount, err = collectScyllaTableCount(ctx, databaseConn, databaseName)
		markUnavailable(metric, "row_count", metricUnsupportedReason)
	default:
		markUnavailable(metric, "table_count", metricUnsupportedReason)
		markUnavailable(metric, "row_count", metricUnsupportedReason)
		return
	}

	if err != nil {
		markDatabaseCountFailure(metric, driver, err)

		return
	}

	if metric.RowCount != nil {
		return
	}

	markRowCountUnavailable(metric, driver, "statistics_unavailable")
}

//nolint:wsl_v5 // Keep the table and optional row-estimate failure handling together.
func markDatabaseCountFailure(metric *databaseMetrics, driver string, err error) {
	reason := collectionFailedReason
	if errors.Is(err, errUnsupportedDriver) {
		reason = metricUnsupportedReason
	}

	markUnavailable(metric, "table_count", reason)
	if metric.RowCount != nil {
		return
	}

	markRowCountUnavailable(metric, driver, reason)
}

func markRowCountUnavailable(metric *databaseMetrics, driver, reason string) {
	unavailableReason := reason
	if driver == database.SQLITE || driver == database.SCYLLA {
		unavailableReason = metricUnsupportedReason
	}

	markUnavailable(metric, "row_count", unavailableReason)
}

func collectClientMetrics(driver string, databaseConn *database.Database, metric *databaseMetrics) {
	stats := databaseConn.OperationalMetrics()
	setMetric(metric, "connection_active", stats.ConnectionActive, &metric.ConnectionActive)
	setMetric(metric, "connection_idle", stats.ConnectionIdle, &metric.ConnectionIdle)
	setMetric(metric, "connection_max", stats.ConnectionMax, &metric.ConnectionMax)
	setMetric(metric, "connection_wait_count", stats.ConnectionWaitCount, &metric.ConnectionWaitCount)
	setMetric(metric, "connection_wait_duration", stats.ConnectionWaitDuration, &metric.ConnectionWaitDuration)
	setMetric(metric, "pool_usage_percent", stats.PoolUsagePercent, &metric.PoolUsagePercent)
	setMetric(metric, "query_total", stats.QueryTotal, &metric.QueryTotal)
	setMetric(metric, "query_slow_total", stats.QuerySlowTotal, &metric.QuerySlowTotal)
	setMetric(metric, "query_error_total", stats.QueryErrorTotal, &metric.QueryErrorTotal)
	setMetric(metric, "transaction_commit_total", stats.TransactionCommitTotal, &metric.TransactionCommitTotal)
	setMetric(metric, "transaction_rollback_total", stats.TransactionRollbackTotal, &metric.TransactionRollbackTotal)

	if databaseConn.SQL != nil && databaseConn.SQL.Stats().MaxOpenConnections == 0 {
		markUnavailable(metric, "connection_max", "unlimited")
		markUnavailable(metric, "pool_usage_percent", "unlimited")
	}

	if driver != database.SCYLLA {
		return
	}

	markUnavailable(metric, "connection_active", metricUnsupportedReason)
	markUnavailable(metric, "connection_idle", metricUnsupportedReason)
	markUnavailable(metric, "connection_max", metricUnsupportedReason)
	markUnavailable(metric, "connection_wait_count", metricUnsupportedReason)
	markUnavailable(metric, "connection_wait_duration", metricUnsupportedReason)
	markUnavailable(metric, "pool_usage_percent", metricUnsupportedReason)
}

func markUnavailable(metric *databaseMetrics, name, reason string) {
	if metric.UnavailableMetrics == nil {
		metric.UnavailableMetrics = make(map[string]string)
	}

	metric.UnavailableMetrics[name] = reason
}

func markAllUnavailable(metric *databaseMetrics, reason string) {
	for _, name := range []string{
		"storage_used", "storage_scope", "storage_measurement", "table_count", "row_count",
		"connection_active", "connection_idle", "connection_max",
		"connection_wait_count", "connection_wait_duration",
		"query_total", "query_slow_total", "query_error_total",
		"transaction_commit_total", "transaction_rollback_total",
		"pool_usage_percent",
	} {
		markUnavailable(metric, name, reason)
	}
}

func setMetric[T any](metric *databaseMetrics, name string, value *T, target **T) {
	if value == nil {
		markUnavailable(metric, name, "unsupported")
		return
	}

	*target = value
}
