package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const migrationRollbackTimeout = 30 * time.Second

type mongoMigrationCompensation struct {
	DropCollections []string                 `json:"dropCollections"`
	DropIndexes     []mongoIndexCompensation `json:"dropIndexes"`
}

type mongoIndexCompensation struct {
	Collection string `json:"collection"`
	Name       string `json:"name"`
}

type structRollbackExecutionBatch struct {
	database             *Database
	config               *Config
	driver               string
	execType             executionType
	tableName            string
	columnName           string
	failed               File
	completed            []File
	failedFileRolledBack bool
}

func rollbackExecutionBatch(req *structRollbackExecutionBatch) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), migrationRollbackTimeout)
	defer cancel()

	attempts := append(append([]File(nil), req.completed...), req.failed)

	var rollbackErrors []error

	for index := len(attempts) - 1; index >= 0; index-- {
		file := attempts[index]
		isFailedFile := index == len(attempts)-1

		//nolint:staticcheck
		if !(isFailedFile && req.failedFileRolledBack) {
			if strings.TrimSpace(file.CompensationContent) == "" {
				rollbackErrors = append(rollbackErrors,
					fmt.Errorf("%s %q has no matching .down compensation file", req.execType, file.Name))

				continue
			}

			if err := executeCompensation(ctx, req.database, req.config, req.driver, req.execType, file); err != nil {
				rollbackErrors = append(rollbackErrors,
					fmt.Errorf("execute compensation for %s %q: %w", req.execType, file.Name, err))

				continue
			}
		}

		if err := deleteExecutionHistory(ctx, req.database, req.config, req.driver, req.tableName, req.columnName, file.Name); err != nil {
			rollbackErrors = append(rollbackErrors,
				fmt.Errorf("remove execution history for %s %q: %w", req.execType, file.Name, err))
		}

		if err := deleteMigrationState(ctx, req.database, req.config, req.driver, string(req.execType)+":"+file.Name); err != nil {
			rollbackErrors = append(rollbackErrors,
				fmt.Errorf("remove execution state for %s %q: %w", req.execType, file.Name, err))
		}
	}

	return len(rollbackErrors) == 0, errors.Join(rollbackErrors...)
}

func executeCompensation(ctx context.Context, database *Database, config *Config, driver string, execType executionType, file File) error {
	switch driver {
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		txDatabase, _, _, err := database.BeginTxContext(ctx, config)
		if err != nil {
			return fmt.Errorf("begin compensation transaction: %w", err)
		}

		if _, _, _, err := txDatabase.ExecSQL(ctx, config, file.CompensationContent); err != nil {
			return errors.Join(fmt.Errorf("execute SQL compensation: %w", err), txDatabase.Tx.Rollback())
		}

		if err := txDatabase.Tx.Commit(); err != nil {
			return errors.Join(fmt.Errorf("commit SQL compensation: %w", err), txDatabase.Tx.Rollback())
		}

		return nil
	case SCYLLA:
		for _, statement := range splitStatements(file.CompensationContent) {
			if _, _, err := database.ExecCQL(ctx, config, statement); err != nil {
				return fmt.Errorf("execute CQL compensation statement %q: %w", statement, err)
			}
		}

		return nil
	case MONGO:
		if execType == typeMigration {
			return executeMongoMigrationCompensation(ctx, database, config, file)
		}

		return executeMongoSeederCompensation(ctx, database, config, file)
	default:
		return fmt.Errorf("unsupported compensation driver %q", driver)
	}
}

func executeMongoMigrationCompensation(ctx context.Context, database *Database, config *Config, file File) error {
	var actions mongoMigrationCompensation

	if err := decodeCompensationJSON(file.CompensationContent, &actions); err != nil {
		return fmt.Errorf("parse MongoDB migration compensation JSON: %w", err)
	}

	if len(actions.DropCollections) == 0 && len(actions.DropIndexes) == 0 {
		return errors.New("MongoDB migration compensation must specify dropCollections or dropIndexes")
	}

	for _, index := range actions.DropIndexes {
		if index.Collection == "" || index.Name == "" {
			return errors.New("MongoDB dropIndexes entries require collection and name")
		}
	}

	for _, collectionName := range actions.DropCollections {
		if collectionName == "" {
			return errors.New("MongoDB dropCollections entries cannot be empty")
		}
	}

	for _, index := range actions.DropIndexes {
		collection, _, _, err := database.Collection(config, index.Collection)
		if err != nil {
			return fmt.Errorf("open MongoDB collection %q: %w", index.Collection, err)
		}

		if err := collection.Indexes().DropOne(ctx, index.Name); err != nil {
			return fmt.Errorf("drop MongoDB index %q from %q: %w", index.Name, index.Collection, err)
		}
	}

	for _, collectionName := range actions.DropCollections {
		collection, _, _, err := database.Collection(config, collectionName)
		if err != nil {
			return fmt.Errorf("open MongoDB collection %q: %w", collectionName, err)
		}

		if err := collection.Drop(ctx); err != nil {
			return fmt.Errorf("drop MongoDB collection %q: %w", collectionName, err)
		}
	}

	return nil
}

func executeMongoSeederCompensation(ctx context.Context, database *Database, config *Config, file File) error {
	var actions struct {
		Collection string   `json:"collection"`
		Delete     []bson.M `json:"delete"`
	}

	if err := decodeCompensationJSON(file.CompensationContent, &actions); err != nil {
		return fmt.Errorf("parse MongoDB seeder compensation JSON: %w", err)
	}

	if actions.Collection == "" || len(actions.Delete) == 0 {
		return errors.New("MongoDB seeder compensation requires collection and non-empty delete filters")
	}

	for _, filter := range actions.Delete {
		if len(filter) == 0 {
			return errors.New("MongoDB seeder compensation delete filters cannot be empty")
		}
	}

	collection, _, _, err := database.Collection(config, actions.Collection)
	if err != nil {
		return fmt.Errorf("open MongoDB collection %q: %w", actions.Collection, err)
	}

	for _, filter := range actions.Delete {
		if _, err := collection.DeleteMany(ctx, filter); err != nil {
			return fmt.Errorf("delete MongoDB seeder data from %q: %w", actions.Collection, err)
		}
	}

	return nil
}

func deleteExecutionHistory(ctx context.Context, database *Database, config *Config, driver, tableName, columnName, fileName string) error {
	switch driver {
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		query := "DELETE FROM " + tableName + " WHERE " + columnName + " = ?"

		//nolint:revive
		switch driver {
		case POSTGRES:
			query = "DELETE FROM " + tableName + " WHERE " + columnName + " = $1"
		case SQLSERVER:
			query = "DELETE FROM " + tableName + " WHERE " + columnName + " = @p1"
		case ORACLE:
			query = "DELETE FROM " + tableName + " WHERE " + columnName + " = :1"
		}

		_, _, _, err := database.ExecSQL(ctx, config, query, fileName)
		if err != nil {
			return fmt.Errorf("delete execution history for %q: %w", fileName, err)
		}
	case MONGO:
		collection, _, _, err := database.Collection(config, tableName)
		if err != nil {
			return fmt.Errorf("delete execution history for %q: %w", fileName, err)
		}

		if _, err := collection.DeleteOne(ctx, bson.M{columnName: fileName}); err != nil {
			return fmt.Errorf("delete execution history for %q: %w", fileName, err)
		}
	case SCYLLA:
		_, _, err := database.ExecCQL(ctx, config,
			"DELETE FROM "+tableName+" WHERE "+columnName+" = ?", fileName)
		if err != nil {
			return fmt.Errorf("delete execution history for %q: %w", fileName, err)
		}
	default:
		return fmt.Errorf("unsupported history driver %q", driver)
	}

	return nil
}

func deleteMigrationState(ctx context.Context, database *Database, config *Config, driver, name string) error {
	switch driver {
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		query := "DELETE FROM " + migrationStateTable + " WHERE migration_name = ?"

		//nolint:revive
		switch driver {
		case POSTGRES:
			query = "DELETE FROM " + migrationStateTable + " WHERE migration_name = $1"
		case SQLSERVER:
			query = "DELETE FROM " + migrationStateTable + " WHERE migration_name = @p1"
		case ORACLE:
			query = "DELETE FROM " + migrationStateTable + " WHERE migration_name = :1"
		}

		_, _, _, err := database.ExecSQL(ctx, config, query, name)
		if err != nil {
			return fmt.Errorf("delete migration state for %q: %w", name, err)
		}
	case MONGO:
		collection, _, _, err := database.Collection(config, migrationStateTable)
		if err != nil {
			return fmt.Errorf("delete migration state for %q: %w", name, err)
		}

		if _, err := collection.DeleteOne(ctx, bson.M{_id: name}); err != nil {
			return fmt.Errorf("delete migration state for %q: %w", name, err)
		}
	case SCYLLA:
		_, _, err := database.ExecCQL(ctx, config,
			"DELETE FROM "+migrationStateTable+" WHERE migration_name = ?", name)
		if err != nil {
			return fmt.Errorf("delete migration state for %q: %w", name, err)
		}
	default:
		return fmt.Errorf("unsupported state driver %q", driver)
	}

	return nil
} //nolint:revive
