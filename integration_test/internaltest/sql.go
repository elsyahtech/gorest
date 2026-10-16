//go:build integration

package internaltest

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/orm"
)

func SQLJoinAndPreload(t *testing.T, driver string) {
	t.Helper()
	app := OpenApp(t, driver)
	suffix := uniqueIdentifier(t, "")[:8]
	authorRelation := "AuditAuthor" + suffix
	authorTable := strings.ToLower(authorRelation) + "s"
	bookTable := "audit_books_" + suffix
	postRelation := "AuditPosts" + suffix
	postTable := strings.ToLower(postRelation) + "s"
	userTable := "audit_users_" + suffix

	authorType := reflect.StructOf([]reflect.StructField{
		{Name: "ID", Type: reflect.TypeFor[int64](), Tag: `gorest:"id, primary_key"`},
		{Name: "Name", Type: reflect.TypeFor[string](), Tag: `gorest:"name"`},
	})
	bookType := reflect.StructOf([]reflect.StructField{
		{Name: "ID", Type: reflect.TypeFor[int64](), Tag: `gorest:"id, primary_key"`},
		{Name: authorRelation + "ID", Type: reflect.TypeFor[int64](), Tag: `gorest:"author_id"`},
		{Name: "Title", Type: reflect.TypeFor[string](), Tag: `gorest:"title"`},
		{Name: authorRelation, Type: authorType, Tag: `gorest:"-" references:"id"`},
	})
	postType := reflect.TypeFor[struct {
		ID     int64  `gorest:"id, primary_key"`
		UserID int64  `gorest:"user_id"`
		Title  string `gorest:"title"`
	}]()
	userType := reflect.StructOf([]reflect.StructField{
		{Name: "ID", Type: reflect.TypeFor[int64](), Tag: `gorest:"id, primary_key"`},
		{Name: "Name", Type: reflect.TypeFor[string](), Tag: `gorest:"name"`},
		{Name: postRelation, Type: reflect.SliceOf(postType), Tag: `gorest:"-" references:"UserID"`},
	})

	createSQLTable(t, app, driver, authorTable, "id "+sqlIntegerType(driver)+" PRIMARY KEY, name "+sqlTextType(driver)+" NOT NULL")
	createSQLTable(t, app, driver, bookTable, "id "+sqlIntegerType(driver)+" PRIMARY KEY, author_id "+sqlIntegerType(driver)+" NOT NULL, title "+sqlTextType(driver)+" NOT NULL")
	createSQLTable(t, app, driver, userTable, "id "+sqlIntegerType(driver)+" PRIMARY KEY, name "+sqlTextType(driver)+" NOT NULL")
	createSQLTable(t, app, driver, postTable, "id "+sqlIntegerType(driver)+" PRIMARY KEY, user_id "+sqlIntegerType(driver)+" NOT NULL, title "+sqlTextType(driver)+" NOT NULL")

	createDynamicRecord(t, app, authorTable, authorType, map[string]any{"ID": int64(11), "Name": "Author One"})
	createDynamicRecord(t, app, bookTable, bookType, map[string]any{"ID": int64(21), authorRelation + "ID": int64(11), "Title": "Boundaries"})
	createDynamicRecord(t, app, userTable, userType, map[string]any{"ID": int64(31), "Name": "Reader"})
	createDynamicRecord(t, app, postTable, postType, map[string]any{"ID": int64(41), "UserID": int64(31), "Title": "First"})
	createDynamicRecord(t, app, postTable, postType, map[string]any{"ID": int64(42), "UserID": int64(31), "Title": "Second"})

	book := reflect.New(bookType)
	query := app.Database().Table(bookTable).Preload(authorRelation).Find(book.Interface())
	if query.Error != nil {
		t.Fatalf("Find() with belongs-to preload failed: %v", query.Error)
	}
	if got := book.Elem().FieldByName(authorRelation).FieldByName("Name").String(); got != "Author One" {
		t.Fatalf("preloaded author = %q, want Author One", got)
	}

	type joinedBook struct {
		ID       int64  `gorest:"id, primary_key"`
		AuthorID int64  `gorest:"author_id"`
		Title    string `gorest:"title"`
	}
	var joined []joinedBook
	query = app.Database().Table(bookTable).
		Join("INNER JOIN "+authorTable+" ON "+authorTable+".id = "+bookTable+".author_id").
		Where(authorTable+".name = ?", "Author One").
		Find(&joined)
	if query.Error != nil {
		t.Fatalf("Find() with explicit JOIN failed: %v", query.Error)
	}
	if len(joined) != 1 || joined[0].ID != 21 || joined[0].Title != "Boundaries" {
		t.Fatalf("joined result = %#v, want the matching book", joined)
	}

	user := reflect.New(userType)
	query = app.Database().Table(userTable).Preload(postRelation).Find(user.Interface())
	if query.Error != nil {
		t.Fatalf("Find() with has-many preload failed: %v", query.Error)
	}
	posts := user.Elem().FieldByName(postRelation)
	if posts.Len() != 2 || posts.Index(0).FieldByName("Title").String() != "First" ||
		posts.Index(1).FieldByName("Title").String() != "Second" {
		t.Fatalf("preloaded posts = %v, want ordered First and Second", posts.Interface())
	}
}

func SQLNullableMapping(t *testing.T, driver string) {
	t.Helper()
	app := OpenApp(t, driver)
	table := "audit_nullable_" + uniqueIdentifier(t, "")[:8]
	createSQLTable(t, app, driver, table, "id "+sqlIntegerType(driver)+" PRIMARY KEY, optional_text "+sqlTextType(driver)+" NULL")
	createDynamicRecord(t, app, table, reflect.TypeFor[struct {
		ID           int64          `gorest:"id, primary_key"`
		OptionalText sql.NullString `gorest:"optional_text"`
	}](), map[string]any{"ID": int64(1), "OptionalText": sql.NullString{}})
	createDynamicRecord(t, app, table, reflect.TypeFor[struct {
		ID           int64          `gorest:"id, primary_key"`
		OptionalText sql.NullString `gorest:"optional_text"`
	}](), map[string]any{"ID": int64(2), "OptionalText": sql.NullString{String: "present", Valid: true}})

	type nullableRecord struct {
		ID           int64          `gorest:"id, primary_key"`
		OptionalText sql.NullString `gorest:"optional_text"`
	}
	var records []nullableRecord
	query := app.Database().Table(table).OrderBy("id", "ASC").Find(&records)
	if query.Error != nil {
		t.Fatalf("Find() with nullable results failed: %v", query.Error)
	}
	if len(records) != 2 || records[0].OptionalText.Valid ||
		!records[1].OptionalText.Valid || records[1].OptionalText.String != "present" {
		t.Fatalf("nullable mapping = %#v, want NULL then valid %q", records, "present")
	}
}

func SQLPaginationBoundaries(t *testing.T, driver string) {
	t.Helper()
	app := OpenApp(t, driver)
	table := "audit_page_" + uniqueIdentifier(t, "")[:8]
	createSQLTable(t, app, driver, table, "id "+sqlIntegerType(driver)+" PRIMARY KEY, name "+sqlTextType(driver)+" NOT NULL")
	type paginationRecord struct {
		ID   int64  `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
	createDynamicRecord(t, app, table, reflect.TypeFor[paginationRecord](), map[string]any{"ID": int64(1), "Name": "one"})

	for _, test := range []struct {
		name   string
		limit  int
		offset int
	}{
		{name: "zero limit", limit: 0},
		{name: "maximum limit and offset", limit: maxInt(), offset: maxInt()},
		{name: "maximum offset", offset: maxInt()},
	} {
		t.Run(test.name, func(t *testing.T) {
			var rows []paginationRecord
			query := app.Database().Table(table).OrderBy("id", "ASC").Limit(test.limit).Offset(test.offset).Find(&rows)
			if query.Error != nil {
				t.Fatalf("boundary pagination failed: %v", query.Error)
			}
			if len(rows) != 0 {
				t.Fatalf("boundary pagination returned %#v, want no rows", rows)
			}
		})
	}
	for _, test := range []struct {
		name   string
		limit  int
		offset int
	}{
		{name: "negative limit", limit: -1},
		{name: "negative offset", offset: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var rows []paginationRecord
			query := app.Database().Table(table).Limit(test.limit).Offset(test.offset).Find(&rows)
			if query.Error == nil {
				t.Fatal("negative pagination value unexpectedly succeeded")
			}
		})
	}
}

func SQLBatchFailureAtomicity(t *testing.T, driver string) {
	t.Helper()
	app := OpenApp(t, driver)
	table := "audit_batch_" + uniqueIdentifier(t, "")[:8]
	createSQLTable(t, app, driver, table, "id "+sqlIntegerType(driver)+" PRIMARY KEY, email "+sqlTextType(driver)+" NOT NULL UNIQUE")
	type row struct {
		ID    int64  `gorest:"id, primary_key"`
		Email string `gorest:"email"`
	}
	records := []row{{ID: 1, Email: "duplicate@example.test"}, {ID: 2, Email: "duplicate@example.test"}}
	query := app.Database().Table(table).Create(&records)
	if query.Error == nil {
		t.Fatal("multi-row Create() with duplicate unique values unexpectedly succeeded")
	}
	var remaining []row
	query = app.Database().Table(table).Find(&remaining)
	if query.Error != nil {
		t.Fatalf("query after failed multi-row insert: %v", query.Error)
	}
	if len(remaining) != 0 {
		t.Fatalf("failed multi-row statement persisted rows: %#v", remaining)
	}
}

func SQLContextCancellation(t *testing.T, driver string) {
	t.Helper()
	app := OpenApp(t, driver)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	var err error

	switch driver {
	case database.POSTGRES:
		var rows *sql.Rows
		rows, _, _, err = app.Database().QuerySQL(ctx, "SELECT pg_sleep(5)")
		if rows != nil {
			rows.Close()
		}
	case database.MYSQL, database.MARIADB:
		var rows *sql.Rows
		rows, _, _, err = app.Database().QuerySQL(ctx, "SELECT SLEEP(5)")
		if rows != nil {
			rows.Close()
		}
	case database.SQLSERVER:
		_, _, _, err = app.Database().ExecSQL(ctx, "WAITFOR DELAY '00:00:05'")
	case database.ORACLE:
		_, _, _, err = app.Database().ExecSQL(ctx, "BEGIN DBMS_SESSION.SLEEP(5); END;")
	case database.SQLITE:
		var rows *sql.Rows
		rows, _, _, err = app.Database().QuerySQL(ctx, "WITH RECURSIVE count(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM count WHERE n < 100000000) SELECT sum(n) FROM count")
		if rows != nil {
			rows.Close()
		}
	default:
		t.Fatalf("context cancellation test unsupported for SQL driver %q", driver)
	}
	if err == nil {
		t.Fatal("long-running database operation unexpectedly succeeded after context deadline")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("canceled database operation took %s to return", elapsed)
	}
}

func SQLUpsertBehavior(t *testing.T, driver string) {
	t.Helper()
	app := OpenApp(t, driver)
	table := "audit_upsert_" + uniqueIdentifier(t, "")[:8]
	createSQLTable(t, app, driver, table,
		"id "+sqlIntegerType(driver)+" PRIMARY KEY, email "+sqlTextType(driver)+" NOT NULL UNIQUE, score "+sqlIntegerType(driver)+" NOT NULL, note "+sqlTextType(driver)+" NOT NULL")
	type row struct {
		ID    int64  `gorest:"id, primary_key"`
		Email string `gorest:"email"`
		Score int64  `gorest:"score"`
		Note  string `gorest:"note"`
	}
	load := func(id int64) row {
		t.Helper()
		var found row
		query := app.Database().Table(table).Where("id = ?", id).Find(&found)
		RequireNoORMError(t, query)
		return found
	}

	RequireNoORMError(t, app.Database().Table(table).Create(&row{ID: 1, Email: "unique@example.test", Score: 10, Note: "initial"}))
	RequireNoORMError(t, app.Database().Table(table).Upsert("id").Create(&row{ID: 1, Email: "unique@example.test", Score: 20, Note: "pk"}))
	if got := load(1); got.Score != 20 || got.Note != "pk" {
		t.Fatalf("primary-key Upsert result = %#v", got)
	}

	if driver != database.MYSQL && driver != database.MARIADB {
		RequireNoORMError(t, app.Database().Table(table).Upsert("id = ? AND score < ?", int64(1), int64(40)).
			Create(&row{ID: 1, Email: "unique@example.test", Score: 30, Note: "conditional"}))
		if got := load(1); got.Score != 30 || got.Note != "conditional" {
			t.Fatalf("matching conditional Upsert result = %#v", got)
		}
		RequireNoORMError(t, app.Database().Table(table).Upsert("id = ? AND score < ?", int64(1), int64(10)).
			Create(&row{ID: 1, Email: "unique@example.test", Score: 40, Note: "must-not-update"}))
		if got := load(1); got.Score != 30 || got.Note != "conditional" {
			t.Fatalf("non-matching conditional Upsert changed row: %#v", got)
		}
	} else {
		query := app.Database().Table(table).Upsert("id = ?", int64(1)).
			Create(&row{ID: 1, Email: "unique@example.test", Score: 40, Note: "unsupported"})
		if query.Error == nil {
			t.Fatal("MySQL conditional Upsert unexpectedly succeeded although this driver path is documented unsupported")
		}
	}

	RequireNoORMError(t, app.Database().Table(table).Upsert("email").Create(&row{
		ID: 2, Email: "unique@example.test", Score: 50, Note: "unique",
	}))
	if got := load(1); got.ID != 1 || got.Score != 50 || got.Note != "unique" {
		t.Fatalf("alternate unique-key Upsert result = %#v", got)
	}
}

func SQLInvalidInputs(t *testing.T, driver string) {
	t.Helper()
	app := OpenApp(t, driver)
	table := "audit_invalid_" + uniqueIdentifier(t, "")[:8]
	createSQLTable(t, app, driver, table, "id "+sqlIntegerType(driver)+" PRIMARY KEY, name "+sqlTextType(driver)+" NOT NULL")
	type row struct {
		ID   int64  `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
	createDynamicRecord(t, app, table, reflect.TypeFor[row](), map[string]any{"ID": int64(1), "Name": "safe"})

	for _, test := range []struct {
		name string
		run  func() *orm.ORM
	}{
		{name: "unknown selected column", run: func() *orm.ORM {
			return app.Database().Table(table).Select("not_a_column").Find(&[]row{})
		}},
		{name: "unsafe order expression", run: func() *orm.ORM {
			return app.Database().Table(table).OrderBy("name; DROP TABLE " + table).Find(&[]row{})
		}},
		{name: "unknown order column", run: func() *orm.ORM {
			return app.Database().Table(table).OrderBy("not_a_column").Find(&[]row{})
		}},
		{name: "empty select expression", run: func() *orm.ORM {
			return app.Database().Table(table).Select("").Find(&[]row{})
		}},
		{name: "mismatched placeholder", run: func() *orm.ORM {
			return app.Database().Table(table).Where("id = ?").Find(&[]row{})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if query := test.run(); query.Error == nil {
				t.Fatal("invalid ORM input unexpectedly succeeded")
			}
		})
	}

	var remaining []row
	RequireNoORMError(t, app.Database().Table(table).Find(&remaining))
	if len(remaining) != 1 || remaining[0].Name != "safe" {
		t.Fatalf("invalid ORM requests modified database state: %#v", remaining)
	}
}

func createSQLTable(t *testing.T, app *gorest.App, driver, table, columns string) {
	t.Helper()
	ExecSQL(t, app, fmt.Sprintf("CREATE TABLE %s (%s)", table, columns))
	t.Cleanup(func() {
		drop := "DROP TABLE IF EXISTS " + table
		if driver == database.ORACLE {
			drop = "DROP TABLE " + table
		}
		if _, _, _, err := app.Database().ExecSQL(context.Background(), drop); err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})
}

func createDynamicRecord(t *testing.T, app *gorest.App, table string, model reflect.Type, values map[string]any) {
	t.Helper()
	record := reflect.New(model)
	for field, value := range values {
		record.Elem().FieldByName(field).Set(reflect.ValueOf(value))
	}
	RequireNoORMError(t, app.Database().Table(table).Create(record.Interface()))
}

func sqlIntegerType(driver string) string {
	if driver == database.ORACLE {
		return "NUMBER(19)"
	}
	return "BIGINT"
}

func sqlTextType(driver string) string {
	if driver == database.ORACLE {
		return "VARCHAR2(200)"
	}
	return "VARCHAR(200)"
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
