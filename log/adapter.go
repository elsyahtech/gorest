package log

import (
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

func (log *Log) Close() (string, int, error) {
	if log == nil || log.logger == nil {
		return "Ensure the logger is properly initialized and running in your application.",
			http.StatusBadRequest,
			errors.New("logger is empty")
	}

	if err := log.logger.Sync(); err != nil {
		return "Ensure the underlying file system or logging output stream is writable and healthy.",
			http.StatusInternalServerError,
			fmt.Errorf("close log sync failed: %w", err)
	}

	return "", http.StatusOK, nil
}

func (log *Log) Log(field map[string]any, noCaller ...bool) *zap.Logger {
	targetLogger := log.logger

	if len(noCaller) > 0 && noCaller[0] {
		targetLogger = targetLogger.WithOptions(zap.WithCaller(false))
	}

	if len(field) == 0 {
		return targetLogger
	}

	zapFields := make([]zap.Field, 0, len(field))
	for k, v := range field {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	return targetLogger.With(zapFields...)
}
