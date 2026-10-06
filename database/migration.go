package database

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

func (database *Database) Migration(ctx context.Context, config *Config, logger *zap.Logger) (string, error) {
	if config == nil {
		const message = "Ensure that app.config.database in " +
			"app.database.Migration(ctx, app.config.database, logger) is not set to nil."

		return message, errors.New("migration: database.Config is nil")
	}

	if logger == nil {
		const message = "Ensure that logger in " +
			"app.database.Migration(ctx, app.config.database, logger) is not set to nil."

		return message, errors.New("migration: logger is nil")
	}

	// Doible check if migration configured in status enabled
	if !config.Migration {
		return "", nil
	}

	// Continue to execute migration
	return migrationExec(ctx, database, config, logger, typeMigration)
}

func (database *Database) Seeder(ctx context.Context, config *Config, logger *zap.Logger) (string, error) {
	if config == nil {
		const message = "Ensure that app.config.database in " +
			"app.database.Migration(ctx, app.config.database, logger) is not set to nil."

		return message, errors.New("seeder: database.Config is nil")
	}

	if logger == nil {
		const message = "Ensure that logger in " +
			"app.database.Migration(ctx, app.config.database, logger) is not set to nil."

		return message, errors.New("seeder: logger is nil")
	}

	// Doible check if seeder configured in status enabled
	if !config.Seeder {
		return "", nil
	}

	// Continue to execute seeder
	return migrationExec(ctx, database, config, logger, typeSeeder)
}
