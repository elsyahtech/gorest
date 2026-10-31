//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func (core redactingCore) With(fields []zap.Field) zapcore.Core {
	return redactingCore{Core: core.Core.With(redactFields(fields))}
}
