package gorest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	golog "log"
	"strings"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/orm"
	gorestredis "github.com/elsyahtech/gorest/redis"
	"github.com/elsyahtech/gorest/server"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// Getter to get app name.
func (app *App) Name() string { return app.config.AppName }

// Getter to get app version.
func (app *App) Version() string { return app.config.Version }

// Getter to get app environment.
func (app *App) Environment() string { return app.config.Environment }

// Getter to get app timezone.
func (app *App) Timezone() string { return app.config.Timezone }

// Getter to get app debug status.
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

// Getter native exec SQL queries.
func (dbSession *DBSession) ExecSQL(ctx context.Context, query string, args ...any) (sql.Result, string, int, error) {
	result, message, httpCode, err := dbSession.database.ExecSQL(ctx, query, args...)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return result, "", httpCode, nil
}

// Getter native query SQL queries.
func (dbSession *DBSession) QuerySQL(ctx context.Context, query string, args ...any) (*sql.Rows, string, int, error) {
	rows, message, httpCode, err := dbSession.database.QuerySQL(ctx, query, args...)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return rows, "", httpCode, nil
}

// Getter native Mongo collectionqueries.
func (dbSession *DBSession) MongoCollection(collectionName string) (*mongo.Collection, string, int, error) {
	coll, message, httpCode, err := dbSession.database.Collection(dbSession.config, collectionName)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return coll, "", httpCode, nil
}

// Getter native Mongo create collection queries.
func (dbSession *DBSession) MongoCreateCollection(
	ctx context.Context,
	collectionName string,
	opts ...options.Lister[options.CreateCollectionOptions],
) (string, int, error) {
	message, httpCode, err := dbSession.database.CreateCollection(ctx, dbSession.config, collectionName, opts...)
	if err != nil {
		return message, httpCode, fmt.Errorf("%w", err)
	}

	return "", httpCode, nil
}

// Getter native exec Scylla queries.
func (dbSession *DBSession) ExecCQL(ctx context.Context, query string, args ...any) (string, int, error) {
	message, httpCode, err := dbSession.database.ExecCQL(ctx, query, args...)
	if err != nil {
		return message, httpCode, fmt.Errorf("%w", err)
	}

	return "", httpCode, nil
}

// Getter native query Scylla queries.
func (dbSession *DBSession) QueryCQL(ctx context.Context, query string, args ...any) (*gocql.Iter, string, int, error) {
	iter, message, httpCode, err := dbSession.database.QueryCQL(ctx, query, args...)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return iter, "", httpCode, nil
}

// QueryBuilderSQL defines the signature for raw SQL complex queries.
type QueryBuilderSQL func(ctx context.Context, db *sql.DB) error

// Escape Hatch: If handler needs direct raw SQL queries (e.g., for complex joins, CTEs, etc.)
func (dbSession *DBSession) RawQuerySQL(ctx context.Context, fn QueryBuilderSQL) error {
	if err := fn(ctx, dbSession.database.SQL); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// QueryBuilderMongo defines the signature for raw MongoDB complex queries.
type QueryBuilderMongo func(ctx context.Context, client *mongo.Client) error

// Escape Hatch: If handler needs direct raw MongoDB queries.
func (app *App) RawQueryMongo(ctx context.Context, fn QueryBuilderMongo) error {
	if err := fn(ctx, app.database.Mongo); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// QueryBuilderScylla defines the signature for raw ScyllaDB complex queries.
type QueryBuilderScylla func(ctx context.Context, session *gocql.Session) error

// Escape Hatch: If handler needs direct raw ScyllaDB queries.
func (dbSession *DBSession) RawQueryScylla(ctx context.Context, fn QueryBuilderScylla) error {
	if err := fn(ctx, dbSession.database.Scylla); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func (dbSession *DBSession) BeginTx(opts ...*sql.TxOptions) (*DBSession, string, int, error) {
	_, message, httpCode, err := dbSession.database.BeginTx(opts...)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return dbSession, "", httpCode, nil
}

func (dbSession *DBSession) Rollback() error {
	if dbSession.database.Tx == nil {
		return errors.New("rollback: no active transaction to rollback")
	}

	err := dbSession.database.Tx.Rollback()
	dbSession.database.Tx = nil

	return fmt.Errorf("%w", err)
}

func (dbSession *DBSession) Commit() error {
	if dbSession.database.Tx == nil {
		return errors.New("commit: no active transaction to commit")
	}

	err := dbSession.database.Tx.Commit()
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	dbSession.database.Tx = nil

	return nil
}

func (app *App) Close() error {
	if app.redis != nil {
		message, err := app.redis.Close()
		if err != nil {
			app.Log(Map{
				LogFieldKeyError: err,
			}).Error(message)

			return fmt.Errorf("%w", err)
		}
	}

	if app.database != nil {
		timeout := app.config.database.Timeout

		ctx, cancel := app.NewContext(timeout)
		defer cancel()

		message, _, err := app.database.Close(ctx, app.config.database)
		if err != nil {
			app.Log(Map{
				LogFieldKeyError: err,
			}).Error(message)

			return fmt.Errorf("failed to disconnect database: %w", err)
		}
	}

	if app.log != nil {
		message, err := app.log.Close()
		if err != nil {
			return fmt.Errorf("failed to disconnect logger: %w. %s", err, message)
		}
	}

	return nil
}

func (dbSession *DBSession) Table(table string) *orm.ORM {
	if dbSession.database == nil || dbSession.config == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ORM, the application must use one of the following databases: " +
			"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA"

		golog.Fatalf("Gorest detected ORM Query in your application, but database is not running. Message: %s", message)
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

// Getter to get log configuration.
func (app *App) GetLogConfig() log.Config {
	return *app.config.log
}

// Getter to get global logger.
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

// Getter to get redis configuration.
func (app *App) GetRedisConfig() gorestredis.Config {
	return *app.config.redis
}

// Getter to get redis query.
func (app *App) Redis() (*redis.Client, string, error) {
	client, message, err := app.redis.Redis()
	if err != nil {
		return nil, message, fmt.Errorf("%w", err)
	}

	return client, "", nil
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
