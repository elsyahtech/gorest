package orm

import (
	"context"
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func buildMongoBulkUpdateFilters(rows []reflect.Value, meta columnMetaData) []bson.M {
	filters := make([]bson.M, 0, len(rows))

	for _, row := range rows {
		filter := bson.M{}

		for pos, idx := range meta.primaryKeyIndex {
			filter[meta.primaryKeyColumns[pos]] = row.Field(idx).Interface()
		}

		filters = append(filters, filter)
	}

	return filters
}

func buildMongoBulkUpdateProjection(meta columnMetaData) bson.M {
	projection := bson.M{}

	for _, col := range meta.primaryKeyColumns {
		projection[col] = 1
	}

	return projection
}

func scanMongoBulkUpdatePrimaryKeys(
	ctx context.Context,
	cursor *mongo.Cursor,
	meta columnMetaData,
) (map[string]bool, error) {
	found := make(map[string]bool)

	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode MongoDB bulk update key: %w", err)
		}

		values := make([]any, len(meta.primaryKeyColumns))
		for idx, column := range meta.primaryKeyColumns {
			values[idx] = doc[column]
		}

		found[primaryKeyTuple(values)] = true
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate MongoDB bulk update keys: %w", err)
	}

	return found, nil
}

func missingBulkUpdatePrimaryKeys(rows []reflect.Value, meta columnMetaData, found map[string]bool) []string {
	var (
		missing []string
		seen    = make(map[string]bool)
	)

	for _, row := range rows {
		values := make([]any, len(meta.primaryKeyIndex))
		for idx, fieldIdx := range meta.primaryKeyIndex {
			values[idx] = row.Field(fieldIdx).Interface()
		}

		key := primaryKeyTuple(values)
		if found[key] || seen[key] {
			continue
		}

		missing = append(missing, primaryKeyLabel(values))
		seen[key] = true
	}

	return missing
}
