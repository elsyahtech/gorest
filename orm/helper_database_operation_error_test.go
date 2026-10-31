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
			if err.Error() != "driver detail" {
				t.Fatalf("source error text was changed: %v", err)
			}
			if !strings.HasPrefix(orm.Message, "The database operation could not be completed.") {
				t.Fatalf("unexpected troubleshooting message: %q", orm.Message)
			}
			if !strings.Contains(orm.Message, databaseErrorReference(driverName, databaseExecutionError)) {
				t.Fatalf("troubleshooting message is missing the %s reference: %q", driverName, orm.Message)
			}
		})
	}
}

func TestDatabaseOperationErrorHTTPClassification(t *testing.T) {
	tests := []struct {
		err  error
		name string
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

func TestMissingSchemaColumnUsesInternalServerErrorClassification(t *testing.T) {
	orm := &ORM{DatabaseConfig: &database.Config{Driver: database.SQLITE}}
	err := orm.setDatabaseOperationError("Find", errors.New("no such column: missing"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if orm.HTTPCode != http.StatusInternalServerError {
		t.Fatalf("HTTP status = %d, want %d", orm.HTTPCode, http.StatusInternalServerError)
	}
	if err.Error() != "no such column: missing" {
		t.Fatalf("source error text was changed: %v", err)
	}
	if !strings.Contains(orm.Message, "database operation could not be completed") {
		t.Fatalf("unexpected troubleshooting message: %q", orm.Message)
	}
}

func TestORMOperationWithoutDatabaseReturnsServiceUnavailable(t *testing.T) {
	query := (&ORM{}).Create(&struct{}{})
	if query.Error == nil || query.HTTPCode != http.StatusServiceUnavailable {
		t.Fatalf("Create() without database = HTTP %d, error %v; want HTTP 503 and error", query.HTTPCode, query.Error)
	}
}

func TestSetScyllaFindErrorClassifiesNilInvalidAndDriverErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code []int
		want int
	}{
		{name: "nil error", want: http.StatusInternalServerError},
		{
			name: "invalid Scylla query",
			err:  errors.New("SELECT DISTINCT queries must only request partition key columns"),
			want: http.StatusBadRequest,
		},
		{
			name: "explicit bad request",
			err:  errors.New("invalid input"),
			code: []int{http.StatusBadRequest},
			want: http.StatusBadRequest,
		},
		{name: "driver error", err: errors.New("connection failed"), want: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &ORM{DatabaseConfig: &database.Config{Driver: database.SCYLLA}}
			err := query.setScyllaFindError("Scylla Find failed", test.err, test.code...)
			if query.HTTPCode != test.want {
				t.Fatalf("HTTP status = %d, want %d", query.HTTPCode, test.want)
			}
			if (err != nil) != (test.err != nil) {
				t.Fatalf("setScyllaFindError() error = %v, want underlying nil state %t", err, test.err != nil)
			}
		})
	}
}
