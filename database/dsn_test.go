package database

import (
	"net/url"
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

func TestOracleConnectionParamsRejectInvalidTimezone(t *testing.T) {
	if _, err := oracleConnectionParams(testDSNConfig(), "Invalid/Timezone"); err == nil {
		t.Fatal("oracleConnectionParams() unexpectedly accepted an invalid timezone")
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
