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

// Database returns a sterile, thread-safe DBSession instance.
//
// Gorest strictly enforce encapsulation here. Handlers are never allowed to touch
// raw database drivers (*sql.DB, *mongo.Client, *scylla) directly to prevent state corruption,
// connection leaks, or accidental nil-pointer panics.
//
// Instead, DBSession acts as a controlled gateway, providing safe entry points
// (like From) to initialize query builders while keeping the underlying connection
// strictly guarded within the App lifecycle.
func (app *App) Database() *DBSession {
	return &DBSession{
		database: app.database,
		config:   app.config.database,
	}
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
// Returns 501 Not Implemented if the database service is not running, otherwise the result,
// message, and status code from the underlying database.
func (dbSession *DBSession) ExecSQL(ctx context.Context, query string, args ...any) (sql.Result, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
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
		return nil, "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
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
// Returns 400 Bad Request if collectionName is empty, and 501 Not Implemented if the MongoDB service is not running.
func (dbSession *DBSession) MongoCollection(collectionName string) (*mongo.Collection, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
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
		return "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
			errors.New("database service is not running")
	}

	return dbSession.database.DecodeMongoCursor(ctx, cursor, dest) //nolint:wrapcheck
}

// EncodeMongoCollection converts a struct (or other value) into a bson.M document,
// for example to build an update or filter document from an entity.
// Field names follow the `bson` struct tags, so make sure they match your collection schema.
func (dbSession *DBSession) EncodeMongoCollection(dest any) (bson.M, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
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
		return "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
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
		return "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
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
func (dbSession *DBSession) QueryCQL(ctx context.Context, query string, args ...any) (*gocql.Iter, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
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
		return false, "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
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
	if dbSession == nil || dbSession.config == nil || dbSession.database == nil {
		return "Ensure that you have run database.Run(database.Config{...}) in your app, " +
				"and the application must be running MONGO",
			http.StatusNotImplemented, errors.New("database service is not running")
	}

	if dbSession.database.Mongo == nil {
		return "Ensure that your application running use the MONGO",
			http.StatusNotImplemented, fmt.Errorf("mongo database is not running. Your application is currently running on %s", dbSession.config.Driver)
	}

	if err := fn(ctx, dbSession.database.Mongo); err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("%w", err)
	}

	return "", http.StatusOK, nil
}

// QueryBuilderScylla is a function that receives the ScyllaDB session and runs queries on it directly,
// for cases where the built-in helpers are not enough.
// It is executed by RawQueryScylla.
type QueryBuilderScylla func(ctx context.Context, session *gocql.Session) error

// RawQueryScylla runs fn with the underlying *gocql.Session, as an escape hatch for cases
// where the built-in helpers are not enough.
// Prefer ExecCQL, QueryCQL, and ScanCQL when possible.
func (dbSession *DBSession) RawQueryScylla(ctx context.Context, fn QueryBuilderScylla) (string, int, error) {
	if dbSession == nil || dbSession.config == nil || dbSession.database == nil {
		return "Ensure that you have run database.Run(database.Config{...}) in your app, " +
				"and the application must be running SCYLLA",
			http.StatusNotImplemented, errors.New("database service is not running")
	}

	if dbSession.database.Scylla == nil {
		return "Ensure that your application running use the SCYLLA",
			http.StatusNotImplemented, fmt.Errorf("scylla database is not running. Your application is currently running on %s", dbSession.config.Driver)
	}

	if err := fn(ctx, dbSession.database.Scylla); err != nil {
		return "Ensure that your application running use the SCYLLA",
			http.StatusInternalServerError, fmt.Errorf("scylla database is not running. Your application is currently running on %s", dbSession.config.Driver)
	}

	return "", http.StatusOK, nil
}

// BeginTx starts a SQL transaction and returns a new DBSession bound to it.
// Use the returned session for all statements that belong to the transaction,
// then finish with Commit or Rollback. The configured database timeout is applied
// to the transaction context.
// Returns 409 Conflict if a transaction is already active, and 500 Internal Server Error if it cannot be started.
func (dbSession *DBSession) BeginTx(opts ...*sql.TxOptions) (*DBSession, string, int, error) {
	return dbSession.BeginTxContext(context.Background(), opts...)
}

// BeginTxContext starts a SQL transaction using ctx and returns a new DBSession
// bound to it. The configured database timeout caps the transaction lifetime,
// while cancellation or an earlier deadline from ctx is also respected.
// Commit or Rollback cancels the derived context and releases its resources.
func (dbSession *DBSession) BeginTxContext(ctx context.Context, opts ...*sql.TxOptions) (*DBSession, string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return nil, "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
			errors.New("database service is not running")
	}

	if dbSession.config == nil {
		return nil, "Ensure that the database configuration is available.",
			http.StatusBadRequest,
			errors.New("database configuration is not running")
	}

	if ctx == nil {
		return nil, "Ensure the context is not empty.",
			http.StatusBadRequest,
			errors.New("context is empty")
	}

	if dbSession.config.Driver == database.MONGO || dbSession.config.Driver == database.SCYLLA {
		return dbSession, "", http.StatusOK, nil
	}

	txCtx := ctx

	var cancel context.CancelFunc

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
// Returns 400 Bad Request if there is no active transaction, and 500 Internal Server Error if the rollback fails.
func (dbSession *DBSession) Rollback() (string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
			errors.New("database service is not running")
	}

	if dbSession.config.Driver == database.MONGO || dbSession.config.Driver == database.SCYLLA {
		return "", http.StatusOK, nil
	}

	if dbSession.database.Tx == nil {
		return "Ensure that a database transaction has been properly started before attempting a rollback",
			http.StatusBadRequest,
			errors.New("rollback: no active transaction to rollback")
	}

	err := dbSession.database.Tx.Rollback()

	dbSession.database.Tx = nil

	if dbSession.txCancel != nil {
		dbSession.txCancel()
		dbSession.txCancel = nil
	}

	if err != nil {
		return "Ensure the database connection is healthy and the transaction is not already closed or committed",
			http.StatusInternalServerError,
			fmt.Errorf("rollback: %w", err)
	}

	return "Transaction rolled back successfully", http.StatusOK, nil
}

// Commit saves all changes made in the active transaction.
// Returns 400 Bad Request if there is no active transaction, and 500 Internal Server Error if the commit fails.
// After Commit, the transaction is finished and Rollback has nothing left to undo.
func (dbSession *DBSession) Commit() (string, int, error) {
	if dbSession == nil || dbSession.database == nil {
		return "Ensure that you have run database.Run(database.Config{...}) in your app. " +
				"To use ORM, the application must use one of the following databases: " +
				"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA",
			http.StatusNotImplemented,
			errors.New("database service is not running")
	}

	if dbSession.config.Driver == database.MONGO || dbSession.config.Driver == database.SCYLLA {
		return "", http.StatusOK, nil
	}

	if dbSession.database.Tx == nil {
		return "Ensure that a database transaction has been properly started before attempting a commit",
			http.StatusBadRequest,
			errors.New("commit: no active transaction to commit")
	}

	err := dbSession.database.Tx.Commit()

	dbSession.database.Tx = nil

	if dbSession.txCancel != nil {
		dbSession.txCancel()
		dbSession.txCancel = nil
	}

	if err != nil {
		return "Ensure the database connection is active and the transaction has not timed out",
			http.StatusInternalServerError,
			fmt.Errorf("commit: %w", err)
	}

	return "Transaction committed successfully", http.StatusOK, nil
}

// Close shuts down all application services
func (app *App) Close() (string, int, error) {
	if app == nil {
		return "Application is not initialized", http.StatusBadRequest,
			errors.New("close: app is nil")
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
			msg := "failed to gracefully shut down HTTP server"
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
		ctx, cancel := app.NewContext(app.config.database.Timeout)
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
// The program exits (log.Fatalf) if the database service is not running, so make sure
// database.Run(...) is registered with app.Use(...) before calling Table.
func (dbSession *DBSession) Table(table string) *orm.ORM {
	if dbSession == nil || dbSession.database == nil || dbSession.config == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ORM, the application must use one of the following databases: " +
			"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA"

		golog.Fatalf("Gorest detected ORM Query in your application, but database is not running. Message: %s", message)

		return nil
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

// GetLogConfig is a getter for the logger configuration used by the application,
// as set with app.Use(log.Run(...)).
func (app *App) GetLogConfig() log.Config {
	return *app.config.log
}

// Log is
func (app *App) Log(field map[string]any, args ...bool) *zap.Logger {
	if app == nil {
		golog.Fatalf("gorest is not running")

		return nil
	}

	if app.log == nil {
		golog.Fatalf("error: logger is not running")

		return nil
	}

	return app.log.Log(field, args...)
}

// GetRedisConfig is a getter for the Redis configuration used by the application,
// as set with app.Use(redis.Run(...)).
func (app *App) GetRedisConfig() gorestredis.Config {
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
// It returns an error if Redis is not enabled, was not registered, or failed to initialize.
func (redisSession *RedisSession) QueryRedis() (*redis.Client, string, int, error) {
	if redisSession == nil || redisSession.redis == nil {
		return nil, "Ensure that you have run redis.Run(redis.Config{...}) in your app. " +
				"To use QueryRedis, Redis must be enabled and initialized with app.Use(...).",
			http.StatusNotImplemented,
			errors.New("redis has not been run in your app")
	}

	client, message, httpCode, err := redisSession.redis.Redis()
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return client, message, httpCode, nil
}

// NewContext returns a context for a Redis operation, capped by the configured
// Redis timeout and any earlier parent deadline. Always call the returned cancel
// function.
func (redisSession *RedisSession) NewContext(ctx context.Context) (context.Context, context.CancelFunc) {
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
func (app *App) Get(path string, handler server.HandlerFunc) {
	app.server.Get(path, handler)
}

// Head registers a route for HEAD methods that asks for a response identical
// to that of a GET request, but without the response body.
func (app *App) Head(path string, handler server.HandlerFunc) {
	app.server.Head(path, handler)
}

// Post registers a route for POST methods that is used to submit an entity to the
// specified resource, often causing a change in state or side effects on the server.
func (app *App) Post(path string, handler server.HandlerFunc) {
	app.server.Post(path, handler)
}

// Put registers a route for PUT methods that replaces all current representations
// of the target resource with the request payload.
func (app *App) Put(path string, handler server.HandlerFunc) {
	app.server.Put(path, handler)
}

// Delete registers a route for DELETE methods that deletes the specified resource.
func (app *App) Delete(path string, handler server.HandlerFunc) {
	app.server.Delete(path, handler)
}

// Connect registers a route for CONNECT methods that establishes a tunnel to the
// server identified by the target resource.
func (app *App) Connect(path string, handler server.HandlerFunc) {
	app.server.Connect(path, handler)
}

// Options registers a route for OPTIONS methods that is used to describe the
// communication options for the target resource.
func (app *App) Options(path string, handler server.HandlerFunc) {
	app.server.Options(path, handler)
}

// Trace registers a route for TRACE methods that performs a message loop-back
// test along the path to the target resource.
func (app *App) Trace(path string, handler server.HandlerFunc) {
	app.server.Trace(path, handler)
}

// Patch registers a route for PATCH methods that is used to apply partial
// modifications to a resource.
func (app *App) Patch(path string, handler server.HandlerFunc) {
	app.server.Patch(path, handler)
}

// Query registers a route for QUERY methods that performs a safe, idempotent
// query with a request body.
func (app *App) Query(path string, handler server.HandlerFunc) {
	app.server.Query(path, handler)
}

// Add allows you to specify multiple HTTP methods to register a route.
// The provided handlers are executed in order, starting with `handler` and then the variadic `handlers`.
func (app *App) Add(methods []string, path string, handler server.HandlerFunc, handlers ...server.HandlerFunc) {
	app.server.Add(methods, path, handler, handlers...)
}

// All will register the handler on all HTTP methods.
func (app *App) All(path string, handler server.HandlerFunc) {
	app.server.All(path, handler)
}

// Group is used for Routes with common prefix to define a new sub-router with optional middleware.
//
//	api := app.Group("/api")
//	api.Get("/users", handler).
func (app *App) Group(prefix string, handlers ...server.HandlerFunc) *server.Server {
	return app.server.Group(prefix, handlers...)
} //nolint:revive
