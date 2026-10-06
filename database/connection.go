package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	//nolint:revive,nolintlint
	_ "github.com/microsoft/go-mssqldb"

	//nolint:revive,nolintlint
	_ "github.com/go-sql-driver/mysql"

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
	return connectDatabase(ctx, cfg, timezone, driver, mysqlDSN)
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
	return connectDatabase(ctx, cfg, timezone, driver, oracleDSN)
}

// ===================================================================
// MONGOConnection establishes a connection to the MongoDB database
// ===================================================================.
func mongoConnection(ctx context.Context, cfg *Config, driver string) (*mongo.Client, string, error) {
	if cfg == nil {
		const message = "ensure the app configuration is set up correctly"

		return nil, message, fmt.Errorf("%s connection error: app configuration is uninitialized", driver)
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

	// Create cluster configuration for gocql
	cluster := gocql.NewCluster(cfg.Host)
	cluster.Port = stringToInt(cfg.Port)

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

	// Open session connection to ScyllaDB
	session, err := cluster.CreateSession()
	if err != nil {
		const message = "verify if ScyllaDB service/daemon is running, " +
			"check host and port settings (default Scylla port is 9042), and ensure network connectivity is stable"

		return nil, message, fmt.Errorf("SCYLLAConnection - open session: %w", err)
	}

	return session, "", nil
}

func connectDatabase(ctx context.Context, cfg *Config, timezone string, driver string, dsnBuilder databaseConnector) (*sql.DB, string, error) {
	if cfg == nil {
		const message = "ensure the app configuration is set up correctly and config is not nil"

		return nil, message, fmt.Errorf("%s connection error: app configuration is uninitialized/nil", driver)
	}

	if driver == SQLITE {
		if err := generateSQLiteDatabase(cfg); err != nil {
			return nil, "ensure database name is configured correctly", err
		}

		const db = ".db"

		if !strings.HasSuffix(cfg.Name, ".db") {
			cfg.Name += db
		}
	}

	dsn := dsnBuilder(cfg, timezone)

	sqlDriver := driver
	if driver == ORACLE {
		sqlDriver = "godror"
	}

	database, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		message := fmt.Sprintf(
			"verify if %s service/daemon is running, check host and port settings, and ensure network connectivity is stable",
			driver,
		)

		return nil, message, fmt.Errorf("open connection %s error: %w", driver, err)
	}

	// Set connection pool parameters
	if cfg.MaxOpenConnections > 0 {
		database.SetMaxOpenConns(cfg.MaxOpenConnections)
	} else {
		database.SetMaxOpenConns(25) // default
	}

	if cfg.MaxIdleConnections > 0 {
		database.SetMaxIdleConns(cfg.MaxIdleConnections)
	} else {
		database.SetMaxIdleConns(5) // default
	}

	if cfg.ConnectionMaxLifetime > 0 {
		database.SetConnMaxLifetime(cfg.ConnectionMaxLifetime)
	} else {
		database.SetConnMaxLifetime(10 * time.Minute) // default
	}

	if err := database.PingContext(ctx); err != nil {
		if errDB := database.Close(); errDB != nil {
			_ = errDB
		}

		message := fmt.Sprintf(
			"verify if %s service/daemon is running, check host, credentials, port settings and network connectivity is stable. "+
				"If you are using oracle, please install Oracle Instant Client (https://www.oracle.com/database/technologies/instant-client/downloads.html) "+
				"and set the correct LibDir path in your configuration (current LibDir: %s).",
			driver,
			cfg.LibDir,
		)

		return nil, message, fmt.Errorf("ping to %s server error: %w", driver, err)
	}

	return database, "", nil
}

// ==========================================================================
// GenerateSQLiteDatabase ensures SQLite database file exists in root folder
// ==========================================================================.
func generateSQLiteDatabase(cfg *Config) error {
	if cfg == nil {
		return errors.New("GenerateSQLiteDatabase - config is nil")
	}

	dbName := cfg.Name
	if dbName == "" {
		return errors.New("GenerateSQLiteDatabase - database name is empty")
	}

	// Ensure .db extension
	if !strings.HasSuffix(dbName, ".db") {
		dbName += ".db"
	}

	// Create the database file if it doesn't exist
	file, err := os.OpenFile(dbName, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("GenerateSQLiteDatabase - create database file: %w", err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("GenerateSQLiteDatabase - close database file: %w", err)
	}

	return nil
}
