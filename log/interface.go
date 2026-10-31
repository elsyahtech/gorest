//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import "go.uber.org/zap"

type Log struct {
	logger  *zap.Logger
	closeFn func() error
}
