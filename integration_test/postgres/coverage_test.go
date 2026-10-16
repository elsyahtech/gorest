//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestPostgresJoinAndRelationPreloadMapping(t *testing.T) {
	app := internaltest.OpenApp(t, database.POSTGRES)
	suffix := integrationSuffix()
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
	userType := reflect.StructOf([]reflect.StructField{
		{Name: "ID", Type: reflect.TypeFor[int64](), Tag: `gorest:"id, primary_key"`},
		{Name: "Name", Type: reflect.TypeFor[string](), Tag: `gorest:"name"`},
		{Name: postRelation, Type: reflect.SliceOf(reflect.TypeFor[struct {
			ID     int64  `gorest:"id, primary_key"`
			UserID int64  `gorest:"user_id"`
			Title  string `gorest:"title"`
		}]()), Tag: `gorest:"-" references:"UserID"`},
	})

	t.Cleanup(func() {
		for _, table := range []string{postTable, userTable, bookTable, authorTable} {
			if _, _, _, err := app.Database().ExecSQL(context.Background(), "DROP TABLE IF EXISTS "+table); err != nil {
				t.Errorf("drop integration table %s: %v", table, err)
			}
		}
	})
	for _, ddl := range []string{
		fmt.Sprintf("CREATE TABLE %s (id BIGINT PRIMARY KEY, name TEXT NOT NULL)", authorTable),
		fmt.Sprintf("CREATE TABLE %s (id BIGINT PRIMARY KEY, author_id BIGINT NOT NULL, title TEXT NOT NULL)", bookTable),
		fmt.Sprintf("CREATE TABLE %s (id BIGINT PRIMARY KEY, name TEXT NOT NULL)", userTable),
		fmt.Sprintf("CREATE TABLE %s (id BIGINT PRIMARY KEY, user_id BIGINT NOT NULL, title TEXT NOT NULL)", postTable),
	} {
		internaltest.ExecSQL(t, app, ddl)
	}

	internaltest.ExecSQL(t, app, "INSERT INTO "+authorTable+" (id, name) VALUES ($1, $2)", int64(11), "Author One")
	internaltest.ExecSQL(t, app, "INSERT INTO "+bookTable+" (id, author_id, title) VALUES ($1, $2, $3)", int64(21), int64(11), "Boundaries")
	internaltest.ExecSQL(t, app, "INSERT INTO "+userTable+" (id, name) VALUES ($1, $2)", int64(31), "Reader")
	internaltest.ExecSQL(t, app, "INSERT INTO "+postTable+" (id, user_id, title) VALUES ($1, $2, $3), ($4, $5, $6)",
		int64(41), int64(31), "First", int64(42), int64(31), "Second")

	book := reflect.New(bookType)
	query := app.Database().Table(bookTable).Preload(authorRelation).Find(book.Interface())
	if query.Error != nil {
		t.Fatalf("Find() with belongs-to preload failed: %v", query.Error)
	}
	if got := book.Elem().FieldByName(authorRelation).FieldByName("Name").String(); got != "Author One" {
		t.Fatalf("preloaded author name = %q, want %q", got, "Author One")
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
		t.Fatalf("joined rows = %#v, want the matching book", joined)
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

func TestPostgresNullableColumnMapping(t *testing.T) {
	app := internaltest.OpenApp(t, database.POSTGRES)
	table := "audit_nullable_" + integrationSuffix()
	internaltest.ExecSQL(t, app, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, optional_text TEXT NULL)")
	t.Cleanup(func() {
		if _, _, _, err := app.Database().ExecSQL(context.Background(), "DROP TABLE "+table); err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})

	internaltest.ExecSQL(t, app,
		"INSERT INTO "+table+" (id, optional_text) VALUES ($1, NULL), ($2, $3)",
		int64(1), int64(2), "present")

	type nullableRecord struct {
		ID           int64          `gorest:"id, primary_key"`
		OptionalText sql.NullString `gorest:"optional_text"`
	}
	var records []nullableRecord
	query := app.Database().Table(table).OrderBy("id", "ASC").Find(&records)
	if query.Error != nil {
		t.Fatalf("Find() with nullable result values failed: %v", query.Error)
	}
	if len(records) != 2 {
		t.Fatalf("loaded %d nullable records, want 2", len(records))
	}
	if records[0].OptionalText.Valid {
		t.Fatalf("NULL mapped to valid sql.NullString: %#v", records[0].OptionalText)
	}
	if !records[1].OptionalText.Valid || records[1].OptionalText.String != "present" {
		t.Fatalf("non-NULL mapping = %#v, want valid value %q", records[1].OptionalText, "present")
	}
}

func TestPostgresPaginationBoundaries(t *testing.T) {
	app := internaltest.OpenApp(t, database.POSTGRES)
	table := "audit_pagination_" + integrationSuffix()
	internaltest.ExecSQL(t, app, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, name TEXT NOT NULL)")
	t.Cleanup(func() {
		if _, _, _, err := app.Database().ExecSQL(context.Background(), "DROP TABLE "+table); err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})
	internaltest.ExecSQL(t, app, "INSERT INTO "+table+" (id, name) VALUES (1, 'one'), (2, 'two')")

	type paginationRecord struct {
		ID   int64  `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
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
				t.Fatalf("Find() with boundary pagination values failed: %v", query.Error)
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

func TestPostgresBulkInsertFailureIsStatementAtomic(t *testing.T) {
	app := internaltest.OpenApp(t, database.POSTGRES)
	table := "audit_batch_" + integrationSuffix()
	internaltest.ExecSQL(t, app, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, email TEXT NOT NULL UNIQUE)")
	t.Cleanup(func() {
		if _, _, _, err := app.Database().ExecSQL(context.Background(), "DROP TABLE "+table); err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})

	type batchRecord struct {
		ID    int64  `gorest:"id, primary_key"`
		Email string `gorest:"email"`
	}
	records := []batchRecord{
		{ID: 1, Email: "duplicate@example.test"},
		{ID: 2, Email: "duplicate@example.test"},
	}
	query := app.Database().Table(table).Create(&records)
	if query.Error == nil {
		t.Fatal("bulk Create() with a duplicate unique value unexpectedly succeeded")
	}

	var count int64
	rows, message, code, err := app.Database().QuerySQL(
		context.Background(),
		"SELECT COUNT(*) FROM "+table,
	)
	if err != nil {
		t.Fatalf("count rows after failed bulk insert (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("count query returned no row: %v", rows.Err())
	}
	if err := rows.Scan(&count); err != nil {
		t.Fatalf("scan post-failure row count: %v", err)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate post-failure count result: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed multi-row INSERT left %d persisted rows; expected statement atomicity", count)
	}
}

func TestPostgresContextCancellationStopsLiveQuery(t *testing.T) {
	app := internaltest.OpenApp(t, database.POSTGRES)
	table := "audit_cancel_" + integrationSuffix()
	internaltest.ExecSQL(t, app, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, name TEXT NOT NULL)")
	t.Cleanup(func() {
		if _, _, _, err := app.Database().ExecSQL(context.Background(), "DROP TABLE "+table); err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})
	internaltest.ExecSQL(t, app, "INSERT INTO "+table+" (id, name) VALUES (1, 'slow')")

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	type cancellationRecord struct {
		ID   int64  `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
	started := time.Now()
	query := app.Database().Table(table).
		Join("CROSS JOIN LATERAL (SELECT pg_sleep(5)) AS delayed")
	query.Context = ctx
	query = query.Find(&[]cancellationRecord{})
	elapsed := time.Since(started)
	if query.Error == nil {
		t.Fatal("live ORM query unexpectedly succeeded despite context cancellation")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("cancelled query took %s to return, want it to stop promptly", elapsed)
	}
}

func TestPostgresUpsertBehaviorMatrix(t *testing.T) {
	app := internaltest.OpenApp(t, database.POSTGRES)
	table := "audit_upsert_" + integrationSuffix()
	internaltest.ExecSQL(t, app, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, email TEXT NOT NULL UNIQUE, score BIGINT NOT NULL, note TEXT NOT NULL)")
	t.Cleanup(func() {
		if _, _, _, err := app.Database().ExecSQL(context.Background(), "DROP TABLE "+table); err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})

	type upsertRecord struct {
		ID    int64  `gorest:"id, primary_key"`
		Email string `gorest:"email"`
		Score int64  `gorest:"score"`
		Note  string `gorest:"note"`
	}
	load := func(id int64) upsertRecord {
		t.Helper()
		var record upsertRecord
		query := app.Database().Table(table).Where("id = ?", id).Find(&record)
		if query.Error != nil {
			t.Fatalf("load upsert row %d: %v", id, query.Error)
		}
		return record
	}
	requireNoError := func(queryErr error, operation string) {
		t.Helper()
		if queryErr != nil {
			t.Fatalf("%s: %v", operation, queryErr)
		}
	}

	requireNoError(app.Database().Table(table).Create(&upsertRecord{
		ID: 1, Email: "upsert@example.test", Score: 10, Note: "initial",
	}).Error, "insert initial row")

	requireNoError(app.Database().Table(table).Upsert("id").Create(&upsertRecord{
		ID: 1, Email: "upsert@example.test", Score: 20, Note: "primary-key",
	}).Error, "upsert by primary key")
	if got := load(1); got.Score != 20 || got.Note != "primary-key" {
		t.Fatalf("primary-key Upsert() row = %#v", got)
	}

	requireNoError(app.Database().Table(table).Upsert("email").Create(&upsertRecord{
		ID: 2, Email: "upsert@example.test", Score: 30, Note: "unique-key",
	}).Error, "upsert by alternate unique key")
	if got := load(1); got.ID != 1 || got.Score != 30 || got.Note != "unique-key" {
		t.Fatalf("alternate-key Upsert() row = %#v, want existing primary key retained", got)
	}

	conditional := app.Database().Table(table).Upsert("id = ? AND score < ?", int64(1), int64(40))
	conditional = conditional.Create(&upsertRecord{
		ID: 1, Email: "upsert@example.test", Score: 40, Note: "conditional",
	})
	if conditional.Error != nil {
		t.Fatalf("conditional upsert matching predicate: %v; query: %s", conditional.Error, conditional.StringBuilder.String())
	}
	if got := load(1); got.Score != 40 || got.Note != "conditional" {
		t.Fatalf("matching conditional Upsert() row = %#v", got)
	}

	conditional = app.Database().Table(table).Upsert("id = ? AND score < ?", int64(1), int64(10))
	conditional = conditional.Create(&upsertRecord{
		ID: 1, Email: "upsert@example.test", Score: 50, Note: "must-not-update",
	})
	if conditional.Error != nil {
		t.Fatalf("conditional upsert non-matching predicate: %v; query: %s", conditional.Error, conditional.StringBuilder.String())
	}
	if got := load(1); got.Score != 40 || got.Note != "conditional" {
		t.Fatalf("non-matching conditional Upsert() changed row: %#v", got)
	}
}

func integrationSuffix() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
