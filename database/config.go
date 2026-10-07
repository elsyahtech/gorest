package database

import (
	"log"
	"path/filepath"
	"time"
)

type Config struct {
	// Database engine/driver type.
	// Supported: MYSQL, POSTGRES, SQLSERVER, ORACLE, SQLITE, MONGO, SCYLLA
	// Default "sqlite"; if driver not configure
	Driver string

	// Database server hostname or IP address.
	// Development: use "localhost" or "127.0.0.1"
	// Production: use DNS name or private IP
	// Default 127.0.0.1
	Host string

	// Database login username.
	// Development defaults: MySQL: root, PostgreSQL: postgres, MSSQL: sa, MongoDB: admin, Oracle: system
	// Production: Create a dedicated app user with minimal permissions.
	// No default
	Username string

	// Database login password.
	// Load from DB_PASSWORD environment variable or secrets manager.
	// Use strong, random password (16+ characters with mixed types).
	// No default
	Password string

	// Database name (schema) to connect to.
	// Each environment should have a separate database.
	// Database must exist before application startup.
	// Oracle: this is the service name (e.g. "FREEPDB1"), not the SID.
	// Default: "gorest"; if database name not configure
	Name string

	// Database server port number.
	// Common defaults: MySQL: 3306, PostgreSQL: 5432, MSSQL: 1433, MongoDB: 27017, ScyllaDB: 9402, Oracle: 1521
	// Default: empty
	Port string

	// Path to Oracle Instant Client library directory (godror/OCI only).
	// Only needed if Instant Client is not in system default library path.
	// Example: "/opt/oracle/instantclient_21_8"
	// Default: empty (assumes Instant Client is in system PATH/LD_LIBRARY_PATH)
	LibDir string

	// Transport Layer Security (encryption) for database connection.
	// PostgreSQL: "disable", "require", "verify-ca", "verify-full"
	// MSSQL/SQLSERVER: disable, optional, mandatory, strict
	// SQLite: leave empty (not applicable)
	// MongoDB: true, false (default: false)
	// Oracle: "enable", "disable" — controls whether connection uses TCPS (TLS)
	// protocol instead of plain TCP. When "enable", a wallet (see SSLCAPath)
	// is required to establish the encrypted connection. Unlike Postgres/MSSQL,
	// Oracle does not support granular modes like "verify-ca"; it's effectively on/off.
	// Development: use "disable" (faster for localhost)
	// Production: use "verify-full" or "strict" (maximum security); Oracle: use "enable"
	// Default: "disable" (change this explicitly in production!); if driver configured using mongo, default to "false"
	TLS string

	// Path to SSL certificate file (client certificate for mutual TLS).
	// Example: "./data/cert/client.crt"
	// Not applicable for Oracle: godror/OCI uses wallet-based auth (see SSLCAPath),
	// not separate client cert/key files like Postgres/MySQL mTLS. Leave empty for Oracle.
	// Default: empty
	SSLCertPath string

	// Path to SSL key file (client private key).
	// Example: "./data/cert/client.key"
	// Not applicable for Oracle: same reason as SSLCertPath, Oracle wallet bundles
	// certificate/key/trust material together instead of using discrete files. Leave empty for Oracle.
	// Default: empty
	SSLKeyPath string

	// Path to CA certificate file for server certificate verification.
	// Example: "./data/cert/ca.crt"
	// Oracle: repurposed as the path to the Oracle Wallet directory (equivalent to
	// TNS_ADMIN), containing cwallet.sso, tnsnames.ora, sqlnet.ora, etc. Required
	// when TLS is "enable". Example: "./data/oracle/wallet"
	// Default: empty
	SSLCAPath string

	// Character set for connection (MySQL).
	// Not applicable for Oracle: character set is controlled via the NLS_LANG
	// environment variable (e.g. "AMERICAN_AMERICA.AL32UTF8") on the client machine,
	// not passed through the connection string. Leave empty for Oracle.
	// Default: "utf8mb4" (recommended)
	Charset string

	// Path to database migration files directory.
	// Example: "./database/migrations"
	// Default: "./database/migrations"
	MigrationDirectory string

	// Path to database seeder files directory.
	// Example: "./database/seeders"
	// Default: "./database/seeders"
	SeederDirectory string

	// Timeout duration limit for database connection and operations.
	// Applied to DialTimeout, ReadTimeout, and WriteTimeout.
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// Recommended: 5 seconds (for production)
	// Default: 5 seconds
	// Example: 5 * time.Second
	Timeout time.Duration

	// Maximum lifetime of connection before being closed and recreated.
	// Prevents stale connections and memory leaks.
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// Recommended: 10-30 minutes
	// Default: 20 minutes
	// Example: 20 * time.Minute
	ConnectionMaxLifetime time.Duration

	// MSSQL/SQLSERVER only: whether to trust self-signed server certificates.
	// false: verify certificate (production, secure)
	// true: trust any certificate (development/testing only - NEVER true in production!)
	// Default: true
	TrustServerCertificate bool

	// Enable query logging/debugging.
	// true: log all queries (slow, development only)
	// false: no logging (production)
	// Default: false
	QueryLogging bool

	// Whether to auto-run database migrations on application startup.
	// Development: true (fast iteration)
	// Production: false (manual control, recommended)
	// Default: false
	Migration bool

	// Whether to auto-run database seeders on application startup.
	// Development: true (fast iteration)
	// Production: false (manual control, recommended)
	// Default: false
	Seeder bool

	// Enabled determines whether the Database connection is active and running.
	// Set to true to enable Database integration, or false to disable it.
	// Example: true
	// Default: false
	Enabled bool

	// Maximum number of open connections to database.
	// Default: unlimited (change this explicitly in production!)
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// High-traffic recommended: 50-100
	// Default: 50
	MaxOpenConnections int

	// Maximum number of idle connections in the pool.
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// Default: 5
	MaxIdleConnections int
}

// If the database service is running but the driver is not configured,
// the application will default to using the SQLite database.
var ConfigDefault = Config{
	// Database engine/driver type.
	// Supported: mysql, postgresql, mssql, sqlite, mongodb, scylladb
	// Default "sqlite"; if driver not configure
	Driver: DefaultDatabaseDriver,

	// Database server hostname or IP address.
	// Development: use "localhost" or "127.0.0.1"
	// Production: use DNS name or private IP
	// Default 127.0.0.1
	Host: DefaultDatabaseHost,

	// Database name (schema) to connect to.
	// Each environment should have a separate database.
	// Database must exist before application startup.
	// Default: "gorest"; if database name not configure
	Name: DefaultDatabaseName,

	// Database server port number.
	// Common defaults: MySQL: 3306, PostgreSQL: 5432, MSSQL: 1433, MongoDB: 27017, ScyllaDB: 9402
	// Default: empty
	Port: "",

	// Path to Oracle Instant Client library directory (godror/OCI only).
	// Only needed if Instant Client is not in system default library path.
	// Example: "/opt/oracle/instantclient_21_8"
	// Default: empty (assumes Instant Client is in system PATH/LD_LIBRARY_PATH)
	LibDir: DefaultDatabaseLibDir,

	// Transport Layer Security (encryption) for database connection.
	// PostgreSQL: "disable", "require", "verify-ca", "verify-full"
	// MSSQL/SQLSERVER: disable, optional, mandatory, strict
	// SQLite: leave empty (not applicable)
	// MongoDB: true, false (default: false)
	// Oracle: "enable", "disable" — controls whether connection uses TCPS (TLS)
	// protocol instead of plain TCP. When "enable", a wallet (see SSLCAPath)
	// is required to establish the encrypted connection. Unlike Postgres/MSSQL,
	// Oracle does not support granular modes like "verify-ca"; it's effectively on/off.
	// Development: use "disable" (faster for localhost)
	// Production: use "verify-full" or "strict" (maximum security); Oracle: use "enable"
	// Default: "disable" (change this explicitly in production!); if driver configured using mongo, default to "false"
	TLS: TLSDisabled,

	// Path to SSL certificate file (client certificate for mutual TLS).
	// Example: "./data/cert/client.crt"
	// Default: empty
	SSLCertPath: DefaultSSLCertPath,

	// Path to SSL key file (client private key).
	// Example: "./data/cert/client.key"
	// Default: empty
	SSLKeyPath: DefaultSSLKeyPath,

	// Path to CA certificate file for server certificate verification.
	// Example: "./data/cert/ca.crt"
	// Default: empty
	SSLCAPath: DefaultSSLCAPath,

	// Character set for connection (MySQL).
	// Default: "utf8mb4" (recommended)
	Charset: DefaultCharset,

	// Path to database migration files directory.
	// Example: "./database/migrations"
	// Default: "./database/migrations"
	MigrationDirectory: DefaultMigrationDirectory,

	// Path to database seeder files directory.
	// Example: "./database/seeders"
	// Default: "./database/seeders"
	SeederDirectory: DefaultSeederDirectory,

	// Timeout duration limit for database connection and operations.
	// Applied to DialTimeout, ReadTimeout, and WriteTimeout.
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// Recommended: 5 seconds (for production)
	// Default: 5 seconds
	Timeout: DefaultTimeout,

	// Maximum lifetime of connection before being closed and recreated.
	// Prevents stale connections and memory leaks.
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// Recommended: 10-30 minutes
	// Default: 20 minutes.
	ConnectionMaxLifetime: DefaultConnectionMaxLifetime,

	// MSSQL/SQLSERVER only: whether to trust self-signed server certificates.
	// false: verify certificate (production, secure)
	// true: trust any certificate (development/testing only - NEVER true in production!)
	// Default: true
	TrustServerCertificate: DefaultTrustServerCertificate,

	// Enable query logging/debugging.
	// true: log all queries (slow, development only)
	// false: no logging (production)
	// Default: false
	QueryLogging: DefaultQueryLogging,

	// Whether to auto-run database migrations on application startup.
	// Development: true (fast iteration)
	// Production: false (manual control, recommended)
	// Default: false
	Migration: MigrationDisabled,

	// Whether to auto-run database seeders on application startup.
	// Development: true (fast iteration)
	// Production: false (manual control, recommended)
	// Default: false
	Seeder: SeederDisabled,

	// Whether to auto-run database integration on application startup.
	// Default: false
	Enabled: DatabaseDisabled,

	// Maximum number of open connections to database.
	// Default: unlimited (change this explicitly in production!)
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// High-traffic recommended: 50-100
	// Default: 50
	MaxOpenConnections: DefaultMaxOpenConnections,

	// Maximum number of idle connections in the pool.
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// Default: 5
	MaxIdleConnections: DefaultMaxIdleConnections,
}

// Helper function to set default values.
//
//nolint:gocyclo,revive
func configDefault(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	// Override default config
	cfg := config[0]

	driver := NormalizeDatabaseDriver(cfg.Driver)

	// Default sqlite
	if driver == "" {
		driver = ConfigDefault.Driver
	} else if !supportedDatabaseDrivers[driver] {
		log.Fatalf("Config Database: unsupported database driver: %q", cfg.Driver)
	}

	cfg.Driver = driver

	// Default: 127.0.0.1
	if cfg.Host == "" {
		cfg.Host = ConfigDefault.Host
	}

	// Default: gorest
	if cfg.Name == "" {
		cfg.Name = ConfigDefault.Name
	}

	// Default: empty
	if cfg.Port == "" {
		port := setDefaultDBPort(driver)

		cfg.Port = port
	}

	// Default: empty
	if cfg.LibDir == "" {
		cfg.LibDir = ConfigDefault.LibDir
	}

	// Default: disable
	if cfg.TLS == "" {
		if driver == MONGO {
			cfg.TLS = TLSFalse
		} else {
			cfg.TLS = ConfigDefault.TLS
		}
	}

	// Default: utf8mb4
	if cfg.Charset == "" {
		cfg.Charset = ConfigDefault.Charset
	}

	// Default: "./database/migrations"
	if cfg.MigrationDirectory == "" {
		cfg.MigrationDirectory = filepath.Join(
			DefaultDatabaseDirectory,
			ConfigDefault.MigrationDirectory,
			driver,
		)
	}

	if cfg.SeederDirectory == "" {
		cfg.SeederDirectory = filepath.Join(
			DefaultDatabaseDirectory,
			ConfigDefault.SeederDirectory,
			driver,
		)
	}

	// Default: 5 second
	if cfg.Timeout == 0 {
		cfg.Timeout = ConfigDefault.Timeout
	}

	if cfg.Timeout < 0 {
		log.Fatalf("Config Database: Timeout must be greater than or equal to 0")
	}

	// default: 10 minutes
	if cfg.ConnectionMaxLifetime == 0 {
		cfg.ConnectionMaxLifetime = ConfigDefault.ConnectionMaxLifetime
	}

	if cfg.ConnectionMaxLifetime < 0 {
		log.Fatalf("Config Database: ConnectionMaxLifetime must be greater than or equal to 0")
	}

	// Default: 50
	if cfg.MaxOpenConnections == 0 {
		cfg.MaxOpenConnections = ConfigDefault.MaxOpenConnections
	}

	if cfg.MaxOpenConnections < 0 {
		log.Fatalf("Config Database: MaxOpenConnections must be greater than or equal to 0") // <--- Fixed error message
	}

	// Default: 5
	if cfg.MaxIdleConnections == 0 {
		cfg.MaxIdleConnections = ConfigDefault.MaxIdleConnections
	}

	if cfg.MaxIdleConnections < 0 {
		log.Fatalf("Config Database: MaxIdleConnections must be greater than or equal to 0")
	}

	return cfg
}

func setDefaultDBPort(driver string) string {
	var port string

	switch driver {
	case MYSQL:
		port = MYSQL_PORT
	case POSTGRES:
		port = POSTGRES_PORT
	case SQLSERVER:
		port = SQLSERVER_PORT
	case ORACLE:
		port = ORACLE_PORT
	case MONGO:
		port = MONGO_PORT
	case SCYLLA:
		port = SCYLLA_PORT
	default:
		port = ""
	}

	return port
}
