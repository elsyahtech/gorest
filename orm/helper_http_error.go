package orm

import (
	"errors"
	"fmt"

	"github.com/elsyahtech/gorest/database"
)

type httpCodeError struct {
	code   int
	driver string
	err    error
}

func (err *httpCodeError) Error() string {
	if err.driver == "" {
		return fmt.Sprintf("ORM error: %v", err.err)
	}

	return fmt.Sprintf("ORM error [%s]: %v", ormDriverLabel(err.driver), err.err)
}

func (err *httpCodeError) Unwrap() error { return err.err }

func (err *httpCodeError) HTTPStatusCode() int { return err.code }

func withHTTPCode(err error, code int, driver ...string) error {
	if err == nil {
		return nil
	}

	var coded interface{ HTTPStatusCode() int }

	if errors.As(err, &coded) {
		return err
	}

	var driverName string
	if len(driver) > 0 {
		driverName = driver[0]
	}

	return &httpCodeError{code: code, driver: driverName, err: err}
}

func ormDriverLabel(driver string) string {
	switch driver {
	case database.MYSQL:
		return "MySQL"
	case database.POSTGRES:
		return "PostgreSQL"
	case database.SQLSERVER:
		return "SQL Server"
	case database.ORACLE:
		return "Oracle"
	case database.SQLITE:
		return "SQLite"
	case database.MONGO:
		return "MongoDB"
	case database.SCYLLA:
		return "ScyllaDB"
	default:
		return driver
	}
}
