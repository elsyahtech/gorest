package orm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// validateBulkUpdateRowsSQL checks all requested primary keys in one read before
// the bulk UPDATE is issued. It deliberately ignores custom update predicates:
// this validation guarantees the supplied records exist.
func (orm *ORM) validateBulkUpdateRowsSQL(ctx context.Context, table string, rows []reflect.Value, meta columnMetaData, driver string) error {
	var clauses []string
	var args []any
	for _, row := range rows {
		var terms []string

		for pos, fieldIdx := range meta.primaryKeyIndex {
			terms = append(terms, meta.primaryKeyColumns[pos]+" = "+getPlaceholder(driver, len(args)+1))
			args = append(args, row.Field(fieldIdx).Interface())
		}
		clauses = append(clauses, "("+strings.Join(terms, " AND ")+")")
	}
	query := "SELECT " + strings.Join(meta.primaryKeyColumns, ", ") + " FROM " + table + " WHERE " + strings.Join(clauses, " OR ")
	result, _, _, err := orm.Database.QuerySQL(ctx, orm.DatabaseConfig, query, args...)
	if err != nil {
		return orm.setDatabaseOperationError("Update", err)
	}
	defer result.Close()
	found := map[string]bool{}
	for result.Next() {
		values := make([]any, len(meta.primaryKeyIndex))
		dest := make([]any, len(values))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := result.Scan(dest...); err != nil {
			return orm.setDatabaseOperationError("Update", err)
		}
		found[primaryKeyTuple(values)] = true
	}

	if err := result.Err(); err != nil {
		return orm.setDatabaseOperationError("Update", err)
	}

	var missing []string

	seen := map[string]bool{}

	for _, row := range rows {
		values := make([]any, len(meta.primaryKeyIndex))
		for i, idx := range meta.primaryKeyIndex {
			values[i] = row.Field(idx).Interface()
		}
		key := primaryKeyTuple(values)
		if !found[key] && !seen[key] {
			missing = append(missing, primaryKeyLabel(values))
			seen[key] = true
		}
	}

	if len(missing) > 0 {
		return orm.setError("Bulk update contains primary keys that do not exist: "+strings.Join(missing, ", "), fmt.Errorf("update: records not found: %s", strings.Join(missing, ", ")), http.StatusNotFound)
	}

	return nil
}

func (orm *ORM) validateBulkUpdateRowsMongo(ctx context.Context, coll *mongo.Collection, rows []reflect.Value, meta columnMetaData) error {
	filters := make([]bson.M, 0, len(rows))
	for _, row := range rows {
		filter := bson.M{}
		for pos, idx := range meta.primaryKeyIndex {
			filter[meta.primaryKeyColumns[pos]] = row.Field(idx).Interface()
		}
		filters = append(filters, filter)
	}
	projection := bson.M{}
	for _, col := range meta.primaryKeyColumns {
		projection[col] = 1
	}
	cursor, err := coll.Find(ctx, bson.M{"$or": filters}, options.Find().SetProjection(projection))
	if err != nil {
		return orm.setMongoWriteError("Update", err)
	}
	defer cursor.Close(ctx)
	found := map[string]bool{}
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return orm.setMongoWriteError("Update", err)
		}
		values := make([]any, len(meta.primaryKeyColumns))
		for i, col := range meta.primaryKeyColumns {
			values[i] = doc[col]
		}
		found[primaryKeyTuple(values)] = true
	}
	if err := cursor.Err(); err != nil {
		return orm.setMongoWriteError("Update", err)
	}
	var missing []string
	seen := map[string]bool{}
	for _, row := range rows {
		values := make([]any, len(meta.primaryKeyIndex))
		for i, idx := range meta.primaryKeyIndex {
			values[i] = row.Field(idx).Interface()
		}
		key := primaryKeyTuple(values)
		if !found[key] && !seen[key] {
			missing = append(missing, primaryKeyLabel(values))
			seen[key] = true
		}
	}
	if len(missing) > 0 {
		return orm.setError("Bulk update contains primary keys that do not exist: "+strings.Join(missing, ", "), errors.New("update: records not found: "+strings.Join(missing, ", ")), http.StatusNotFound)
	}
	return nil
}

func primaryKeyTuple(values []any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		if b, ok := v.([]byte); ok {
			v = string(b)
		}
		value := reflect.ValueOf(v)
		if value.IsValid() {
			switch value.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				parts[i] = "number:" + fmt.Sprint(value.Int())
				continue
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				parts[i] = "number:" + fmt.Sprint(value.Uint())
				continue
			case reflect.String:
				parts[i] = "string:" + value.String()
				continue
			}
		}
		parts[i] = fmt.Sprintf("%T:%v", v, v)
	}
	return strings.Join(parts, "\x00")
}

func primaryKeyLabel(values []any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, "/")
}
