package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	//nolint:revive,nolintlint
	_ "github.com/microsoft/go-mssqldb"

	//nolint:revive,nolintlint
	_ "github.com/lib/pq"

	//nolint:revive,nolintlint
	_ "modernc.org/sqlite"

	//nolint:revive,nolintlint
	_ "github.com/godror/godror"
)

// ==========================================================================
// MYSQLConnection establishes a connection to the MYSQL Server
// database using global configuration settings
// ==========================================================================.
func mysqlConnection(ctx context.Context, cfg *Config, timezone, driver string) (*sql.DB, string, error) {
	return connectDatabase(ctx, cfg, timezone, driver, nil)
}

// ==========================================================================
// MSSQLConnection establishes a connection to the Microsoft SQL Server
// database using global configuration settings
// ==========================================================================.
func mssqlConnection(ctx context.Context, cfg *Config, timezone, driver string) (*sql.DB, string, error) {
	return connectDatabase(ctx, cfg, timezone, driver, mssqlDSN)
}

// ==========================================================================
// POSTGRESQLConnection establishes a connection to the POSTGRESQL Server
// database using global configuration settings
// ==========================================================================.
func postgresqlConnection(ctx context.Context, cfg *Config, timezone, driver string) (*sql.DB, string, error) {
	return connectDatabase(ctx, cfg, timezone, driver, postgresDSN)
}

// ==========================================================================
// SQLITEConnection establishes a connection to the SQLite
// database using global configuration settings
// ==========================================================================.
func sqliteConnection(ctx context.Context, cfg *Config, timezone, driver string) (*sql.DB, string, error) {
	return connectDatabase(ctx, cfg, timezone, driver, sqliteDSN)
}

// ==========================================================================
// ORACLEConnection establishes a connection to the ORACLE
// database using global configuration settings
// ==========================================================================.
func oracleConnection(ctx context.Context, cfg *Config, timezone, driver string) (*sql.DB, string, error) {
	return connectDatabase(ctx, cfg, timezone, driver, nil)
}

// ===================================================================
// MONGOConnection establishes a connection to the MongoDB database
// ===================================================================.
func mongoConnection(ctx context.Context, cfg *Config, driver string) (*mongo.Client, string, error) {
	if cfg == nil {
		const message = "ensure the app configuration is set up correctly"

		return nil, message, fmt.Errorf("%s connection error: app configuration is uninitialized", driver)
	}
	if (cfg.SSLCertPath == "") != (cfg.SSLKeyPath == "") {
		const message = "configure both MongoDB client certificate and key for mTLS"
		return nil, message, fmt.Errorf("%s connection error: SSLCertPath and SSLKeyPath must both be set for mutual TLS", driver)
	}
	if cfg.SSLCertPath != "" && !strings.EqualFold(cfg.TLS, "true") {
		const message = `enable MongoDB TLS with TLS: "true" when using a client certificate`
		return nil, message, fmt.Errorf("%s connection error: MongoDB client certificates require TLS", driver)
	}

	dsn := mongoDSN(cfg)

	// Configure MongoDB client options with URI and server selection timeout
	clientOpts := options.Client().ApplyURI(dsn).SetServerSelectionTimeout(cfg.Timeout)

	// Connect to the MongoDB server instance
	client, err := mongo.Connect(clientOpts)
	if err != nil {
		const message = "verify if MongoDB service/daemon is running, " +
			"check host and port settings, and ensure network connectivity to the server is stable"

		return nil, message, fmt.Errorf("MongoConnection - open connection: %w", err)
	}

	// Ping the MongoDB server to verify connection health and responsiveness
	if err := client.Ping(ctx, nil); err != nil {
		if errDB := client.Disconnect(ctx); errDB != nil {
			_ = errDB
		}

		const message = "verify database credentials (username/password) and ensure authSource (e.g., admin/root) is correct"

		return nil, message, fmt.Errorf("MongoConnection - ping mongo server: %w", err)
	}

	return client, "", nil
}

// =================================================================
// SCYLLAConnection establishes a connection to the SCYLLA database
// =================================================================.
func scyllaConnection(cfg *Config, driver string) (*gocql.Session, string, error) {
	if cfg == nil {
		const message = "ensure the app configuration is set up correctly"

		return nil, message, fmt.Errorf("%s connection error: app configuration is uninitialized", driver)
	}
	if cfg.TLS != "" && cfg.TLS != TLSDisabled && cfg.TLS != TLSEnabled {
		const message = `ScyllaDB TLS mode must be "enable" or "disable"`

		return nil, message, fmt.Errorf("%s connection error: unsupported TLS mode %q", driver, cfg.TLS)
	}

	// Create cluster configuration for gocql
	cluster := gocql.NewCluster(cfg.Host)
	cluster.Port = stringToInt(cfg.Port)
	// Server-assigned timestamps avoid client clock skew making writes stale after LWT operations.
	cluster.DefaultTimestamp = false

	// Set timeout
	cluster.Timeout = cfg.Timeout
	cluster.ConnectTimeout = cfg.Timeout

	// Configuring authentication
	if cfg.Username != "" && cfg.Password != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	}

	// Adjust default keyspace
	if cfg.Name != "" {
		cluster.Keyspace = cfg.Name
	}

	// Configuring TLS for ScyllaDB connection.
	if cfg.TLS == "enable" {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
		}

		if cfg.SSLCAPath != "" {
			caCert, err := os.ReadFile(cfg.SSLCAPath)
			if err != nil {
				return nil, "failed to read CA certificate", fmt.Errorf("%s connection error: - read CA cert: %w", driver, err)
			}

			caCertPool := x509.NewCertPool()

			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, "failed to append CA certificate", fmt.Errorf("%s connection error: - append CA cert failed", driver)
			}

			tlsConfig.RootCAs = caCertPool
		}

		if (cfg.SSLCertPath == "") != (cfg.SSLKeyPath == "") {
			return nil, "client certificate and key must be configured together", errors.New("SCYLLA connection error: SSLCertPath and SSLKeyPath must both be set for mutual TLS")
		}
		if cfg.SSLCertPath != "" {
			cert, err := tls.LoadX509KeyPair(cfg.SSLCertPath, cfg.SSLKeyPath)
			if err != nil {
				return nil, "failed to load client certificate and key", fmt.Errorf("%s connection error: - load client keys: %w", driver, err)
			}

			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		tlsConfig.ServerName = cfg.Host
		cluster.SslOpts = &gocql.SslOptions{
			Config:                 tlsConfig,
			EnableHostVerification: true,
		}
	}

	// Open session connection to ScyllaDB
	session, err := cluster.CreateSession()
	if err != nil {
		const message = "verify if ScyllaDB service/daemon is running, " +
			"check host and port settings (default Scylla port is 9042), and ensure network connectivity is stable"

		return nil, message, fmt.Errorf("%s connection error: - open session: %w", driver, err)
	}

	return session, "", nil
}

func connectDatabase(ctx context.Context, cfg *Config, timezone string, driver string, dsnBuilder databaseConnector) (*sql.DB, string, error) {
	if cfg == nil {
		const message = "ensure the app configuration is set up correctly and config is not nil"

		return nil, message, fmt.Errorf("%s connection error: app configuration is uninitialized/nil", driver)
	}

	if driver == SQLITE {
		if _, err := generateSQLiteDatabase(cfg); err != nil {
			return nil, "Ensure the database name and directory are configured correctly", err
		}
	}

	sqlDriver := driver
	if driver == ORACLE {
		sqlDriver = "godror"
	}

	var (
		database *sql.DB
		err      error
	)

	switch {
	case driver == MYSQL:
		driverConfig, configErr := mysqlDriverConfig(cfg, timezone)
		if configErr != nil {
			err = configErr
		} else {
			connector, connectorErr := mysql.NewConnector(driverConfig)
			if connectorErr != nil {
				err = connectorErr
			} else {
				database = sql.OpenDB(connector)
			}
		}
	case driver == ORACLE:
		dsn, configErr := oracleDSN(cfg, timezone)
		if configErr != nil {
			err = configErr
		} else {
			database, err = sql.Open(sqlDriver, dsn)
		}
	case dsnBuilder == nil:
		err = fmt.Errorf("missing DSN builder for %s", driver)
	default:
		database, err = sql.Open(sqlDriver, dsnBuilder(cfg, timezone))
	}

	if err != nil {
		message := fmt.Sprintf(
			"verify if %s service/daemon is running, check host and port settings, and ensure network connectivity is stable",
			driver,
		)

		return nil, message, fmt.Errorf("open connection %s error: %w", driver, err)
	}

	database.SetMaxOpenConns(cfg.MaxOpenConnections)
	database.SetMaxIdleConns(cfg.MaxIdleConnections)
	database.SetConnMaxLifetime(cfg.ConnectionMaxLifetime)

	if err := database.PingContext(ctx); err != nil {
		if errDB := database.Close(); errDB != nil {
			_ = errDB
		}

		message := fmt.Sprintf(
			"verify if %s service/daemon is running, database name, check host, credentials, port settings, TLS/ssl_mode and network connectivity is stable. "+
				"If you are using oracle, please install Oracle Instant Client (https://www.oracle.com/database/technologies/instant-client/downloads.html) "+
				"and set the correct LibDir path in your configuration (current LibDir: %s).",
			driver,
			cfg.LibDir,
		)

		return nil, message, fmt.Errorf("ping to %s server error: %w", driver, err)
	}

	return database, "", nil
}

// generateSQLiteDatabase creates the SQLite database file (and its directory) if it doesn't exist.
// It returns the final file path, which must be the same path used in the SQLite DSN.
// An existing database file is never truncated.
func generateSQLiteDatabase(cfg *Config) (string, error) {
	if cfg == nil {
		return "", errors.New("generate sqlite database: config is nil")
	}

	dbPath, err := sqliteFilePath(cfg)
	if err != nil {
		return "", err
	}

	// In-memory database has no file to create.
	if dbPath == ":memory:" {
		return dbPath, nil
	}

	// MkdirAll is idempotent: no error if the directory already exists.
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return "", fmt.Errorf("generate sqlite database: create directory: %w", err)
	}

	// O_CREATE without O_TRUNC: creates the file if missing, leaves existing data untouched.
	file, err := os.OpenFile(dbPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("generate sqlite database: create file %s: %w", dbPath, err)
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("generate sqlite database: close file %s: %w", dbPath, err)
	}

	return dbPath, nil
}

// sqliteFilePath returns the final SQLite file path. Single source of truth.
func sqliteFilePath(cfg *Config) (string, error) {
	name := cfg.Name

	if name == "" {
		return "", errors.New("generate sqlite database: database name is empty")
	}

	if name == ":memory:" {
		return name, nil
	}

	if !strings.HasSuffix(name, ".db") {
		name += ".db"
	}

	dir := DefaultDatabaseDirectory
	if cfg.DatabaseDirectory != "" {
		dir = cfg.DatabaseDirectory
	}

	return filepath.Join(dir, name), nil
}
