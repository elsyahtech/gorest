package database

import (
	"log"
	"path/filepath"
	"strings"
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

	// MongoDB authentication database. Used as authSource when credentials are set.
	// Defaults to "admin" when empty.
	AuthSource string

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

	// Transport Layer Security (TLS) mode for database connections. The accepted
	// value is driver-specific:
	//   - PostgreSQL: "disable", "require", "verify-ca", or "verify-full".
	//   - MySQL: driver TLS mode; a non-empty SSLCAPath enables passing this
	//     mode to the driver, but the path itself is not loaded by Gorest.
	//     "true" uses the driver's system certificate roots. For a custom CA
	//     or client certificate, register a driver TLS profile and set TLS to
	//     its name; SSLCAPath must still be non-empty to enable that option.
	//   - SQL Server: "disable", "optional", "mandatory", or "strict"; keep
	//     TrustServerCertificate false to validate the server certificate.
	//   - Oracle: "enable" uses TCPS and requires an Oracle wallet via SSLCAPath;
	//     "disable" uses plain TCP.
	//   - MongoDB: "true" or "false"; SSLCertPath and SSLKeyPath enable mTLS.
	//   - ScyllaDB: "enable" or "disable".
	//   - SQLite and ScyllaDB: SQLite has no network TLS. ScyllaDB supports
	//     "enable" and verifies server certificates and hostnames when configured with
	//     SSLCAPath; SSLCertPath and SSLKeyPath enable optional client certificates (mTLS).
	//
	// An empty value intentionally defaults to "disable" for non-Mongo network
	// drivers (and "false" for MongoDB). This compatibility-oriented default keeps
	// local development and existing deployments working without requiring
	// certificates, wallets, or a driver-specific TLS mode. It is NOT a secure
	// production default: when connecting to a remote database over an untrusted
	// network, plaintext traffic can expose credentials and data or allow
	// interception/modification. Explicitly configure and verify TLS for the
	// selected driver in production; do not assume that a remote connection is
	// encrypted merely because it uses a private address or network firewall.
	TLS string

	// AllowInsecureProduction explicitly records client approval to run this database
	// connection without verified transport encryption in production. It does not
	// disable validation or replace deployment requirements; GOREST logs this choice.
	AllowInsecureProduction bool

	// Path to SSL certificate file (client certificate for mutual TLS).
	// Example: "./data/cert/client.crt"
	// PostgreSQL and MongoDB use this with SSLKeyPath for client-certificate authentication.
	// MySQL does not load this path; use a registered driver TLS profile instead.
	// Oracle uses a wallet via SSLCAPath and does not use this field.
	// Default: empty
	SSLCertPath string

	// Path to SSL key file (client private key).
	// Example: "./data/cert/client.key"
	// MongoDB uses this with SSLCertPath for client-certificate authentication.
	// Not applicable for Oracle: same reason as SSLCertPath, Oracle wallet bundles
	// certificate/key/trust material together instead of using discrete files. Leave empty for Oracle.
	// Default: empty
	SSLKeyPath string

	// Path to CA certificate file for server certificate verification.
	// Example: "./data/cert/ca.crt"
	// MySQL note: a non-empty value only enables its TLS option; Gorest does not
	// load this path as a CA file. The driver uses system roots for TLS "true";
	// use a registered driver TLS profile for a custom CA or client certificate.
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

	// Path to database files directory.
	// Example: "./database"
	// Default: "./database"
	DatabaseDirectory string

	// Migration directory, relative to DatabaseDirectory.
	// The driver name is appended automatically.
	// Example: "migrations" -> ./database/migrations/{Driver}
	// Default: "migrations"
	MigrationDirectory string

	// Seeder directory, relative to DatabaseDirectory.
	// The driver name is appended automatically.
	// Example: "seeders" -> ./app/database/seeders/{Driver}
	// Default: "seeders"
	SeederDirectory string

	// Timeout duration limit for database connections and operations.
	// Applied to driver connection/read/write timeouts where supported, and
	// to transactions started through DBSession.BeginTx or BeginTxContext.
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// Recommended: 5 seconds (for production)
	// Default: 5 seconds
	// Example: 5 * time.Second
	Timeout time.Duration

	// Maximum lifetime of connection before being closed and recreated.
	// Prevents stale connections and memory leaks.
	// Must be greater than or equal to 0, Please do not set < 0 or lower then 0 (minus)
	// Recommended: 10-30 minutes
	// Default: 10 minutes
	// Example: 10 * time.Minute
	ConnectionMaxLifetime time.Duration

	// MSSQL/SQLSERVER only: whether to trust self-signed server certificates.
	// false: verify certificate (production, secure)
	// true: trust any certificate (development/testing only - NEVER true in production!)
	// Default: false
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

	// Disable controls whether app.Use initializes the database.
	// The default false value enables database initialization.
	Disable bool

	// Maximum number of open connections to database.
	// Zero uses the configured default of 50 connections.
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

	// TLS default is intentionally compatibility-oriented; see Config.TLS for
	// driver-specific options and the security implications. Remote production
	// connections must explicitly configure TLS where supported.
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

	// Path to database files directory.
	// Example: "./database"
	// Default: "./database"
	DatabaseDirectory: DefaultDatabaseDirectory,

	// Migration directory, relative to DatabaseDirectory.
	// The driver name is appended automatically.
	// Example: "migrations" -> ./database/migrations/{Driver}
	// Default: "migrations"
	MigrationDirectory: DefaultMigrationDirectory,

	// Seeder directory, relative to DatabaseDirectory.
	// The driver name is appended automatically.
	// Example: "seeders" -> ./app/database/seeders/{Driver}
	// Default: "seeders"
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
	// Default: 10 minutes.
	ConnectionMaxLifetime: DefaultConnectionMaxLifetime,

	// MSSQL/SQLSERVER only: whether to trust self-signed server certificates.
	// false: verify certificate (production, secure)
	// true: trust any certificate (development/testing only - NEVER true in production!)
	// Default: false
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
	Disable: DatabaseDisabled,

	// Maximum number of open connections to database.
	// Zero uses the configured default of 50 connections.
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
//nolint:revive
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

	// Keep the documented compatibility default when callers omit TLS. Do not
	// silently infer a TLS mode from Host: each driver has different TLS modes
	// and certificate/wallet requirements, so callers must opt in explicitly.
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

	// Default: "./database"
	databaseDir := DefaultDatabaseDirectory

	if cfg.DatabaseDirectory != "" {
		databaseDir = cfg.DatabaseDirectory
	}

	// Default: "migrations" -> ./database/migrations/{Driver}
	cfg.MigrationDirectory = resolveDir(databaseDir, cfg.MigrationDirectory, DefaultMigrationDirectory, driver)

	// Default: "seeders" -> ./database/seeders/{Driver}
	cfg.SeederDirectory = resolveDir(databaseDir, cfg.SeederDirectory, DefaultSeederDirectory, driver)

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
		log.Fatalf("Config Database: MaxOpenConnections must be greater than or equal to 0")
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

func resolveDir(baseDir, dirParam, defaultDir, driver string) string {
	dir := dirParam

	if dir == "" {
		dir = defaultDir
	}

	resolvedDir := filepath.Clean(dir)

	relativeDir, err := filepath.Rel(filepath.Clean(baseDir), resolvedDir)
	if err == nil && relativeDir != "." && relativeDir != ".." &&
		!strings.HasPrefix(relativeDir, ".."+string(filepath.Separator)) &&
		filepath.Base(relativeDir) == driver {
		return resolvedDir
	}

	return filepath.Join(baseDir, dir, driver)
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
