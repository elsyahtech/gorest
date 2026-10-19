package orm

import (
	"errors"
	"net/http"
)

func (orm *ORM) setError(message string, err error, code ...int) error {
	orm.Message = message
	orm.HTTPCode = http.StatusInternalServerError

	if len(code) > 0 {
		orm.HTTPCode = code[0]
	}

	var driver string

	if orm.DatabaseConfig != nil {
		driver = orm.DatabaseConfig.Driver
	}

	orm.Error = withHTTPCode(err, orm.HTTPCode, driver)

	return orm.Error
}

func (orm *ORM) setNotFound(opName string) error {
	message := "Ensure the requested record exists and that the filter for this operation matches its key."
	message += " Reference: https://github.com/elsyahtech/gorest/blob/main/docs/guide/orm.md#shared-database-error-contract"
	return orm.setError(
		message,
		errors.New(opName+": record not found"),
		http.StatusNotFound)
}
