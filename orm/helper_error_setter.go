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

	orm.Error = withHTTPCode(err, orm.HTTPCode)

	return orm.Error
}

func (orm *ORM) setNotFound(opName string) error {
	return orm.setError(
		"Ensure that the record is exists and that the filter condition for "+opName+" data matches the intended record.",
		errors.New("record not found"),
		http.StatusNotFound)
}
