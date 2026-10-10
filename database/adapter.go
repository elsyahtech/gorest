package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TransactionFunc is the callback function that executes within a transaction
// It receives the transaction object and should return an error.
type TransactionFunc func(ctx context.Context, tx *sql.Tx) error

// ========================================================================================================
// MYSQL, POSTGRES, SQLSERVER, ORACLE and SQLITE
// ========================================================================================================.
func (db *Database) ExecSQL(ctx context.Context, query string, args ...any) (sql.Result, string, int, error) {
	if db == nil || db.SQL == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"To use ExecSQL, the application must use one of the following databases: "+
				"MYSQL, POSTGRES, SQLSERVER, ORACLE, or SQLITE.",
			errors.New("execSQL: SQL database service is not running"),
		)

		return nil, message, code, err
	}

	if ctx == nil || query == "" {
		message, code, err := setError(
			"Ensure the context and SQL query are not empty.",
			errors.New("execSQL: context and SQL query cannot be empty"),
			http.StatusBadRequest,
		)

		return nil, message, code, err
	}

	var (
		result sql.Result
		err    error
	)

	if db.Tx != nil {
		result, err = db.Tx.ExecContext(ctx, query, args...)
	} else {
		result, err = db.SQL.ExecContext(ctx, query, args...)
	}

	if err != nil {
		message := "Ensure the target table exists and is accessible, " +
			"and check that your SQL syntax, table names, column names, and parameter types are correct."
		code := http.StatusInternalServerError

		if IsDuplicateKeyError(err) {
			message = "A record with the same value for a primary key or unique column already exists."
			code = http.StatusConflict
		}

		message, code, err = setError(message, fmt.Errorf("execSQL: %w", err), code)

		return nil, message, code, err
	}

	return result, "", http.StatusOK, nil
}

func (db *Database) QuerySQL(ctx context.Context, query string, args ...any) (*sql.Rows, string, int, error) {
	if db == nil || db.SQL == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"To use ExecSQL, the application must use one of the following databases: "+
				"MYSQL, POSTGRES, SQLSERVER, ORACLE, or SQLITE.",
			errors.New("querySQL: SQL database service is not running"),
		)

		return nil, message, code, err
	}

	if ctx == nil || query == "" {
		message, code, err := setError(
			"Ensure the context and SQL query are not empty.",
			errors.New("querySQL: context and SQL query cannot be empty"),
			http.StatusBadRequest,
		)

		return nil, message, code, err
	}

	var (
		result *sql.Rows
		err    error
	)

	if db.Tx != nil {
		result, err = db.Tx.QueryContext(ctx, query, args...)
	} else {
		result, err = db.SQL.QueryContext(ctx, query, args...)
	}

	if err != nil {
		message := "Ensure the target table exists and is accessible, " +
			"and check that your SQL syntax, table names, column names, and parameter types are correct."
		code := http.StatusInternalServerError

		if IsDuplicateKeyError(err) {
			message = "A record with the same value for a primary key or unique column already exists."
			code = http.StatusConflict
		}

		message, code, err = setError(message, fmt.Errorf("querySQL: %w", err), code)

		return nil, message, code, err
	}

	return result, "", http.StatusOK, nil
}

func (db *Database) BeginTx(opts ...*sql.TxOptions) (*Database, string, int, error) {
	if db == nil || db.SQL == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"To use ExecSQLTx, the application must use one of the following databases: "+
				"MYSQL, POSTGRES, SQLSERVER, ORACLE, or SQLITE.",
			errors.New("BeginTx: SQL database service is not running"),
		)

		return nil, message, code, err
	}

	if db.Tx != nil {
		message, code, err := setError(
			"Ensure you properly commit or rollback the previous transaction (using Commit() or Rollback()) "+
				"before starting a new one, or avoid calling BeginTx multiple times consecutively.",
			errors.New("BeginTx: a transaction is already active in this database session"),
			http.StatusConflict,
		)

		return nil, message, code, err
	}

	var opt *sql.TxOptions

	if len(opts) > 0 && opts[0] != nil {
		opt = opts[0]
	}

	transaction, err := db.SQL.BeginTx(context.Background(), opt)
	if err != nil {
		message, code, err := setError(
			"Check your database connection health, verify that the database server is running and reachable",
			fmt.Errorf("BeginTx: failed to start transaction: %w", err),
		)

		return nil, message, code, err
	}

	txDB := *db
	txDB.Tx = transaction

	return &txDB, "", http.StatusOK, nil
}

// ========================================================================================================
// MONGO
// ========================================================================================================.
func (db *Database) Collection(config *Config, collectionName string) (*mongo.Collection, string, int, error) {
	if db == nil || db.Mongo == nil || config == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"To use Collection, the application must use MONGO",
			errors.New("collection: mongo service is not running"),
		)

		return nil, message, code, err
	}

	if collectionName == "" {
		message, code, err := setError(
			"ensure collection name is not empty.",
			errors.New("collection: collection name cannot be empty"),
			http.StatusBadRequest,
		)

		return nil, message, code, err
	}

	return db.Mongo.Database(config.Name).Collection(collectionName), "", http.StatusOK, nil
}

// CreateCollection creates a new collection in MongoDB with optional configurations (like schema validator).
func (db *Database) CreateCollection(
	ctx context.Context,
	config *Config,
	collectionName string,
	opts ...options.Lister[options.CreateCollectionOptions],
) (string, int, error) {
	if db == nil || db.Mongo == nil || config == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"To use Collection, the application must use MONGO",
			errors.New("createCollection: mongo service is not running"),
		)

		return message, code, err
	}

	if ctx == nil || collectionName == "" {
		message, code, err := setError(
			"ensure the context and collection name are not empty",
			errors.New("createCollection: context and collection name cannot be empty"),
			http.StatusBadRequest,
		)

		return message, code, err
	}

	err := db.Mongo.Database(config.Name).CreateCollection(ctx, collectionName, opts...)
	if err != nil {
		message, code, err := setError(
			"verify the database user permissions, network state, or if the collection already exists",
			fmt.Errorf("createCollection: %w", err),
		)

		return message, code, err
	}

	return "", http.StatusOK, nil
}

// ========================================================================================================
// SCYLLA
// ========================================================================================================.
func (db *Database) ExecCQL(ctx context.Context, query string, args ...any) (string, int, error) {
	if db == nil || db.Scylla == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"To use ExecCQL, the application must use SCYLLA",
			errors.New("execCQL: scylla service is not running"),
		)

		return message, code, err
	}

	if ctx == nil || query == "" {
		message, code, err := setError(
			"ensure the context and CQL query are not empty",
			errors.New("execCQL: context and CQL query cannot be empty"),
			http.StatusBadRequest,
		)

		return message, code, err
	}

	err := db.Scylla.Query(query, args...).ExecContext(ctx)
	if err != nil {
		message, code, err := setError(
			"Ensure the target table exists and is accessible, "+
				"and check that your CQL syntax, table names, column names, partition keys and parameter types are correct.",
			fmt.Errorf("execCQL: %w", err),
		)

		return message, code, err
	}

	return "", http.StatusOK, nil
}

func (db *Database) QueryCQL(ctx context.Context, query string, args ...any) (*gocql.Iter, string, int, error) {
	if db == nil || db.Scylla == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"To use ExecCQL, the application must use SCYLLA",
			errors.New("queryCQL: scylla service is not running"),
		)

		return nil, message, code, err
	}

	if ctx == nil || query == "" {
		message, code, err := setError(
			"ensure the context and CQL query are not empty",
			errors.New("queryCQL: context and CQL query cannot be empty"),
			http.StatusBadRequest,
		)

		return nil, message, code, err
	}

	return db.Scylla.Query(query, args...).IterContext(ctx), "", http.StatusOK, nil
}

func (db *Database) ScanCQL(ctx context.Context, query string, args ...any) (applied bool, troubleshoot string, httpCode int, err error) {
	if db == nil || db.Scylla == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. To use ScanCQL, the application must use SCYLLA.",
			errors.New("scanCQL: scylla service is not running"),
		)

		return false, message, code, err
	}

	if ctx == nil || query == "" {
		message, code, err := setError(
			"ensure the context and CQL query are not empty",
			errors.New("scanCQL: context and CQL query cannot be empty"),
			http.StatusBadRequest,
		)

		return false, message, code, err
	}

	qry := db.Scylla.Query(query, args...)

	pyld := make(map[string]any, 10)

	applied, err = qry.MapScanCASContext(ctx, pyld)
	if err != nil {
		message, code, err := setError(
			"Ensure that the record exists.",
			err,
		)

		return false, message, code, err
	}

	return applied, "", http.StatusOK, nil
}

func (db *Database) ExecBatchCQL(ctx context.Context, queries []string, batchArgs [][]any) (string, int, error) {
	if db == nil || db.Scylla == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"To use ExecuteBatchCQL, the application must use SCYLLA",
			errors.New("executeBatchCQL: scylla service is not running"),
		)

		return message, code, err
	}

	if ctx == nil || len(queries) == 0 || len(queries) != len(batchArgs) {
		message, code, err := setError(
			"ensure the context is valid and queries/arguments list match and are not empty",
			errors.New("executeBatchCQL: invalid batch parameters"),
			http.StatusBadRequest,
		)

		return message, code, err
	}

	batch := db.Scylla.Batch(gocql.UnloggedBatch)

	for idx, query := range queries {
		queryStr := strings.TrimSpace(query)

		if queryStr == "" {
			continue
		}

		queryStr = strings.TrimSuffix(queryStr, ";")

		if idx < len(batchArgs) {
			args := batchArgs[idx]
			batch.Query(queryStr, args...)
		}
	}

	err := batch.ExecContext(ctx)
	if err != nil {
		message, code, err := setError(
			"Ensure that your CQL batch syntax, table names, and parameter bindings are correct.",
			fmt.Errorf("executeBatchCQL: %w", err),
		)

		return message, code, err
	}

	return "", http.StatusOK, nil
}

//nolint:revive
func (db *Database) Close(ctx context.Context, cfg *Config) (string, int, error) {
	if db == nil || cfg == nil {
		message, code, err := setError(
			"Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"The application must use one of the following databases: "+
				"MYSQL, POSTGRES, SQLSERVER, ORACLE, SQLITE, MONGO, SCYLLA",
			errors.New("close: database service is not running"),
		)
		return message, code, err
	}

	if ctx == nil {
		message, code, err := setError(
			"Ensure the context is not empty",
			errors.New("context cannot be empty"),
			http.StatusBadRequest,
		)

		return message, code, err
	}

	var (
		errs     []error
		messages []string
	)

	activeDriver := cfg.Driver

	switch activeDriver {
	case MYSQL, POSTGRES, SQLITE, SQLSERVER, ORACLE:
		if db.SQL == nil {
			messages = append(messages, "Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"The application must use one of the following databases: "+
				"MYSQL, POSTGRES, SQLSERVER, ORACLE, or SQLITE.")

			errs = append(errs, errors.New("close SQL connection: SQL database service is not running"))
		} else if err := db.SQL.Close(); err != nil {
			messages = append(messages, "Ensure that all active database transactions or prepared statements "+
				"are finished/closed before shutting down the SQL connection.")

			errs = append(errs, fmt.Errorf("close SQL connection: %w", err))
		}
	case MONGO:
		if db.Mongo == nil {
			messages = append(messages, "Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"The application must use MONGO")

			errs = append(errs, errors.New("close MongoDB connection: Mongo database service is not running"))
		} else if err := db.Mongo.Disconnect(ctx); err != nil {
			messages = append(messages, "Ensure that the context timeout is sufficient and MongoDB client "+
				"sessions/cursors are properly released before disconnecting.")

			errs = append(errs, fmt.Errorf("close MongoDB connection: %w", err))
		}
	case SCYLLA:
		if db.Scylla == nil {
			messages = append(messages, "Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"The application must use SCYLLA")

			errs = append(errs, errors.New("close Scylla connection: scylla database service is not running"))
		} else {
			db.Scylla.Close()
		}

		if db.Scylla != nil && !db.Scylla.Closed() {
			messages = append(messages, "Ensure that all ScyllaDB query sessions are fully terminated "+
				"and background workers/cluster rings have finished executing.")

			errs = append(errs, errors.New("failed to cleanly disconnect ScyllaDB connection"))
		}
	default:
		message := "The application must use one of the following databases: " +
			"MySQL, PostgreSQL, SQL Server, SQLite, MongoDB, and Scylla. " +
			"Gorest does not currently support any database types other than these."

		msg, code, err := setError(message, fmt.Errorf("database driver %q is not supported", activeDriver))
		return msg, code, err
	}

	if len(errs) > 0 {
		combinedMsg := "failed to close one or more database connections. "

		if len(messages) > 0 {
			combinedMsg += messages[0] // Bisa disesuaikan jika ingin menggabungkan semua pesan
		}

		msg, code, err := setError(combinedMsg, errors.Join(errs...))
		return msg, code, err
	}

	return "", http.StatusOK, nil
}
