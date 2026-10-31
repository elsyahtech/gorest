package orm

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestMySQLReturnValidationAndStatementBuilders(t *testing.T) {
	query := &ORM{DatabaseConfig: &database.Config{Driver: database.MYSQL}}
	meta := columnMetaData{primaryKeyColumns: []string{"id", "tenant_id"}, primaryKeyIndex: []int{0, 1}}
	if err := query.validateMySQLBulkReturnKeys(meta); err != nil {
		t.Fatalf("validateMySQLBulkReturnKeys() error = %v", err)
	}
	if err := query.validateMySQLBulkReturnKeys(columnMetaData{primaryKeyColumns: []string{"id"}}); err == nil {
		t.Fatal("validateMySQLBulkReturnKeys() accepted an incomplete key mapping")
	}

	field := &resultBuildUpdatePayload{values: []any{"updated"}}
	where := &resBuildSingleWhereClause{
		parts: []string{"id = ?", "tenant_id = ?"},
		args:  []any{"updated", 7, 9},
	}
	if err := query.validateMySQLSingleReturnInput(field, where, meta); err != nil {
		t.Fatalf("validateMySQLSingleReturnInput() error = %v", err)
	}
	if err := query.validateMySQLSingleReturnInput(field, where, columnMetaData{}); err == nil {
		t.Fatal("validateMySQLSingleReturnInput() accepted missing primary keys")
	}
	if err := query.validateMySQLSingleReturnInput(field, &resBuildSingleWhereClause{}, meta); err == nil {
		t.Fatal("validateMySQLSingleReturnInput() accepted missing filter arguments")
	}

	whereSQL, whereArgs := query.mysqlSingleReturnWhere(field, where)
	if whereSQL != "id = ? AND tenant_id = ?" || !reflect.DeepEqual(whereArgs, []any{7, 9}) {
		t.Fatalf("mysqlSingleReturnWhere() = (%q, %#v), want filter keys and arguments", whereSQL, whereArgs)
	}

	statement, args := buildMySQLReturnSelect(
		"users",
		[]string{"id", "name"},
		[]string{"id", "tenant_id"},
		[][]any{{1, 10}, {2, 20}},
	)
	if want := "SELECT id, name FROM users WHERE (id = ? AND tenant_id = ?) OR (id = ? AND tenant_id = ?)"; statement != want {
		t.Fatalf("return SELECT = %q, want %q", statement, want)
	}
	if !reflect.DeepEqual(args, []any{1, 10, 2, 20}) {
		t.Fatalf("return SELECT args = %#v, want %#v", args, []any{1, 10, 2, 20})
	}
}

func TestMySQLReturnChunkSizingAndArgumentSanitization(t *testing.T) {
	for _, test := range []struct {
		keyWidth int
		want     int
	}{
		{keyWidth: 0, want: 1000},
		{keyWidth: 1, want: 1000},
		{keyWidth: 100, want: 600},
		{keyWidth: 60001, want: 1},
	} {
		if got := mysqlReturnSelectChunkSize(test.keyWidth); got != test.want {
			t.Errorf("mysqlReturnSelectChunkSize(%d) = %d, want %d", test.keyWidth, got, test.want)
		}
	}

	oracle := &ORM{DatabaseConfig: &database.Config{Driver: database.ORACLE}}
	enabled := true
	args := []any{true, &enabled, (*bool)(nil), "text"}
	got := sanitizeMySQLReturnArgs(oracle, args)
	if !reflect.DeepEqual(got, []any{1, 1, nil, "text"}) {
		t.Fatalf("sanitized MySQL return args = %#v", got)
	}
	if args[0] != true {
		t.Fatal("sanitizing return args modified the input slice")
	}
}

func TestScanMySQLReturnKeysHandlesStringAndCompositeKeys(t *testing.T) {
	connection, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	defer connection.Close()

	rows, err := connection.Query(
		"SELECT CAST('alpha' AS BLOB), 7 UNION ALL SELECT CAST('beta' AS BLOB), 8",
	)
	if err != nil {
		t.Fatalf("query test keys: %v", err)
	}
	defer rows.Close()

	type record struct {
		ID       string
		TenantID int
	}
	keys, err := scanMySQLReturnKeys(
		rows,
		2,
		columnMetaData{primaryKeyIndex: []int{0, 1}},
		reflect.TypeFor[record](),
	)
	if err != nil {
		t.Fatalf("scanMySQLReturnKeys() error = %v", err)
	}
	if !reflect.DeepEqual(keys, [][]any{{"alpha", int64(7)}, {"beta", int64(8)}}) {
		t.Fatalf("scanned keys = %#v", keys)
	}

	if _, err := scanMySQLReturnKeys(nil, 1, columnMetaData{}, reflect.TypeFor[record]()); err == nil {
		t.Fatal("scanMySQLReturnKeys() accepted nil rows")
	}
}

func TestMySQLReturnTransactionLifecycleAndEmptyBulkChunks(t *testing.T) {
	connection, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	defer connection.Close()

	query := &ORM{Database: &database.Database{SQL: connection}}
	execDB, transaction, ownsTx, err := query.beginMySQLReturnTransaction(context.Background())
	if err != nil {
		t.Fatalf("beginMySQLReturnTransaction() error = %v", err)
	}
	if !ownsTx || transaction == nil || execDB.Tx != transaction {
		t.Fatal("beginMySQLReturnTransaction() did not return an owned transaction-bound database")
	}
	rollbackMySQLReturnTransaction(transaction, new(bool))
	if _, err := transaction.Exec("SELECT 1"); err == nil {
		t.Fatal("rollbackMySQLReturnTransaction() left the transaction open")
	}

	if _, tx, owned, err := query.beginMySQLReturnTransaction(context.Background()); err != nil || tx == nil || !owned {
		t.Fatalf("second beginMySQLReturnTransaction() = (%v, %t, %v)", tx, owned, err)
	} else {
		committed := true
		rollbackMySQLReturnTransaction(tx, &committed)
		if err := tx.Rollback(); err != nil {
			t.Fatalf("rollback second transaction: %v", err)
		}
	}

	existingTx, err := connection.Begin()
	if err != nil {
		t.Fatalf("begin existing transaction: %v", err)
	}
	query.Database.Tx = existingTx
	execDB, tx, owned, err := query.beginMySQLReturnTransaction(context.Background())
	if err != nil || execDB != query.Database || tx != nil || owned {
		t.Fatalf("existing transaction result = (%p, %v, %t, %v)", execDB, tx, owned, err)
	}
	if err := existingTx.Rollback(); err != nil {
		t.Fatalf("rollback existing transaction: %v", err)
	}

	keys, err := query.updateMySQLBulkChunks(
		context.Background(),
		&database.Database{SQL: connection},
		"users",
		nil,
		columnMetaData{primaryKeyIndex: []int{0}},
		[]updatableColumn{{name: "name", idx: 1}},
	)
	if err != nil || len(keys) != 0 {
		t.Fatalf("empty updateMySQLBulkChunks() = (%#v, %v), want no keys and no error", keys, err)
	}

	emptyRows, err := query.selectMySQLBulkReturnRows(
		context.Background(),
		&database.Database{SQL: connection},
		"users",
		nil,
		columnMetaData{primaryKeyColumns: []string{"id"}},
		&returnPlan{},
	)
	if err != nil || len(emptyRows) != 0 {
		t.Fatalf("empty selectMySQLBulkReturnRows() = (%#v, %v), want no rows and no error", emptyRows, err)
	}
}

func TestBuildMySQLBulkUpdateQueryProducesBoundCaseExpressions(t *testing.T) {
	type record struct {
		ID   int
		Name string
	}
	rows := []reflect.Value{
		reflect.ValueOf(record{ID: 1, Name: "Ada"}),
		reflect.ValueOf(record{ID: 2, Name: "Lin"}),
	}
	query := &ORM{StringBuilder: &strings.Builder{}}
	built := query.buildMySQLBulkUpdateQuery(
		rows,
		columnMetaData{
			columns:           []string{"id", "name"},
			columnIndex:       []int{0, 1},
			primaryKeyColumns: []string{"id"},
			primaryKeyIndex:   []int{0},
		},
		[]updatableColumn{{name: "name", idx: 1}},
	)

	if len(built.clauses) != 1 || !strings.Contains(built.clauses[0], "CASE") {
		t.Fatalf("bulk update clauses = %#v, want one CASE clause", built.clauses)
	}
	if !reflect.DeepEqual(built.args, []any{1, "Ada", 2, "Lin"}) {
		t.Fatalf("bulk update args = %#v, want %#v", built.args, []any{1, "Ada", 2, "Lin"})
	}
}
