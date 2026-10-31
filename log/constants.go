//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

const (
	DefaultLogDir      = "./logs"
	DefaultLogFile     = "app.log"
	DefaultCaller      = true
	DefaultMaxFileSize = 100
	DefaultMaxBackups  = 5
	DefaultLogMaxAge   = 7
)
