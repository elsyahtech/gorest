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

func TestPostgresRowLevelSecurityEnforcesTenantPolicy(t *testing.T) {
	app := internaltest.OpenAppWithConfig(t, database.POSTGRES, func(config *database.Config) {
		config.MaxOpenConnections = 1
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, message, code, err := app.Database().QuerySQL(ctx,
		"SELECT current_user, rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user")
	if err != nil {
		t.Fatalf("check PostgreSQL role RLS bypass attributes (HTTP %d, %s): %v", code, message, err)
	}
	if !rows.Next() {
		rows.Close()
		t.Fatalf("current PostgreSQL role was not found: %v", rows.Err())
	}
	var role string
	var bypassesRLS bool
	if err := rows.Scan(&role, &bypassesRLS); err != nil {
		rows.Close()
		t.Fatalf("scan PostgreSQL role RLS bypass attributes: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close PostgreSQL role RLS query: %v", err)
	}
	if bypassesRLS {
		t.Skip("PostgreSQL integration role is superuser or has BYPASSRLS; use a non-bypass role to exercise row-level security")
	}
	t.Logf("PostgreSQL RLS test connected as role %q", role)

	table := "audit_rls_" + integrationSuffix()
	internaltest.ExecSQL(t, app,
		"CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, tenant_id TEXT NOT NULL, value TEXT NOT NULL)")
	t.Cleanup(func() {
		if _, _, _, err := app.Database().ExecSQL(context.Background(), "DROP TABLE IF EXISTS "+table); err != nil {
			t.Errorf("drop integration table %s: %v", table, err)
		}
	})
	internaltest.ExecSQL(t, app,
		"INSERT INTO "+table+" (id, tenant_id, value) VALUES ($1, $2, $3), ($4, $5, $6)",
		int64(1), "tenant-a", "visible", int64(2), "tenant-b", "hidden")
	internaltest.ExecSQL(t, app, "ALTER TABLE "+table+" ENABLE ROW LEVEL SECURITY")
	internaltest.ExecSQL(t, app, "ALTER TABLE "+table+" FORCE ROW LEVEL SECURITY")
	internaltest.ExecSQL(t, app,
		"CREATE POLICY tenant_isolation ON "+table+
			" USING (tenant_id = current_setting('gorest.tenant_id', true))"+
			" WITH CHECK (tenant_id = current_setting('gorest.tenant_id', true))")

	type rlsRecord struct {
		ID       int64  `gorest:"id, primary_key"`
		TenantID string `gorest:"tenant_id"`
		Value    string `gorest:"value"`
	}
	tx, message, code, err := app.Database().BeginTx()
	if err != nil {
		t.Fatalf("begin tenant-a transaction (HTTP %d, %s): %v", code, message, err)
	}
	committed := false
	defer func() {
		if !committed {
			if _, _, err := tx.Rollback(); err != nil {
				t.Errorf("rollback tenant-a RLS transaction: %v", err)
			}
		}
	}()
	backendPID := postgresBackendPID(t, tx, ctx)
	setPostgresTenantContext(t, tx, ctx, "tenant-a")

	var visible []rlsRecord
	query := tx.Table(table).OrderBy("id", "ASC").Find(&visible)
	if query.Error != nil {
		t.Fatalf("query rows under tenant-a RLS policy: %v", query.Error)
	}
	if len(visible) != 1 || visible[0].ID != 1 || visible[0].Value != "visible" {
		t.Fatalf("tenant-a RLS result = %#v, want only tenant-a row", visible)
	}

	internaltest.RequireNoORMError(t, tx.Table(table).Create(&rlsRecord{
		ID: 3, TenantID: "tenant-a", Value: "inserted",
	}))
	if _, _, err := tx.Commit(); err != nil {
		t.Fatalf("commit allowed tenant-a write: %v", err)
	}
	committed = true

	tx, message, code, err = app.Database().BeginTx()
	if err != nil {
		t.Fatalf("begin tenant-b transaction (HTTP %d, %s): %v", code, message, err)
	}
	txCommitted := false
	defer func() {
		if !txCommitted {
			if _, _, err := tx.Rollback(); err != nil {
				t.Errorf("rollback tenant-b RLS transaction: %v", err)
			}
		}
	}()
	if got := postgresBackendPID(t, tx, ctx); got != backendPID {
		t.Fatalf("tenant-b transaction used backend PID %d, want reused pooled connection PID %d", got, backendPID)
	}

	visible = nil
	query = tx.Table(table).OrderBy("id", "ASC").Find(&visible)
	if query.Error != nil {
		t.Fatalf("query rows before setting tenant-b context: %v", query.Error)
	}
	if len(visible) != 0 {
		t.Fatalf("tenant context leaked from committed transaction: %#v", visible)
	}

	setPostgresTenantContext(t, tx, ctx, "tenant-b")

	visible = nil
	query = tx.Table(table).OrderBy("id", "ASC").Find(&visible)
	if query.Error != nil {
		t.Fatalf("query rows under tenant-b RLS policy: %v", query.Error)
	}
	if len(visible) != 1 || visible[0].ID != 2 || visible[0].Value != "hidden" {
		t.Fatalf("tenant-b RLS result = %#v, want only tenant-b row", visible)
	}

	query = tx.Table(table).Create(&rlsRecord{
		ID: 4, TenantID: "tenant-a", Value: "must be rejected",
	})
	if query.Error == nil {
		t.Fatal("RLS policy allowed tenant-b to insert a tenant-a row")
	}
	if _, _, err := tx.Rollback(); err != nil {
		t.Fatalf("rollback rejected cross-tenant RLS write: %v", err)
	}
	txCommitted = true

	rows, message, code, err = app.Database().QuerySQL(ctx, "SELECT pg_backend_pid()")
	if err != nil {
		t.Fatalf("query backend PID after transaction-local context reset (HTTP %d, %s): %v", code, message, err)
	}
	if !rows.Next() {
		rows.Close()
		t.Fatalf("query backend PID after transaction-local context reset returned no row: %v", rows.Err())
	}
	var reusedPID int
	if err := rows.Scan(&reusedPID); err != nil {
		rows.Close()
		t.Fatalf("scan backend PID after transaction-local context reset: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close backend PID query: %v", err)
	}
	if reusedPID != backendPID {
		t.Fatalf("post-transaction query used backend PID %d, want reused pooled connection PID %d", reusedPID, backendPID)
	}

	visible = nil
	query = app.Database().Table(table).OrderBy("id", "ASC").Find(&visible)
	if query.Error != nil {
		t.Fatalf("query rows without transaction-local tenant context: %v", query.Error)
	}
	if len(visible) != 0 {
		t.Fatalf("tenant rows visible after transaction-local context reset: %#v", visible)
	}
}

func setPostgresTenantContext(t *testing.T, tx interface {
	QuerySQL(context.Context, string, ...any) (*sql.Rows, string, int, error)
}, ctx context.Context, tenant string) {
	t.Helper()

	rows, message, code, err := tx.QuerySQL(ctx,
		"SELECT set_config('gorest.tenant_id', $1, true)", tenant)
	if err != nil {
		t.Fatalf("set transaction-local tenant context (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("set_config() returned no row: %v", rows.Err())
	}
	var configuredTenant string
	if err := rows.Scan(&configuredTenant); err != nil {
		t.Fatalf("scan transaction-local tenant context: %v", err)
	}
	if configuredTenant != tenant {
		t.Fatalf("configured tenant = %q, want %q", configuredTenant, tenant)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate transaction-local tenant context result: %v", err)
	}
}

func postgresBackendPID(t *testing.T, tx interface {
	QuerySQL(context.Context, string, ...any) (*sql.Rows, string, int, error)
}, ctx context.Context) int {
	t.Helper()

	rows, message, code, err := tx.QuerySQL(ctx, "SELECT pg_backend_pid()")
	if err != nil {
		t.Fatalf("query PostgreSQL backend PID (HTTP %d, %s): %v", code, message, err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("query PostgreSQL backend PID returned no row: %v", rows.Err())
	}
	var pid int
	if err := rows.Scan(&pid); err != nil {
		t.Fatalf("scan PostgreSQL backend PID: %v", err)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate PostgreSQL backend PID result: %v", err)
	}
	return pid
}

func integrationSuffix() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
