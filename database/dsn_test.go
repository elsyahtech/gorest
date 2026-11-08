package database

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"github.com/microsoft/go-mssqldb/msdsn"
)

func TestMySQLDriverConfigPreservesCredentials(t *testing.T) {
	cfg := testDSNConfig()
	cfg.Port = MYSQL_PORT
	cfg.Charset = "utf8mb4"
	cfg.TLS = "false"
	cfg.Timeout = 15 * time.Second
	driverConfig, err := mysqlDriverConfig(cfg, "UTC")
	if err != nil {
		t.Fatalf("mysqlDriverConfig() error = %v", err)
	}

	if driverConfig.User != cfg.Username {
		t.Errorf("username = %q, want %q", driverConfig.User, cfg.Username)
	}
	if driverConfig.Passwd != cfg.Password {
		t.Errorf("password = %q, want %q", driverConfig.Passwd, cfg.Password)
	}
	if driverConfig.DBName != cfg.Name {
		t.Errorf("database = %q, want %q", driverConfig.DBName, cfg.Name)
	}
	if driverConfig.Addr != cfg.Host+":"+cfg.Port {
		t.Errorf("address = %q, want %q:%s", driverConfig.Addr, cfg.Host, cfg.Port)
	}
	if !driverConfig.ParseTime {
		t.Error("ParseTime = false, want true")
	}
	if driverConfig.Timeout != cfg.Timeout {
		t.Errorf("dial timeout = %s, want %s", driverConfig.Timeout, cfg.Timeout)
	}
	if driverConfig.ReadTimeout != cfg.Timeout {
		t.Errorf("read timeout = %s, want %s", driverConfig.ReadTimeout, cfg.Timeout)
	}
	if driverConfig.WriteTimeout != cfg.Timeout {
		t.Errorf("write timeout = %s, want %s", driverConfig.WriteTimeout, cfg.Timeout)
	}
	if driverConfig.Loc == nil || driverConfig.Loc.String() != "UTC" {
		t.Errorf("location = %v, want UTC", driverConfig.Loc)
	}
	if driverConfig.TLSConfig != cfg.TLS {
		t.Errorf("TLSConfig = %q, want %q", driverConfig.TLSConfig, cfg.TLS)
	}
	if len(driverConfig.Params) != 0 {
		t.Errorf("unexpected generic session parameters = %#v", driverConfig.Params)
	}
	if _, err := mysql.NewConnector(driverConfig); err != nil {
		t.Fatalf("mysql.NewConnector() error = %v", err)
	}
}

func TestMySQLDriverConfigLoadsOptionalCAAndClientCertificate(t *testing.T) {
	caPath, clientCertPath, clientKeyPath := writeMySQLTestCertificates(t)
	cfg := testDSNConfig()
	cfg.TLS = TLSTrue
	cfg.SSLCAPath = caPath
	cfg.SSLCertPath = clientCertPath
	cfg.SSLKeyPath = clientKeyPath

	driverConfig, err := mysqlDriverConfig(cfg, "UTC")
	if err != nil {
		t.Fatalf("mysqlDriverConfig() error = %v", err)
	}
	if driverConfig.TLS == nil {
		t.Fatal("TLS config is nil, want direct TLS config for custom certificate paths")
	}
	if driverConfig.TLS.ServerName != cfg.Host {
		t.Errorf("TLS ServerName = %q, want %q", driverConfig.TLS.ServerName, cfg.Host)
	}
	if driverConfig.TLS.InsecureSkipVerify {
		t.Error("TLS config enabled InsecureSkipVerify")
	}
	if driverConfig.TLS.RootCAs == nil || len(driverConfig.TLS.RootCAs.Subjects()) == 0 {
		t.Error("TLS RootCAs is empty, want configured CA certificate")
	}
	if len(driverConfig.TLS.Certificates) != 1 {
		t.Errorf("TLS client certificates = %d, want 1", len(driverConfig.TLS.Certificates))
	}
	if _, err := mysql.NewConnector(driverConfig); err != nil {
		t.Fatalf("mysql.NewConnector() with custom TLS config error = %v", err)
	}
}

func TestMySQLDriverConfigRejectsInvalidCertificatePaths(t *testing.T) {
	t.Run("incomplete client certificate pair", func(t *testing.T) {
		cfg := testDSNConfig()
		cfg.TLS = TLSTrue
		cfg.SSLCertPath = "client.crt"

		if _, err := mysqlDriverConfig(cfg, "UTC"); err == nil {
			t.Fatal("mysqlDriverConfig() error = nil, want incomplete client certificate pair error")
		}
	})

	t.Run("invalid CA PEM", func(t *testing.T) {
		caPath := filepath.Join(t.TempDir(), "ca.pem")
		if err := os.WriteFile(caPath, []byte("not a PEM certificate"), 0o600); err != nil {
			t.Fatalf("write invalid CA file: %v", err)
		}
		cfg := testDSNConfig()
		cfg.TLS = TLSTrue
		cfg.SSLCAPath = caPath

		if _, err := mysqlDriverConfig(cfg, "UTC"); err == nil {
			t.Fatal("mysqlDriverConfig() error = nil, want invalid CA PEM error")
		}
	})

	t.Run("named profile takes precedence over certificate paths", func(t *testing.T) {
		cfg := testDSNConfig()
		cfg.TLS = "private-profile"
		cfg.SSLCAPath = filepath.Join(t.TempDir(), "unused-ca.pem")

		driverConfig, err := mysqlDriverConfig(cfg, "UTC")
		if err != nil {
			t.Fatalf("mysqlDriverConfig() error = %v", err)
		}
		if driverConfig.TLSConfig != cfg.TLS {
			t.Errorf("TLSConfig = %q, want named profile %q", driverConfig.TLSConfig, cfg.TLS)
		}
		if driverConfig.TLS != nil {
			t.Error("TLS config is set, want named profile to take precedence")
		}
	})
}

func TestMySQLDriverConfigTLSDoesNotRequireCAPath(t *testing.T) {
	tests := []struct {
		name    string
		tls     string
		wantTLS string
	}{
		{name: "system roots", tls: TLSTrue, wantTLS: TLSTrue},
		{name: "custom profile", tls: "private-ca", wantTLS: "private-ca"},
		{name: "disabled", tls: TLSDisabled},
		{name: "omitted", tls: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testDSNConfig()
			cfg.TLS = test.tls

			driverConfig, err := mysqlDriverConfig(cfg, "UTC")
			if err != nil {
				t.Fatalf("mysqlDriverConfig() error = %v", err)
			}
			if driverConfig.TLSConfig != test.wantTLS {
				t.Errorf("TLSConfig = %q, want %q", driverConfig.TLSConfig, test.wantTLS)
			}
		})
	}
}

func writeMySQLTestCertificates(t *testing.T) (caPath, clientCertPath, clientKeyPath string) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "GOREST test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "GOREST test client"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client certificate: %v", err)
	}

	directory := t.TempDir()
	caPath = filepath.Join(directory, "ca.pem")
	clientCertPath = filepath.Join(directory, "client.crt")
	clientKeyPath = filepath.Join(directory, "client.key")
	for path, data := range map[string][]byte{
		caPath:         pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		clientCertPath: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER}),
		clientKeyPath:  pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)}),
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write test TLS file %q: %v", path, err)
		}
	}

	return caPath, clientCertPath, clientKeyPath
}

func TestMSSQLDSNRoundTripsCredentials(t *testing.T) {
	cfg := testDSNConfig()
	cfg.TLS = "mandatory"
	cfg.TrustServerCertificate = false

	parsed, err := msdsn.Parse(mssqlDSN(cfg, ""))
	if err != nil {
		t.Fatalf("msdsn.Parse() error = %v", err)
	}

	if parsed.User != cfg.Username {
		t.Errorf("username = %q, want %q", parsed.User, cfg.Username)
	}
	if parsed.Password != cfg.Password {
		t.Errorf("password = %q, want %q", parsed.Password, cfg.Password)
	}
	if parsed.Database != cfg.Name {
		t.Errorf("database = %q, want %q", parsed.Database, cfg.Name)
	}
	if parsed.Host != cfg.Host || parsed.Port != 1433 {
		t.Errorf("server = %q:%d, want %q:%s", parsed.Host, parsed.Port, cfg.Host, cfg.Port)
	}
	if parsed.TrustServerCertificate != cfg.TrustServerCertificate {
		t.Errorf("TrustServerCertificate = %t, want %t", parsed.TrustServerCertificate, cfg.TrustServerCertificate)
	}
}

func TestPostgresDSNRoundTripsCredentials(t *testing.T) {
	cfg := testDSNConfig()
	cfg.SSLCertPath = "/tmp/client cert.pem"
	cfg.SSLKeyPath = "/tmp/client&key.pem"
	cfg.SSLCAPath = "/tmp/ca; cert.pem"
	cfg.TLS = "verify-full"

	dsn := postgresDSN(cfg, "")
	if _, err := pq.NewConnector(dsn); err != nil {
		t.Fatalf("pq.NewConnector() error = %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	username := parsed.User.Username()
	password, hasPassword := parsed.User.Password()
	if username != cfg.Username {
		t.Errorf("username = %q, want %q", username, cfg.Username)
	}
	if !hasPassword || password != cfg.Password {
		t.Errorf("password = %q (present %t), want %q", password, hasPassword, cfg.Password)
	}
	if parsed.Path != "/"+cfg.Name {
		t.Errorf("database = %q, want %q", parsed.Path, "/"+cfg.Name)
	}
	if parsed.Query().Get("sslmode") != cfg.TLS {
		t.Errorf("sslmode = %q, want %q", parsed.Query().Get("sslmode"), cfg.TLS)
	}
	if parsed.Query().Get("sslcert") != cfg.SSLCertPath ||
		parsed.Query().Get("sslkey") != cfg.SSLKeyPath ||
		parsed.Query().Get("sslrootcert") != cfg.SSLCAPath {
		t.Errorf("certificate parameters = %v", parsed.Query())
	}
}

func TestOracleConnectionParamsUseConfiguredTimezone(t *testing.T) {
	cfg := testDSNConfig()
	params, err := oracleConnectionParams(cfg, "Asia/Jakarta")
	if err != nil {
		t.Fatalf("oracleConnectionParams() error = %v", err)
	}

	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("time.LoadLocation() error = %v", err)
	}
	if params.Timezone.String() != location.String() {
		t.Errorf("timezone = %q, want %q", params.Timezone, location)
	}
}

func TestOracleConnectionParamsUseTCPSAndWalletWhenTLSIsEnabled(t *testing.T) {
	cfg := testDSNConfig()
	cfg.Host = "db.example.test"
	cfg.Port = "2484"
	cfg.Name = "XEPDB1"
	cfg.TLS = TLSEnabled
	cfg.SSLCAPath = "/tmp/oracle-wallet"
	cfg.Timeout = 15 * time.Second

	params, err := oracleConnectionParams(cfg, "UTC")
	if err != nil {
		t.Fatalf("oracleConnectionParams() error = %v", err)
	}

	if want := "tcps://db.example.test:2484/XEPDB1?connect_timeout=15"; params.ConnectString != want {
		t.Errorf("connect string = %q, want %q", params.ConnectString, want)
	}
	if params.ConfigDir != cfg.SSLCAPath {
		t.Errorf("config directory = %q, want wallet directory %q", params.ConfigDir, cfg.SSLCAPath)
	}
}

func TestOracleConnectionParamsRejectInvalidTimezone(t *testing.T) {
	if _, err := oracleConnectionParams(testDSNConfig(), "Invalid/Timezone"); err == nil {
		t.Fatal("oracleConnectionParams() unexpectedly accepted an invalid timezone")
	}
}

func TestOracleDSNIncludesConnectionSettings(t *testing.T) {
	cfg := testDSNConfig()
	cfg.TLS = TLSEnabled
	cfg.Timeout = 15 * time.Second

	dsn, err := oracleDSN(cfg, "UTC")
	if err != nil {
		t.Fatalf("oracleDSN() error = %v", err)
	}
	for _, expected := range []string{"localhost:1433/db", "tcps://", "connect_timeout=15"} {
		if !strings.Contains(dsn, expected) {
			t.Errorf("Oracle DSN %q does not contain %q", dsn, expected)
		}
	}
}

func TestMongoDSNRoundTripsAuthenticationAndTLS(t *testing.T) {
	cfg := testDSNConfig()
	cfg.Port = "27017"
	cfg.TLS = "true"
	cfg.SSLCAPath = "/tmp/mongo ca.pem"
	cfg.SSLCertPath = "/tmp/client cert.pem"
	cfg.AuthSource = "gorest"

	dsn := mongoDSN(cfg)
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(Mongo DSN) error = %v", err)
	}
	username := parsed.User.Username()
	password, hasPassword := parsed.User.Password()
	if parsed.Scheme != "mongodb" || parsed.Hostname() != cfg.Host || parsed.Port() != cfg.Port {
		t.Fatalf("Mongo endpoint = %q, want %s:%s", parsed.Host, cfg.Host, cfg.Port)
	}
	if username != cfg.Username || !hasPassword || password != cfg.Password {
		t.Fatalf("Mongo credentials = %q / %q (present %t)", username, password, hasPassword)
	}
	if parsed.Path != "/"+cfg.Name {
		t.Fatalf("Mongo database = %q, want %q", parsed.Path, "/"+cfg.Name)
	}
	query := parsed.Query()
	if query.Get("authSource") != "gorest" || query.Get("tls") != "true" ||
		query.Get("tlsCaFile") != cfg.SSLCAPath || query.Get("tlsCertificateKeyFile") != cfg.SSLCertPath {
		t.Fatalf("Mongo connection options = %v", query)
	}
}

func TestMongoDSNDefaultsAuthenticationDatabaseToAdmin(t *testing.T) {
	cfg := testDSNConfig()
	parsed, err := url.Parse(mongoDSN(cfg))
	if err != nil {
		t.Fatalf("url.Parse(Mongo DSN) error = %v", err)
	}
	if got := parsed.Query().Get("authSource"); got != "admin" {
		t.Fatalf("Mongo authentication database = %q, want admin", got)
	}
}

func TestMongoDSNUsesSeparateClientCertificateAndKey(t *testing.T) {
	cfg := testDSNConfig()
	cfg.SSLCertPath = "/tmp/client.crt"
	cfg.SSLKeyPath = "/tmp/client.key"

	parsed, err := url.Parse(mongoDSN(cfg))
	if err != nil {
		t.Fatalf("url.Parse(Mongo DSN) error = %v", err)
	}
	query := parsed.Query()
	if query.Get("tlsCertificateFile") != cfg.SSLCertPath ||
		query.Get("tlsPrivateKeyFile") != cfg.SSLKeyPath ||
		query.Has("tlsCertificateKeyFile") {
		t.Fatalf("Mongo client certificate options = %v", query)
	}
}

func TestMongoDSNWithoutCredentialsAndNilConfig(t *testing.T) {
	cfg := &Config{Host: "::1", Name: "gorest"}
	parsed, err := url.Parse(mongoDSN(cfg))
	if err != nil {
		t.Fatalf("url.Parse(unauthenticated Mongo DSN) error = %v", err)
	}
	if parsed.Hostname() != "::1" || parsed.User != nil || parsed.Path != "/gorest" {
		t.Fatalf("unauthenticated Mongo URI = %#v", parsed)
	}
	if got := mongoDSN(nil); got != "" {
		t.Fatalf("mongoDSN(nil) = %q, want empty", got)
	}

	if got := databaseURLHost("::1", "27017"); got != "[::1]:27017" {
		t.Fatalf("databaseURLHost(IPv6, port) = %q", got)
	}
	if got := databaseURLHost("::1", ""); got != "[::1]" {
		t.Fatalf("databaseURLHost(IPv6, no port) = %q", got)
	}
}

func testDSNConfig() *Config {
	return &Config{
		Host:     "localhost",
		Port:     "1433",
		Username: "user@name:part;? &",
		Password: "p@ss:word;? &",
		Name:     "db name;?&",
	}
}
