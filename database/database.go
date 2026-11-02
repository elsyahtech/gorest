package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/elsyahtech/gorest/monitoring/dbmetrics"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func Run(cfg Config) Config {
	return configDefault(cfg)
}

func New(ctx context.Context, config *Config, timezone string) (*Database, string, error) {
	if ctx == nil {
		return nil, "provide a non-nil startup context", errors.New("database startup context is nil")
	}

	if ctx.Err() != nil {
		message := "provide a valid startup context before running the database"
		return nil, message, fmt.Errorf("database startup context is canceled or expired: %w", ctx.Err())
	}

	if config == nil {
		const message = "Register the database config with app.Use(database.Run(...))."

		return nil, message, errors.New("database service is not running")
	}

	metrics := dbmetrics.New()

	database, message, err := execConn(ctx, config, timezone, config.Driver, metrics)
	if err != nil {
		return nil, message, err
	}

	if database == nil {
		message := "ensure the database configuration is set up correctly and " +
			"the application must use one of the following databases: " +
			"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO, SCYLLA. " +
			"Gorest does not currently support any database types other than these."

		return nil, message, errors.New("gorest error configuring database")
	}

	return database, "", nil
}

//nolint:revive
func execConn(ctx context.Context, config *Config, timezone string, driver string, metrics *dbmetrics.Stats) (*Database, string, error) {
	switch driver {
	case MYSQL:
		mysqlConn, message, err := mysqlConnection(ctx, config, timezone, driver, metrics)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(mysqlConn, nil, nil, metrics), "", nil

	case POSTGRES:
		postgreConn, message, err := postgresqlConnection(ctx, config, timezone, driver, metrics)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(postgreConn, nil, nil, metrics), "", nil

	case SQLSERVER:
		mssqlConn, message, err := mssqlConnection(ctx, config, timezone, driver, metrics)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(mssqlConn, nil, nil, metrics), "", nil

	case SQLITE:
		sqliteConn, message, err := sqliteConnection(ctx, config, timezone, driver, metrics)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(sqliteConn, nil, nil, metrics), "", nil

	case ORACLE:
		oracleConn, message, err := oracleConnection(ctx, config, timezone, driver, metrics)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(oracleConn, nil, nil, metrics), "", nil

	case MONGO:
		mongoConn, message, err := mongoConnection(ctx, config, driver, metrics)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(nil, mongoConn, nil, metrics), "", nil

	case SCYLLA:
		scyllaConn, message, err := scyllaConnection(config, driver, metrics)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(nil, nil, scyllaConn, metrics), "", nil

	default:
		message := "The application must use one of the following databases: " +
			"MySQL, PostgreSQL, SQL Server, SQLite, MongoDB, and Scylla. " +
			"Gorest does not currently support any database types other than these."

		return nil, message, fmt.Errorf("database driver %q is not supported", driver)
	}
}

func initDB(sqlDB *sql.DB, mongoDB *mongo.Client, scylla *gocql.Session, metrics *dbmetrics.Stats) *Database {
	return &Database{
		SQL:     sqlDB,
		Mongo:   mongoDB,
		Scylla:  scylla,
		metrics: metrics,
	}
}
