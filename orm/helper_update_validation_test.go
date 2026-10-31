package orm

import (
	"context"
	"database/sql"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestNormalizeScannedSQLPrimaryKey(t *testing.T) {
	type oracleNumber string

	numericInput := reflect.ValueOf(struct {
		ID int64
	}{ID: 42}).Field(0)
	normalized, err := normalizeScannedSQLPrimaryKey(oracleNumber("42"), numericInput)
	if err != nil {
		t.Fatalf("normalize Oracle numeric primary key: %v", err)
	}
	if normalized != int64(42) {
		t.Fatalf("normalized Oracle primary key = %#v (%T), want int64(42)", normalized, normalized)
	}
	if primaryKeyTuple([]any{normalized}) != primaryKeyTuple([]any{int64(42)}) {
		t.Fatal("normalized Oracle primary key does not match the ORM input key")
	}

	stringInput := reflect.ValueOf(struct {
		ID string
	}{ID: "42"}).Field(0)
	normalized, err = normalizeScannedSQLPrimaryKey([]byte("42"), stringInput)
	if err != nil {
		t.Fatalf("normalize string primary key: %v", err)
	}
	if normalized != "42" {
		t.Fatalf("normalized string primary key = %#v (%T), want string", normalized, normalized)
	}

	if _, err := normalizeScannedSQLPrimaryKey(oracleNumber("not-a-number"), numericInput); err == nil {
		t.Fatal("invalid numeric primary key representation was accepted")
	}
}

func TestValidateTaggedUniqueBulkUpdateValues(t *testing.T) {
	type updateRow struct {
		Email string `gorest:"email,unique"`
		ID    int    `gorest:"id,primary_key"`
	}

	meta := columnMetaData{
		columnIndex:     []int{0},
		primaryKeyIndex: []int{1},
	}

	t.Run("rejects duplicate values", func(t *testing.T) {
		rows := []reflect.Value{
			reflect.ValueOf(updateRow{ID: 1, Email: "same@example.com"}),
			reflect.ValueOf(updateRow{ID: 2, Email: "same@example.com"}),
		}

		orm := &ORM{DatabaseConfig: &database.Config{Driver: database.MYSQL}}
		err := orm.validateTaggedUniqueBulkUpdateValues(rows, meta)
		if err == nil {
			t.Fatal("duplicate unique values were accepted")
		}
		if !strings.Contains(err.Error(), "email=same@example.com") {
			t.Fatalf("error = %q, want duplicate email detail", err)
		}
	})

	t.Run("allows distinct values", func(t *testing.T) {
		rows := []reflect.Value{
			reflect.ValueOf(updateRow{ID: 1, Email: "one@example.com"}),
			reflect.ValueOf(updateRow{ID: 2, Email: "two@example.com"}),
		}

		orm := &ORM{DatabaseConfig: &database.Config{Driver: database.MYSQL}}
		if err := orm.validateTaggedUniqueBulkUpdateValues(rows, meta); err != nil {
			t.Fatalf("validateTaggedUniqueBulkUpdateValues() error = %v", err)
		}
	})
}

func TestBulkUpdateSQLKeyValidationAndScanning(t *testing.T) {
	connection, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	defer connection.Close()
	connection.SetMaxOpenConns(1)
	if _, err := connection.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY); INSERT INTO users VALUES (1), (2)"); err != nil {
		t.Fatalf("create test rows: %v", err)
	}

	type record struct {
		ID int `gorest:"id, primary_key"`
	}
	rows := []reflect.Value{reflect.ValueOf(record{ID: 1}), reflect.ValueOf(record{ID: 2})}
	meta := columnMetaData{
		columns:           []string{"id"},
		primaryKeyColumns: []string{"id"},
		primaryKeyIndex:   []int{0},
	}

	query, args := buildSQLBulkUpdateKeyQuery("users", rows, meta, database.POSTGRES)
	if query != "SELECT id FROM users WHERE (id = $1) OR (id = $2)" {
		t.Fatalf("key query = %q", query)
	}
	if !reflect.DeepEqual(args, []any{1, 2}) {
		t.Fatalf("key query args = %#v", args)
	}

	rowsResult, err := connection.Query("SELECT id FROM users")
	if err != nil {
		t.Fatalf("query existing keys: %v", err)
	}
	found, err := scanSQLBulkUpdateKeys(rowsResult, rows, meta)
	if err != nil {
		t.Fatalf("scanSQLBulkUpdateKeys() error = %v", err)
	}
	if err := rowsResult.Close(); err != nil {
		t.Fatalf("close key rows: %v", err)
	}
	if missing := missingSQLBulkUpdatePrimaryKeys(rows, meta, found); len(missing) != 0 {
		t.Fatalf("missing keys = %v, want none", missing)
	}

	missing := missingSQLBulkUpdatePrimaryKeys(
		[]reflect.Value{reflect.ValueOf(record{ID: 3}), reflect.ValueOf(record{ID: 3})},
		meta,
		found,
	)
	if !reflect.DeepEqual(missing, []string{"3"}) {
		t.Fatalf("deduplicated missing keys = %v, want [3]", missing)
	}

	orm := &ORM{
		Database:       &database.Database{SQL: connection},
		DatabaseConfig: &database.Config{Driver: database.SQLITE},
	}
	if err := orm.validateBulkUpdateRowsSQL(context.Background(), "users", rows, meta, database.SQLITE); err != nil {
		t.Fatalf("validate existing bulk update keys: %v", err)
	}

	missingRows := []reflect.Value{reflect.ValueOf(record{ID: 1}), reflect.ValueOf(record{ID: 3})}
	if err := orm.validateBulkUpdateRowsSQL(context.Background(), "users", missingRows, meta, database.SQLITE); err == nil ||
		orm.HTTPCode != http.StatusNotFound {
		t.Fatalf("missing bulk update key result = HTTP %d, error %v; want HTTP 404", orm.HTTPCode, err)
	}

	duplicateRows := []reflect.Value{reflect.ValueOf(record{ID: 1}), reflect.ValueOf(record{ID: 1})}
	if err := orm.validateBulkUpdateRowsSQL(context.Background(), "users", duplicateRows, meta, database.SQLITE); err == nil ||
		orm.HTTPCode != http.StatusBadRequest {
		t.Fatalf("duplicate bulk update key result = HTTP %d, error %v; want HTTP 400", orm.HTTPCode, err)
	}
}

func TestMongoBulkUpdateValidationBuilders(t *testing.T) {
	type record struct {
		TenantID int `gorest:"tenant_id, primary_key"`
		ID       int `gorest:"id, primary_key"`
	}
	rows := []reflect.Value{
		reflect.ValueOf(record{TenantID: 4, ID: 11}),
		reflect.ValueOf(record{TenantID: 5, ID: 12}),
	}
	meta := columnMetaData{
		primaryKeyColumns: []string{"tenant_id", "id"},
		primaryKeyIndex:   []int{0, 1},
	}

	filters := buildMongoBulkUpdateFilters(rows, meta)
	if !reflect.DeepEqual(filters, []bson.M{
		{"tenant_id": 4, "id": 11},
		{"tenant_id": 5, "id": 12},
	}) {
		t.Fatalf("Mongo bulk update filters = %#v", filters)
	}
	if projection := buildMongoBulkUpdateProjection(meta); !reflect.DeepEqual(projection, bson.M{"tenant_id": 1, "id": 1}) {
		t.Fatalf("Mongo bulk update projection = %#v", projection)
	}

	orm := &ORM{}
	if err := orm.validateBulkUpdateRowsMongo(context.Background(), nil, []reflect.Value{rows[0], rows[0]}, meta); err == nil ||
		orm.HTTPCode != http.StatusBadRequest {
		t.Fatalf("duplicate Mongo bulk update keys = HTTP %d, error %v; want HTTP 400", orm.HTTPCode, err)
	}
	if missing := missingBulkUpdatePrimaryKeys(rows, meta, map[string]bool{}); !reflect.DeepEqual(missing, []string{"4/11", "5/12"}) {
		t.Fatalf("missing Mongo bulk update keys = %v", missing)
	}
}
