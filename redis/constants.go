package redis

import "time"

const (
	DefaultRedisHost            = "127.0.0.1"
	DefaultRedisPort            = "6379"
	DefaultRedisSSLCAPath       = ""
	DefaultRedisSSLCertPath     = ""
	DefaultRedisSSLKeyPath      = ""
	DefaultRedisTimeout         = 5 * time.Second
	DefaultRedisPoolTimeout     = DefaultRedisTimeout + time.Second
	DefaultRedisConnMaxLifetime = 20 * time.Minute
	DefaultRedisConnMaxIdleTime = 30 * time.Minute
	DefaultRedisTLS             = false
	DefaultRedisDatabase        = 0
	DefaultRedisMaxRetries      = 3
	DefaultRedisPoolSize        = 0
	DefaultRedisMinIdleConns    = 5
	DefaultRedisMaxActiveConns  = DefaultRedisPoolSize * 2
	DefaultRedisReadBufferSize  = 32 * 1024
	DefaultRedisWriteBufferSize = 32 * 1024
)
