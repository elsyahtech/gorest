package orm

import (
	"errors"
	"strings"

	"github.com/elsyahtech/gorest/database"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/godror/godror"
	"github.com/lib/pq"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (orm *ORM) setFindSQLQueryError(_ string, err error) error {
	return orm.setDatabaseOperationError("Find", err)
}

func isInvalidDatabaseQueryError(driver string, err error) bool {
	if err == nil {
		return false
	}

	if isGroupBySelectionError(driver, err) || isUnknownColumnError(driver, err) {
		return true
	}

	switch driver {
	case database.ORACLE:
		if oracleErr, ok := godror.AsOraErr(err); ok {
			switch oracleErr.Code() {
			case 933, 936, 942:
				return true
			}
		}
	case database.MYSQL:
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			switch mysqlErr.Number {
			case 1064, 1146:
				return true
			}
		}
	case database.POSTGRES:
		var postgresErr *pq.Error
		if errors.As(err, &postgresErr) {
			switch postgresErr.Code {
			case "42601", "42P01":
				return true
			}
		}
	case database.SQLSERVER:
		var sqlServerErr interface{ SQLErrorNumber() int32 }
		if errors.As(err, &sqlServerErr) {
			switch sqlServerErr.SQLErrorNumber() {
			case 102, 156, 208:
				return true
			}
		}
	case database.SQLITE:
		errText := strings.ToLower(err.Error())
		return strings.Contains(errText, "syntax error") ||
			strings.Contains(errText, "incomplete input") ||
			strings.Contains(errText, "no such table")
	case database.MONGO:
		var serverErr mongo.ServerError
		if errors.As(err, &serverErr) {
			for _, code := range []int{2, 9, 14, 168, 40324} {
				if serverErr.HasErrorCode(code) {
					return true
				}
			}
		}
	case database.SCYLLA:
		errText := strings.ToLower(err.Error())
		return strings.Contains(errText, "select distinct queries must only request partition key columns") ||
			strings.Contains(errText, "group by non-primary-key column") ||
			strings.Contains(errText, "undefined column") ||
			strings.Contains(errText, "unknown column") ||
			strings.Contains(errText, "unconfigured columnfamily") ||
			strings.Contains(errText, "order by is only supported when the partition key is restricted by an eq or an in")
	}

	return false
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
