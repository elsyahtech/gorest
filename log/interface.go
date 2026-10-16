//nolint:revive,nolintlint
package log

import "go.uber.org/zap"

type Log struct {
	logger  *zap.Logger
	closeFn func() error
}
