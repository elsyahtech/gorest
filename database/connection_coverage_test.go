package database

import (
	"context"
	"net/http"
	"os"
	"testing"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
)

func TestNewRejectsInvalidStartupContext(t *testing.T) {
	if _, message, err := New(nil, &Config{Driver: SQLITE}, "UTC"); err == nil || message == "" {
		t.Fatalf("New(nil context) = (%q, %v), want fail-fast guidance", message, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, message, err := New(ctx, &Config{Driver: SQLITE}, "UTC"); err == nil || message == "" {
		t.Fatalf("New(canceled context) = (%q, %v), want fail-fast guidance", message, err)
	}
}

func TestConnectionHelpersRejectNilConfiguration(t *testing.T) {
	tests := []struct {
		name string
		call func() (string, error)
	}{
		{name: "mysql", call: func() (string, error) {
			_, message, err := mysqlConnection(context.Background(), nil, "", MYSQL)
			return message, err
		}},
		{name: "sql server", call: func() (string, error) {
			_, message, err := mssqlConnection(context.Background(), nil, "", SQLSERVER)
			return message, err
		}},
		{name: "postgres", call: func() (string, error) {
			_, message, err := postgresqlConnection(context.Background(), nil, "", POSTGRES)
			return message, err
		}},
		{name: "sqlite", call: func() (string, error) {
			_, message, err := sqliteConnection(context.Background(), nil, "", SQLITE)
			return message, err
		}},
		{name: "oracle", call: func() (string, error) {
			_, message, err := oracleConnection(context.Background(), nil, "", ORACLE)
			return message, err
		}},
		{name: "mongo", call: func() (string, error) {
			_, message, err := mongoConnection(context.Background(), nil, MONGO)
			return message, err
		}},
		{name: "scylla", call: func() (string, error) { _, message, err := scyllaConnection(nil, SCYLLA); return message, err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, err := test.call()
			if err == nil || message == "" {
				t.Fatalf("connection error = (%q, %v), want a message and error", message, err)
			}
		})
	}
}

func TestScyllaTLSRejectsIncompleteConfiguration(t *testing.T) {
	for _, config := range []*Config{
		{Host: "127.0.0.1", Port: "9042", TLS: TLSEnabled, SSLCertPath: "client.pem"},
		{Host: "127.0.0.1", Port: "9042", TLS: TLSEnabled, SSLKeyPath: "client-key.pem"},
	} {
		if _, message, err := scyllaConnection(config, SCYLLA); err == nil || message == "" {
			t.Fatalf("incomplete Scylla mTLS config = (%q, %v), want validation error", message, err)
		}
	}
}

func TestMongoTLSRejectsIncompleteClientCertificate(t *testing.T) {
	for _, config := range []*Config{
		{Host: "127.0.0.1", Port: "27017", TLS: "true", SSLCertPath: "client.crt"},
		{Host: "127.0.0.1", Port: "27017", TLS: "true", SSLKeyPath: "client.key"},
	} {
		if _, message, err := mongoConnection(context.Background(), config, MONGO); err == nil || message == "" {
			t.Fatalf("incomplete MongoDB mTLS config = (%q, %v), want validation error", message, err)
		}
	}
}

func TestMongoTLSRequiresTLSForClientCertificate(t *testing.T) {
	_, message, err := mongoConnection(context.Background(), &Config{
		Host:        "127.0.0.1",
		Port:        "27017",
		SSLCertPath: "client.crt",
		SSLKeyPath:  "client.key",
	}, MONGO)
	if err == nil || message == "" {
		t.Fatalf("MongoDB client certificate without TLS = (%q, %v), want validation error", message, err)
	}
}

func TestScyllaTLSRejectsUnsupportedMode(t *testing.T) {
	_, message, err := scyllaConnection(&Config{
		Host: "127.0.0.1",
		Port: "9042",
		TLS:  TLSStrict,
	}, SCYLLA)
	if err == nil || message == "" {
		t.Fatalf("Scylla TLS strict mode = (%q, %v), want an unsupported-mode error", message, err)
	}
}

func TestScyllaTLSRejectsInvalidCAFile(t *testing.T) {
	caFile := t.TempDir() + "/invalid-ca.pem"
	if err := os.WriteFile(caFile, []byte("not a PEM certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, message, err := scyllaConnection(&Config{Host: "127.0.0.1", Port: "9042", TLS: TLSEnabled, SSLCAPath: caFile}, SCYLLA); err == nil || message == "" {
		t.Fatalf("invalid Scylla CA config = (%q, %v), want validation error", message, err)
	}
}

func TestMapScanCASRejectsUnavailableScylla(t *testing.T) {
	_, message, code, err := (*Database)(nil).MapScanCAS(context.Background(), &Config{Driver: SCYLLA}, "UPDATE users SET name = ?")
	if err == nil || message == "" || code != http.StatusServiceUnavailable {
		t.Fatalf("nil database MapScanCAS() = (%q, %d, %v)", message, code, err)
	}

	db := &Database{}
	_, message, code, err = db.MapScanCAS(context.Background(), &Config{Driver: SCYLLA}, "UPDATE users SET name = ?")
	if err == nil || message == "" || code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable Scylla MapScanCAS() = (%q, %d, %v)", message, code, err)
	}

	_, message, code, err = db.MapScanCAS(context.Background(), nil, "")
	if err == nil || message == "" || code != http.StatusServiceUnavailable {
		t.Fatalf("nil config MapScanCAS() = (%q, %d, %v)", message, code, err)
	}

	db.Scylla = &gocql.Session{}
	_, message, code, err = db.MapScanCAS(nil, &Config{Driver: SCYLLA}, "UPDATE users SET name = ?")
	if err == nil || message == "" || code != http.StatusBadRequest {
		t.Fatalf("nil context MapScanCAS() = (%q, %d, %v)", message, code, err)
	}
}

func TestCloseRejectsMissingServiceOrContext(t *testing.T) {
	_, _, err := (*Database)(nil).Close(context.Background(), &Config{Driver: SQLITE})
	if err == nil {
		t.Fatalf("nil Database.Close() error = %v", err)
	}

	_, _, err = (&Database{}).Close(context.Background(), nil)
	if err == nil {
		t.Fatal("Close() accepted a nil configuration")
	}

	_, _, err = (&Database{}).Close(nil, &Config{Driver: SQLITE})
	if err == nil {
		t.Fatal("Close() accepted a nil context")
	}

	message, code, err := (&Database{}).Close(context.Background(), &Config{Driver: SQLITE})
	if err == nil || message == "" || code != http.StatusInternalServerError {
		t.Fatalf("Close() without SQL service = (%q, %d, %v)", message, code, err)
	}
}
