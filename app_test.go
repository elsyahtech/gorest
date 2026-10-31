package gorest

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestNewInitializesApplicationDefaultsAndServices(t *testing.T) {
	t.Chdir(t.TempDir())

	app := New()
	if app == nil {
		t.Fatal("New() returned nil")
	}
	if app.Name() != DefaultAppName || app.Version() != DefaultVersion || app.Timezone() != DefaultTimezone {
		t.Fatalf("application defaults = (%q, %q, %q)", app.Name(), app.Version(), app.Timezone())
	}
	if app.server == nil || app.log == nil || app.services == nil {
		t.Fatal("New() did not initialize server, logger, and service registry")
	}
	if message, code, err := app.Close(); err != nil {
		t.Fatalf("close newly initialized application: HTTP %d, %s: %v", code, message, err)
	}
}

func TestValidateProductionSQLiteHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{name: "empty", host: ""},
		{name: "localhost", host: "localhost"},
		{name: "ipv4 loopback", host: "127.0.0.1"},
		{name: "ipv6 loopback", host: "::1"},
		{name: "remote dns", host: "db.internal.example", wantErr: true},
		{name: "remote ip", host: "10.0.0.4", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateProductionSQLiteHost(test.host)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateProductionSQLiteHost(%q) error = %v, wantErr %t", test.host, err, test.wantErr)
			}
		})
	}
}

func TestInsecureProductionApproval(t *testing.T) {
	originalInput := insecureProductionPromptInput
	originalOutput := insecureProductionPromptOutput
	originalInteractive := isInteractiveTerminal
	t.Cleanup(func() {
		insecureProductionPromptInput = originalInput
		insecureProductionPromptOutput = originalOutput
		isInteractiveTerminal = originalInteractive
	})

	t.Run("default terminal detection", func(t *testing.T) {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		defer writer.Close()
		insecureProductionPromptInput = reader
		if defaultIsInteractiveTerminal() {
			t.Fatal("pipe unexpectedly reported as an interactive terminal")
		}
	})

	t.Run("explicit top level approval", func(t *testing.T) {
		app := New(Config{AllowInsecureProduction: true, Environment: Production})
		approved, err := app.requestInsecureProductionApproval("Redis", false)
		if err != nil || !approved {
			t.Fatalf("approval = %t, err = %v", approved, err)
		}
	})

	t.Run("headless startup requires approval", func(t *testing.T) {
		isInteractiveTerminal = func() bool { return false }
		app := New(Config{Environment: Production})
		approved, err := app.requestInsecureProductionApproval("Redis", false)
		if err == nil || approved {
			t.Fatalf("approval = %t, err = %v; want startup policy error", approved, err)
		}
	})

	t.Run("interactive operator can approve", func(t *testing.T) {
		isInteractiveTerminal = func() bool { return true }
		insecureProductionPromptInput = strings.NewReader("yes\n")
		insecureProductionPromptOutput = io.Discard
		app := New(Config{Environment: Production})
		approved, err := app.requestInsecureProductionApproval("PostgreSQL database", false)
		if err != nil || !approved {
			t.Fatalf("approval = %t, err = %v", approved, err)
		}
	})

	t.Run("interactive operator defaults to denial", func(t *testing.T) {
		isInteractiveTerminal = func() bool { return true }
		insecureProductionPromptInput = strings.NewReader("\n")
		insecureProductionPromptOutput = io.Discard
		app := New(Config{Environment: Production})
		approved, err := app.requestInsecureProductionApproval("Redis", false)
		if err == nil || approved {
			t.Fatalf("approval = %t, err = %v; want denied startup", approved, err)
		}
	})
}

func TestProductionDatabaseTLSWarning(t *testing.T) {
	tests := []struct {
		config     *database.Config
		name       string
		wantReason string
		wantWarn   bool
	}{
		{
			name: "no database configured",
		},
		{
			name: "database disabled",
			config: &database.Config{
				Disable: true,
				Driver:  database.POSTGRES,
			},
		},
		{
			name: "sqlite does not use transport tls",
			config: &database.Config{
				Disable: false,
				Driver:  database.SQLITE,
			},
		},
		{
			name: "mysql tls missing ca setting",
			config: &database.Config{
				Disable: false,
				Driver:  database.MYSQL,
				TLS:     database.TLSTrue,
			},
			wantWarn:   true,
			wantReason: "MySQL TLS",
		},
		{
			name: "mysql tls preferred can fall back to plaintext",
			config: &database.Config{
				Disable:   false,
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
				Disable:   false,
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
				Disable:   false,
				Driver:    database.MYSQL,
				TLS:       database.TLSTrue,
				SSLCAPath: "/etc/ssl/db-ca.pem",
			},
		},
		{
			name: "postgres plaintext",
			config: &database.Config{
				Disable: false,
				Driver:  database.POSTGRES,
				TLS:     database.TLSDisabled,
			},
			wantWarn:   true,
			wantReason: "PostgreSQL TLS is disabled",
		},
		{
			name: "postgres encrypted without identity verification",
			config: &database.Config{
				Disable: false,
				Driver:  database.POSTGRES,
				TLS:     "require",
			},
			wantWarn:   true,
			wantReason: "does not ensure server identity verification",
		},
		{
			name: "postgres ca verified without hostname verification",
			config: &database.Config{
				Disable: false,
				Driver:  database.POSTGRES,
				TLS:     "verify-ca",
			},
			wantWarn:   true,
			wantReason: "not the server hostname",
		},
		{
			name: "postgres prefer can fall back to plaintext",
			config: &database.Config{
				Disable: false,
				Driver:  database.POSTGRES,
				TLS:     "prefer",
			},
			wantWarn:   true,
			wantReason: "may fall back to an unencrypted connection",
		},
		{
			name: "postgres verified tls",
			config: &database.Config{
				Disable: false,
				Driver:  database.POSTGRES,
				TLS:     database.TLSVerifyFull,
			},
		},
		{
			name: "sql server trusts any certificate",
			config: &database.Config{
				Disable:                false,
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
				Disable:                false,
				Driver:                 database.SQLSERVER,
				TLS:                    database.TLSMandatory,
				TrustServerCertificate: false,
			},
		},
		{
			name: "oracle missing wallet",
			config: &database.Config{
				Disable: false,
				Driver:  database.ORACLE,
				TLS:     database.TLSEnabled,
			},
			wantWarn:   true,
			wantReason: "Oracle TLS is not configured",
		},
		{
			name: "oracle tcps with wallet",
			config: &database.Config{
				Disable:   false,
				Driver:    database.ORACLE,
				TLS:       database.TLSEnabled,
				SSLCAPath: "/etc/oracle/wallet",
			},
		},
		{
			name: "mongo tls disabled",
			config: &database.Config{
				Disable: false,
				Driver:  database.MONGO,
				TLS:     database.TLSFalse,
			},
			wantWarn:   true,
			wantReason: "MongoDB TLS is disabled",
		},
		{
			name: "scylla tls enabled",
			config: &database.Config{
				Disable: false,
				Driver:  database.SCYLLA,
				TLS:     database.TLSEnabled,
			},
			wantWarn: false,
		},
		{
			name: "scylla tls disabled",
			config: &database.Config{
				Disable: false,
				Driver:  database.SCYLLA,
				TLS:     database.TLSDisabled,
			},
			wantWarn:   true,
			wantReason: "ScyllaDB TLS is disabled",
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

func TestRetryStartupTransientThenSuccess(t *testing.T) {
	attempts := 0
	value, message, err := retryStartup(context.Background(), func(context.Context) (int, string, error) {
		attempts++
		if attempts < 3 {
			return 0, "database is still starting", errors.New("connection refused")
		}
		return 42, "", nil
	})
	if err != nil || value != 42 || message != "" || attempts != 3 {
		t.Fatalf("retryStartup() = (%d, %q, %v), attempts=%d", value, message, err, attempts)
	}
}

func TestRetryStartupNonTransientFailure(t *testing.T) {
	attempts := 0
	_, _, err := retryStartup(context.Background(), func(context.Context) (int, string, error) {
		attempts++
		return 0, "certificate guidance", errors.New("certificate signed by unknown authority")
	})
	if err == nil || attempts != 1 {
		t.Fatalf("retryStartup() attempts=%d error=%v, want immediate non-transient failure", attempts, err)
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
