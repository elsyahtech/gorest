package database

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"
)

func migrationExec(ctx context.Context, database *Database, config *Config, log *zap.Logger, execType executionType) (string, error) {
	driver := NormalizeDatabaseDriver(config.Driver)

	var subDir, tableName, columnName string

	switch execType {
	case typeMigration:
		subDir = config.MigrationDirectory
		tableName, columnName = migrationTableName, migrationColumnName
	default:
		subDir = config.SeederDirectory
		tableName, columnName = seederTableName, seederColumnName
	}

	log.WithOptions(zap.WithCaller(false)).Info(fmt.Sprintf("starting %s %ss", driver, execType))

	if message, err := ensureDir(subDir, execType); err != nil {
		return message, err
	}

	// 1. Read files
	files, message, err := readFiles(driver, subDir)
	if err != nil {
		return message, fmt.Errorf("read %s %s files: %w", driver, execType, err)
	}

	if len(files) == 0 {
		log.WithOptions(zap.WithCaller(false)).With(
			zap.String("driver", driver),
			zap.String("directory ./", subDir)).
			Info(fmt.Sprintf("%s migration ignored! %s files not found", driver, execType))

		return "", nil
	}

	log.WithOptions(zap.WithCaller(false)).With(
		zap.Int("count", len(files)),
		zap.String("driver", driver),
		zap.Strings("files", getNames(files))).
		Info(fmt.Sprintf("found %s %s files", driver, execType))

	// 2. Create History Table & Get Executed Records
	executedRecords, message, err := createHistoryTable(ctx, driver, database, config, execType, tableName, columnName)
	if err != nil {
		return message, fmt.Errorf("get executed %s %ss failed: %w", driver, execType, err)
	}

	// 3. Filter Pending
	pendingFiles := filterPendingMigrationExec(files, executedRecords)
	if len(pendingFiles) == 0 {
		log.WithOptions(zap.WithCaller(false)).With(zap.String("driver", driver)).
			Info(fmt.Sprintf("%s database %s ignored! all were already executed", driver, execType))

		return "", nil
	}

	// 4. Execute Pending Files
	for i, fileItem := range pendingFiles {
		nameField := zap.String(string(execType)+"_name", fileItem.Name)

		log.WithOptions(zap.WithCaller(false)).With(zap.Int("steps", i+1), zap.Int("total", len(pendingFiles)), nameField).
			Info(fmt.Sprintf("executing %s %s", driver, execType))

		message, err = dataMigrationExec(ctx, driver, database, config, execType, fileItem, tableName, columnName)
		if err != nil {
			return message, fmt.Errorf("execute %s %s %s failed: %w", driver, execType, fileItem.Name, err)
		}

		log.WithOptions(zap.WithCaller(false)).With(nameField).Info(fmt.Sprintf("%s %s executed successfully", driver, execType))
	}

	log.With(zap.Int("count", len(pendingFiles)), zap.String("driver", driver)).
		Info(fmt.Sprintf("all %s %ss completed successfully", driver, execType))

	return "", nil
}

// ensureDir creates the migration/seeder directory if it doesn't exist.
// MkdirAll is idempotent: no error if the directory already exists.
func ensureDir(dir string, execType executionType) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		message := fmt.Sprintf("Ensure the application has write permission to create the %s directory: %s", execType, dir)

		return message, fmt.Errorf("create %s directory %s: %w", execType, dir, err)
	}

	return "", nil
}
