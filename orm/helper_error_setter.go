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
	return orm.setError(opName+": record not found", errors.New("record not found"), http.StatusNotFound)
}
