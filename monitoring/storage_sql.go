package monitoring

import (
	"context"
	"errors"
	"fmt"

	"github.com/elsyahtech/gorest/database"
)

func querySQLStorage(
	ctx context.Context,
	driver string,
	databaseConn *database.Database,
) (usedBytes int64, measurement string, scope string, resultErr error) {
	query := ""
	scope = "configured_database"

	switch driver {
	case database.POSTGRES:
		measurement = "database_size"
		query = "SELECT pg_database_size(current_database())"
	case database.MYSQL:
		measurement = "table_data_and_index_size"
		query = "SELECT COALESCE(SUM(data_length + index_length), 0) FROM information_schema.tables WHERE table_schema = DATABASE()"
	case database.SQLSERVER:
		measurement = "allocated_data_file_bytes"
		query = "SELECT COALESCE(SUM(CAST(FILEPROPERTY(name, 'SpaceUsed') AS bigint)), 0) * 8192 FROM sys.database_files WHERE type_desc = 'ROWS'"
	case database.ORACLE:
		scope = "configured_schema"
		measurement = "schema_segment_bytes"
		query = "SELECT COALESCE(SUM(bytes), 0) FROM user_segments"
	case database.SQLITE:
		scope = "configured_database_file"
		measurement = "database_page_bytes"
		query = "SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()"
	default:
		return 0, "", "", fmt.Errorf("unsupported SQL storage driver %q", driver)
	}

	usedBytes, resultErr = querySQLInt64(ctx, databaseConn, query)
	if resultErr != nil {
		return 0, "", "", resultErr
	}

	return usedBytes, measurement, scope, nil
}

func querySQLInt64(ctx context.Context, databaseConn *database.Database, query string) (value int64, resultErr error) {
	if databaseConn.SQL == nil {
		return 0, errors.New("SQL database connection is unavailable")
	}

	rows, err := databaseConn.SQL.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("query database storage statistics: %w", err)
	}

	if rows == nil {
		return 0, errors.New("storage query returned no rows")
	}

	defer func() {
		if err := rows.Close(); resultErr == nil && err != nil {
			resultErr = fmt.Errorf("close storage statistics rows: %w", err)
		}
	}()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, fmt.Errorf("read storage statistics rows: %w", err)
		}

		return 0, errors.New("storage query returned no result")
	}

	if err := rows.Scan(&value); err != nil {
		return 0, fmt.Errorf("scan database storage statistics: %w", err)
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read storage statistics rows: %w", err)
	}

	return value, nil
}
