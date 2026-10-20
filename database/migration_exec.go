package database

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

func migrationExec(ctx context.Context, database *Database, config *Config, log *zap.Logger, execType executionType) (resultMessage string, resultErr error) {
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
			zap.String("directory", fmt.Sprintf("./%s", subDir)),
			zap.String("sample_file", sampleFile(driver, execType))).
			Info(fmt.Sprintf("%s migration ignored! %s files not found", driver, execType))
	}

	log.WithOptions(zap.WithCaller(false)).With(
		zap.Int("count", len(files)),
		zap.String("driver", driver),
		zap.Strings("files", getNames(files))).
		Info(fmt.Sprintf("found %s %s files", driver, execType))

	// Serialize runners across processes and hosts. The lock is deliberately
	// non-expiring: recovering after a crash must be an explicit operator action
	// because the failed process may have left partial database changes.
	releaseLock, message, err := acquireMigrationLock(ctx, database, config)
	if err != nil {
		return message, err
	}
	defer func() {
		if resultErr != nil {
			resultMessage += " The database startup lock was retained because this run failed; inspect the database state before manually removing the lock."
			return
		}

		cleanupTimeout := config.Timeout
		if cleanupTimeout <= 0 {
			cleanupTimeout = DefaultTimeout
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()

		if releaseErr := releaseLock(cleanupCtx); releaseErr != nil {
			releaseMessage := "Migration lock could not be released. Inspect the migration lock record and database state before retrying."
			resultMessage = releaseMessage
			resultErr = fmt.Errorf("release database startup lock: %w", releaseErr)
		}
	}()

	if len(files) == 0 {
		states, err := loadMigrationStates(ctx, database, config, driver)
		if err != nil {
			return "Ensure the database migration state store is readable and writable.", err
		}
		statePrefix := string(execType) + ":"
		for name, state := range states {
			if strings.HasPrefix(name, statePrefix) && state.Status != "applied" {
				return "A previous migration or seeder did not finish. Inspect its database changes and state record before recovery.",
					fmt.Errorf("%s %q is left in state %q", execType, strings.TrimPrefix(name, statePrefix), state.Status)
			}
		}
		return "", nil
	}

	// 2. Create History Table & Get Executed Records
	executedRecords, message, err := createHistoryTable(ctx, driver, database, config, execType, tableName, columnName)
	if err != nil {
		return message, fmt.Errorf("get executed %s %ss failed: %w", driver, execType, err)
	}

	// 3. Load checksums and execution states. A running state is deliberately
	// left in place after a failure so a potentially partial script is never
	// retried automatically.
	states, err := loadMigrationStates(ctx, database, config, driver)
	if err != nil {
		return "Ensure the database migration state store is readable and writable.", err
	}
	statePrefix := string(execType) + ":"
	for name, state := range states {
		if strings.HasPrefix(name, statePrefix) && state.Status != "applied" {
			return "A previous migration or seeder did not finish. Inspect its database changes and state record before recovery.",
				fmt.Errorf("%s %q is left in state %q", execType, strings.TrimPrefix(name, statePrefix), state.Status)
		}
	}
	executed := make(map[string]struct{}, len(executedRecords))
	for _, name := range executedRecords {
		executed[name] = struct{}{}
	}

	// 4. Validate applied files and execute pending files in filename order.
	pendingCount := 0
	for _, fileItem := range files {
		if _, alreadyExecuted := executed[fileItem.Name]; !alreadyExecuted {
			pendingCount++
		}
	}
	step := 0
	for _, fileItem := range files {
		stateKey := string(execType) + ":" + fileItem.Name
		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(fileItem.Content)))
		state, hasState := states[stateKey]
		_, alreadyExecuted := executed[fileItem.Name]

		if hasState {
			if state.Checksum != checksum {
				return "A previously tracked migration or seeder file has changed. Add a new file instead of editing an applied file.",
					fmt.Errorf("%s %q checksum changed", execType, fileItem.Name)
			}
			if state.Status != "applied" {
				return "A previous migration or seeder did not finish. Inspect its database changes and state record before recovery.",
					fmt.Errorf("%s %q is left in state %q", execType, fileItem.Name, state.Status)
			}
			if !alreadyExecuted {
				return "Migration state and execution history disagree. Inspect both records before recovery.",
					fmt.Errorf("%s %q is marked applied but absent from execution history", execType, fileItem.Name)
			}
			continue
		}

		if alreadyExecuted {
			// Backfill state for files recorded by older GOREST releases. Their
			// historical contents cannot be verified because no old checksum exists.
			if err := insertMigrationState(ctx, database, config, driver, stateKey, checksum, "applied"); err != nil {
				return "Ensure the database migration state store accepts legacy history records.", err
			}
			continue
		}

		step++
		nameField := zap.String(string(execType)+"_name", fileItem.Name)
		log.WithOptions(zap.WithCaller(false)).With(zap.Int("steps", step), zap.Int("total", pendingCount), nameField).
			Info(fmt.Sprintf("executing %s %s", driver, execType))

		if err := insertMigrationState(ctx, database, config, driver, stateKey, checksum, "running"); err != nil {
			return "Ensure the database migration state store accepts new execution records.", err
		}

		message, err = dataMigrationExec(ctx, driver, database, config, execType, fileItem, tableName, columnName)
		if err != nil {
			return message, fmt.Errorf("execute %s %s %s failed: %w", driver, execType, fileItem.Name, err)
		}
		if err := updateMigrationState(ctx, database, config, driver, stateKey, "applied"); err != nil {
			return "The migration ran but its state could not be marked applied. Inspect the database before retrying.", err
		}

		log.WithOptions(zap.WithCaller(false)).With(nameField).Info(fmt.Sprintf("%s %s executed successfully", driver, execType))
	}

	log.WithOptions(zap.WithCaller(false)).With(zap.Int("count", pendingCount), zap.String("driver", driver)).
		Info(fmt.Sprintf("all %s %ss completed successfully", driver, execType))

	return "", nil
}

// ensureDir creates the migration/seeder directory if it doesn't exist.
// MkdirAll is idempotent: no error if the directory already exists.
func ensureDir(dir string, execType executionType) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		message := fmt.Sprintf("Ensure the application has write permission to create the %s directory: %s", execType, dir)

		return message, fmt.Errorf("create %s directory %s: %w", execType, dir, err)
	}

	return "", nil
}

const samplesBaseURL = "https://github.com/elsyahtech/gorest/tree/main/database"

// sampleFile returns the GitHub folder containing sample files for a driver.
// Example: https://github.com/elsyahtech/gorest/tree/main/database/migrations/{Driver}
// Example: https://github.com/elsyahtech/gorest/tree/main/database/seeders/{Driver}
func sampleFile(driver string, execType executionType) string {
	folder := "migrations"
	if execType != typeMigration {
		folder = "seeders"
	}

	return fmt.Sprintf("%s/%s/%s", samplesBaseURL, folder, driver)
}
