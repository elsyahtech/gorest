package orm

import (
	"errors"
)

type httpCodeError struct {
	code int
	err  error
}

func (err *httpCodeError) Error() string {
	return err.err.Error()
}

func (err *httpCodeError) Unwrap() error { return err.err }

func (err *httpCodeError) HTTPStatusCode() int { return err.code }

func withHTTPCode(err error, code int, driver ...string) error {
	if err == nil {
		return nil
	}

	var coded interface{ HTTPStatusCode() int }

	if errors.As(err, &coded) {
		if coded.HTTPStatusCode() == code {
			return err
		}
	}

	return &httpCodeError{code: code, err: err}
}
