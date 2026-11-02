package monitoring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/elsyahtech/gorest/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func collectSQLTableCounts(
	ctx context.Context,
	driver string,
	databaseConn *database.Database,
) (tableCountValue *int64, rowCountValue *int64, resultErr error) {
	if databaseConn.SQL == nil {
		return nil, nil, errors.New("SQL database connection is unavailable")
	}

	var query string

	switch driver {
	case database.POSTGRES:
		query = `
			SELECT COUNT(*)::bigint,
				CASE WHEN COUNT(*) FILTER (WHERE c.relkind <> 'p' AND c.reltuples < 0) > 0
					THEN NULL
					ELSE CAST(COALESCE(SUM(c.reltuples) FILTER (WHERE c.relkind <> 'p'), 0) AS bigint)
				END
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relkind IN ('r', 'm', 'p')
				AND n.nspname NOT IN ('pg_catalog', 'information_schema')
				AND n.nspname NOT LIKE 'pg_toast%'
		`
	case database.MYSQL:
		query = `
			SELECT COUNT(*), COALESCE(SUM(TABLE_ROWS), 0)
			FROM information_schema.tables
			WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'
		`
	case database.SQLSERVER:
		query = `
			SELECT COUNT(DISTINCT t.object_id),
				COALESCE(SUM(CAST(p.rows AS bigint)), 0)
			FROM sys.tables t
			LEFT JOIN sys.partitions p
				ON p.object_id = t.object_id AND p.index_id IN (0, 1)
			WHERE t.is_ms_shipped = 0
		`
	case database.ORACLE:
		query = `
			SELECT COUNT(*),
				CASE WHEN COUNT(*) = 0 THEN 0
					WHEN COUNT(*) = COUNT(num_rows) THEN SUM(num_rows)
					ELSE NULL
				END
			FROM user_tables
		`
	case database.SQLITE:
		query = `SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`
	default:
		return nil, nil, fmt.Errorf("unsupported SQL table metrics driver %q", driver)
	}

	var tableCount int64
	if driver == database.SQLITE {
		if err := databaseConn.SQL.QueryRowContext(ctx, query).Scan(&tableCount); err != nil {
			return nil, nil, fmt.Errorf("query SQLite table count: %w", err)
		}

		return &tableCount, nil, nil
	}

	var rowCount sql.NullInt64
	if err := databaseConn.SQL.QueryRowContext(ctx, query).Scan(&tableCount, &rowCount); err != nil {
		return nil, nil, fmt.Errorf("query database table and row estimates: %w", err)
	}

	if !rowCount.Valid {
		return &tableCount, nil, nil
	}

	if rowCount.Int64 < 0 {
		return nil, nil, errors.New("database returned a negative row estimate")
	}

	return &tableCount, &rowCount.Int64, nil
}

func collectMongoCounts(
	ctx context.Context,
	databaseConn *database.Database,
	databaseName string,
) (tableCount *int64, rowCount *int64, resultErr error) {
	if databaseName == "" {
		return nil, nil, errors.New("configured MongoDB database name is empty")
	}

	if databaseConn.Mongo == nil {
		return nil, nil, errors.New("MongoDB connection is unavailable")
	}

	var stats struct {
		Collections int64 `bson:"collections"`
		Objects     int64 `bson:"objects"`
	}

	result := databaseConn.Mongo.Database(databaseName).RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}})
	if err := result.Decode(&stats); err != nil {
		return nil, nil, fmt.Errorf("query MongoDB collection and object estimates: %w", err)
	}

	if stats.Collections < 0 || stats.Objects < 0 {
		return nil, nil, errors.New("MongoDB returned negative database statistics")
	}

	return &stats.Collections, &stats.Objects, nil
}

func collectScyllaTableCount(ctx context.Context, databaseConn *database.Database, keyspace string) (*int64, error) {
	if keyspace == "" {
		return nil, errors.New("configured ScyllaDB keyspace is empty")
	}

	if databaseConn.Scylla == nil {
		return nil, errors.New("ScyllaDB connection is unavailable")
	}

	iter := databaseConn.Scylla.Query(
		"SELECT table_name FROM system_schema.tables WHERE keyspace_name = ?",
		keyspace,
	).IterContext(ctx)
	if iter == nil {
		return nil, errors.New("ScyllaDB returned no table metadata iterator")
	}

	var (
		tableName  string
		tableCount int64
	)
	for iter.Scan(&tableName) {
		tableCount++
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("read ScyllaDB table metadata: %w", err)
	}

	return &tableCount, nil
}
