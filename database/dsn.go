package database

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/godror/godror"
)

func mysqlDriverConfig(cfg *Config, timezone string) (*mysql.Config, error) {
	dsnConfig := mysql.NewConfig()
	dsnConfig.User = cfg.Username
	dsnConfig.Passwd = cfg.Password
	dsnConfig.Net = "tcp"
	dsnConfig.Addr = net.JoinHostPort(cfg.Host, cfg.Port)
	dsnConfig.DBName = cfg.Name
	dsnConfig.Params = map[string]string{
		"parseTime": "true",
		"loc":       timezone,
	}
	if err := dsnConfig.Apply(mysql.Charset(cfg.Charset, "")); err != nil {
		return nil, fmt.Errorf("configure MySQL charset: %w", err)
	}

	// SSL/TLS support untuk MySQL (if needed)
	if cfg.SSLCAPath != "" {
		dsnConfig.Params["tls"] = cfg.TLS
	}

	return dsnConfig, nil
}

// MSSQL/SQLServer DSN builders dengan SSL support.
func mssqlDSN(cfg *Config, _ string) string {
	query := url.Values{}
	query.Set("database", cfg.Name)
	query.Set("encrypt", cfg.TLS)
	query.Set("TrustServerCertificate", fmt.Sprintf("%t", cfg.TrustServerCertificate))

	// Add SSL cert paths if provided
	if cfg.SSLCertPath != "" {
		query.Set("certificate", cfg.SSLCertPath)
	}

	return (&url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(cfg.Username, cfg.Password),
		Host:     databaseURLHost(cfg.Host, cfg.Port),
		RawQuery: query.Encode(),
	}).String()
}

// PostgreSQL DSN builders dengan SSL support.
func postgresDSN(cfg *Config, _ string) string {
	query := url.Values{}
	query.Set("sslmode", cfg.TLS)

	// Add SSL certificate paths if provided
	if cfg.SSLCertPath != "" {
		query.Set("sslcert", cfg.SSLCertPath)
	}

	if cfg.SSLKeyPath != "" {
		query.Set("sslkey", cfg.SSLKeyPath)
	}

	if cfg.SSLCAPath != "" {
		query.Set("sslrootcert", cfg.SSLCAPath)
	}

	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.Username, cfg.Password),
		Host:     databaseURLHost(cfg.Host, cfg.Port),
		Path:     cfg.Name,
		RawQuery: query.Encode(),
	}).String()
}

func databaseURLHost(host, port string) string {
	if port != "" {
		return net.JoinHostPort(host, port)
	}

	if net.ParseIP(host) != nil && strings.Contains(host, ":") {
		return "[" + host + "]"
	}

	return host
}

// SQLite DSN builder.
func sqliteDSN(cfg *Config, _ string) string {
	filePath, _ := sqliteFilePath(cfg)

	return "file:" + filePath
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
