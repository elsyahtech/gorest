package server

import "time"

const (
	DefaultServerHost            = "127.0.0.1"
	DefaultServerReadTimeout     = 10 * time.Second
	DefaultServerWriteTimeout    = 30 * time.Second
	DefaultServerIdleTimeout     = 120 * time.Second
	DefaultServerShutdownTimeout = 30 * time.Second
	DefaultServerPort            = 3000
	DefaultServerBodyLimit       = 4 * 1024 * 1024
)
