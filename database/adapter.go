package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
func (db *Database) ExecSQL(ctx context.Context, query string, args ...any) (sql.Result, string, error) {
	if db.SQL == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ExecSQL, the application must use one of the following databases: " +
			"MYSQL, POSTGRES, SQLSERVER, ORACLE, or SQLITE."

		return nil, message, errors.New("execSQL: SQL database service is not running")
	}

	if ctx == nil || query == "" {
		const message = "Ensure the context and SQL query are not empty."

		return nil, message, errors.New("execSQL: context and SQL query cannot be empty")
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

		return nil, message, fmt.Errorf("execSQL: %w", err)
	}

	return result, "", nil
}

func (db *Database) QuerySQL(ctx context.Context, query string, args ...any) (*sql.Rows, string, error) {
	if db.SQL == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ExecSQL, the application must use one of the following databases: " +
			"MYSQL, POSTGRES, SQLSERVER, ORACLE, or SQLITE."

		return nil, message, errors.New("querySQL: SQL database service is not running")
	}

	if ctx == nil || query == "" {
		const message = "Ensure the context and SQL query are not empty."

		return nil, message, errors.New("querySQL: context and SQL query cannot be empty")
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

		return nil, message, fmt.Errorf("querySQL: %w", err)
	}

	return result, "", nil
}

func (db *Database) BeginTx(opts ...*sql.TxOptions) (*Database, string, error) {
	if db == nil || db.SQL == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ExecSQLTx, the application must use one of the following databases: " +
			"MYSQL, POSTGRES, SQLSERVER, ORACLE, or SQLITE."

		return nil, message, errors.New("BeginTx: SQL database service is not running")
	}

	if db.Tx != nil {
		const message = "Ensure you properly commit or rollback the previous transaction (using Commit() or Rollback()) " +
			"before starting a new one, or avoid calling BeginTx multiple times consecutively."

		return nil, message, errors.New("BeginTx: a transaction is already active in this database session")
	}

	var opt *sql.TxOptions
	if len(opts) > 0 && opts[0] != nil {
		opt = opts[0]
	}

	transaction, err := db.SQL.BeginTx(context.Background(), opt)
	if err != nil {
		const message = "Check your database connection health, verify that the database server is running and reachable"

		return nil, message, fmt.Errorf("BeginTx: failed to start transaction: %w", err)
	}

	db.Tx = transaction

	return db, "", nil
}

// ========================================================================================================
// MONGO
// ========================================================================================================.
func (db *Database) Collection(config *Config, collectionName string) (*mongo.Collection, string, error) {
	if db == nil || db.Mongo == nil || config == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use Collection, the application must use MONGO"

		return nil, message, errors.New("collection: mongo service is not running")
	}

	if collectionName == "" {
		const message = "ensure collection name is not empty."

		return nil, message, errors.New("collection: collection name cannot be empty")
	}

	return db.Mongo.Database(config.Name).Collection(collectionName), "", nil
}

// CreateCollection creates a new collection in MongoDB with optional configurations (like schema validator).
func (db *Database) CreateCollection(
	ctx context.Context,
	config *Config,
	collectionName string,
	opts ...options.Lister[options.CreateCollectionOptions],
) (string, error) {
	if db == nil || db.Mongo == nil || config == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use Collection, the application must use MONGO"

		return message, errors.New("createCollection: mongo service is not running")
	}

	if ctx == nil || collectionName == "" {
		const message = "ensure the context and collection name are not empty"

		return message, errors.New("createCollection: context and collection name cannot be empty")
	}

	err := db.Mongo.Database(config.Name).CreateCollection(ctx, collectionName, opts...)
	if err != nil {
		const message = "verify the database user permissions, network state, or if the collection already exists"

		return message, fmt.Errorf("createCollection: %w", err)
	}

	return "", nil
}

// ========================================================================================================
// SCYLLA
// ========================================================================================================.
func (db *Database) ExecCQL(ctx context.Context, query string, args ...any) (string, error) {
	if db == nil || db.Scylla == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ExecCQL, the application must use SCYLLA"

		return message, errors.New("execCQL: scylla service is not running")
	}

	if ctx == nil || query == "" {
		const message = "ensure the context and CQL query are not empty"

		return message, errors.New("execCQL: context and CQL query cannot be empty")
	}

	err := db.Scylla.Query(query, args...).ExecContext(ctx)
	if err != nil {
		message := "Ensure the target table exists and is accessible, " +
			"and check that your CQL syntax, table names, column names, partition keys and parameter types are correct."

		return message, fmt.Errorf("execCQL: %w", err)
	}

	return "", nil
}

func (db *Database) QueryCQL(ctx context.Context, query string, args ...any) (*gocql.Iter, string, error) {
	if db == nil || db.Scylla == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ExecCQL, the application must use SCYLLA"

		return nil, message, errors.New("queryCQL: scylla service is not running")
	}

	if ctx == nil || query == "" {
		const message = "ensure the context and CQL query are not empty"

		return nil, message, errors.New("queryCQL: context and CQL query cannot be empty")
	}

	return db.Scylla.Query(query, args...).IterContext(ctx), "", nil
}

func (db *Database) ScanCQL(ctx context.Context, query string, args ...any) (applied bool, troubleshoot string, err error) {
	if ctx == nil || query == "" {
		const message = "ensure the context and CQL query are not empty"

		return false, message, errors.New("scanCQL: context and CQL query cannot be empty")
	}

	qry := db.Scylla.Query(query, args...)

	pyld := make(map[string]any, 10)

	applied, err = qry.MapScanCASContext(ctx, pyld)
	if err != nil {
		const message = "Ensure that the record exists."

		return false, message, fmt.Errorf("%w", err)
	}

	return applied, "", nil
}

func (db *Database) ExecBatchCQL(ctx context.Context, queries []string, batchArgs [][]any) (string, error) {
	if db == nil || db.Scylla == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ExecuteBatchCQL, the application must use SCYLLA"

		return message, errors.New("executeBatchCQL: scylla service is not running")
	}

	if ctx == nil || len(queries) == 0 || len(queries) != len(batchArgs) {
		const message = "ensure the context is valid and queries/arguments list match and are not empty"

		return message, errors.New("executeBatchCQL: invalid batch parameters")
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
		const message = "Ensure that your CQL batch syntax, table names, and parameter bindings are correct."

		return message, fmt.Errorf("executeBatchCQL: %w", err)
	}

	return "", nil
}

//nolint:revive
func (db *Database) Close(ctx context.Context, cfg *Config) (string, error) {
	if db == nil || cfg == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"The application must use one of the following databases: " +
			"MYSQL, POSTGRES, SQLSERVER, ORACLE, SQLITE, MONGO, SCYLLA"

		return message, errors.New("close: database service is not running")
	}

	if ctx == nil {
		const message = "Ensure the context is not empty"

		return message, errors.New("context cannot be empty")
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
		}

		if err := db.SQL.Close(); err != nil {
			messages = append(messages, "Ensure that all active database transactions or prepared statements "+
				"are finished/closed before shutting down the SQL connection.")

			errs = append(errs, fmt.Errorf("close SQL connection: %w", err))
		}
	case MONGO:
		if db.Mongo == nil {
			messages = append(messages, "Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"The application must use MONGO")

			errs = append(errs, errors.New("close MongoDB connection: Mongo database service is not running"))
		}

		if err := db.Mongo.Disconnect(ctx); err != nil {
			messages = append(messages, "Ensure that the context timeout is sufficient and MongoDB client "+
				"sessions/cursors are properly released before disconnecting.")

			errs = append(errs, fmt.Errorf("close MongoDB connection: %w", err))
		}
	case SCYLLA:
		if db.Scylla == nil {
			messages = append(messages, "Ensure that you have run database.Run(database.Config{...}) in your app. "+
				"The application must use SCYLLA")

			errs = append(errs, errors.New("close Scylla connection: scylla database service is not running"))
		}

		db.Scylla.Close()

		if !db.Scylla.Closed() {
			messages = append(messages, "Ensure that all ScyllaDB query sessions are fully terminated "+
				"and background workers/cluster rings have finished executing.")

			errs = append(errs, errors.New("failed to cleanly disconnect ScyllaDB connection"))
		}
	default:
		message := "The application must use one of the following databases: " +
			"MySQL, PostgreSQL, SQL Server, SQLite, MongoDB, and Scylla. " +
			"Gorest does not currently support any database types other than these."

		return message, fmt.Errorf("database driver %q is not supported", activeDriver)
	}

	if len(errs) > 0 {
		combinedMsg := "failed to close one or more database connections. "

		if len(messages) > 0 {
			combinedMsg += messages[0] // Bisa disesuaikan jika ingin menggabungkan semua pesan
		}

		return combinedMsg, errors.Join(errs...)
	}

	return "", nil
}
