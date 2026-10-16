package gorest

import (
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestProductionDatabaseTLSWarning(t *testing.T) {
	tests := []struct {
		name       string
		config     *database.Config
		wantWarn   bool
		wantReason string
	}{
		{
			name: "no database configured",
		},
		{
			name: "database disabled",
			config: &database.Config{
				Driver: database.POSTGRES,
			},
		},
		{
			name: "sqlite does not use transport tls",
			config: &database.Config{
				Enabled: true,
				Driver:  database.SQLITE,
			},
		},
		{
			name: "mysql tls missing ca setting",
			config: &database.Config{
				Enabled: true,
				Driver:  database.MYSQL,
				TLS:     database.TLSTrue,
			},
			wantWarn:   true,
			wantReason: "MySQL TLS",
		},
		{
			name: "mysql tls preferred can fall back to plaintext",
			config: &database.Config{
				Enabled:   true,
				Driver:    database.MYSQL,
				TLS:       "preferred",
				SSLCAPath: "/etc/ssl/db-ca.pem",
			},
			wantWarn:   true,
			wantReason: "MySQL TLS is disabled or cannot be verified",
		},
		{
			name: "mysql custom tls profile cannot be inspected",
			config: &database.Config{
				Enabled:   true,
				Driver:    database.MYSQL,
				TLS:       "private-ca-profile",
				SSLCAPath: "/etc/ssl/db-ca.pem",
			},
			wantWarn:   true,
			wantReason: "cannot be verified from this configuration",
		},
		{
			name: "mysql tls configured",
			config: &database.Config{
				Enabled:   true,
				Driver:    database.MYSQL,
				TLS:       database.TLSTrue,
				SSLCAPath: "/etc/ssl/db-ca.pem",
			},
		},
		{
			name: "postgres plaintext",
			config: &database.Config{
				Enabled: true,
				Driver:  database.POSTGRES,
				TLS:     database.TLSDisabled,
			},
			wantWarn:   true,
			wantReason: "PostgreSQL TLS is disabled",
		},
		{
			name: "postgres encrypted without identity verification",
			config: &database.Config{
				Enabled: true,
				Driver:  database.POSTGRES,
				TLS:     "require",
			},
			wantWarn:   true,
			wantReason: "does not ensure server identity verification",
		},
		{
			name: "postgres ca verified without hostname verification",
			config: &database.Config{
				Enabled: true,
				Driver:  database.POSTGRES,
				TLS:     "verify-ca",
			},
			wantWarn:   true,
			wantReason: "not the server hostname",
		},
		{
			name: "postgres prefer can fall back to plaintext",
			config: &database.Config{
				Enabled: true,
				Driver:  database.POSTGRES,
				TLS:     "prefer",
			},
			wantWarn:   true,
			wantReason: "may fall back to an unencrypted connection",
		},
		{
			name: "postgres verified tls",
			config: &database.Config{
				Enabled: true,
				Driver:  database.POSTGRES,
				TLS:     database.TLSVerifyFull,
			},
		},
		{
			name: "sql server trusts any certificate",
			config: &database.Config{
				Enabled:                true,
				Driver:                 database.SQLSERVER,
				TLS:                    database.TLSMandatory,
				TrustServerCertificate: true,
			},
			wantWarn:   true,
			wantReason: "TrustServerCertificate false",
		},
		{
			name: "sql server verified tls",
			config: &database.Config{
				Enabled:                true,
				Driver:                 database.SQLSERVER,
				TLS:                    database.TLSMandatory,
				TrustServerCertificate: false,
			},
		},
		{
			name: "oracle missing wallet",
			config: &database.Config{
				Enabled: true,
				Driver:  database.ORACLE,
				TLS:     database.TLSEnabled,
			},
			wantWarn:   true,
			wantReason: "Oracle TLS is not configured",
		},
		{
			name: "oracle tcps with wallet",
			config: &database.Config{
				Enabled:   true,
				Driver:    database.ORACLE,
				TLS:       database.TLSEnabled,
				SSLCAPath: "/etc/oracle/wallet",
			},
		},
		{
			name: "mongo tls disabled",
			config: &database.Config{
				Enabled: true,
				Driver:  database.MONGO,
				TLS:     database.TLSFalse,
			},
			wantWarn:   true,
			wantReason: "MongoDB TLS is disabled",
		},
		{
			name: "scylla tls unsupported by config field",
			config: &database.Config{
				Enabled: true,
				Driver:  database.SCYLLA,
				TLS:     database.TLSEnabled,
			},
			wantWarn:   true,
			wantReason: "does not configure ScyllaDB transport encryption",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			warning, shouldWarn := productionDatabaseTLSWarning(test.config)
			if shouldWarn != test.wantWarn {
				t.Fatalf("shouldWarn = %t, want %t (warning: %q)", shouldWarn, test.wantWarn, warning)
			}
			if test.wantReason != "" && !strings.Contains(warning, test.wantReason) {
				t.Errorf("warning = %q, want it to contain %q", warning, test.wantReason)
			}
		})
	}
}

func TestIsProductionLikeEnvironment(t *testing.T) {
	tests := []struct {
		environment string
		want        bool
	}{
		{environment: Development, want: false},
		{environment: Testing, want: false},
		{environment: Production, want: true},
		{environment: "staging", want: true},
		{environment: "uat", want: true},
		{environment: "preview", want: true},
		{environment: " Production ", want: true},
		{environment: ""},
	}

	for _, test := range tests {
		if got := isProductionEnvironment(test.environment); got != test.want {
			t.Errorf("isProductionEnvironment(%q) = %t, want %t", test.environment, got, test.want)
		}
	}
}

func TestConfigDefaultEnvironment(t *testing.T) {
	if got := configDefault(Config{}).Environment; got != Development {
		t.Errorf("configDefault(Config{}).Environment = %q, want %q", got, Development)
	}

	if got := configDefault(Config{Environment: "staging"}).Environment; got != "staging" {
		t.Errorf("custom environment = %q, want %q", got, "staging")
	}
}
