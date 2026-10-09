package database

import (
	"context"
	"fmt"
	"os"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ======================================================================================
// getMongoRecords - Generic helper for migrations & seeders
// ======================================================================================.
func getMongoRecords(ctx context.Context, database *Database, config *Config, collectionName, fieldName string) ([]string, string, error) {
	coll, message, _, err := database.Collection(config, collectionName)
	if err != nil {
		return nil, message, fmt.Errorf("get mongo collection '%s' failed: %w", collectionName, err)
	}

	// Sort by executed_at
	const executedAt = "executed_at"

	findOptions := options.Find().SetSort(bson.D{{Key: executedAt, Value: 1}})

	cursor, err := coll.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		message := fmt.Sprintf("query the '%s' collection failed. Ensure collection exists and database connection is healthy.", collectionName)
		return nil, message, fmt.Errorf("find records failed: %w", err)
	}

	// Proper defer with error handling
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "close cursor failed: %v\n", err)
		}
	}()

	var executed []string

	for cursor.Next(ctx) {
		var record bson.M
		if err := cursor.Decode(&record); err != nil {
			message := fmt.Sprintf("decode record from '%s' collection failed.", collectionName)
			return nil, message, fmt.Errorf("decode failed: %w", err)
		}

		val, recordOk := record[fieldName]
		if !recordOk {
			message := fmt.Sprintf(
				"field '%s' not found in '%s' collection documents. ensure the field exists and documents are properly structured.",
				fieldName,
				collectionName,
			)

			return nil, message, fmt.Errorf("field '%s' missing from document", fieldName)
		}

		strVal, stringOk := val.(string)
		if !stringOk {
			message := fmt.Sprintf(
				"field '%s' in '%s' collection is not a string type. ensure the field contains string values.",
				fieldName,
				collectionName,
			)

			return nil, message, fmt.Errorf("field '%s' type assertion failed: got %T, expected string", fieldName, val)
		}

		executed = append(executed, strVal)
	}

	if err := cursor.Err(); err != nil {
		message := fmt.Sprintf("error iterating through '%s' cursor.", collectionName)
		return nil, message, fmt.Errorf("cursor error: %w", err)
	}

	return executed, "", nil
}

// ======================================================================================
// getExecutedSQLRecords - Generic helper for migrations & seeders
// ======================================================================================.
func getSQLRecords(ctx context.Context, database *Database, tableName, columnName string) ([]string, string, error) {
	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY executed_at ASC", columnName, tableName)

	rows, message, _, err := database.QuerySQL(ctx, query)
	if err != nil {
		msg := fmt.Sprintf("query the '%s' table failed. Ensure table exists, "+
			"database connection is healthy, or %s", tableName, message)

		return nil, msg, fmt.Errorf("query %s failed: %w", tableName, err)
	}

	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close rows failed: %v\n", err)
		}
	}()

	var executed []string

	for rows.Next() {
		var name string

		if err := rows.Scan(&name); err != nil {
			message = fmt.Sprintf("scan %s from '%s' table failed", columnName, tableName)
			return nil, message, fmt.Errorf("scan failed: %w", err)
		}

		executed = append(executed, name)
	}

	if err := rows.Err(); err != nil {
		message = fmt.Sprintf("error iterating '%s' rows", tableName)
		return nil, message, fmt.Errorf("iteration error: %w", err)
	}

	return executed, "", nil
}

// ======================================================================================
// getExecutedSQLRecords - Generic helper for migrations & seeders
// ======================================================================================.
func getScyllaRecords(ctx context.Context, database *Database, tableName, columnName string) ([]string, string, error) {
	query := fmt.Sprintf("SELECT %s FROM %s", columnName, tableName)

	iter, message, _, err := database.QueryCQL(ctx, query)
	if err != nil {
		msg := fmt.Sprintf("query the %s table in ScyllaDB failed. Ensure that the table exists "+
			"and the database connection is healthy, or %s", tableName, message)

		return nil, msg, fmt.Errorf("%w", err)
	}

	defer func() {
		if err := iter.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close scylla iter failed: %v\n", err)
		}
	}()

	var executed []string

	row := make(map[string]any)

	for iter.MapScan(row) {
		if name, ok := row[columnName].(string); ok {
			executed = append(executed, name)
		}

		row = make(map[string]any)
	}

	if err := iter.Close(); err != nil {
		message := ""

		return nil, message, fmt.Errorf("%w", err)
	}

	return executed, "", nil
}
