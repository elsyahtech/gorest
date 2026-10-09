package database

import "net/http"

func setError(message string, err error, code ...int) (string, int, error) {
	if err == nil {
		return "", http.StatusOK, nil
	}

	httpCode := http.StatusInternalServerError
	if len(code) > 0 {
		httpCode = code[0]
	}

	return message, httpCode, err
}
