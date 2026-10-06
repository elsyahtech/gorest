package log

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
)

func (log *Log) Close() (string, error) {
	if log == nil || log.logger == nil {
		return "Ensure logger called after 'app.Start() and defer app.Close()'", errors.New("logger is empty")
	}

	if err := log.logger.Sync(); err != nil {
		return "", fmt.Errorf("close log sync failed: %w", err)
	}

	return "", nil
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
