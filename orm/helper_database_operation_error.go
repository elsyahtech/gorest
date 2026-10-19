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
	"github.com/go-sql-driver/mysql"
	"github.com/godror/godror"
	"github.com/lib/pq"
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
	_ string,
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

	message := fmt.Sprintf("%s Reference: %s", databaseTroubleshootingMessage(kind), databaseErrorReference(driverName, kind))
	// Preserve the adapter's original error text. The handler owns operation-level
	// wording; ORM only attaches HTTP status metadata while retaining %w behavior.
	standardizedErr := fmt.Errorf("%w", err)

	return orm.setError(message, standardizedErr, status)
}

func classifyDatabaseOperationError(driverName string, err error) (databaseOperationErrorKind, int) {
	if database.IsDuplicateKeyError(err) || mongo.IsDuplicateKeyError(err) || isDuplicateRequestValueError(err) {
		return databaseConflictError, http.StatusConflict
	}

	if errors.Is(err, context.DeadlineExceeded) || mongo.IsTimeout(err) {
		return databaseTimeoutError, http.StatusGatewayTimeout
	}
	if isDriverTimeoutError(driverName, err) {
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
	if isDriverUnavailableError(driverName, err) {
		return databaseUnavailableError, http.StatusServiceUnavailable
	}

	if isInvalidDatabaseQueryError(driverName, err) {
		return databaseInvalidQuery, http.StatusBadRequest
	}

	return databaseExecutionError, http.StatusInternalServerError
}

func isDriverTimeoutError(driverName string, err error) bool {
	switch driverName {
	case database.ORACLE:
		if oracleErr, ok := godror.AsOraErr(err); ok {
			return oracleErr.Code() == 1013
		}
	case database.MYSQL:
		var mysqlErr *mysql.MySQLError
		return errors.As(err, &mysqlErr) && (mysqlErr.Number == 1205 || mysqlErr.Number == 3024)
	case database.POSTGRES:
		var postgresErr *pq.Error
		return errors.As(err, &postgresErr) && postgresErr.Code == "57014"
	case database.SQLSERVER:
		var sqlServerErr interface{ SQLErrorNumber() int32 }
		return errors.As(err, &sqlServerErr) && (sqlServerErr.SQLErrorNumber() == -2 || sqlServerErr.SQLErrorNumber() == 1222)
	}
	return false
}

func isDriverUnavailableError(driverName string, err error) bool {
	switch driverName {
	case database.ORACLE:
		if oracleErr, ok := godror.AsOraErr(err); ok {
			code := oracleErr.Code()
			return code == 3113 || code == 3114 || code >= 12500 && code <= 12599
		}
	case database.MYSQL:
		var mysqlErr *mysql.MySQLError
		return errors.As(err, &mysqlErr) && (mysqlErr.Number == 2002 || mysqlErr.Number == 2003 || mysqlErr.Number == 2006 || mysqlErr.Number == 2013)
	case database.POSTGRES:
		var postgresErr *pq.Error
		return errors.As(err, &postgresErr) && strings.HasPrefix(string(postgresErr.Code), "08")
	case database.SQLSERVER:
		var sqlServerErr interface{ SQLErrorNumber() int32 }
		if errors.As(err, &sqlServerErr) {
			switch sqlServerErr.SQLErrorNumber() {
			case 53, 64, 233, 10053, 10054, 10060:
				return true
			}
		}
	}
	return false
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
		return "The submitted data conflicts with a primary-key or unique constraint. Check the conflicting key values and database constraints."
	case databaseTimeoutError:
		return "The database operation timed out. Check database health, query duration, and the configured timeout."
	case databaseUnavailableError:
		return "The database is unavailable. Check that the database service is running and reachable."
	case databaseInvalidQuery:
		return "The database rejected the query syntax or requested operation. Check the query clauses and supported database features."
	default:
		return "The database operation could not be completed. Check database connectivity, permissions, schema, query syntax, and data types."
	}
}

func databaseErrorReference(driverName string, kind databaseOperationErrorKind) string {
	refs := map[string]map[databaseOperationErrorKind]string{
		database.ORACLE: {
			databaseConflictError:    "https://docs.oracle.com/en/error-help/db/ora-00001/",
			databaseTimeoutError:     "https://docs.oracle.com/en/error-help/db/ora-01013/",
			databaseUnavailableError: "https://docs.oracle.com/en/error-help/db/ora-03113/",
			databaseInvalidQuery:     "https://docs.oracle.com/en/error-help/db/ora-00933/",
			databaseExecutionError:   "https://docs.oracle.com/en/database/oracle/oracle-database/23/errmg/",
		},
		database.SQLSERVER: {
			databaseConflictError:    "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/database-engine-events-and-errors-2000-to-2999?view=sql-server-ver17",
			databaseTimeoutError:     "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/database-engine-events-and-errors-1000-to-1999?view=sql-server-ver17",
			databaseUnavailableError: "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/errors-and-events-reference-database-engine?view=sql-server-ver17",
			databaseInvalidQuery:     "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/database-engine-events-and-errors-0-to-999?view=sql-server-ver17",
			databaseExecutionError:   "https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/errors-and-events-reference-database-engine?view=sql-server-ver17",
		},
		database.MYSQL: {
			databaseConflictError:    "https://dev.mysql.com/doc/mysql-errors/8.4/en/server-error-reference.html",
			databaseTimeoutError:     "https://dev.mysql.com/doc/mysql-errors/8.4/en/server-error-reference.html",
			databaseUnavailableError: "https://dev.mysql.com/doc/mysql-errors/8.4/en/client-error-reference.html",
			databaseInvalidQuery:     "https://dev.mysql.com/doc/mysql-errors/8.4/en/server-error-reference.html",
			databaseExecutionError:   "https://dev.mysql.com/doc/mysql-errors/8.4/en/server-error-reference.html",
		},
		database.POSTGRES: {
			databaseConflictError:    "https://www.postgresql.org/docs/current/errcodes-appendix.html",
			databaseTimeoutError:     "https://www.postgresql.org/docs/current/errcodes-appendix.html",
			databaseUnavailableError: "https://www.postgresql.org/docs/current/errcodes-appendix.html",
			databaseInvalidQuery:     "https://www.postgresql.org/docs/current/errcodes-appendix.html",
			databaseExecutionError:   "https://www.postgresql.org/docs/current/errcodes-appendix.html",
		},
		database.SQLITE: {
			databaseConflictError:    "https://www.sqlite.org/rescode.html#constraint",
			databaseTimeoutError:     "https://www.sqlite.org/rescode.html#busy",
			databaseUnavailableError: "https://www.sqlite.org/rescode.html#cantopen",
			databaseInvalidQuery:     "https://www.sqlite.org/rescode.html#error",
			databaseExecutionError:   "https://www.sqlite.org/rescode.html",
		},
		database.MONGO: {
			databaseConflictError:    "https://www.mongodb.com/docs/manual/reference/error-codes/#mongodb-error-11000",
			databaseTimeoutError:     "https://www.mongodb.com/docs/manual/reference/error-codes/",
			databaseUnavailableError: "https://www.mongodb.com/docs/manual/reference/error-codes/",
			databaseInvalidQuery:     "https://www.mongodb.com/docs/manual/reference/error-codes/",
			databaseExecutionError:   "https://www.mongodb.com/docs/manual/reference/error-codes/",
		},
		database.SCYLLA: {
			databaseConflictError:    "https://docs.scylladb.com/manual/stable/cql/",
			databaseTimeoutError:     "https://docs.scylladb.com/manual/stable/troubleshooting/",
			databaseUnavailableError: "https://docs.scylladb.com/manual/stable/troubleshooting/",
			databaseInvalidQuery:     "https://docs.scylladb.com/manual/stable/cql/",
			databaseExecutionError:   "https://docs.scylladb.com/manual/stable/troubleshooting/",
		},
	}
	if driverRefs, ok := refs[driverName]; ok {
		if reference, ok := driverRefs[kind]; ok {
			return reference
		}
	}
	return "https://github.com/elsyahtech/gorest/blob/main/docs/guide/orm.md#shared-database-error-contract"
}
