package orm

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildQueryStringFindScylla(t *testing.T) {
	query := &ORM{
		StringBuilder: &strings.Builder{},
		IsDistinct:    true,
	}

	query.buildQueryStringFindScylla("id, email", "users")

	if got, want := query.StringBuilder.String(), "SELECT DISTINCT id, email FROM users"; got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestBuildColumnsFindScyllaIncludesPreloadKeys(t *testing.T) {
	selected := &resultBuildSelectedColumnsByTable{
		table: map[string][]string{"users": {"name"}},
	}
	preload := &scyllaPreloadPlan{
		hasMany: []*scyllaHasMany{{
			rel: &hasManyRelation{parentKeyCol: "id"},
		}},
	}

	if got, want := buildColumnsFindScylla(selected, "users", preload), "name, id"; got != want {
		t.Fatalf("selected columns = %q, want %q", got, want)
	}
	if got := buildColumnsFindScylla(&resultBuildSelectedColumnsByTable{isSelect: true}, "users", preload); got != "*" {
		t.Fatalf("wildcard columns = %q, want %q", got, "*")
	}
}

func TestBuildWhereClauseFindScyllaBindsAndValidatesArguments(t *testing.T) {
	query := &ORM{
		StringBuilder: &strings.Builder{},
		WhereClauses:  []string{"users.id = ?", "users.status = ?"},
		WhereArgs:     []any{7, "active"},
	}

	args, limit, err := query.buildWhereClauseFindScylla(false, "users.")
	if err != nil {
		t.Fatalf("buildWhereClauseFindScylla() error = %v", err)
	}
	if got, want := query.StringBuilder.String(), " WHERE id = ? AND status = ?"; got != want {
		t.Fatalf("WHERE SQL = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, []any{7, "active"}) {
		t.Fatalf("arguments = %#v, want %#v", args, []any{7, "active"})
	}
	if limit != 0 {
		t.Fatalf("query with WHERE implicit limit = %d, want 0", limit)
	}

	query = &ORM{StringBuilder: &strings.Builder{}}
	_, limit, err = query.buildWhereClauseFindScylla(false, "")
	if err != nil {
		t.Fatalf("struct buildWhereClauseFindScylla() error = %v", err)
	}
	if limit != 1 {
		t.Fatalf("struct implicit limit = %d, want 1", limit)
	}

	query = &ORM{StringBuilder: &strings.Builder{}}
	_, limit, err = query.buildWhereClauseFindScylla(true, "")
	if err != nil {
		t.Fatalf("slice buildWhereClauseFindScylla() error = %v", err)
	}
	if limit != 0 {
		t.Fatalf("slice implicit limit = %d, want 0", limit)
	}
}

func TestBuildWhereClauseFindScyllaRejectsUnsupportedAndMismatchedClauses(t *testing.T) {
	for _, test := range []struct {
		name        string
		clauses     []string
		args        []any
		wantMessage string
	}{
		{
			name:        "OR clause",
			clauses:     []string{"id = ? OR email = ?"},
			args:        []any{7, "ada@example.test"},
			wantMessage: "OR is not supported by ScyllaDB",
		},
		{
			name:        "missing argument",
			clauses:     []string{"id = ?"},
			wantMessage: "mismatched where clause placeholders and arguments",
		},
		{
			name:        "unused argument",
			clauses:     []string{"id = ?"},
			args:        []any{7, "extra"},
			wantMessage: "unused arguments provided to .Where()",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := &ORM{
				StringBuilder: &strings.Builder{},
				WhereClauses:  test.clauses,
				WhereArgs:     test.args,
			}

			_, _, err := query.buildWhereClauseFindScylla(false, "")
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("error = %v, want message containing %q", err, test.wantMessage)
			}
		})
	}
}
