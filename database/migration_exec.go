package database

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

func migrationExec(ctx context.Context, database *Database, config *Config, log *zap.Logger, execType executionType) (resultMessage string, resultErr error) {
	driver := NormalizeDatabaseDriver(config.Driver)
	subDir, tableName, columnName := executionDetails(config, execType)

	log.WithOptions(zap.WithCaller(false)).Info(fmt.Sprintf("starting %s %ss", driver, execType))

	files, message, err := loadExecutionFiles(driver, subDir, execType, log)
	if err != nil {
		return message, err
	}

	// Serialize runners across processes and hosts. The lock is deliberately
	// non-expiring: recovering after a crash must be an explicit operator action
	// because the failed process may have left partial database changes.
	releaseLock, message, err := acquireMigrationLock(ctx, database, config)
	if err != nil {
		return message, err
	}

	releaseLockAfterRollback := false
	defer func() {
		if resultErr != nil && !releaseLockAfterRollback {
			resultMessage += " The database startup lock was retained because this run failed; inspect the database state before manually removing the lock."
			return
		}

		cleanupTimeout := config.Timeout

		if cleanupTimeout <= 0 {
			cleanupTimeout = DefaultTimeout
		}

		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()

		releaseErr := releaseLock(cleanupCtx)
		if releaseErr == nil {
			return
		}

		releaseMessage := "Migration lock could not be released. Inspect the migration lock record and database state before retrying."

		resultMessage = releaseMessage
		resultErr = fmt.Errorf("release database startup lock: %w", releaseErr)
	}()

	if len(files) == 0 {
		return validateEmptyExecution(ctx, database, config, driver, execType)
	}

	plan, message, err := prepareExecution(&structPrepareMigrationExecution{
		ctx:        ctx,
		database:   database,
		config:     config,
		driver:     driver,
		execType:   execType,
		tableName:  tableName,
		columnName: columnName,
		files:      files,
	})
	if err != nil {
		return message, err
	}

	return runMigrationFiles(ctx, &structMigrationExecution{
		database:                 database,
		config:                   config,
		log:                      log,
		driver:                   driver,
		execType:                 execType,
		tableName:                tableName,
		columnName:               columnName,
		plan:                     plan,
		releaseLockAfterRollback: &releaseLockAfterRollback,
	}, files)
}

type structMigrationExecutionPlan struct {
	states       map[string]migrationState
	executed     map[string]struct{}
	pendingCount int
}

type structPrepareMigrationExecution struct {
	ctx        context.Context
	database   *Database
	config     *Config
	driver     string
	execType   executionType
	tableName  string
	columnName string
	files      []File
}

type structMigrationExecution struct {
	database                 *Database
	config                   *Config
	log                      *zap.Logger
	plan                     *structMigrationExecutionPlan
	releaseLockAfterRollback *bool
	driver                   string
	execType                 executionType
	tableName                string
	columnName               string
	completed                []File
}

func executionDetails(config *Config, execType executionType) (dir string, tableName string, columnName string) {
	if execType == typeMigration {
		return config.MigrationDirectory, migrationTableName, migrationColumnName
	}

	return config.SeederDirectory, seederTableName, seederColumnName
}

func loadExecutionFiles(driver, directory string, execType executionType, log *zap.Logger) ([]File, string, error) {
	if message, err := ensureDir(directory, execType); err != nil {
		return nil, message, err
	}

	files, message, err := readFiles(driver, directory)
	if err != nil {
		return nil, message, fmt.Errorf("read %s %s files: %w", driver, execType, err)
	}

	if len(files) == 0 {
		log.WithOptions(zap.WithCaller(false)).With(
			zap.String("driver", driver),
			zap.String("directory", fmt.Sprintf("./%s", directory)),
			zap.String("sample_file", sampleFile(driver, execType))).
			Info(fmt.Sprintf("%s migration ignored! %s files not found", driver, execType))
	}

	log.WithOptions(zap.WithCaller(false)).With(
		zap.Int("count", len(files)),
		zap.String("driver", driver),
		zap.Strings("files", getNames(files))).
		Info(fmt.Sprintf("found %s %s files", driver, execType))

	return files, "", nil
}

func validateEmptyExecution(ctx context.Context, database *Database, config *Config, driver string, execType executionType) (string, error) {
	states, err := loadMigrationStates(ctx, database, config, driver)
	if err != nil {
		return "Ensure the database migration state store is readable and writable.", err
	}

	return validateInterruptedExecution(states, execType)
}

func prepareExecution(req *structPrepareMigrationExecution) (*structMigrationExecutionPlan, string, error) {
	executedRecords, message, err := createHistoryTable(
		req.ctx, req.driver, req.database, req.config, req.execType, req.tableName, req.columnName,
	)
	if err != nil {
		return nil, message, fmt.Errorf("get executed %s %ss failed: %w", req.driver, req.execType, err)
	}

	states, err := loadMigrationStates(req.ctx, req.database, req.config, req.driver)
	if err != nil {
		return nil, "Ensure the database migration state store is readable and writable.", err
	}

	if message, err := validateInterruptedExecution(states, req.execType); err != nil {
		return nil, message, err
	}

	executed := make(map[string]struct{}, len(executedRecords))
	for _, name := range executedRecords {
		executed[name] = struct{}{}
	}

	pendingCount := 0

	for _, file := range req.files {
		if _, alreadyExecuted := executed[file.Name]; !alreadyExecuted {
			pendingCount++
		}
	}

	return &structMigrationExecutionPlan{
		states: states, executed: executed, pendingCount: pendingCount,
	}, "", nil
}

func validateInterruptedExecution(states map[string]migrationState, execType executionType) (string, error) {
	statePrefix := string(execType) + ":"
	for name, state := range states {
		if state.Status != "applied" && strings.HasPrefix(name, statePrefix) {
			message := "A previous migration or seeder did not finish. Inspect its database changes and state record before recovery."
			return message, fmt.Errorf("%s %q is left in state %q", execType, strings.TrimPrefix(name, statePrefix), state.Status)
		}
	}

	return "", nil
}

func runMigrationFiles(ctx context.Context, execution *structMigrationExecution, files []File) (string, error) {
	step := 0

	for _, file := range files {
		handled, message, err := handleTrackedFile(ctx, execution, file)
		if err != nil {
			return message, err
		}

		if handled {
			continue
		}

		step++
		if message, err := executePendingFile(ctx, execution, file, step); err != nil {
			return message, err
		}
	}

	execution.log.WithOptions(zap.WithCaller(false)).With(
		zap.Int("count", execution.plan.pendingCount),
		zap.String("driver", execution.driver),
	).Info(fmt.Sprintf("all %s %ss completed successfully", execution.driver, execution.execType))

	return "", nil
}

func handleTrackedFile(ctx context.Context, execution *structMigrationExecution, file File) (bool, string, error) {
	stateKey := string(execution.execType) + ":" + file.Name
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(file.Content)))
	state, hasState := execution.plan.states[stateKey]
	_, alreadyExecuted := execution.plan.executed[file.Name]

	if hasState {
		if message, err := validateTrackedFile(execution.execType, file, state); err != nil {
			return true, message, err
		}

		if !alreadyExecuted {
			return true, "Migration state and execution history disagree. Inspect both records before recovery.",
				fmt.Errorf("%s %q is marked applied but absent from execution history", execution.execType, file.Name)
		}

		return true, "", nil
	}

	if !alreadyExecuted {
		return false, "", nil
	}

	// Backfill state for files recorded by older GOREST releases. Their
	// historical contents cannot be verified because no old checksum exists.
	if err := insertMigrationState(ctx, execution.database, execution.config, execution.driver, stateKey, checksum, "applied"); err != nil {
		return true, "Ensure the database migration state store accepts legacy history records.", err
	}

	return true, "", nil
}

func validateTrackedFile(execType executionType, file File, state migrationState) (string, error) {
	if state.Checksum != fmt.Sprintf("%x", sha256.Sum256([]byte(file.Content))) {
		return "A previously tracked migration or seeder file has changed. Add a new file instead of editing an applied file.",
			fmt.Errorf("%s %q checksum changed", execType, file.Name)
	}

	if state.Status != "applied" {
		return "A previous migration or seeder did not finish. Inspect its database changes and state record before recovery.",
			fmt.Errorf("%s %q is left in state %q", execType, file.Name, state.Status)
	}

	return "", nil
}

func executePendingFile(ctx context.Context, execution *structMigrationExecution, file File, step int) (string, error) {
	stateKey := string(execution.execType) + ":" + file.Name
	nameField := zap.String(string(execution.execType)+"_name", file.Name)
	execution.log.WithOptions(zap.WithCaller(false)).With(
		zap.Int("steps", step),
		zap.Int("total", execution.plan.pendingCount),
		nameField,
	).Info(fmt.Sprintf("executing %s %s", execution.driver, execution.execType))

	err := insertMigrationState(
		ctx,
		execution.database,
		execution.config,
		execution.driver,
		stateKey,
		fmt.Sprintf("%x", sha256.Sum256([]byte(file.Content))), "running",
	)
	if err != nil {
		return "Ensure the database migration state store accepts new execution records.", err
	}

	if err := migrationIntegrationPause(stateKey); err != nil {
		return "Integration migration pause failed before executing the migration file.", err
	}

	message, err := dataMigrationExec(
		ctx,
		execution.driver,
		execution.database,
		execution.config,
		execution.execType,
		file,
		execution.tableName,
		execution.columnName,
	)
	if err != nil {
		return execution.rollbackFailure(file, message, err)
	}

	if err := updateMigrationState(ctx, execution.database, execution.config, execution.driver, stateKey, "applied"); err != nil {
		return execution.rollbackFailure(file, "The startup task ran but its state could not be marked applied.", err)
	}

	execution.completed = append(execution.completed, file)
	execution.log.WithOptions(zap.WithCaller(false)).With(nameField).
		Info(fmt.Sprintf("%s %s executed successfully", execution.driver, execution.execType))

	return "", nil
}

func (execution *structMigrationExecution) rollbackFailure(file File, failureMessage string, cause error) (string, error) {
	var sqlFailure *sqlMigrationFailure

	failedFileRolledBack := false

	if errors.As(cause, &sqlFailure) {
		failedFileRolledBack = sqlFailure.rolledBack
	}

	complete, rollbackErr := rollbackExecutionBatch(&structRollbackExecutionBatch{
		database:             execution.database,
		config:               execution.config,
		driver:               execution.driver,
		execType:             execution.execType,
		failed:               file,
		completed:            execution.completed,
		tableName:            execution.tableName,
		columnName:           execution.columnName,
		failedFileRolledBack: failedFileRolledBack,
	})

	executionErr := fmt.Errorf("execute %s %s %q failed: %w", execution.driver, execution.execType, file.Name, cause)
	if complete {
		*execution.releaseLockAfterRollback = true
		execution.logRollbackComplete(file)

		return failureMessage + " Gorest rolled back all changes made by this startup run; startup remains aborted.", executionErr
	}

	execution.logRollbackIncomplete(file, rollbackErr)

	return failureMessage + " Gorest could not fully roll back this startup run; inspect database changes and the retained migration lock before retrying.",
		errors.Join(executionErr, fmt.Errorf("rollback incomplete: %w", rollbackErr))
}

func (execution *structMigrationExecution) logRollbackComplete(file File) {
	execution.log.WithOptions(zap.WithCaller(false)).With(execution.rollbackLogFields(file)...).
		Warn("startup task failed; all changes from this run were rolled back")
}

func (execution *structMigrationExecution) logRollbackIncomplete(file File, rollbackErr error) {
	fields := append(execution.rollbackLogFields(file), zap.Error(rollbackErr))
	execution.log.WithOptions(zap.WithCaller(false)).With(fields...).
		Error("startup task failed; rollback is incomplete")
}

func (execution *structMigrationExecution) rollbackLogFields(file File) []zap.Field {
	return []zap.Field{
		zap.String("driver", execution.driver),
		zap.String("execution_type", string(execution.execType)),
		zap.String("failed_file", file.Name),
	}
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
// Example: https://github.com/elsyahtech/gorest/tree/main/database/seeders/{Driver}.
func sampleFile(driver string, execType executionType) string {
	folder := "migrations"

	if execType != typeMigration {
		folder = "seeders"
	}

	return fmt.Sprintf("%s/%s/%s", samplesBaseURL, folder, driver)
} //nolint:revive
