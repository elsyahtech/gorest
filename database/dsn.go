package database

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/godror/godror"
)

// MYSQL DSN builders dengan charset dan SSL support.
func mysqlDSN(cfg *Config, timezone string) string {
	query := url.Values{}

	query.Set("parseTime", "true")
	query.Set("loc", timezone)
	query.Set("charset", cfg.Charset)

	// SSL/TLS support untuk MySQL (if needed)
	if cfg.SSLCAPath != "" {
		query.Set("tls", cfg.TLS)
	}

	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?%s",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Name,
		query.Encode(),
	)
}

// MSSQL/SQLServer DSN builders dengan SSL support.
func mssqlDSN(cfg *Config, _ string) string {
	dsn := fmt.Sprintf(
		"server=%s;port=%s;database=%s;user id=%s;password=%s;encrypt=%s;trustservercertificate=%t",
		cfg.Host,
		cfg.Port,
		cfg.Name,
		cfg.Username,
		cfg.Password,
		cfg.TLS,
		cfg.TrustServerCertificate,
	)

	// Add SSL cert paths if provided
	if cfg.SSLCertPath != "" {
		dsn += fmt.Sprintf(";certificate=%s", cfg.SSLCertPath)
	}

	return dsn
}

// PostgreSQL DSN builders dengan SSL support.
func postgresDSN(cfg *Config, _ string) string {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host,
		cfg.Port,
		cfg.Username,
		cfg.Password,
		cfg.Name,
		cfg.TLS,
	)

	// Add SSL certificate paths if provided
	if cfg.SSLCertPath != "" {
		dsn += fmt.Sprintf(" sslcert=%s", cfg.SSLCertPath)
	}

	if cfg.SSLKeyPath != "" {
		dsn += fmt.Sprintf(" sslkey=%s", cfg.SSLKeyPath)
	}

	if cfg.SSLCAPath != "" {
		dsn += fmt.Sprintf(" sslrootcert=%s", cfg.SSLCAPath)
	}

	return dsn
}

// SQLite DSN builders.
func sqliteDSN(cfg *Config, _ string) string {
	fileName := cfg.Name

	const id = ".id"

	if !strings.HasSuffix(fileName, ".db") {
		fileName += id
	}

	filePath := filepath.Join(DefaultDatabaseDirectory, fileName)

	return fmt.Sprintf("file:%s", filePath)
}

// Oracle DSN builder dengan wallet (TLS) dan Instant Client support (godror/OCI).
func oracleDSN(cfg *Config, _ string) string {
	scheme := ""

	if cfg.TLS == TLSEnabled {
		scheme = "tcps://"
	}

	connectString := fmt.Sprintf(
		"%s%s:%s/%s?connect_timeout=%d",
		scheme,
		cfg.Host,
		cfg.Port,
		cfg.Name,
		int(cfg.Timeout.Seconds()),
	)

	params := godror.ConnectionParams{}
	params.Username = cfg.Username
	params.Password = godror.NewPassword(cfg.Password)
	params.ConnectString = connectString
	params.LibDir = cfg.LibDir
	params.ConfigDir = cfg.SSLCAPath
	params.PoolParams.MaxSessions = cfg.MaxOpenConnections       //nolint:staticcheck
	params.PoolParams.MinSessions = cfg.MaxIdleConnections       //nolint:staticcheck
	params.PoolParams.SessionTimeout = cfg.ConnectionMaxLifetime //nolint:staticcheck
	params.Timezone = time.Local

	return params.StringWithPassword()
}

// Mongo DSN builders dengan SSL support.
func mongoDSN(cfg *Config) string {
	if cfg == nil {
		return ""
	}

	query := url.Values{}

	// Add SSL/TLS if configured
	if cfg.TLS != "" {
		query.Set("tls", cfg.TLS)
	}

	if cfg.SSLCAPath != "" {
		query.Set("tlsCaFile", cfg.SSLCAPath)
	}

	if cfg.SSLCertPath != "" {
		query.Set("tlsCertificateKeyFile", cfg.SSLCertPath)
	}

	queryStr := query.Encode()

	if cfg.Username != "" && cfg.Password != "" {
		// Escape username dan password
		username := url.QueryEscape(cfg.Username)
		password := url.QueryEscape(cfg.Password)

		dsn := fmt.Sprintf(
			"mongodb://%s:%s@%s:%s/%s?authSource=admin",
			username,
			password,
			cfg.Host,
			cfg.Port,
			cfg.Name,
		)

		if queryStr != "" {
			dsn += "&" + queryStr
		}

		return dsn
	}

	// Unauthenticated
	dsn := fmt.Sprintf(
		"mongodb://%s:%s/%s",
		cfg.Host,
		cfg.Port,
		cfg.Name,
	)

	if queryStr != "" {
		dsn += "?" + queryStr
	}

	return dsn
}
