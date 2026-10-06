package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

//nolint:revive
func dataMigrationExec(
	ctx context.Context,
	driver string,
	database *Database,
	config *Config,
	execType executionType,
	fileItem File,
	tableName,
	columnName string,
) (message string, err error) {
	switch driver {
	case MONGO:
		if execType == typeMigration {
			message, err = mongoDataMigrationExec(ctx, database, config, fileItem)
			if err != nil {
				return message, fmt.Errorf("%w", err)
			}
		} else {
			message, err = mongoDataSeederExec(ctx, database, config, fileItem)
			if err != nil {
				return message, fmt.Errorf("%w", err)
			}
		}
	case MYSQL, POSTGRES, SQLSERVER, SQLITE, ORACLE:
		message, err = sqlDataMigrationExec(ctx, driver, database, fileItem, tableName, columnName)
		if err != nil {
			return message, fmt.Errorf("%w", err)
		}
	case SCYLLA:
		message, err = scyllaDataMigrationExec(ctx, database, fileItem, tableName, columnName)
		if err != nil {
			return message, fmt.Errorf("%w", err)
		}
	default:
		const message = "ensure your driver database configured using mysql, postgresql, mssql, oracle, sqlite, mongodb or scylladb"

		return message, fmt.Errorf("migration process for driver %s not support", driver)
	}

	return message, nil
}

// migrationMongoDataExec executes a single migration JSON file and records it in history.
// Parses the JSON content, creates the collection and indexes if specified,
// then inserts a record into the migration_history collection.
// If any step fails, returns an error (entire migration considered failed).
//
//nolint:gocognit,revive,funlen
func mongoDataMigrationExec(ctx context.Context, database *Database, config *Config, migration File) (res string, eror error) {
	// 1. Struct mapping for MongoDB migration JSON file.
	type MongoIndex struct {
		Name   string          `json:"name,omitempty"`
		Key    json.RawMessage `json:"key"`
		Unique bool            `json:"unique,omitempty"`
	}

	type MongoMigrationPayload struct {
		Collection string       `json:"collection"`
		Validator  bson.M       `json:"validator,omitempty"`
		Indexes    []MongoIndex `json:"indexes,omitempty"`
	}

	var payload MongoMigrationPayload
	if err := json.Unmarshal([]byte(migration.Content), &payload); err != nil {
		message := fmt.Sprintf(
			"failed to parse JSON content of migration file '%s'. "+
				"Check if the JSON syntax is valid.",
			migration.Name,
		)

		return message, fmt.Errorf(
			"unmarshal migration json failed: %w",
			err,
		)
	}

	// 2. Create collection with optional validator.
	//nolint:nestif
	if payload.Collection != "" {
		createOptions := options.CreateCollection()

		if payload.Validator != nil {
			createOptions.SetValidator(payload.Validator)
		}

		message, err := database.CreateCollection(
			ctx,
			config,
			payload.Collection,
			createOptions,
		)
		if err != nil {
			// Ignore if collection already exists (idempotent safeguard).
			if !strings.Contains(err.Error(), "already exists") {
				return message, fmt.Errorf(
					"create mongo collection failed: %w",
					err,
				)
			}
		}

		// 3. Create indexes if specified.
		if len(payload.Indexes) > 0 {
			coll, message, err := database.Collection(config, payload.Collection)
			if err != nil {
				return message, fmt.Errorf(
					"get mongo collection instance failed: %w",
					err,
				)
			}

			var indexModels []mongo.IndexModel

			for _, idx := range payload.Indexes {
				// Parse index key while preserving JSON field order.
				var keyMap map[string]any

				if err := json.Unmarshal(idx.Key, &keyMap); err != nil {
					message := fmt.Sprintf(
						"failed to parse index key in migration file '%s'. "+
							"Check index key JSON syntax.",
						migration.Name,
					)

					return message, fmt.Errorf(
						"unmarshal mongo index key failed: %w",
						err,
					)
				}

				if len(keyMap) == 0 {
					message := fmt.Sprintf(
						"index key cannot be empty in migration file '%s'.",
						migration.Name,
					)

					return message, errors.New(
						"mongo index key is empty",
					)
				}

				keysD := bson.D{}

				for key, val := range keyMap {
					keysD = append(keysD, bson.E{
						Key:   key,
						Value: val,
					})
				}

				indexModel := mongo.IndexModel{
					Keys: keysD,
				}

				indexOpts := options.Index()

				if idx.Name != "" {
					indexOpts.SetName(idx.Name)
				}

				if idx.Unique {
					indexOpts.SetUnique(true)
				}

				indexModel.Options = indexOpts
				indexModels = append(indexModels, indexModel)
			}

			if len(indexModels) > 0 {
				_, err := coll.Indexes().CreateMany(ctx, indexModels)
				if err != nil {
					message := fmt.Sprintf(
						"failed to create indexes for collection '%s' in migration file '%s'. "+
							"Check index configurations.",
						payload.Collection,
						migration.Name,
					)

					return message, fmt.Errorf(
						"create mongo indexes failed: %w",
						err,
					)
				}
			}
		}
	}

	// 4. Write migration history to migration_history collection.
	historyColl, message, err := database.Collection(config, "migration_history")
	if err != nil {
		return message, fmt.Errorf(
			"get mongo history collection failed: %w",
			err,
		)
	}

	historyRecord := bson.M{
		"migration_name": migration.Name,
		"executed_at":    time.Now(),
	}

	_, err = historyColl.InsertOne(ctx, historyRecord)
	if err != nil {
		message := fmt.Sprintf(
			"migration '%s' executed successfully, but failed to record its history "+
				"into the 'migration_history' collection. "+
				"Check database write permissions or unique index constraints.",
			migration.Name,
		)

		return message, fmt.Errorf(
			"record mongo migration history failed: %w",
			err,
		)
	}

	return "", nil
}

// mongoDataSeederExec parses a single seeder JSON file, inserts data into the target collection,
// and records it in the seeder_history collection.
//
//nolint:revive,funlen
func mongoDataSeederExec(ctx context.Context, database *Database, config *Config, seeder File) (res string, eror error) {
	var rawPayload map[string]any

	if err := json.Unmarshal([]byte(seeder.Content), &rawPayload); err != nil {
		message := fmt.Sprintf("failed to parse JSON content of seeder file '%s'. "+
			"Check if the JSON syntax is valid.", seeder.Name)

		return message, fmt.Errorf("unmarshal seeder json failed: %w", err)
	}

	collectionName, isColNameOk := rawPayload["collection"].(string)
	if !isColNameOk {
		return "", nil
	}

	rawDocs, isRawDocsOk := rawPayload["data"].([]any)
	if !isRawDocsOk {
		return "", nil
	}

	if collectionName == "" || len(rawDocs) == 0 {
		message := fmt.Sprintf("seeder file '%s' is missing 'collection' field or 'data' is empty.", seeder.Name)

		return message, errors.New("invalid mongo seeder payload structure")
	}

	var parseDates func(val any) any

	parseDates = func(val any) any {
		switch value := val.(type) {
		case string:
			if parseTime, err := time.Parse(time.RFC3339, value); err == nil {
				return parseTime
			}

			if parseTime, err := time.Parse("2006-01-02T15:04:05Z07:00", value); err == nil {
				return parseTime
			}

			return value
		case map[string]any:
			mAke := make(map[string]any)

			for mk, mv := range value {
				mAke[mk] = parseDates(mv)
			}

			return mAke
		case []any:
			arr := make([]any, len(value))

			for index, val := range value {
				arr[index] = parseDates(val)
			}

			return arr
		default:
			return value
		}
	}

	var documents []any

	for _, doc := range rawDocs {
		parsedDoc := parseDates(doc)

		documents = append(documents, parsedDoc)
	}

	targetColl, message, err := database.Collection(config, collectionName)
	if err != nil {
		return message, fmt.Errorf("%w", err)
	}

	_, err = targetColl.InsertMany(ctx, documents)
	if err != nil {
		message := fmt.Sprintf("failed to insert seed data into collection '%s' from file '%s'. "+
			"Check document schema or duplicate key errors.", collectionName, seeder.Name)

		return message, fmt.Errorf("%w", err)
	}

	historyColl, message, err := database.Collection(config, "seeder_history")
	if err != nil {
		return message, fmt.Errorf("get mongo seeder history collection failed: %w", err)
	}

	historyRecord := bson.M{
		"seeder_name": seeder.Name,
		"executed_at": time.Now(),
	}

	_, err = historyColl.InsertOne(ctx, historyRecord)
	if err != nil {
		message := fmt.Sprintf("seeder '%s' executed successfully, but failed to record its history into 'seeder_history'.", seeder.Name)

		return message, fmt.Errorf("%w", err)
	}

	return "", nil
}

// sqlDataMigrationExec executes a single migration SQL file and records it in history
// Executes the SQL, then inserts record in migration_history table
// If any step fails, returns error (entire migration considered failed).
func sqlDataMigrationExec(ctx context.Context, driver string, database *Database, file File, tableName, columnName string) (string, error) {
	queryPostgres := "INSERT INTO " + tableName + " (" + columnName + ", executed_at) VALUES ($1, $2)"
	queryMssql := "INSERT INTO " + tableName + " (" + columnName + ", executed_at) VALUES (@p1, @p2)"
	queryMysql := "INSERT INTO " + tableName + " (" + columnName + ", executed_at) VALUES (?, ?)"
	queryOracle := "INSERT INTO " + tableName + " (" + columnName + ", executed_at) VALUES (:1, :2)"

	if _, message, err := database.ExecSQL(ctx, file.Content); err != nil {
		msg := fmt.Sprintf("execute SQL statements inside migration file '%s' failed. "+
			"Check the migration script syntax, schema constraints, or "+
			"potential SQL runtime errors or %s", file.Name, message)

		return msg, fmt.Errorf("execute migration SQL failed: %w", err)
	}

	var insertQuery string

	switch driver {
	case POSTGRES:
		insertQuery = queryPostgres
	case SQLSERVER:
		insertQuery = queryMssql
	case MYSQL, SQLITE:
		insertQuery = queryMysql
	case ORACLE:
		insertQuery = queryOracle
	default:
		message := fmt.Sprintf("the database driver '%s' is unsupported for recording migration history. "+
			"Check your configuration driver", driver)

		return message, fmt.Errorf("unsupported database driver: %s", driver)
	}

	// Record migration in migration_history table (mark as executed)
	if _, message, err := database.ExecSQL(ctx, insertQuery, file.Name, time.Now()); err != nil {
		msg := fmt.Sprintf("migration '%s' executed successfully, but failed to record its history into the 'migration_history' table. "+
			"Check database write permissions or unique index constraints or %s", file.Name, message)

		return msg, fmt.Errorf("record migration in history failed: %w", err)
	}

	return "", nil
}

func scyllaDataMigrationExec(ctx context.Context, database *Database, migration File, tableName, columnName string) (string, error) {
	// Split berdasarkan titik koma
	rawStatements := strings.Split(migration.Content, ";")

	var statements []string

	for _, stmt := range rawStatements {
		trimmed := strings.TrimSpace(stmt)

		if trimmed == "" {
			continue
		}

		statements = append(statements, trimmed)
	}

	for _, stmt := range statements {
		if message, err := database.ExecCQL(ctx, stmt); err != nil {
			msg := fmt.Sprintf("execute CQL statement inside migration file '%s' failed. "+
				"Statement: [%s]. Check the CQL syntax, keyspace settings, or runtime errors or %s",
				migration.Name, stmt, message)

			return msg, fmt.Errorf("execute migration CQL failed: %w", err)
		}
	}

	query := "INSERT INTO " + tableName + " (" + columnName + ", executed_at) VALUES (?, ?)"
	if message, err := database.ExecCQL(ctx, query, migration.Name, time.Now()); err != nil {
		msg := fmt.Sprintf("migration '%s' executed successfully, but failed to record its history into ScyllaDB. "+
			"Check database write permissions or %s", migration.Name, message)

		return msg, fmt.Errorf("record ScyllaDB %s failed: %w", tableName, err)
	}

	return "", nil
} //nolint:revive
