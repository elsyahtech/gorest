package gorest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	golog "log"
	"net/http"
	"strings"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/filter"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/orm"
	gorestredis "github.com/elsyahtech/gorest/redis"
	"github.com/elsyahtech/gorest/server"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// Name is a getter for the application name set in the config.
func (app *App) Name() string { return app.config.AppName }

// Version is a getter for the application version set in the config.
func (app *App) Version() string { return app.config.Version }

// Environment is a getter for the application environment set in the config
// (for example "Development" or "Production").
func (app *App) Environment() string { return app.config.Environment }

// Timezone is a getter for the timezone name used by the application
// (for example "Asia/Jakarta").
func (app *App) Timezone() string { return app.config.Timezone }

// IsDebug is a getter that reports whether debug mode is enabled in the config.
func (app *App) IsDebug() bool { return app.config.Debug }

// Database returns a new DBSession wrapper around the application's registered
// database services. The wrapper does not create a new connection pool.
// Common query methods validate inputs and return errors; RawQuery methods are
// explicit escape hatches that expose native driver handles and place query
// construction, resource cleanup, and driver-specific behavior on the caller.
// BeginTx returns a separate transaction-bound session. Keep related operations
// and transaction finalization in a deliberate order; concurrent Commit/Rollback
// calls are mutually guarded, but transaction ordering remains the caller's job.
func (app *App) Database() *DBSession {
	if app == nil {
		return &DBSession{}
	}

	return &DBSession{
		database: app.database,
		config:   app.config.database,
	}
}

// Ping verifies database availability from the application-facing session.
func (session *DBSession) Ping(ctx context.Context) (string, error) {
	if session == nil || session.database == nil {
		return "Register the database config with app.Use(database.Run(...)).", errors.New("database service is not running")
	}
	return session.database.Ping(ctx, session.config)
}

// GetDBConfig is a getter for the database configuration used by the application,
// as set with app.Use(database.Run(...)).
func (app *App) GetDBConfig() database.Config {
	if app == nil || app.config.database == nil {
		return database.Config{}
	}

	return *app.config.database
}

// GetServerConfig is a getter for the server configuration used by the application,
// as set with app.Use(server.Run(...)).
func (app *App) GetServerConfig() server.Config {
	if app == nil || app.config.server == nil {
		return server.Config{}
	}

	return *app.config.server
}

// GetLogConfig is a getter for the logger configuration used by the application,
// as set with app.Use(log.Run(...)).
func (app *App) GetLogConfig() log.Config {
	if app == nil || app.config.log == nil {
		return log.Config{}
	}

	return *app.config.log
}

// NewContext creates a context for an operation, derived from the application context
// (or context.Background() if none is set).
// If timeout is greater than zero and the context has no deadline yet, the returned context
// is canceled after the timeout. Otherwise the context is returned as is.
// Always call the returned cancel function (usually with defer) to release resources.
func (app *App) NewContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx := app.context
	if ctx == nil {
		ctx = context.Background()
	}

	if timeout > 0 {
		if _, ok := ctx.Deadline(); !ok {
			return context.WithTimeout(ctx, timeout)
		}
	}

	return ctx, func() {}
}

// ExecSQL executes a native SQL statement (INSERT, UPDATE, DELETE, or DDL) within this session.
// Bind user-controlled values through args using the configured driver's placeholder syntax;
// do not concatenate those values into query. Placeholders generally cannot bind identifiers
// such as table/column names or sort directions, which must come from fixed application choices
// or an explicit allowlist. This method does not sanitize SQL text assembled by the caller.
// Returns 503 Service Unavailable if the database service is not running, otherwise the result,
// message, and status code from the underlying database.
func (dbSession *DBSession) ExecSQL(ctx context.Context, query string, args ...any) (sql.Result, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation.",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	result, message, httpCode, err := dbSession.database.ExecSQL(ctx, dbSession.config, query, args...)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return result, message, httpCode, nil
}

// QuerySQL executes a native SQL query (typically SELECT) within this session and returns the resulting rows.
// Bind user-controlled values through args using the configured driver's placeholder syntax;
// do not concatenate those values into query. Placeholders generally cannot bind identifiers
// such as table/column names or sort directions, which must come from fixed application choices
// or an explicit allowlist. This method does not sanitize SQL text assembled by the caller.
// The caller must close the returned *sql.Rows (usually with defer rows.Close()) and check rows.Err()
// after iterating, otherwise the connection is not released back to the pool.
func (dbSession *DBSession) QuerySQL(ctx context.Context, query string, args ...any) (*sql.Rows, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation.",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	rows, message, httpCode, err := dbSession.database.QuerySQL(ctx, dbSession.config, query, args...)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return rows, message, httpCode, nil
}

// MongoCollection returns a handle to the named MongoDB collection in the configured database.
// It does not contact the server or create the collection; it only prepares the handle for operations
// such as InsertOne, Find, UpdateOne, and DeleteOne.
// Returns 400 Bad Request if collectionName is empty, and 503 Service Unavailable if MongoDB is not running.
func (dbSession *DBSession) MongoCollection(collectionName string) (*mongo.Collection, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation.",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	coll, message, httpCode, err := dbSession.database.Collection(dbSession.config, collectionName)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return coll, message, httpCode, nil
}

// DecodeMongoCursor decodes all documents from a MongoDB cursor into dest, which must be a pointer to a slice
// (for example *[]entity.User). The cursor is closed after decoding, so the caller does not need to close it.
// Returns 400 Bad Request if the cursor or dest is invalid, and 500 Internal Server Error if decoding fails.
func (dbSession *DBSession) DecodeMongoCursor(ctx context.Context, cursor *mongo.Cursor, dest any) (string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation.",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	return dbSession.database.DecodeMongoCursor(ctx, cursor, dest) //nolint:wrapcheck
}

// EncodeMongoCollection converts a struct (or other value) into a bson.M document,
// for example to build an update or filter document from an entity.
// Field names follow the `bson` struct tags, so make sure they match your collection schema.
func (dbSession *DBSession) EncodeMongoCollection(dest any) (bson.M, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation.",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	return dbSession.database.EncodeMongoCollection(dest) //nolint:wrapcheck
}

// MongoCreateCollection creates a new MongoDB collection in the configured database.
// Returns 400 Bad Request if the collection name is empty, 409 Conflict if the collection already exists,
// and 500 Internal Server Error for other failures.
func (dbSession *DBSession) MongoCreateCollection(
	ctx context.Context,
	collectionName string,
	opts ...options.Lister[options.CreateCollectionOptions],
) (string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation.",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	message, httpCode, err := dbSession.database.CreateCollection(ctx, dbSession.config, collectionName, opts...)
	if err != nil {
		return message, httpCode, fmt.Errorf("%w", err)
	}

	return message, httpCode, nil
}

// ExecCQL executes a CQL statement (INSERT, UPDATE, DELETE, or DDL) on ScyllaDB.
// Use placeholders (?) for args; never build the query from user input.
func (dbSession *DBSession) ExecCQL(ctx context.Context, query string, args ...any) (string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation.",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	message, httpCode, err := dbSession.database.ExecCQL(ctx, dbSession.config, query, args...)
	if err != nil {
		return message, httpCode, fmt.Errorf("%w", err)
	}

	return message, httpCode, nil
}

// QueryCQL executes a CQL query (typically SELECT) and returns an iterator over the resulting rows.
// The caller must close the returned *gocql.Iter (usually with defer iter.Close()),
// and check the error returned by Close after iterating.
// ExecBatchCQL executes multiple CQL statements with their corresponding arguments.
func (dbSession *DBSession) ExecBatchCQL(ctx context.Context, queries []string, batchArgs [][]any) (string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return "Register the database config with app.Use(database.Run(...)). " +
				"To use ScyllaDB, the application must be configured with the SCYLLA driver",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	message, httpCode, err := dbSession.database.ExecBatchCQL(ctx, dbSession.config, queries, batchArgs)
	if err != nil {
		return message, httpCode, fmt.Errorf("%w", err)
	}

	return message, httpCode, nil
}

func (dbSession *DBSession) QueryCQL(ctx context.Context, query string, args ...any) (*gocql.Iter, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation.",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	iter, message, httpCode, err := dbSession.database.QueryCQL(ctx, dbSession.config, query, args...)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return iter, message, httpCode, nil
}

// ScanCQL executes a lightweight transaction (a CQL statement with an IF clause) and reports
// whether the condition was met and the change was applied.
// A false result with a nil error means the condition was not met; the caller decides how to handle it
// (for example 404 for IF EXISTS, 409 for IF NOT EXISTS).
func (dbSession *DBSession) MapScanCAS(ctx context.Context, query string, args ...any) (bool, string, int, error) {
	if dbSession == nil || dbSession.database == nil || dbSession.config == nil {
		return false, "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation.",
			http.StatusServiceUnavailable,
			errors.New("database service is not running")
	}

	applied, message, httpCode, err := dbSession.database.MapScanCAS(ctx, dbSession.config, query, args...)
	if err != nil {
		return false, message, httpCode, fmt.Errorf("%w", err)
	}

	return applied, message, httpCode, nil
}

// QueryBuilderSQL is a function that receives the SQL connection and builds or runs queries on it
// using a third-party query builder or ORM (for example sqlx).
// It is executed by RawQuerySQL.
type QueryBuilderSQL func(ctx context.Context, db *sql.DB) error

// SQLExecutor is the subset of database/sql operations available from either
// a connection pool or a SQL transaction.
type SQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

// QueryBuilderSQLSession receives the active SQL executor for a DBSession.
// The executor is the active transaction when one exists, otherwise the connection pool.
type QueryBuilderSQLSession func(ctx context.Context, executor SQLExecutor) error

// RawQuerySQL runs fn with the underlying *sql.DB, for cases where the built-in helpers are not enough.
// Prefer ExecSQL and QuerySQL when possible. This escape hatch leaves query construction and safe
// parameter binding to fn: use QueryContext/ExecContext with driver placeholders and bound values,
// and select dynamic identifiers only from fixed application choices or an explicit allowlist.
// Gorest does not sanitize SQL text assembled by the callback. This method is only for operations
// outside a transaction. It returns an error when this session has an active transaction; use
// RawQuerySQLWithTx to execute raw queries through the active transaction instead.
//
// Deprecated: use RawQuerySQLWithTx to have operations participate in the active transaction.
func (dbSession *DBSession) RawQuerySQL(ctx context.Context, fun QueryBuilderSQL) error {
	if dbSession == nil || dbSession.database == nil {
		return errors.New("database service is not running")
	}

	if dbSession.database.SQL == nil {
		return errors.New("SQL database is not running")
	}

	if dbSession.database.Tx != nil {
		return errors.New("RawQuerySQL uses the connection pool and cannot be called with an active transaction; use RawQuerySQLWithTx")
	}

	if fun == nil {
		return errors.New("SQL query callback is nil")
	}

	if err := fun(ctx, dbSession.database.SQL); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// RawQuerySQLWithTx executes fn using this session's active *sql.Tx when the
// session is transaction-bound; otherwise it uses the underlying *sql.DB pool.
// All statements executed through the callback participate in the active transaction
// and are committed or rolled back with it. Prefer ExecSQL and QuerySQL when possible.
// The callback remains responsible for safe SQL construction, parameter binding,
// and closing any rows it opens.
func (dbSession *DBSession) RawQuerySQLWithTx(ctx context.Context, fun QueryBuilderSQLSession) error {
	if dbSession == nil || dbSession.database == nil {
		return errors.New("database service is not running")
	}

	if dbSession.database.SQL == nil {
		return errors.New("SQL database is not running")
	}

	if fun == nil {
		return errors.New("SQL query callback is nil")
	}

	if ctx == nil {
		return errors.New("context is empty")
	}

	var executor SQLExecutor = dbSession.database.SQL

	if dbSession.database.Tx != nil {
		executor = dbSession.database.Tx
	}

	if err := fun(ctx, executor); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// QueryBuilderMongo is a function that receives the MongoDB client and runs operations on it directly,
// for cases where the built-in helpers are not enough.
// It is executed by RawQueryMongo.
type QueryBuilderMongo func(ctx context.Context, client *mongo.Client) error

// RawQueryMongo runs fn with the underlying *mongo.Client, as an escape hatch when the handler
// needs direct raw MongoDB operations.
// Prefer the built-in helpers (MongoCollection, DecodeMongoCursor, etc.) when possible.
func (dbSession *DBSession) RawQueryMongo(ctx context.Context, fn QueryBuilderMongo) (string, int, error) {
	return runRawQuery(ctx, dbSession, mongoRawQueryAdapter{callback: fn})
}

// QueryBuilderScylla is a function that receives the ScyllaDB session and runs queries on it directly,
// for cases where the built-in helpers are not enough.
// It is executed by RawQueryScylla.
type QueryBuilderScylla func(ctx context.Context, session *gocql.Session) error

// RawQueryScylla runs fn with the underlying *gocql.Session, as an escape hatch for cases
// where the built-in helpers are not enough.
// Prefer ExecCQL, QueryCQL, and ScanCQL when possible.
func (dbSession *DBSession) RawQueryScylla(ctx context.Context, fn QueryBuilderScylla) (string, int, error) {
	return runRawQuery(ctx, dbSession, scyllaRawQueryAdapter{callback: fn})
}

type rawQueryAdapter interface {
	driverName() string
	driverDisplayName() string
	databaseName() string
	methodName() string
	callbackErrorMessage() string
	callbackIsNil() bool
	adapterIsAvailable(*database.Database) bool
	run(context.Context, *database.Database) error
}

type mongoRawQueryAdapter struct {
	callback QueryBuilderMongo
}

func (mongoRawQueryAdapter) driverName() string        { return "MONGO" }
func (mongoRawQueryAdapter) driverDisplayName() string { return "MongoDB" }
func (mongoRawQueryAdapter) databaseName() string      { return "mongo" }
func (mongoRawQueryAdapter) methodName() string        { return "rawQueryMongo" }
func (mongoRawQueryAdapter) callbackErrorMessage() string {
	return "Check the MongoDB operation, collection permissions, and document/query values."
}
func (adapter mongoRawQueryAdapter) callbackIsNil() bool { return adapter.callback == nil }
func (mongoRawQueryAdapter) adapterIsAvailable(db *database.Database) bool {
	return db.Mongo != nil
}
func (adapter mongoRawQueryAdapter) run(ctx context.Context, db *database.Database) error {
	return adapter.callback(ctx, db.Mongo)
}

type scyllaRawQueryAdapter struct {
	callback QueryBuilderScylla
}

func (scyllaRawQueryAdapter) driverName() string        { return "SCYLLA" }
func (scyllaRawQueryAdapter) driverDisplayName() string { return "ScyllaDB" }
func (scyllaRawQueryAdapter) databaseName() string      { return "scylla" }
func (scyllaRawQueryAdapter) methodName() string        { return "rawQueryScylla" }
func (scyllaRawQueryAdapter) callbackErrorMessage() string {
	return "Check the CQL statement, table schema, permissions, and ScyllaDB connectivity."
}
func (adapter scyllaRawQueryAdapter) callbackIsNil() bool { return adapter.callback == nil }
func (scyllaRawQueryAdapter) adapterIsAvailable(db *database.Database) bool {
	return db.Scylla != nil
}
func (adapter scyllaRawQueryAdapter) run(ctx context.Context, db *database.Database) error {
	return adapter.callback(ctx, db.Scylla)
}

func runRawQuery(ctx context.Context, session *DBSession, adapter rawQueryAdapter) (string, int, error) {
	if session == nil || session.config == nil || session.database == nil {
		return "Register the database config with app.Use(database.Run(...)) and select the " +
				adapter.driverName() + " driver.",
			http.StatusServiceUnavailable, errors.New("database service is not running")
	}

	if ctx == nil || adapter.callbackIsNil() {
		return "Provide a non-nil context and " + adapter.driverDisplayName() + " query callback.", http.StatusBadRequest,
			fmt.Errorf("%s: context or callback is nil", adapter.methodName())
	}

	if !adapter.adapterIsAvailable(session.database) {
		return "Configure app.Use(database.Run(...)) with the " + adapter.driverName(),
			http.StatusServiceUnavailable, fmt.Errorf(
				"%s database is not running. Your application is currently running on %s",
				adapter.databaseName(), session.config.Driver,
			)
	}

	if err := adapter.run(ctx, session.database); err != nil {
		return adapter.callbackErrorMessage(), http.StatusInternalServerError, fmt.Errorf("%s: %w", adapter.methodName(), err)
	}

	return "", http.StatusOK, nil
}

// BeginTx starts a SQL transaction and returns a new DBSession bound to it.
// Use the returned session for all statements that belong to the transaction,
// then finish with Commit or Rollback. The configured database timeout is applied
// to the transaction context.
// Returns 409 Conflict if this session is already transaction-bound, and 500 Internal Server Error if a transaction cannot be started.
func (dbSession *DBSession) BeginTx(opts ...*sql.TxOptions) (*DBSession, string, int, error) {
	return dbSession.BeginTxContext(context.Background(), opts...)
}

// BeginTxContext starts a SQL transaction using ctx and returns a new DBSession
// bound to it. The configured database timeout caps the transaction lifetime,
// while cancellation or an earlier deadline from ctx is also respected.
// Commit or Rollback cancels the derived context and releases its resources.
// Returns 400 for a nil context, 409 if the session is already transaction-bound,
// 503 if the database service is unavailable, and 500 if the driver cannot start it.
func (dbSession *DBSession) BeginTxContext(ctx context.Context, opts ...*sql.TxOptions) (*DBSession, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		const message = "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation."

		return nil, message, http.StatusServiceUnavailable, errors.New("database service is not running")
	}

	if dbSession.config == nil {
		const message = "Ensure that the database configuration is available."

		return nil, message, http.StatusServiceUnavailable, errors.New("database configuration is missing")
	}

	if ctx == nil {
		const message = "Ensure the context is not empty."

		return nil, message, http.StatusBadRequest, errors.New("context is empty")
	}

	if dbSession.config.Driver == database.MONGO || dbSession.config.Driver == database.SCYLLA {
		return dbSession, "", http.StatusOK, nil
	}

	var (
		txCtx  context.Context
		cancel context.CancelFunc
	)

	if dbSession.config.Timeout > 0 {
		txCtx, cancel = context.WithTimeout(ctx, dbSession.config.Timeout)
	} else {
		txCtx, cancel = context.WithCancel(ctx)
	}

	txDB, message, httpCode, err := dbSession.database.BeginTxContext(txCtx, dbSession.config, opts...)
	if err != nil {
		cancel()
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return &DBSession{
		database: txDB,
		config:   dbSession.config,
		txCancel: cancel,
	}, message, httpCode, nil
}

// Rollback aborts the active transaction, discarding all changes made since BeginTx.
// Returns 400 Bad Request if there is no active transaction or it has already been finalized, and 500 Internal Server Error if rollback fails.
func (dbSession *DBSession) Rollback() (string, int, error) {
	return dbSession.finalizeTransaction(&structFinalizeTransaction{
		operation:        "rollback",
		noTransaction:    "Ensure that a database transaction has been properly started before attempting a rollback",
		operationFailure: "Ensure the database connection is healthy and the transaction is not already closed or committed",
		success:          "Transaction rolled back successfully",
		finish:           (*sql.Tx).Rollback,
	})
}

// Commit saves all changes made in the active transaction.
// Returns 400 Bad Request if there is no active transaction or it has already been finalized, and 500 Internal Server Error if commit fails.
// After Commit, the transaction is finished and Rollback has nothing left to undo.
func (dbSession *DBSession) Commit() (string, int, error) {
	return dbSession.finalizeTransaction(&structFinalizeTransaction{
		operation:        "commit",
		noTransaction:    "Ensure that a database transaction has been properly started before attempting a commit",
		operationFailure: "Ensure the database connection is active and the transaction has not timed out",
		success:          "Transaction committed successfully",
		finish:           (*sql.Tx).Commit,
	})
}

type structFinalizeTransaction struct {
	finish           func(*sql.Tx) error
	operation        string
	noTransaction    string
	operationFailure string
	success          string
}

func (dbSession *DBSession) finalizeTransaction(req *structFinalizeTransaction) (string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		const message = "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation."

		return message, http.StatusServiceUnavailable, errors.New("database service is not running")
	}

	if dbSession.config == nil {
		const message = "Ensure that the database configuration is available."

		return message, http.StatusServiceUnavailable, fmt.Errorf("%s: database configuration is not available", req.operation)
	}

	if dbSession.config.Driver == database.MONGO || dbSession.config.Driver == database.SCYLLA {
		return "", http.StatusOK, nil
	}

	if dbSession.database.Tx == nil {
		return req.noTransaction, http.StatusBadRequest, fmt.Errorf("%s: no active transaction to %s", req.operation, req.operation)
	}

	if !dbSession.txDone.CompareAndSwap(false, true) {
		const message = "The transaction has already been committed or rolled back."

		return message, http.StatusBadRequest, fmt.Errorf("%s: transaction is already finished", req.operation)
	}

	err := req.finish(dbSession.database.Tx)

	if dbSession.txCancel != nil {
		dbSession.txCancel()
	}

	if err != nil {
		return req.operationFailure, http.StatusInternalServerError, fmt.Errorf("%s: %w", req.operation, err)
	}

	return req.success, http.StatusOK, nil
}

// Close shuts down all application services.
func (app *App) Close() (string, int, error) {
	if app == nil {
		const message = "Application is not running"

		return message, http.StatusBadRequest, errors.New("close: app is nil")
	}

	var (
		errs     []error
		message  string
		httpCode = http.StatusOK
	)

	if app.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), server.DefaultServerShutdownTimeout)
		err := app.server.Shutdown(ctx)

		defer cancel()

		if err != nil {
			const msg = "failed to gracefully shut down HTTP server"

			app.Log(Map{LogFieldKeyError: err}).Error(msg)

			message, httpCode = msg, http.StatusInternalServerError

			errs = append(errs, fmt.Errorf("failed to shut down HTTP server: %w", err))
		}
	}

	if app.redis != nil {
		msg, err := app.redis.Close()
		if err != nil {
			app.Log(Map{LogFieldKeyError: err}).Error(msg)

			message, httpCode = msg, http.StatusInternalServerError

			errs = append(errs, fmt.Errorf("failed to disconnect redis: %w", err))
		}
	}

	if app.database != nil {
		timeout := app.config.database.Timeout
		if timeout <= 0 {
			timeout = database.DefaultTimeout
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		msg, code, err := app.database.Close(ctx, app.config.database)
		if err != nil {
			app.Log(Map{LogFieldKeyError: err}).Error(msg)

			message, httpCode = msg, code

			errs = append(errs, fmt.Errorf("failed to disconnect database: %w", err))
		}
	}

	// Logger is closed last so other services can still log during shutdown.
	if app.log != nil {
		msg, code, err := app.log.Close()
		if err != nil {
			message, httpCode = msg, code

			errs = append(errs, fmt.Errorf("failed to disconnect logger: %w", err))
		}
	}

	if len(errs) > 0 {
		return message, httpCode, errors.Join(errs...)
	}

	return "Application closed successfully", http.StatusOK, nil
}

// Table starts an ORM query chain for the given table (or collection, for MongoDB).
// Chain methods such as Where, Select, Join, OrderBy, Limit, and Offset to build the query,
// then finish with an operation such as Create, Update, Delete, or Find.
// If the transaction is active in this session, the query runs inside it.
// If the database service is unavailable, the returned ORM contains an error
// and the corresponding HTTP status instead of terminating the process.
func (dbSession *DBSession) Table(table string) *orm.ORM {
	if dbSession == nil || dbSession.database == nil || dbSession.config == nil {
		const message = "Register the database config with app.Use(database.Run(...)) and select a driver supported by this operation."

		return &orm.ORM{
			Error:          errors.New("table: database service is not running"),
			Message:        message,
			HTTPCode:       http.StatusServiceUnavailable,
			Database:       nil,
			DatabaseConfig: nil,
		}
	}

	return &orm.ORM{
		Error:              nil,
		Result:             nil,
		Context:            context.Background(),
		ReturnDest:         nil,
		Database:           dbSession.database,
		Tx:                 dbSession.database.Tx,
		StringBuilder:      &strings.Builder{},
		DatabaseConfig:     dbSession.config,
		Table:              &table,
		Preloads:           nil,
		Message:            "",
		LastInsertId:       "",
		UpsertConflictCols: nil,
		WhereClauses:       nil,
		SelectedCols:       nil,
		DistinctCols:       nil,
		GroupByClauses:     nil,
		HavingClauses:      nil,
		HavingArgs:         nil,
		UpsertUpdateCols:   nil,
		UpsertClauses:      nil,
		UpsertArgs:         nil,
		PreloadClauses:     nil,
		WhereArgs:          nil,
		JoinClauses:        nil,
		OrderByClauses:     nil,
		ReturnCols:         nil,
		RowsAffected:       0,
		OffsetVal:          0,
		LimitVal:           0,
		HTTPCode:           0,
		IsUpsert:           false,
		AllowFilteringFlag: false,
		IsDistinct:         false,
		IsReturn:           false,
	}
}

// Log returns the configured application logger. If the app or logger is not
// running, it reports the condition through the standard logger and returns
// a no-op logger so logging misuse does not terminate the process.
func (app *App) Log(field map[string]any, args ...bool) *zap.Logger {
	if app == nil {
		golog.Print("gorest: cannot write application log because the app is nil")
		return zap.NewNop()
	}

	if app.log == nil {
		golog.Print("gorest: cannot write application log because the logger is not configured")
		return zap.NewNop()
	}

	return app.log.Log(field, args...)
}

// GetRedisConfig is a getter for the Redis configuration used by the application,
// as set with app.Use(redis.Run(...)).
func (app *App) GetRedisConfig() gorestredis.Config {
	if app == nil || app.config.redis == nil {
		return gorestredis.Config{}
	}

	return *app.config.redis
}

func (app *App) Redis() *RedisSession {
	if app == nil {
		return &RedisSession{}
	}

	return &RedisSession{
		redis:  app.redis,
		config: app.config.redis,
	}
}

// QueryRedis returns the Redis client for Redis commands. Pass a context to every
// command; RedisSession.NewContext can apply the configured operation timeout.
// It returns an error if Redis is disabled, not registered, or failed to start.
func (redisSession *RedisSession) QueryRedis() (*redis.Client, string, int, error) {
	if redisSession == nil || redisSession.redis == nil {
		const message = "Register the Redis config with app.Use(redis.Run(...)) and ensure it is enabled."

		return nil, message, http.StatusServiceUnavailable, errors.New("Redis service is not running")
	}

	client, message, httpCode, err := redisSession.redis.Redis()
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return client, message, httpCode, nil
}

// Ping verifies Redis availability from the application-facing session.
func (redisSession *RedisSession) Ping(ctx context.Context) error {
	if redisSession == nil || redisSession.redis == nil {
		return errors.New("redis service is not running")
	}

	return redisSession.redis.Ping(ctx)
}

// NewContext returns a context for a Redis operation, capped by the configured
// Redis timeout and any earlier parent deadline. Always call the returned cancel
// function.
func (redisSession *RedisSession) NewContext(ctxParam context.Context) (context.Context, context.CancelFunc) {
	ctx := ctxParam

	if ctx == nil {
		ctx = context.Background()
	}

	if redisSession != nil && redisSession.config != nil && redisSession.config.Timeout > 0 {
		return context.WithTimeout(ctx, redisSession.config.Timeout)
	}

	return ctx, func() {}
}

// Get registers a route for GET methods that requests a representation
// of the specified resource. Requests using GET should only retrieve data.
func (app *App) Get(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Get(path, handler, handlers...)
}

// Head registers a route for HEAD methods that asks for a response identical
// to that of a GET request, but without the response body.
func (app *App) Head(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Head(path, handler, handlers...)
}

// Post registers a route for POST methods that is used to submit an entity to the
// specified resource, often causing a change in state or side effects on the server.
func (app *App) Post(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Post(path, handler, handlers...)
}

// Put registers a route for PUT methods that replaces all current representations
// of the target resource with the request payload.
func (app *App) Put(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Put(path, handler, handlers...)
}

// Delete registers a route for DELETE methods that deletes the specified resource.
func (app *App) Delete(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Delete(path, handler, handlers...)
}

// Connect registers a route for CONNECT methods that establishes a tunnel to the
// server identified by the target resource.
func (app *App) Connect(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Connect(path, handler, handlers...)
}

// Options registers a route for OPTIONS methods that is used to describe the
// communication options for the target resource.
func (app *App) Options(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Options(path, handler, handlers...)
}

// Trace registers a route for TRACE methods that performs a message loop-back
// test along the path to the target resource.
func (app *App) Trace(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Trace(path, handler, handlers...)
}

// Patch registers a route for PATCH methods that is used to apply partial
// modifications to a resource.
func (app *App) Patch(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Patch(path, handler, handlers...)
}

// Query registers a route for QUERY methods that performs a safe, idempotent
// query with a request body.
func (app *App) Query(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Query(path, handler, handlers...)
}

// Add allows you to specify multiple HTTP methods to register a route.
// The provided handlers are executed in order, starting with `handler` and then the variadic `handlers`.
func (app *App) Add(methods []string, path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Add(methods, path, handler, handlers...)
}

// All will register the handler on all HTTP methods.
func (app *App) All(path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.All(path, handler, handlers...)
}

// Group is used for Routes with common prefix to define a new sub-router with optional middleware.
//
//	api := app.Group("/api")
//	api.Get("/users", handler).
func (app *App) Group(prefix string, handlers ...server.HandlerFunc) *server.Server {
	return app.server.Group(prefix, handlers...)
}

// Filter returns the running JWT filter and aborts startup if JWT is disabled.
func (app *App) Filter() *filter.Filter {
	if app == nil {
		return nil
	}

	if app.filter == nil {
		app.abortStartup(errors.New("JWT filter is not running; ensure that the filter is registered with app.Use(...) " +
			"in your main function before calling app.Filter() in your endpoint route, and that the filter " +
			"has been properly configured. Check the log to see the error details."))
	}

	return app.filter
}

func (flt *Filter) CreateToken(ctx Context, tokenID string) (string, int, error) {
	message, httpCode, err := flt.filter.CreateToken(ctx, tokenID)
	if err != nil {
		return message, httpCode, fmt.Errorf("%w", err)
	}

	return message, httpCode, nil
} //nolint:revive
