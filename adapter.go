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
func (dbSession *DBSession) ExecSQL(ctx context.Context, query string, args ...any) (sql.Result, string, error) {
	result, message, err := dbSession.database.ExecSQL(ctx, query, args...)
	if err != nil {
		return nil, message, fmt.Errorf("%w", err)
	}

	return result, "", nil
}

// Getter native query SQL queries.
func (dbSession *DBSession) QuerySQL(ctx context.Context, query string, args ...any) (*sql.Rows, string, error) {
	rows, message, err := dbSession.database.QuerySQL(ctx, query, args...)
	if err != nil {
		return nil, message, fmt.Errorf("%w", err)
	}

	return rows, "", nil
}

// Getter native Mongo collectionqueries.
func (app *App) MongoCollection(collectionName string) (*mongo.Collection, string, error) {
	coll, message, err := app.database.Collection(app.config.database, collectionName)
	if err != nil {
		return nil, message, fmt.Errorf("%w", err)
	}

	return coll, "", nil
}

// Getter native Mongo create collection queries.
func (app *App) MongoCreateCollection(
	ctx context.Context,
	collectionName string,
	opts ...options.Lister[options.CreateCollectionOptions],
) (string, error) {
	message, err := app.database.CreateCollection(ctx, app.config.database, collectionName, opts...)
	if err != nil {
		return message, fmt.Errorf("%w", err)
	}

	return "", nil
}

// Getter native exec Scylla queries.
func (app *App) ExecCQL(ctx context.Context, query string, args ...any) (string, error) {
	message, err := app.database.ExecCQL(ctx, query, args...)
	if err != nil {
		return message, fmt.Errorf("%w", err)
	}

	return "", nil
}

// Getter native query Scylla queries.
func (app *App) QueryCQL(ctx context.Context, query string, args ...any) (*gocql.Iter, string, error) {
	iter, message, err := app.database.QueryCQL(ctx, query, args...)
	if err != nil {
		return nil, message, fmt.Errorf("%w", err)
	}

	return iter, "", nil
}

// QueryBuilderSQL defines the signature for raw SQL complex queries.
type QueryBuilderSQL func(ctx context.Context, db *sql.DB) error

// Escape Hatch: If handler needs direct raw SQL queries (e.g., for complex joins, CTEs, etc.)
func (app *App) RawQuerySQL(ctx context.Context, fn QueryBuilderSQL) error {
	if err := fn(ctx, app.database.SQL); err != nil {
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
func (app *App) RawQueryScylla(ctx context.Context, fn QueryBuilderScylla) error {
	if err := fn(ctx, app.database.Scylla); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func (dbSession *DBSession) BeginTx(opts ...*sql.TxOptions) (*DBSession, string, error) {
	_, message, err := dbSession.database.BeginTx(opts...)
	if err != nil {
		return nil, message, fmt.Errorf("%w", err)
	}

	return dbSession, "", nil
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

		message, err := app.database.Close(ctx, app.config.database)
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
	return &orm.ORM{
		Error:          nil,
		Database:       dbSession.database,
		Tx:             dbSession.database.Tx,
		StringBuilder:  &strings.Builder{},
		Result:         nil,
		DatabaseConfig: dbSession.config,
		Context:        context.Background(),
		Message:        "",
		Table:          &table,

		RowsAffected: 0,
		LastInsertId: "",

		SelectedCols:       nil,
		IsUpsert:           false,
		UpsertConflictCols: nil,
		UpsertUpdateCols:   nil,
		WhereClauses:       nil,
		WhereArgs:          nil,
		JoinClauses:        nil,
		PreloadClauses:     nil,
		Preloads:           nil,

		OrderByClauses: nil,
		LimitVal:       0,
		OffsetVal:      0,
		IsDistinct:     false,

		IsReturn:   false,
		ReturnDest: nil,
		ReturnCols: nil,
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
} //nolint:revive
