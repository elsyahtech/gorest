package orm

import (
	"errors"
	"fmt"
	golog "log"
	"net/http"
	"strings"

	"github.com/elsyahtech/gorest/database"
)

func (orm *ORM) execOrm(data any, opName string, sqlFn, mongoFn, scyllaFn func(any) error) *ORM {
	if orm.Database == nil || orm.DatabaseConfig == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ORM, the application must use one of the following databases: " +
			"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA"

		golog.Fatalf("Database is not running. Message: %s", message)

		return orm
	}

	if data == nil {
		orm.Message = fmt.Sprintf("%s: ensure the payload data passed is a struct, slice of structs (array), or pointers", opName)
		orm.HTTPCode = http.StatusBadRequest
		orm.Error = withHTTPCode(fmt.Errorf("%s: struct not found/nil", opName), orm.HTTPCode)

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
		orm.Error = withHTTPCode(orm.Error, orm.HTTPCode)
	} else if orm.HTTPCode == 0 {
		orm.HTTPCode = http.StatusOK
	}

	return orm
}

func (orm *ORM) validateSelectColumns() error {
	for _, column := range orm.SelectedCols {
		for _, expression := range strings.Split(column, ",") {
			if strings.TrimSpace(expression) == "" {
				return orm.setError(
					"Use Select('*') to select all columns, or provide one or more valid column names.",
					errors.New("invalid SELECT clause: column expression cannot be empty"),
					http.StatusBadRequest,
				)
			}
		}
	}

	return nil
}

func (orm *ORM) validateLimit() error {
	if orm.LimitVal < 0 {
		return orm.setError(
			"Limit() must be a positive integer.",
			errors.New("find: limit cannot be negative"),
			http.StatusBadRequest,
		)
	}

	if orm.LimitVal == 0 {
		return orm.setError(
			"Limit() must be greater than 0",
			errors.New("find: limit cannot be 0"),
			http.StatusBadRequest,
		)
	}

	return nil
}
