package database

import (
	"errors"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/godror/godror"
	"github.com/lib/pq"
	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func isDuplicateKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}

	var postgresErr *pq.Error
	if errors.As(err, &postgresErr) && postgresErr.Code == "23505" {
		return true
	}

	var sqlServerErr interface{ SQLErrorNumber() int32 }
	if errors.As(err, &sqlServerErr) && (sqlServerErr.SQLErrorNumber() == 2601 || sqlServerErr.SQLErrorNumber() == 2627) {
		return true
	}

	if oracleErr, ok := godror.AsOraErr(err); ok && oracleErr.Code() == 1 {
		return true
	}

	var sqliteErr *sqlite.Error

	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY, sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_ROWID:
			return true
		}
	}

	return false
}
