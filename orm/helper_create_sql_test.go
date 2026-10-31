package orm

import (
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestBuildUpsertConditionUsesDriverPlaceholdersAndArguments(t *testing.T) {
	tests := []struct {
		driver string
		start  int
		want   string
	}{
		{driver: database.POSTGRES, start: 4, want: "(target.status = $4) AND (target.age > $5)"},
		{driver: database.SQLSERVER, start: 4, want: "(target.status = @p4) AND (target.age > @p5)"},
		{driver: database.ORACLE, start: 4, want: "(target.status = :4) AND (target.age > :5)"},
		{driver: database.MYSQL, start: 4, want: "(status = ?) AND (age > ?)"},
	}

	for _, test := range tests {
		t.Run(test.driver, func(t *testing.T) {
			query := &ORM{
				StringBuilder: &strings.Builder{},
				UpsertClauses: []string{"status = ?", "age > ?"},
				UpsertArgs:    []any{"active", 18},
			}
			got, args := query.buildUpsertCondition("target", test.driver, test.start)
			if got != test.want {
				t.Fatalf("condition = %q, want %q", got, test.want)
			}
			if len(args) != 2 || args[0] != "active" || args[1] != 18 {
				t.Fatalf("condition args = %#v", args)
			}
		})
	}
}

func TestBuildSQLServerMergeUpsertStatement(t *testing.T) {
	query := &ORM{
		StringBuilder:      &strings.Builder{},
		UpsertConflictCols: []string{"email"},
	}
	statement, args := query.buildMergeSQL(
		"users",
		[]string{"email", "name", "id"},
		[]string{"id"},
		[]string{"(@p1, @p2, @p3)"},
		database.SQLSERVER,
		4,
	)
	for _, fragment := range []string{
		"MERGE INTO users WITH (HOLDLOCK) AS target",
		"ON (target.email = source.email OR (target.email IS NULL AND source.email IS NULL))",
		"WHEN MATCHED THEN UPDATE SET target.name = source.name",
		"WHEN NOT MATCHED THEN INSERT (email, name, id) VALUES (source.email, source.name, source.id);",
	} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("MERGE statement is missing %q:\n%s", fragment, statement)
		}
	}
	if len(args) != 0 {
		t.Fatalf("MERGE args = %#v, want none", args)
	}
}

func TestBuildOracleMergeUpsertStatementWithConditionalUpdate(t *testing.T) {
	query := &ORM{
		StringBuilder:      &strings.Builder{},
		UpsertConflictCols: []string{"email"},
		UpsertClauses:      []string{"status = ?"},
		UpsertArgs:         []any{"active"},
	}
	statement, args := query.buildOracleMergeSQL(
		"users",
		[]string{"email", "name", "id"},
		[]string{"id"},
		2,
		database.ORACLE,
		7,
	)
	for _, fragment := range []string{
		"MERGE INTO users tgt USING (SELECT :1 AS email, :2 AS name, :3 AS id FROM dual UNION ALL SELECT :4, :5, :6 FROM dual) src",
		"ON ((tgt.email = src.email OR (tgt.email IS NULL AND src.email IS NULL)))",
		"WHEN MATCHED THEN UPDATE SET tgt.name = src.name WHERE (tgt.status = :7)",
		"WHEN NOT MATCHED THEN INSERT (email, name, id) VALUES (src.email, src.name, src.id)",
	} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("Oracle MERGE statement is missing %q:\n%s", fragment, statement)
		}
	}
	if len(args) != 1 || args[0] != "active" {
		t.Fatalf("Oracle MERGE args = %#v, want [active]", args)
	}

	if got := upsertMergeOn([]string{"tenant_id", "id"}, "target", "source"); !strings.Contains(got, "target.tenant_id") ||
		!strings.Contains(got, "source.id") || !strings.Contains(got, " AND ") {
		t.Fatalf("composite MERGE ON condition = %q", got)
	}
}
