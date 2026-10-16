package gorest

import (
	"strings"

	"github.com/elsyahtech/gorest/database"
)

func productionDatabaseTLSWarning(cfg *database.Config) (string, bool) {
	if cfg == nil || !cfg.Enabled {
		return "", false
	}

	driver := database.NormalizeDatabaseDriver(cfg.Driver)
	tlsMode := strings.ToLower(strings.TrimSpace(cfg.TLS))

	switch driver {
	case database.SQLITE:
		return "", false
	case database.MYSQL:
		if cfg.SSLCAPath == "" || tlsMode != database.TLSTrue {
			return "MySQL TLS is disabled or cannot be verified from this configuration. Set TLS to \"true\" to use the driver's system certificate roots, or register and verify a go-sql-driver/mysql TLS profile; GOREST only passes the TLS option when SSLCAPath is set.", true
		}
	case database.POSTGRES:
		switch tlsMode {
		case "", database.TLSDisabled:
			return "PostgreSQL TLS is disabled. Set TLS to \"verify-full\" and configure SSLCAPath with the trusted CA certificate for production.", true
		case "require":
			return "PostgreSQL traffic is encrypted, but TLS mode \"require\" does not ensure server identity verification. Prefer TLS \"verify-full\" with the trusted CA configured in SSLCAPath.", true
		case "verify-ca":
			return "PostgreSQL verifies the certificate chain but not the server hostname. Prefer TLS \"verify-full\" with the trusted CA configured in SSLCAPath.", true
		case "allow", "prefer":
			return "PostgreSQL TLS mode \"" + tlsMode + "\" may fall back to an unencrypted connection. Set TLS to \"verify-full\" and configure SSLCAPath with the trusted CA certificate for production.", true
		}
	case database.SQLSERVER:
		if tlsMode == "" || tlsMode == database.TLSDisabled || tlsMode == database.TLSFalse ||
			tlsMode == database.TLSOptional || cfg.TrustServerCertificate {
			return "SQL Server encryption is disabled or optional. Set TLS to \"mandatory\" or \"strict\" and keep TrustServerCertificate false so the server certificate is validated.", true
		}
	case database.ORACLE:
		if tlsMode != database.TLSEnabled || cfg.SSLCAPath == "" {
			return "Oracle TLS is not configured. Set TLS to \"enable\" and SSLCAPath to the Oracle wallet directory for a TCPS connection.", true
		}
	case database.MONGO:
		if tlsMode != database.TLSTrue {
			return "MongoDB TLS is disabled. Set TLS to \"true\" and configure certificate verification for production.", true
		}
	case database.SCYLLA:
		return "The database TLS setting does not configure ScyllaDB transport encryption. Configure TLS using the ScyllaDB driver's cluster TLS options or protect the connection with a trusted encrypted network.", true
	}

	return "", false
}

func isProductionEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "", Development, Testing:
		return false
	default:
		return true
	}
}
