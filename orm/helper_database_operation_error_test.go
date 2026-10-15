package orm

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
	mysql "github.com/go-sql-driver/mysql"
)

func TestDatabaseOperationErrorsHaveConsistentDriverContract(t *testing.T) {
	drivers := []string{
		database.ORACLE,
		database.SQLSERVER,
		database.MYSQL,
		database.POSTGRES,
		database.SQLITE,
		database.MONGO,
		database.SCYLLA,
	}

	for _, driverName := range drivers {
		t.Run(driverName, func(t *testing.T) {
			orm := &ORM{DatabaseConfig: &database.Config{Driver: driverName}}
			err := orm.setDatabaseOperationError("Create", errors.New("driver detail"))
			if err == nil {
				t.Fatal("expected an error")
			}

			var coded interface{ HTTPStatusCode() int }
			if !errors.As(err, &coded) || coded.HTTPStatusCode() != http.StatusInternalServerError {
				t.Fatalf("HTTP status = %v, want %d", orm.HTTPCode, http.StatusInternalServerError)
			}
			if !strings.Contains(err.Error(), "database: execution_failed: driver detail") {
				t.Fatalf("error name is not standardized: %v", err)
			}
			if !strings.HasPrefix(orm.Message, "create failed: ") {
				t.Fatalf("unexpected troubleshooting message: %q", orm.Message)
			}
			if !strings.Contains(orm.Message, databaseErrorReference(driverName)) {
				t.Fatalf("troubleshooting message is missing the %s reference: %q", driverName, orm.Message)
			}
		})
	}
}

func TestDatabaseOperationErrorHTTPClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "unique constraint",
			err:  &mysql.MySQLError{Number: 1062, Message: "duplicate key"},
			want: http.StatusConflict,
		},
		{
			name: "timeout",
			err:  context.DeadlineExceeded,
			want: http.StatusGatewayTimeout,
		},
		{
			name: "network unavailable",
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			want: http.StatusServiceUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			orm := &ORM{DatabaseConfig: &database.Config{Driver: database.POSTGRES}}
			err := orm.setDatabaseOperationError("Update", test.err)
			if err == nil {
				t.Fatal("expected an error")
			}
			if orm.HTTPCode != test.want {
				t.Fatalf("HTTP status = %d, want %d", orm.HTTPCode, test.want)
			}
		})
	}
}

func TestInvalidDatabaseQueryUsesBadRequestClassification(t *testing.T) {
	orm := &ORM{DatabaseConfig: &database.Config{Driver: database.SQLITE}}
	err := orm.setDatabaseOperationError("Find", errors.New("no such column: missing"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if orm.HTTPCode != http.StatusBadRequest {
		t.Fatalf("HTTP status = %d, want %d", orm.HTTPCode, http.StatusBadRequest)
	}
	if !strings.Contains(err.Error(), "database: invalid_query:") {
		t.Fatalf("error name is not standardized: %v", err)
	}
	if !strings.Contains(orm.Message, "database rejected the query") {
		t.Fatalf("unexpected troubleshooting message: %q", orm.Message)
	}
}
