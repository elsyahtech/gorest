package orm

import (
	"net/http"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestValidatePlaceholderArgumentsUsesDriverAwareCount(t *testing.T) {
	tests := []struct {
		name    string
		driver  string
		clause  string
		args    []any
		wantErr bool
	}{
		{
			name:   "mysql ignores quoted and commented question marks",
			driver: database.MYSQL,
			clause: "`column?` = ? AND note = 'a\\'?' # ?\n",
			args:   []any{"value"},
		},
		{
			name:   "postgres ignores dollar-quoted question marks",
			driver: database.POSTGRES,
			clause: "body = $tag$? :1 @p2$tag$ AND id = ?",
			args:   []any{1},
		},
		{
			name:   "sql server ignores bracket-quoted question marks",
			driver: database.SQLSERVER,
			clause: "[column?] = ? -- ?",
			args:   []any{"value"},
		},
		{
			name:   "oracle ignores alternative-quoted question marks",
			driver: database.ORACLE,
			clause: "value = q'[? :1]' AND id = ?",
			args:   []any{1},
		},
		{
			name:    "missing argument",
			driver:  database.SQLITE,
			clause:  "id = ?",
			wantErr: true,
		},
		{
			name:    "surplus argument",
			driver:  database.SQLITE,
			clause:  "id = ?",
			args:    []any{1, "extra"},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &ORM{}
			err := query.validatePlaceholderArguments("Find", "WHERE clause", []string{test.clause}, test.args, test.driver)
			if (err != nil) != test.wantErr {
				t.Fatalf("validatePlaceholderArguments() error = %v, wantErr %t", err, test.wantErr)
			}
			if test.wantErr && query.HTTPCode != http.StatusBadRequest {
				t.Fatalf("HTTP code = %d, want %d", query.HTTPCode, http.StatusBadRequest)
			}
			if !test.wantErr && query.HTTPCode != 0 {
				t.Fatalf("HTTP code = %d, want no error status", query.HTTPCode)
			}
		})
	}
}
