package orm

import (
	"errors"
	"fmt"
	golog "log"
	"net/http"

	"github.com/elsyahtech/gorest/database"
)

func (orm *ORM) execOrm(data any, opName string, sqlFn, mongoFn, scyllaFn func(any) error) *ORM {
	if orm.Database == nil || orm.DatabaseConfig == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ORM, the application must use one of the following databases: " +
			"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA"

		golog.Fatalf("Database is not running. Message: %s", message)

		orm.Message = message
		orm.HTTPCode = http.StatusNotImplemented
		orm.Error = fmt.Errorf("%s: database is not running", opName)

		return orm
	}

	if data == nil {
		orm.Message = fmt.Sprintf("%s: ensure the payload data passed is a struct, slice of structs (array), or pointers", opName)
		orm.HTTPCode = http.StatusBadRequest
		orm.Error = withHTTPCode(fmt.Errorf("%s: struct not found/nil", opName), orm.HTTPCode, orm.DatabaseConfig.Driver)

		return orm
	}

	driver := orm.DatabaseConfig.Driver

	switch driver {
	case database.SQLSERVER, database.MYSQL, database.POSTGRES, database.SQLITE, database.ORACLE:
		orm.Error = sqlFn(data)
	case database.MONGO:
		orm.Error = mongoFn(data)
	case database.SCYLLA:
		orm.Error = scyllaFn(data)
	default:
		orm.Error = fmt.Errorf("unsupported database driver: %s", driver)
	}

	if orm.Error != nil {
		if orm.HTTPCode == 0 {
			orm.HTTPCode = http.StatusInternalServerError
		}
		orm.Error = withHTTPCode(orm.Error, orm.HTTPCode, driver)
	} else if orm.HTTPCode == 0 {
		orm.HTTPCode = http.StatusOK
	}

	return orm
}

func (orm *ORM) validateLimitOffset() error {
	if orm.LimitVal < 0 {
		return orm.setError(
			"Limit() must be zero or a positive integer.",
			errors.New("find: limit cannot be negative"),
			http.StatusBadRequest,
		)
	}
	if orm.OffsetVal < 0 {
		return orm.setError(
			"Offset() must be zero or a positive integer.",
			errors.New("find: offset cannot be negative"),
			http.StatusBadRequest,
		)
	}

	return nil
}
