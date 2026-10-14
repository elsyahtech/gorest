package orm

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/elsyahtech/gorest/database"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/godror/godror"
	"github.com/lib/pq"
)

func (orm *ORM) setFindSQLQueryError(driver, adapterMessage string, err error, httpCode int) error {
	if isGroupBySelectionError(driver, err) {
		return orm.setError(
			fmt.Sprintf("%s requires every selected non-aggregate column in a grouped query to appear in GroupBy(). Select grouped columns and aggregate expressions such as COUNT or SUM.", databaseDriverLabel(driver)),
			fmt.Errorf("find: invalid grouped selection: %w", err),
			http.StatusBadRequest,
		)
	}

	if isUnknownColumnError(driver, err) {
		return orm.setError(
			fmt.Sprintf("The %s query references an unknown column. Check column names in Select(), GroupBy(), Having(), and OrderBy() against the table schema.", databaseDriverLabel(driver)),
			fmt.Errorf("find: query references an unknown column: %w", err),
			http.StatusBadRequest,
		)
	}

	return orm.setError(adapterMessage, err, httpCode)
}

func isGroupBySelectionError(driver string, err error) bool {
	switch driver {
	case database.ORACLE:
		if oracleErr, ok := godror.AsOraErr(err); ok {
			return oracleErr.Code() == 979
		}
	case database.MYSQL:
		var mysqlErr *mysql.MySQLError
		return errors.As(err, &mysqlErr) && mysqlErr.Number == 1055
	case database.POSTGRES:
		var postgresErr *pq.Error
		return errors.As(err, &postgresErr) && postgresErr.Code == "42803"
	case database.SQLSERVER:
		var sqlServerErr interface{ SQLErrorNumber() int32 }
		return errors.As(err, &sqlServerErr) && sqlServerErr.SQLErrorNumber() == 8120
	}

	// SQLite permits non-grouped result columns, so it does not report this
	// class of GROUP BY error.
	return false
}

func isUnknownColumnError(driver string, err error) bool {
	switch driver {
	case database.ORACLE:
		if oracleErr, ok := godror.AsOraErr(err); ok {
			return oracleErr.Code() == 904
		}
	case database.MYSQL:
		var mysqlErr *mysql.MySQLError
		return errors.As(err, &mysqlErr) && mysqlErr.Number == 1054
	case database.POSTGRES:
		var postgresErr *pq.Error
		return errors.As(err, &postgresErr) && postgresErr.Code == "42703"
	case database.SQLSERVER:
		var sqlServerErr interface{ SQLErrorNumber() int32 }
		return errors.As(err, &sqlServerErr) && sqlServerErr.SQLErrorNumber() == 207
	case database.SQLITE:
		return strings.Contains(strings.ToLower(err.Error()), "no such column")
	}

	return false
}

func databaseDriverLabel(driver string) string {
	switch driver {
	case database.SQLSERVER:
		return "SQL Server"
	case database.MYSQL:
		return "MySQL"
	case database.POSTGRES:
		return "PostgreSQL"
	case database.SQLITE:
		return "SQLite"
	case database.ORACLE:
		return "Oracle"
	default:
		return driver
	}
}
