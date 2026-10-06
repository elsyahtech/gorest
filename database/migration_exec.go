package database

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

func migrationExec(ctx context.Context, database *Database, config *Config, log *zap.Logger, execType executionType) (string, error) {
	driver := NormalizeDatabaseDriver(config.Driver)

	migrationDir := config.MigrationDirectory
	seederDir := config.SeederDirectory

	var subDir, tableName, columnName string
	if execType == typeMigration {
		subDir = migrationDir
		tableName = migrationTableName
		columnName = migrationColumnName
	} else {
		subDir = seederDir
		tableName = seederTableName
		columnName = seederColumnName
	}

	log.Info(fmt.Sprintf("starting %s %ss", driver, execType))

	// 1. Read files
	files, message, err := readFiles(driver, subDir, execType)
	if err != nil {
		return message, fmt.Errorf("read %s %s files: %w", driver, execType, err)
	}

	if len(files) == 0 {
		log.With(
			zap.String("driver", driver),
			zap.String("directory", subDir)).
			Info(fmt.Sprintf("%s migration ignored! %s files not found", driver, execType))

		return "", nil
	}

	log.With(
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
		log.With(zap.String("driver", driver)).
			Info(fmt.Sprintf("%s database %s ignored! all were already executed", driver, execType))

		return "", nil
	}

	// 4. Execute Pending Files
	for i, fileItem := range pendingFiles {
		nameField := zap.String(string(execType)+"_name", fileItem.Name)

		log.With(zap.Int("steps", i+1), zap.Int("total", len(pendingFiles)), nameField).
			Info(fmt.Sprintf("executing %s %s", driver, execType))

		message, err = dataMigrationExec(ctx, driver, database, config, execType, fileItem, tableName, columnName)
		if err != nil {
			return message, fmt.Errorf("execute %s %s %s failed: %w", driver, execType, fileItem.Name, err)
		}

		log.With(nameField).Info(fmt.Sprintf("%s %s executed successfully", driver, execType))
	}

	log.With(zap.Int("count", len(pendingFiles)), zap.String("driver", driver)).
		Info(fmt.Sprintf("all %s %ss completed successfully", driver, execType))

	return "", nil
}
