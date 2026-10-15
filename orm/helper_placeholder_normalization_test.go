package orm

import (
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestNormalizeWherePlaceholdersSkipsQuotedTextAndComments(t *testing.T) {
	clause := "name = '?' AND code = \"?\" AND value = ? -- ? :2\n/* ? /* @p3 */ $4 */ AND id = $12 AND n = :4"
	got, next := normalizeWherePlaceholders(clause, database.POSTGRES, 1)
	want := "name = '?' AND code = \"?\" AND value = $1 -- ? :2\n/* ? /* @p3 */ $4 */ AND id = $2 AND n = $3"
	if got != want {
		t.Fatalf("normalized clause:\n got: %s\nwant: %s", got, want)
	}
	if next != 4 {
		t.Fatalf("next placeholder = %d, want 4", next)
	}
	if count := countActivePlaceholders(clause, database.POSTGRES); count != 3 {
		t.Fatalf("active placeholder count = %d, want 3", count)
	}
}

func TestNormalizeWherePlaceholdersHandlesMySQLHashCommentsAndEscapes(t *testing.T) {
	clause := `label = 'a\'?' AND id = ? # ? :2
AND other_id = ?`
	got, next := normalizeWherePlaceholders(clause, database.MYSQL, 1)
	if got != clause {
		t.Fatalf("normalized clause changed quoted text or comments:\n got: %s\nwant: %s", got, clause)
	}
	if next != 3 {
		t.Fatalf("next placeholder = %d, want 3", next)
	}
}

func TestNormalizeWherePlaceholdersHandlesDialectCommentRules(t *testing.T) {
	mysqlClause := `a--b = ? AND c = ?`
	_, next := normalizeWherePlaceholders(mysqlClause, database.MYSQL, 1)
	if next != 3 {
		t.Fatalf("MySQL placeholder count after non-comment -- = %d, want 2", next-1)
	}

	sqlServerClause := `a--b = ?`
	_, next = normalizeWherePlaceholders(sqlServerClause, database.SQLSERVER, 1)
	if next != 1 {
		t.Fatalf("SQL Server placeholder count after -- comment = %d, want 0", next-1)
	}
}

func TestNormalizeWherePlaceholdersHandlesDialectQuotedIdentifiers(t *testing.T) {
	mysqlClause := "`column?` = ?"
	got, next := normalizeWherePlaceholders(mysqlClause, database.MYSQL, 1)
	if got != "`column?` = ?" || next != 2 {
		t.Fatalf("MySQL backtick identifier changed: got %q, next %d", got, next)
	}

	sqlServerClause := "[column?] = ?"
	got, next = normalizeWherePlaceholders(sqlServerClause, database.SQLSERVER, 1)
	if got != "[column?] = @p1" || next != 2 {
		t.Fatalf("SQL Server bracket identifier changed: got %q, next %d", got, next)
	}
}

func TestNormalizeWherePlaceholdersHandlesPostgresEscapeStrings(t *testing.T) {
	clause := `value = E'a\'?' AND id = ?`
	got, next := normalizeWherePlaceholders(clause, database.POSTGRES, 1)
	want := `value = E'a\'?' AND id = $1`
	if got != want {
		t.Fatalf("normalized clause:\n got: %s\nwant: %s", got, want)
	}
	if next != 2 {
		t.Fatalf("next placeholder = %d, want 2", next)
	}
}

func TestNormalizeWherePlaceholdersHandlesPostgresDollarQuotes(t *testing.T) {
	clause := `body = $tag$? :1 @p2 $tag$ AND id = ? AND value = $$? :3$$`
	got, next := normalizeWherePlaceholders(clause, database.POSTGRES, 4)
	want := `body = $tag$? :1 @p2 $tag$ AND id = $4 AND value = $$? :3$$`
	if got != want {
		t.Fatalf("normalized clause:\n got: %s\nwant: %s", got, want)
	}
	if next != 5 {
		t.Fatalf("next placeholder = %d, want 5", next)
	}
}

func TestNormalizeWherePlaceholdersHandlesOracleAlternativeQuotes(t *testing.T) {
	clause := `value = q'[? :1]' AND id = ?`
	got, next := normalizeWherePlaceholders(clause, database.ORACLE, 1)
	want := `value = q'[? :1]' AND id = :1`
	if got != want {
		t.Fatalf("normalized clause:\n got: %s\nwant: %s", got, want)
	}
	if next != 2 {
		t.Fatalf("next placeholder = %d, want 2", next)
	}
}

func TestNormalizeWherePlaceholdersPreservesEscapedJSONOperators(t *testing.T) {
	clause := `payload ??| ? AND payload ??& ? AND payload ??`
	got, next := normalizeWherePlaceholders(clause, database.POSTGRES, 1)
	want := `payload ?| $1 AND payload ?& $2 AND payload ?`
	if got != want {
		t.Fatalf("normalized clause:\n got: %s\nwant: %s", got, want)
	}
	if next != 3 {
		t.Fatalf("next placeholder = %d, want 3", next)
	}
}
