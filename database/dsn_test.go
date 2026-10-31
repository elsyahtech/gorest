package database

import (
	"net/url"
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
	cfg.SSLCAPath = "/tmp/ca cert.pem"
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
