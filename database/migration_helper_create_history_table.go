package database

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func createHistoryTable(
	ctx context.Context,
	driver string,
	database *Database,
	config *Config,
	execType executionType,
	tableName,
	columnName string,
) (res []string, msg string, eror error) {
	var (
		executedRecords []string
		message         string
		err             error
	)

	switch driver {
	case MONGO:
		var colPtr *string

		if execType == typeMigration {
			colPtr = &columnName
		}

		if message, err = createMongoHistoryTable(ctx, database, config, tableName, colPtr); err != nil {
			return nil, message, fmt.Errorf("create MONGO %s history table failed: %w", execType, err)
		}

		executedRecords, message, err = getMongoRecords(ctx, database, config, tableName, columnName)
		if err != nil {
			return nil, message, fmt.Errorf("get %s %s history table failed: %w", driver, execType, err)
		}
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		if message, err := createSQLHistoryTable(ctx, database, driver, tableName, columnName); err != nil {
			return nil, message, fmt.Errorf("create %s %s history table failed: %w", driver, execType, err)
		}

		executedRecords, message, err = getSQLRecords(ctx, database, tableName, columnName)
		if err != nil {
			return nil, message, fmt.Errorf("get %s %s history table failed: %w", driver, execType, err)
		}
	case SCYLLA:
		if message, err := createScylaHistoryTable(ctx, database, tableName, columnName); err != nil {
			return nil, message, fmt.Errorf("create %s %s history table failed: %w", driver, execType, err)
		}

		executedRecords, message, err = getScyllaRecords(ctx, database, tableName, columnName)
		if err != nil {
			return nil, message, fmt.Errorf("get %s %s history table failed: %w", driver, execType, err)
		}
	default:
		message = "ensure your driver database configured using mysql, postgresql, mssql, oracle, sqlite, mongodb or scylladb"

		return nil, message, fmt.Errorf("%s process for driver %s not support", execType, driver)
	}

	return executedRecords, message, nil
}

// ======================================================================================
// createMongoHistoryTable
// ======================================================================================.
func createMongoHistoryTable(ctx context.Context, database *Database, config *Config, collectionName string, uniqueField *string) (string, error) {
	// Step 1: Create collection (idempotent)
	message, _, err := database.CreateCollection(ctx, config, collectionName)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return message, fmt.Errorf("create collection failed: %w", err)
		}
	}

	// Step 2: Create unique index kalau diperlukan
	if uniqueField != nil {
		coll, message, _, err := database.Collection(config, collectionName)
		if err != nil {
			return message, fmt.Errorf("get collection failed: %w", err)
		}

		indexModel := mongo.IndexModel{
			Keys:    bson.D{{Key: *uniqueField, Value: 1}},
			Options: options.Index().SetUnique(true),
		}

		_, err = coll.Indexes().CreateOne(ctx, indexModel)
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return fmt.Sprintf("failed to create unique index on '%s'", *uniqueField), err
		}
	}

	return "", nil
}

// ======================================================================================
// createSQLHistoryTable - Generic helper for migration & seeder history
// ======================================================================================.
func createSQLHistoryTable(ctx context.Context, database *Database, driver, tableName, columnName string) (string, error) {
	var createTableSQL string

	switch driver {
	case MYSQL, POSTGRES, SQLITE:
		createTableSQL = fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			%s VARCHAR(255) NOT NULL PRIMARY KEY,
			executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
		`, tableName, columnName)
	case SQLSERVER:
		createTableSQL = fmt.Sprintf(`
		IF NOT EXISTS (SELECT * FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = '%s')
		CREATE TABLE %s (
			%s VARCHAR(255) NOT NULL PRIMARY KEY,
			executed_at DATETIME DEFAULT GETDATE()
		)
		`, tableName, tableName, columnName)
	case ORACLE:
		createTableSQL = fmt.Sprintf(`
        DECLARE
            v_count NUMBER;
        BEGIN
            SELECT COUNT(*) INTO v_count 
            FROM user_tables 
            WHERE table_name = UPPER('%s');

            IF v_count = 0 THEN
                EXECUTE IMMEDIATE '
                    CREATE TABLE %s (
                        %s VARCHAR2(255) NOT NULL PRIMARY KEY,
                        executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
                    )
                ';
            END IF;
        END;
        `, tableName, tableName, columnName)
	default:
		message := fmt.Sprintf("the database driver '%s' is not supported", driver)
		return message, fmt.Errorf("unsupported database driver: %s", driver)
	}

	if _, message, _, err := database.ExecSQL(ctx, createTableSQL); err != nil {
		msg := fmt.Sprintf("failed to create '%s' table in %s. %s", tableName, driver, message)
		return msg, fmt.Errorf("create table failed: %w", err)
	}

	return "", nil
}

// ======================================================================================
// createScyllaHistoryTable
// ======================================================================================.
func createScylaHistoryTable(ctx context.Context, database *Database, keyspaceName, columnName string) (string, error) {
	// Step 1: Create collection (idempotent)
	createTableCQL := fmt.Sprintf(`
    CREATE TABLE IF NOT EXISTS %s (
        %s text,
        executed_at timestamp,
        PRIMARY KEY (%s)
    );
    `, keyspaceName, columnName, columnName)

	if message, _, err := database.ExecCQL(ctx, createTableCQL); err != nil {
		msg := fmt.Sprintf("failed to execute CQL statement for creating the 'migration_history' table in ScyllaDB. "+
			"Verify database keyspace, user privileges, connection state, or %s", message)

		return msg, fmt.Errorf("%w", err)
	}

	return "", nil
}
