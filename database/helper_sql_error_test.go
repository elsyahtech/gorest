package database

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

func TestIsDuplicateKeyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "unique violation",
			err:  &pq.Error{Code: "23505"},
			want: true,
		},
		{
			name: "wrapped unique violation",
			err:  fmt.Errorf("insert record: %w", &pq.Error{Code: "23505"}),
			want: true,
		},
		{
			name: "different postgres error",
			err:  &pq.Error{Code: "23503"},
		},
		{
			name: "unrelated error",
			err:  errors.New("database unavailable"),
		},
		{
			name: "nil error",
		},
		{
			name: "mysql duplicate key",
			err:  &mysql.MySQLError{Number: 1062},
			want: true,
		},
		{
			name: "wrapped mysql duplicate key",
			err:  fmt.Errorf("insert record: %w", &mysql.MySQLError{Number: 1062}),
			want: true,
		},
		{
			name: "different mysql error",
			err:  &mysql.MySQLError{Number: 1064},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsDuplicateKeyError(test.err); got != test.want {
				t.Fatalf("IsDuplicateKeyError(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}
