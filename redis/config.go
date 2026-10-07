package redis

import (
	"log"
	"time"
)

type Config struct {
	// Host is the Redis server host address.
	// Set to your Redis server IP or hostname (e.g., 192.168.1.xxx or localhost).
	// Default: "127.0.0.1"
	Host string

	// Port is the Redis server port number.
	// Set to your Redis server listening port if customized.
	// Default: "6379"
	Port string

	// Username is the Redis authentication username (Redis 6.0+ ACL).
	// Leave blank if using password-only auth or no auth.
	// Default: empty
	Username string

	// Password is the Redis authentication password.
	// Leave blank if no password is required.
	// Default: empty
	Password string

	// SSLCAPath is the path to the CA certificate file for TLS verification.
	// Used to verify Redis server certificate authenticity. Only used if TLS is enabled.
	// Example: "./certs/ca.crt"
	// Default: empty
	SSLCAPath string

	// SSLCertPath is the path to the client certificate file for mutual TLS authentication.
	// Only used if TLS is enabled and mutual authentication is required.
	// Example: "./certs/client.crt"
	// Default: empty
	SSLCertPath string

	// SSLKeyPath is the path to the client certificate private key file for mutual TLS.
	// Only used if TLS is enabled and mutual authentication is required.
	// Example: "./certs/client.key"
	// Default: empty
	SSLKeyPath string

	// Timeout is the timeout duration limit for Redis connection and operation timeouts.
	// Applied to DialTimeout, ReadTimeout, and WriteTimeout.
	// Recommended: 5 seconds (for production)
	// Example: 5 * time.Second
	// Default: 5 * time.Second
	Timeout time.Duration

	// PoolTimeout is the time to wait for a connection if the pool is busy.
	// Recommended: Timeout + 1 second
	// Example: Timeout + time.Second
	// Default: Timeout + time.Second (5 second + 1 second)
	PoolTimeout time.Duration

	// ConnMaxLifetime is the maximum lifetime of a connection before it is closed and recreated.
	// Prevents stale connections and memory leaks.
	// Recommended: 10-30 minutes
	// Example: 20 * time.Minute
	// Default: 20 * time.Minute
	ConnMaxLifetime time.Duration

	// ConnMaxIdleTime is the maximum time a connection may remain idle before being closed.
	// Should be less than the Redis server timeout.
	// Recommended: 30 minutes
	// Example: 30 * time.Minute
	// Default: 30 * time.Minute
	ConnMaxIdleTime time.Duration

	// TLS enables TLS/SSL for the Redis connection (if the Redis server supports it).
	// Production: true (mandatory for secure connection)
	// Development: false (faster for localhost, if encryption is not needed)
	// Default: false
	TLS bool

	// Enabled determines whether the Redis connection is active and running.
	// Set to true to enable Redis integration, or false to disable it.
	// Example: true
	// Default: false
	Enabled bool

	// Database is the Redis database index number (usually 0 to 15).
	// Default: 0
	Database int

	// MaxRetries is the maximum number of retries for failed commands.
	// -1 disables retries. Retries on network errors and timeouts.
	// Default: 3
	MaxRetries int

	// PoolSize is the connection pool size (maximum number of connections).
	// Adjust based on expected concurrency and traffic volume.
	// Recommended: 10 * runtime.GOMAXPROCS(0)
	// Note: If running in containers (Docker/K8s), ensure CPU limits
	// are set correctly so runtime.GOMAXPROCS doesn't read the host's total cores.
	// Default: 0 (no-limit)
	PoolSize int

	// MinIdleConns is the minimum number of idle connections to maintain in the pool.
	// Idle connections are kept alive even if not actively used.
	// Recommended: 5-10 (or a fraction of PoolSize for high-traffic apps)
	// Default: 5
	MinIdleConns int

	// MaxActiveConns is the maximum number of connections allocated by the pool at any given time.
	// When zero, there is no limit. When full, the next call blocks until a connection is released.
	// Recommended: PoolSize * 2 for buffer capacity
	// Default: PoolSize * 2 for buffer capacity
	MaxActiveConns int

	// ReadBufferSize is the size of the bufio.Reader buffer for each connection.
	// Larger buffers improve performance for large responses; smaller buffers save memory.
	// Recommended: 32-64 KiB (32768-65536 bytes)
	// Example: 32 * 1024
	// Default: 32 KiB
	ReadBufferSize int

	// WriteBufferSize is the size of the bufio.Writer buffer for each connection.
	// Larger buffers improve performance for large pipelines; smaller buffers save memory.
	// Recommended: 32-64 KiB (32768-65536 bytes)
	// Example: 32 * 1024
	// Default: 32 KiB
	WriteBufferSize int
}

var ConfigDefault = Config{
	// Default: "127.0.0.1"
	Host: DefaultRedisHost,

	// Default: "6379"
	Port: DefaultRedisPort,

	// Default: empty
	SSLCAPath: DefaultRedisSSLCAPath,

	// Default: empty
	SSLCertPath: DefaultRedisSSLCertPath,

	// Default: empty
	SSLKeyPath: DefaultRedisSSLKeyPath,

	// Default: 5 * time.Second
	Timeout: DefaultRedisTimeout,

	// Default: Timeout + time.Second
	PoolTimeout: DefaultRedisPoolTimeout,

	// Default: 20 * time.Minute
	ConnMaxLifetime: DefaultRedisConnMaxLifetime,

	// Default: 30 * time.Minute
	ConnMaxIdleTime: DefaultRedisConnMaxIdleTime,

	// Default: false
	TLS: DefaultRedisTLS,

	// Default: false
	Enabled: RedisDisabled,

	// Default: 0
	Database: DefaultRedisDatabase,

	// Default: 3
	MaxRetries: DefaultRedisMaxRetries,

	// Default: 0 (no-limit)
	PoolSize: DefaultRedisPoolSize,

	// Default: 5
	MinIdleConns: DefaultRedisMinIdleConns,

	// Default: PoolSize * 2 for buffer capacity
	MaxActiveConns: DefaultRedisMaxActiveConns,

	// Default: 32 KiB
	ReadBufferSize: DefaultRedisReadBufferSize,

	// Default: 32 KiB
	WriteBufferSize: DefaultRedisWriteBufferSize,
}

//nolint:gocyclo,revive,funlen
func configDefault(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	// Override default config
	cfg := config[0]

	if cfg.Host == "" {
		cfg.Host = ConfigDefault.Host
	}

	// Default: "6379"
	if cfg.Port == "" {
		cfg.Port = ConfigDefault.Port
	}

	// Default: empty
	if cfg.SSLCAPath == "" {
		cfg.SSLCAPath = ConfigDefault.SSLCAPath
	}

	// Default: empty
	if cfg.SSLCertPath == "" {
		cfg.SSLCertPath = ConfigDefault.SSLCertPath
	}

	// Default: empty
	if cfg.SSLKeyPath == "" {
		cfg.SSLKeyPath = ConfigDefault.SSLKeyPath
	}

	// Default: 5 * time.Second
	if cfg.Timeout == 0 {
		cfg.Timeout = ConfigDefault.Timeout
	}

	if cfg.Timeout < 0 {
		log.Fatalf("Redis Config: Timeout must be greater than or equal to 0") //nolint:revive
	}

	// Default: Timeout + time.Second
	if cfg.PoolTimeout == 0 {
		cfg.PoolTimeout = ConfigDefault.PoolTimeout
	}

	if cfg.PoolTimeout < 0 {
		log.Fatalf("Redis Config: PoolTimeout must be greater than or equal to 0") //nolint:revive
	}

	// Default: 20 * time.Minute
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = ConfigDefault.ConnMaxLifetime
	}

	if cfg.ConnMaxLifetime < 0 {
		log.Fatalf("Redis Config: ConnMaxLifetime must be greater than or equal to 0") //nolint:revive
	}

	// Default: 30 * time.Minute
	if cfg.ConnMaxIdleTime == 0 {
		cfg.ConnMaxIdleTime = ConfigDefault.ConnMaxIdleTime
	}

	if cfg.ConnMaxIdleTime < 0 {
		log.Fatalf("Redis Config: ConnMaxIdleTime must be greater than or equal to 0") //nolint:revive
	}

	// Default: 0
	if cfg.Database == 0 {
		cfg.Database = ConfigDefault.Database
	}

	if cfg.Database < 0 {
		log.Fatalf("Redis Config: Database must be greater than or equal to 0") //nolint:revive
	}

	// Default: 3
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = ConfigDefault.MaxRetries
	}

	if cfg.MaxRetries < 0 {
		log.Fatalf("Redis Config: MaxRetries must be greater than or equal to 0") //nolint:revive
	}

	// Default: 0 (no-limit)
	if cfg.PoolSize == 0 {
		cfg.PoolSize = ConfigDefault.PoolSize
	}

	if cfg.PoolSize < 0 {
		log.Fatalf("Redis Config: PoolSize must be greater than or equal to 0") //nolint:revive
	}

	// Default: 5
	if cfg.MinIdleConns == 0 {
		cfg.MinIdleConns = ConfigDefault.MinIdleConns
	}

	if cfg.MinIdleConns < 0 {
		log.Fatalf("Redis Config: MinIdleConns must be greater than or equal to 0") //nolint:revive
	}

	// Default: PoolSize * 2 for buffer capacity
	if cfg.MaxActiveConns == 0 {
		cfg.MaxActiveConns = ConfigDefault.MaxActiveConns
	}

	if cfg.MaxActiveConns < 0 {
		log.Fatalf("Redis Config: MinIdleConns must be greater than or equal to 0") //nolint:revive
	}

	// Default: 32 KiB
	if cfg.ReadBufferSize == 0 {
		cfg.ReadBufferSize = ConfigDefault.ReadBufferSize
	}

	if cfg.ReadBufferSize == 0 {
		log.Fatalf("Redis Config: ReadBufferSize must be greater than or equal to 0") //nolint:revive
	}

	// Default: 32 KiB
	if cfg.WriteBufferSize == 0 {
		cfg.ReadBufferSize = ConfigDefault.WriteBufferSize
	}

	if cfg.WriteBufferSize < 0 {
		log.Fatalf("Redis Config: ReadBufferSize must be greater than or equal to 0") //nolint:revive
	}

	return cfg
}
