package orm

import "errors"

type httpCodeError struct {
	code int
	err  error
}

func (err *httpCodeError) Error() string { return err.err.Error() }

func (err *httpCodeError) Unwrap() error { return err.err }

func (err *httpCodeError) HTTPStatusCode() int { return err.code }

func withHTTPCode(err error, code int) error {
	if err == nil {
		return nil
	}

	var coded interface{ HTTPStatusCode() int }

	if errors.As(err, &coded) {
		return err
	}

	return &httpCodeError{code: code, err: err}
}
