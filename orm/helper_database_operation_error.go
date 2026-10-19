package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/elsyahtech/gorest/database"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type databaseOperationErrorKind string

const (
	databaseConflictError    databaseOperationErrorKind = "conflict"
	databaseTimeoutError     databaseOperationErrorKind = "timeout"
	databaseUnavailableError databaseOperationErrorKind = "unavailable"
	databaseInvalidQuery     databaseOperationErrorKind = "invalid_query"
	databaseExecutionError   databaseOperationErrorKind = "execution_failed"
)

func (orm *ORM) setDatabaseOperationError(operation string, err error) error {
	if err == nil {
		return nil
	}

	driverName := ""

	if orm.DatabaseConfig != nil {
		driverName = orm.DatabaseConfig.Driver
	}

	kind, status := classifyDatabaseOperationError(driverName, err)

	return orm.setDatabaseOperationErrorKind(operation, err, kind, status)
}

func (orm *ORM) setDatabaseOperationErrorKind(
	operation string,
	err error,
	kind databaseOperationErrorKind,
	status int,
) error {
	if err == nil {
		return nil
	}

	driverName := ""
	if orm.DatabaseConfig != nil {
		driverName = orm.DatabaseConfig.Driver
	}

	troubleshooting := databaseTroubleshootingMessage(kind)
	message := fmt.Sprintf(
		"%s failed: %s Reference: %s",
		strings.ToLower(operation),
		troubleshooting,
		databaseErrorReference(driverName),
	)
	standardizedErr := fmt.Errorf("database: %s: %w", kind, err)

	return orm.setError(message, standardizedErr, status)
}

func classifyDatabaseOperationError(driverName string, err error) (databaseOperationErrorKind, int) {
	if database.IsDuplicateKeyError(err) || mongo.IsDuplicateKeyError(err) || isDuplicateRequestValueError(err) {
		return databaseConflictError, http.StatusConflict
	}

	if errors.Is(err, context.DeadlineExceeded) || mongo.IsTimeout(err) {
		return databaseTimeoutError, http.StatusGatewayTimeout
	}

	var (
		scyllaWriteTimeout *gocql.RequestErrWriteTimeout
		scyllaReadTimeout  *gocql.RequestErrReadTimeout
	)

	if errors.As(err, &scyllaWriteTimeout) || errors.As(err, &scyllaReadTimeout) {
		return databaseTimeoutError, http.StatusGatewayTimeout
	}

	var scyllaUnavailable *gocql.RequestErrUnavailable

	if errors.As(err, &scyllaUnavailable) {
		return databaseUnavailableError, http.StatusServiceUnavailable
	}

	var networkErr net.Error
	if errors.As(err, &networkErr) {
		if networkErr.Timeout() {
			return databaseTimeoutError, http.StatusGatewayTimeout
		}

		return databaseUnavailableError, http.StatusServiceUnavailable
	}

	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return databaseUnavailableError, http.StatusServiceUnavailable
	}

	if isInvalidDatabaseQueryError(driverName, err) {
		return databaseInvalidQuery, http.StatusBadRequest
	}

	return databaseExecutionError, http.StatusInternalServerError
}

// Some databases and application triggers report request-level unique-value
// collisions as text instead of a native unique-constraint error code.
func isDuplicateRequestValueError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate email in request") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "unique violation")
}

func databaseTroubleshootingMessage(kind databaseOperationErrorKind) string {
	switch kind {
	case databaseConflictError:
		return "data violates a primary-key or unique constraint. Verify the key values and database constraints."
	case databaseTimeoutError:
		return "the database operation timed out. Check database health, query duration, and the configured timeout."
	case databaseUnavailableError:
		return "the database is unavailable. Check that the database service is running and reachable."
	case databaseInvalidQuery:
		return "the database rejected the query. Verify the query clauses, table or collection schema, and field names."
	default:
		return "the database operation could not be completed. Verify connectivity, permissions, schema, query syntax, and data types."
	}
}

func databaseErrorReference(driverName string) string {
	switch driverName {
	case database.ORACLE:
		return "https://docs.oracle.com/en/database/oracle/oracle-database/23/"
	case database.SQLSERVER:
		return "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/database-engine-events-and-errors?view=sql-server-ver17"
	case database.MYSQL:
		return "https://dev.mysql.com/doc/mysql-errors/8.4/en/"
	case database.POSTGRES:
		return "https://www.postgresql.org/docs/current/errcodes-appendix.html"
	case database.SQLITE:
		return "https://www.sqlite.org/rescode.html"
	case database.MONGO:
		return "https://www.mongodb.com/docs/manual/reference/error-codes/"
	case database.SCYLLA:
		return "https://docs.scylladb.com/manual/stable/cql/"
	default:
		return "the configured database vendor's official documentation"
	}
}
