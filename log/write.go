//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func (core redactingCore) Write(entry zapcore.Entry, fields []zap.Field) error {
	entry.Message = redactString(entry.Message)

	return core.Core.Write(entry, redactFields(fields)) //nolint:wrapcheck
}
