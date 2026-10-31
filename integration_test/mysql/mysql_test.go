//go:build integration

package mysql_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
	"github.com/go-sql-driver/mysql"
)

func TestMySQLServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.MYSQL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, message, code, err := app.Database().QuerySQL(ctx, "SELECT VERSION(), @@version_comment, @@character_set_connection")
	if err != nil {
		t.Fatalf("query MySQL server version failed (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("MySQL server version query returned no row: %v", rows.Err())
	}

	var version, product, charset string
	if err := rows.Scan(&version, &product, &charset); err != nil {
		t.Fatalf("scan MySQL server product, version, and charset: %v", err)
	}
	if !strings.EqualFold(charset, database.ConfigDefault.Charset) {
		t.Errorf("connection charset = %q, want %q", charset, database.ConfigDefault.Charset)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate MySQL server version result: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close MySQL server version rows: %v", err)
	}

	t.Logf("MySQL server product: %s, version: %s, connection charset: %s", product, version, charset)
}

func TestMySQLTLSVerifiesServerCertificateAndHostname(t *testing.T) {
	const (
		verifiedProfile  = "gorest_integration_verified"
		wrongHostProfile = "gorest_integration_wrong_host"
		untrustedProfile = "gorest_integration_untrusted_ca"
	)
	for name, config := range map[string]*tls.Config{
		verifiedProfile: {
			MinVersion: tls.VersionTLS12,
		},
		wrongHostProfile: {
			MinVersion: tls.VersionTLS12,
			ServerName: "wrong-host.gorest.invalid",
		},
		untrustedProfile: {
			MinVersion: tls.VersionTLS12,
			RootCAs:    x509.NewCertPool(),
		},
	} {
		if err := mysql.RegisterTLSConfig(name, config); err != nil {
			t.Fatalf("register MySQL TLS profile %q: %v", name, err)
		}
		t.Cleanup(func() {
			mysql.DeregisterTLSConfig(name)
		})
	}

	if os.Getenv("GOREST_IT_MYSQL_HOST") == "" &&
		!strings.EqualFold(os.Getenv("GOREST_IT_REQUIRED"), "true") &&
		os.Getenv("GOREST_IT_REQUIRED") != "1" {
		t.Skip("set GOREST_IT_MYSQL_HOST to run MySQL TLS verification integration tests")
	}

	cfg := database.ConfigFromEnv("GOREST_IT_MYSQL")
	cfg.Driver = database.MYSQL
	cfg.Timeout = 10 * time.Second
	cfg.TLS = verifiedProfile
	if cfg.SSLCAPath == "" {
		t.Fatal("MySQL TLS profile requires GOREST_IT_MYSQL_SSL_CA to be set")
	}

	connect := func(t *testing.T, tlsProfile string) error {
		t.Helper()
		testConfig := database.Run(cfg)
		testConfig.TLS = tlsProfile
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		db, _, err := database.New(ctx, &testConfig, "UTC")
		if err != nil {
			return err
		}
		if tlsProfile == verifiedProfile {
			var status, cipher string
			if err := db.SQL.QueryRowContext(ctx, "SHOW SESSION STATUS LIKE 'Ssl_cipher'").Scan(&status, &cipher); err != nil {
				t.Fatalf("query MySQL TLS status: %v", err)
			}
			if status != "Ssl_cipher" || cipher == "" {
				t.Fatalf("MySQL connection did not negotiate TLS (status=%q, cipher=%q)", status, cipher)
			}
		}
		if _, _, err := db.Close(ctx, &testConfig); err != nil {
			t.Fatalf("close MySQL connection using TLS profile %q: %v", tlsProfile, err)
		}
		return nil
	}

	if err := connect(t, verifiedProfile); err != nil {
		t.Fatalf("MySQL TLS connection with system roots and hostname verification failed: %v", err)
	}
	err := connect(t, wrongHostProfile)
	var hostnameErr x509.HostnameError
	if !errors.As(err, &hostnameErr) {
		t.Fatalf("MySQL TLS connection error = %v, want hostname verification failure", err)
	}
	err = connect(t, untrustedProfile)
	var untrustedCAErr x509.UnknownAuthorityError
	if !errors.As(err, &untrustedCAErr) {
		t.Fatalf("MySQL TLS connection error = %v, want untrusted CA verification failure", err)
	}
}

func TestMySQLMutualTLSRequiresClientCertificate(t *testing.T) {
	const (
		noClientCertificateProfile = "gorest_integration_mtls_without_client_cert"
		clientCertificateProfile   = "gorest_integration_mtls_with_client_cert"
	)
	if os.Getenv("GOREST_IT_MYSQL_MTLS_HOST") == "" &&
		!strings.EqualFold(os.Getenv("GOREST_IT_REQUIRED"), "true") &&
		os.Getenv("GOREST_IT_REQUIRED") != "1" {
		t.Skip("set GOREST_IT_MYSQL_MTLS_HOST to run MySQL mTLS integration tests")
	}

	cfg := database.ConfigFromEnv("GOREST_IT_MYSQL_MTLS")
	cfg.Driver = database.MYSQL
	cfg.Timeout = 10 * time.Second
	for env, value := range map[string]string{
		"GOREST_IT_MYSQL_MTLS_HOST":        cfg.Host,
		"GOREST_IT_MYSQL_MTLS_NAME":        cfg.Name,
		"GOREST_IT_MYSQL_MTLS_USERNAME":    cfg.Username,
		"GOREST_IT_MYSQL_MTLS_PASSWORD":    cfg.Password,
		"GOREST_IT_MYSQL_MTLS_SSL_CA":      cfg.SSLCAPath,
		"GOREST_IT_MYSQL_MTLS_CLIENT_CERT": os.Getenv("GOREST_IT_MYSQL_MTLS_CLIENT_CERT"),
		"GOREST_IT_MYSQL_MTLS_CLIENT_KEY":  os.Getenv("GOREST_IT_MYSQL_MTLS_CLIENT_KEY"),
	} {
		if value == "" {
			t.Fatalf("required integration-test environment variable %s is not set", env)
		}
	}

	clientCert, err := tls.LoadX509KeyPair(
		os.Getenv("GOREST_IT_MYSQL_MTLS_CLIENT_CERT"),
		os.Getenv("GOREST_IT_MYSQL_MTLS_CLIENT_KEY"),
	)
	if err != nil {
		t.Fatalf("load MySQL mTLS client certificate and key: %v", err)
	}
	for name, config := range map[string]*tls.Config{
		noClientCertificateProfile: {
			MinVersion: tls.VersionTLS12,
		},
		clientCertificateProfile: {
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{clientCert},
		},
	} {
		if err := mysql.RegisterTLSConfig(name, config); err != nil {
			t.Fatalf("register MySQL TLS profile %q: %v", name, err)
		}
		t.Cleanup(func() {
			mysql.DeregisterTLSConfig(name)
		})
	}

	connect := func(t *testing.T, tlsProfile string) error {
		t.Helper()
		testConfig := database.Run(cfg)
		testConfig.TLS = tlsProfile
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		db, _, err := database.New(ctx, &testConfig, "UTC")
		if err != nil {
			return err
		}
		if tlsProfile == clientCertificateProfile {
			var status, cipher string
			if err := db.SQL.QueryRowContext(ctx, "SHOW SESSION STATUS LIKE 'Ssl_cipher'").Scan(&status, &cipher); err != nil {
				t.Fatalf("query MySQL mTLS status: %v", err)
			}
			if status != "Ssl_cipher" || cipher == "" {
				t.Fatalf("MySQL mTLS connection did not negotiate TLS (status=%q, cipher=%q)", status, cipher)
			}
		}
		if _, _, err := db.Close(ctx, &testConfig); err != nil {
			t.Fatalf("close MySQL connection using TLS profile %q: %v", tlsProfile, err)
		}
		return nil
	}

	err = connect(t, noClientCertificateProfile)
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		t.Fatalf("MySQL mTLS connection without a client certificate error = %v, want server access denial", err)
	}
	if mysqlErr.Number != 1045 {
		t.Fatalf("MySQL mTLS connection without a client certificate error number = %d, want 1045 (access denied)", mysqlErr.Number)
	}
	if err := connect(t, clientCertificateProfile); err != nil {
		t.Fatalf("MySQL mTLS connection with the configured client certificate failed: %v", err)
	}
}

func TestORMCRUDAndBoundValues(t *testing.T) {
	internaltest.SQLCRUD(t, database.MYSQL)
}

func TestORMTransactionRollback(t *testing.T) {
	internaltest.SQLTransactionRollback(t, database.MYSQL)
}

func TestORMParallelTransactions(t *testing.T) {
	internaltest.SQLParallelTransactions(t, database.MYSQL)
}

func TestORMPaginationAndGrouping(t *testing.T) {
	internaltest.SQLPaginationAndGrouping(t, database.MYSQL)
}

func TestORMCoverage(t *testing.T) {
	t.Run("joins and relation mapping", func(t *testing.T) {
		internaltest.SQLJoinAndPreload(t, database.MYSQL)
	})
	t.Run("nullable mapping", func(t *testing.T) {
		internaltest.SQLNullableMapping(t, database.MYSQL)
	})
	t.Run("pagination boundaries", func(t *testing.T) {
		internaltest.SQLPaginationBoundaries(t, database.MYSQL)
	})
	t.Run("batch failure atomicity", func(t *testing.T) {
		internaltest.SQLBatchFailureAtomicity(t, database.MYSQL)
	})
	t.Run("live query cancellation", func(t *testing.T) {
		internaltest.SQLContextCancellation(t, database.MYSQL)
	})
	t.Run("upsert behavior", func(t *testing.T) {
		internaltest.SQLUpsertBehavior(t, database.MYSQL)
	})
	t.Run("invalid ORM inputs", func(t *testing.T) {
		internaltest.SQLInvalidInputs(t, database.MYSQL)
	})
}
