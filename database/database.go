package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func Run(cfg Config) Config {
	return configDefault(cfg)
}

func New(ctx context.Context, config *Config, timezone string) (*Database, string, error) {
	if config == nil {
		const message = "ensure that you have run database.Run(database.Config{...} in your app"

		return nil, message, errors.New("database has not been run in your app")
	}

	database, message, err := execConn(ctx, config, timezone, config.Driver)
	if err != nil {
		return nil, message, err
	}

	if database == nil {
		message := "ensure the database configuration is set up correctly and " +
			"the application must use one of the following databases: " +
			"MySQL, PostgreSQL, SQL Server, SQLite, MongoDB, and Scylla. " +
			"Gorest does not currently support any database types other than these."

		return nil, message, errors.New("gorest error configuring database")
	}

	return database, "", nil
}

//nolint:revive
func execConn(ctx context.Context, config *Config, timezone string, driver string) (*Database, string, error) {
	switch driver {
	case MYSQL:
		mysqlConn, message, err := mysqlConnection(ctx, config, timezone, driver)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(mysqlConn, nil, nil), "", nil

	case POSTGRES:
		postgreConn, message, err := postgresqlConnection(ctx, config, timezone, driver)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(postgreConn, nil, nil), "", nil

	case SQLSERVER:
		mssqlConn, message, err := mssqlConnection(ctx, config, timezone, driver)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(mssqlConn, nil, nil), "", nil

	case SQLITE:
		sqliteConn, message, err := sqliteConnection(ctx, config, timezone, driver)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(sqliteConn, nil, nil), "", nil

	case ORACLE:
		oracleConn, message, err := oracleConnection(ctx, config, timezone, driver)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(oracleConn, nil, nil), "", nil

	case MONGO:
		mongoConn, message, err := mongoConnection(ctx, config, driver)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(nil, mongoConn, nil), "", nil

	case SCYLLA:
		scyllaConn, message, err := scyllaConnection(config, driver)
		if err != nil {
			return nil, message, fmt.Errorf("%w", err)
		}

		return initDB(nil, nil, scyllaConn), "", nil

	default:
		message := "The application must use one of the following databases: " +
			"MySQL, PostgreSQL, SQL Server, SQLite, MongoDB, and Scylla. " +
			"Gorest does not currently support any database types other than these."

		return nil, message, fmt.Errorf("database driver %q is not supported", driver)
	}
}

func initDB(sqlDB *sql.DB, mongoDB *mongo.Client, scylla *gocql.Session) *Database {
	return &Database{
		SQL:    sqlDB,
		Mongo:  mongoDB,
		Scylla: scylla,
	}
}
