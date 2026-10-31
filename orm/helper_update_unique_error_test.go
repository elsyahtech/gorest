package orm

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestParseUniqueViolationDetailsAcrossDrivers(t *testing.T) {
	meta := columnMetaData{
		columns:           []string{"id", "email"},
		columnIndex:       []int{0, 1},
		primaryKeyColumns: []string{"id"},
		primaryKeyIndex:   []int{0},
	}

	tests := []struct {
		name        string
		driver      string
		message     string
		wantColumns []string
		wantValues  []string
	}{
		{
			name:        "Oracle",
			driver:      database.ORACLE,
			message:     "unique constraint violated for columns (email)",
			wantColumns: []string{"email"},
		},
		{
			name:        "PostgreSQL",
			driver:      database.POSTGRES,
			message:     "Key (email)=(duplicate@example.test) already exists.",
			wantColumns: []string{"email"},
			wantValues:  []string{"duplicate@example.test"},
		},
		{
			name:        "SQLite",
			driver:      database.SQLITE,
			message:     "UNIQUE constraint failed: users.email",
			wantColumns: []string{"email"},
		},
		{
			name:        "MySQL",
			driver:      database.MYSQL,
			message:     "Duplicate entry 'duplicate@example.test' for key 'uniq_email'",
			wantColumns: []string{"email"},
			wantValues:  []string{"duplicate@example.test"},
		},
		{
			name:        "SQL Server",
			driver:      database.SQLSERVER,
			message:     "duplicate key value is (duplicate@example.test) for email",
			wantColumns: []string{"email"},
			wantValues:  []string{"duplicate@example.test"},
		},
		{
			name:        "MongoDB",
			driver:      database.MONGO,
			message:     `E11000 dup key: { email: "duplicate@example.test" }`,
			wantColumns: []string{"email"},
			wantValues:  []string{"duplicate@example.test"},
		},
		{
			name:        "ScyllaDB",
			driver:      database.SCYLLA,
			message:     "duplicate value already exists",
			wantColumns: []string{"id"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			columns, values := parseUniqueViolationDetails(test.message, meta, test.driver)
			if !reflect.DeepEqual(columns, test.wantColumns) {
				t.Fatalf("parsed columns = %#v, want %#v", columns, test.wantColumns)
			}
			if !reflect.DeepEqual(values, test.wantValues) {
				t.Fatalf("parsed values = %#v, want %#v", values, test.wantValues)
			}
		})
	}
}

func TestEnrichBulkUpdateUniqueViolationIncludesRequestRows(t *testing.T) {
	type updateRow struct {
		ID    int
		Email string
	}

	rows := []reflect.Value{
		reflect.ValueOf(updateRow{ID: 1, Email: "duplicate@example.test"}),
		reflect.ValueOf(updateRow{ID: 2, Email: "duplicate@example.test"}),
	}
	meta := columnMetaData{
		columns:           []string{"id", "email"},
		columnIndex:       []int{0, 1},
		primaryKeyColumns: []string{"id"},
		primaryKeyIndex:   []int{0},
	}
	original := errors.New("Duplicate entry 'duplicate@example.test' for key 'uniq_email'")

	orm := &ORM{DatabaseConfig: &database.Config{Driver: database.MYSQL}}
	got := orm.enrichBulkUpdateUniqueViolation(original, rows, meta, database.MYSQL)
	if !errors.Is(got, original) {
		t.Fatalf("enriched error = %v, want original database error", got)
	}
	for _, expected := range []string{"email", "duplicate@example.test", "1", "2"} {
		if !strings.Contains(orm.Message, expected) {
			t.Errorf("error message %q does not include %q", orm.Message, expected)
		}
	}

	orm = &ORM{DatabaseConfig: &database.Config{Driver: database.ORACLE}}
	orm.enrichBulkUpdateUniqueViolation(errors.New("unique constraint violated for columns (email)"), []reflect.Value{
		reflect.ValueOf(updateRow{ID: 3, Email: "first@example.test"}),
		reflect.ValueOf(updateRow{ID: 4, Email: "second@example.test"}),
	}, meta, database.ORACLE)
	for _, expected := range []string{"submitted values", "first@example.test", "second@example.test"} {
		if !strings.Contains(orm.Message, expected) {
			t.Errorf("fallback error message %q does not include %q", orm.Message, expected)
		}
	}
}
