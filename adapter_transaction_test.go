package gorest

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/elsyahtech/gorest/database"
	gorestfilter "github.com/elsyahtech/gorest/filter"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func newSQLiteDBSession(t *testing.T, timeout time.Duration) *DBSession {
	t.Helper()

	sqlDB, err := sql.Open(database.SQLITE, ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})

	return &DBSession{
		database: &database.Database{SQL: sqlDB},
		config:   &database.Config{Driver: database.SQLITE, Timeout: timeout},
	}
}

func TestBeginTxUsesConfiguredTimeout(t *testing.T) {
	dbSession := newSQLiteDBSession(t, 30*time.Millisecond)

	txSession, _, _, err := dbSession.BeginTx()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	_, _, _, err = txSession.ExecSQL(context.Background(), "CREATE TABLE timeout_test (id INTEGER)")
	if err != nil {
		t.Fatalf("create table in transaction: %v", err)
	}

	time.Sleep(60 * time.Millisecond)
	if _, _, err := txSession.Commit(); err == nil {
		t.Fatal("commit succeeded after the configured transaction timeout")
	}

	var tableCount int
	if err := dbSession.database.SQL.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'timeout_test'",
	).Scan(&tableCount); err != nil {
		t.Fatalf("check table after transaction timeout: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("transaction changes remained after configured timeout")
	}
}

func TestDBSessionQuerySQLForwardsRowsAndUnavailableErrors(t *testing.T) {
	t.Run("query result", func(t *testing.T) {
		dbSession := newSQLiteDBSession(t, time.Second)

		rows, message, status, err := dbSession.QuerySQL(context.Background(), "SELECT ?", 42)
		if err != nil {
			t.Fatalf("QuerySQL() failed (HTTP %d, %s): %v", status, message, err)
		}
		defer func() {
			if err := rows.Close(); err != nil {
				t.Errorf("close query rows: %v", err)
			}
		}()
		if status != http.StatusOK {
			t.Fatalf("QuerySQL() status = %d, want %d", status, http.StatusOK)
		}
		if !rows.Next() {
			t.Fatalf("QuerySQL() returned no rows: %v", rows.Err())
		}

		var got int
		if err := rows.Scan(&got); err != nil {
			t.Fatalf("scan query result: %v", err)
		}
		if got != 42 {
			t.Fatalf("query result = %d, want 42", got)
		}
	})

	t.Run("unavailable session", func(t *testing.T) {
		_, _, status, err := (&DBSession{}).QuerySQL(context.Background(), "SELECT 1")
		if err == nil || status != http.StatusServiceUnavailable {
			t.Fatalf("QuerySQL() = HTTP %d, error %v; want HTTP 503 and an error", status, err)
		}
	})
}

func TestFilterAdapterReportsUnderlyingConfigurationError(t *testing.T) {
	adapter := &Filter{filter: &gorestfilter.Filter{}}

	_, status, err := adapter.CreateToken(Context{}, "user-1")
	if err == nil || status != http.StatusNotImplemented {
		t.Fatalf("CreateToken() = HTTP %d, error %v; want missing-configuration error", status, err)
	}
}

func TestBeginTxContextRespectsCallerCancellation(t *testing.T) {
	dbSession := newSQLiteDBSession(t, time.Second)
	ctx, cancel := context.WithCancel(context.Background())

	txSession, _, _, err := dbSession.BeginTxContext(ctx)
	if err != nil {
		cancel()
		t.Fatalf("begin transaction: %v", err)
	}

	_, _, _, err = txSession.ExecSQL(context.Background(), "CREATE TABLE canceled_test (id INTEGER)")
	if err != nil {
		t.Fatalf("create table in transaction: %v", err)
	}
	cancel()
	time.Sleep(20 * time.Millisecond)
	if _, _, err := txSession.Commit(); err == nil {
		t.Fatal("commit succeeded after the caller context was canceled")
	}

	var tableCount int
	if err := dbSession.database.SQL.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'canceled_test'",
	).Scan(&tableCount); err != nil {
		t.Fatalf("check table after caller cancellation: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("transaction changes remained after caller cancellation")
	}
}

func TestRawQuerySQLRejectsActiveTransaction(t *testing.T) {
	dbSession := newSQLiteDBSession(t, time.Second)
	txSession, _, _, err := dbSession.BeginTx()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	called := false
	err = txSession.RawQuerySQL(context.Background(), func(context.Context, *sql.DB) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("RawQuerySQL succeeded with an active transaction")
	}
	if called {
		t.Fatal("RawQuerySQL callback ran with an active transaction")
	}

	if _, _, rollbackErr := txSession.Rollback(); rollbackErr != nil {
		t.Fatalf("rollback transaction: %v", rollbackErr)
	}
}

func TestExecBatchCQLSession(t *testing.T) {
	db := &DBSession{}
	message, code, err := db.ExecBatchCQL(context.Background(), nil, nil)
	if err == nil || code != http.StatusServiceUnavailable || message == "" {
		t.Fatalf("nil session ExecBatchCQL = (%q, %d, %v)", message, code, err)
	}
}

func TestRawQuerySQLWithTxUsesTransaction(t *testing.T) {
	dbSession := newSQLiteDBSession(t, time.Second)
	if _, err := dbSession.database.SQL.Exec(
		"CREATE TABLE raw_query_transaction_test (value TEXT)",
	); err != nil {
		t.Fatalf("create test table: %v", err)
	}

	txSession, _, _, err := dbSession.BeginTx()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	err = txSession.RawQuerySQLWithTx(context.Background(), func(ctx context.Context, executor SQLExecutor) error {
		_, err := executor.ExecContext(
			ctx,
			"INSERT INTO raw_query_transaction_test (value) VALUES (?)",
			"uncommitted",
		)
		return err
	})
	if err != nil {
		t.Fatalf("execute raw SQL in transaction: %v", err)
	}

	if _, _, err := txSession.Rollback(); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}

	var count int
	if err := dbSession.database.SQL.QueryRow(
		"SELECT count(*) FROM raw_query_transaction_test WHERE value = ?",
		"uncommitted",
	).Scan(&count); err != nil {
		t.Fatalf("check rolled-back row: %v", err)
	}
	if count != 0 {
		t.Fatalf("found %d rows after rollback, want 0", count)
	}
}

func TestRawQueryNoSQLAdaptersReportAvailabilityAndCallbackErrors(t *testing.T) {
	tests := []struct {
		name        string
		driver      string
		unavailable *database.Database
		available   *database.Database
		run         func(*DBSession, func() error) (string, int, error)
	}{
		{
			name:        "MongoDB",
			driver:      database.MONGO,
			unavailable: &database.Database{},
			available:   &database.Database{Mongo: &mongo.Client{}},
			run: func(session *DBSession, callback func() error) (string, int, error) {
				return session.RawQueryMongo(context.Background(), func(context.Context, *mongo.Client) error {
					return callback()
				})
			},
		},
		{
			name:        "ScyllaDB",
			driver:      database.SCYLLA,
			unavailable: &database.Database{},
			available:   &database.Database{Scylla: &gocql.Session{}},
			run: func(session *DBSession, callback func() error) (string, int, error) {
				return session.RawQueryScylla(context.Background(), func(context.Context, *gocql.Session) error {
					return callback()
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unavailable := &DBSession{
				database: test.unavailable,
				config:   &database.Config{Driver: test.driver},
			}
			message, status, err := test.run(unavailable, func() error { return nil })
			if err == nil || status != http.StatusServiceUnavailable || message == "" {
				t.Fatalf("unavailable adapter result = (%q, %d, %v)", message, status, err)
			}

			available := &DBSession{
				database: test.available,
				config:   &database.Config{Driver: test.driver},
			}
			callbackErr := errors.New("callback failed")
			message, status, err = test.run(available, func() error { return callbackErr })
			if !errors.Is(err, callbackErr) || status != http.StatusInternalServerError || message == "" {
				t.Fatalf("callback failure result = (%q, %d, %v)", message, status, err)
			}
		})
	}
}

func TestNoSQLAndScyllaSessionAdaptersReportUnavailableService(t *testing.T) {
	session := &DBSession{}
	calls := []struct {
		name string
		call func() (int, error)
	}{
		{
			name: "MongoCollection",
			call: func() (int, error) {
				_, _, status, err := session.MongoCollection("users")
				return status, err
			},
		},
		{
			name: "DecodeMongoCursor",
			call: func() (int, error) {
				_, status, err := session.DecodeMongoCursor(context.Background(), nil, &[]map[string]any{})
				return status, err
			},
		},
		{
			name: "EncodeMongoCollection",
			call: func() (int, error) {
				_, _, status, err := session.EncodeMongoCollection(map[string]any{"name": "Ada"})
				return status, err
			},
		},
		{
			name: "MongoCreateCollection",
			call: func() (int, error) {
				_, status, err := session.MongoCreateCollection(context.Background(), "users")
				return status, err
			},
		},
		{
			name: "ExecCQL",
			call: func() (int, error) {
				_, status, err := session.ExecCQL(context.Background(), "INSERT INTO users (id) VALUES (?)", 1)
				return status, err
			},
		},
		{
			name: "QueryCQL",
			call: func() (int, error) {
				_, _, status, err := session.QueryCQL(context.Background(), "SELECT * FROM users")
				return status, err
			},
		},
		{
			name: "MapScanCAS",
			call: func() (int, error) {
				_, _, status, err := session.MapScanCAS(context.Background(), "DELETE FROM users WHERE id = ? IF EXISTS", 1)
				return status, err
			},
		},
	}

	for _, test := range calls {
		t.Run(test.name, func(t *testing.T) {
			status, err := test.call()
			if err == nil || status != http.StatusServiceUnavailable {
				t.Fatalf("adapter result = HTTP %d, error %v; want HTTP 503 and an error", status, err)
			}
		})
	}
}

func TestTransactionFinalizationIsRaceSafe(t *testing.T) {
	dbSession := newSQLiteDBSession(t, time.Second)
	txSession, _, _, err := dbSession.BeginTx()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	start := make(chan struct{})
	statuses := make(chan int, 2)
	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		<-start
		_, status, _ := txSession.Commit()
		statuses <- status
	}()
	go func() {
		defer workers.Done()
		<-start
		_, status, _ := txSession.Rollback()
		statuses <- status
	}()

	close(start)
	workers.Wait()
	close(statuses)

	var succeeded, rejected int
	for status := range statuses {
		switch status {
		case http.StatusOK:
			succeeded++
		case http.StatusBadRequest:
			rejected++
		default:
			t.Fatalf("transaction finalization returned HTTP %d, want 200 or 400", status)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("transaction finalization results: succeeded=%d rejected=%d, want one of each", succeeded, rejected)
	}
}

func TestFinishedTransactionSessionDoesNotFallBackToPool(t *testing.T) {
	dbSession := newSQLiteDBSession(t, time.Second)
	if _, err := dbSession.database.SQL.Exec("CREATE TABLE finished_tx_test (value TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	txSession, _, _, err := dbSession.BeginTx()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	if _, _, err := txSession.Commit(); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}

	if _, _, _, err := txSession.ExecSQL(context.Background(), "INSERT INTO finished_tx_test (value) VALUES (?)", "must-not-be-autocommitted"); err == nil {
		t.Fatal("query through a finished transaction session unexpectedly succeeded")
	}

	var count int
	if err := dbSession.database.SQL.QueryRow("SELECT count(*) FROM finished_tx_test").Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("finished transaction session wrote %d rows through the pool", count)
	}
}

func TestTransactionLifecycleMethodsRejectMissingConfig(t *testing.T) {
	for _, operation := range []struct {
		call func(*DBSession) (string, int, error)
		name string
	}{
		{name: "commit", call: (*DBSession).Commit},
		{name: "rollback", call: (*DBSession).Rollback},
	} {
		t.Run(operation.name, func(t *testing.T) {
			dbSession := newSQLiteDBSession(t, time.Second)
			dbSession.config = nil
			_, status, err := operation.call(dbSession)
			if err == nil || status != http.StatusServiceUnavailable {
				t.Fatalf("result = HTTP %d, error %v; want HTTP 503 and error", status, err)
			}
		})
	}
}

func TestTableWithoutDatabaseReturnsORMError(t *testing.T) {
	query := (&DBSession{}).Table("users").Find(&[]struct{}{})
	if query.Error == nil || query.HTTPCode != http.StatusServiceUnavailable {
		t.Fatalf("Table().Find() result = HTTP %d, error %v; want HTTP 503 and error", query.HTTPCode, query.Error)
	}
}

func TestNilAppDatabaseReturnsUnavailableSession(t *testing.T) {
	query := (*App)(nil).Database().Table("users").Find(&struct{}{})
	if query.Error == nil || query.HTTPCode != http.StatusServiceUnavailable {
		t.Fatalf("nil App Database().Table().Find() = HTTP %d, error %v; want HTTP 503 and error", query.HTTPCode, query.Error)
	}
}

func TestLogWithoutConfiguredAppDoesNotTerminateProcess(t *testing.T) {
	logger := (*App)(nil).Log(nil)
	if logger == nil {
		t.Fatal("Log() returned a nil logger")
	}
	logger.Info("safe no-op logger")
}

func TestRawMongoAndScyllaRejectNilCallbacks(t *testing.T) {
	dbSession := &DBSession{
		database: &database.Database{},
		config:   &database.Config{Driver: database.MONGO},
	}
	if _, status, err := dbSession.RawQueryMongo(context.Background(), nil); err == nil || status != http.StatusBadRequest {
		t.Fatalf("RawQueryMongo(nil) = HTTP %d, error %v; want HTTP 400 and error", status, err)
	}

	scyllaSession := &DBSession{
		database: &database.Database{},
		config:   &database.Config{Driver: database.SCYLLA},
	}
	if _, status, err := scyllaSession.RawQueryScylla(context.Background(), nil); err == nil || status != http.StatusBadRequest {
		t.Fatalf("RawQueryScylla(nil) = HTTP %d, error %v; want HTTP 400 and error", status, err)
	}
}
