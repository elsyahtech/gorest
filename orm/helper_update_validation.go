package orm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// validateBulkUpdateRowsSQL checks all requested primary keys in one read before
// the bulk UPDATE is issued. It deliberately ignores custom update predicates:
// this validation guarantees the supplied records exist.
func (orm *ORM) validateBulkUpdateRowsSQL(ctx context.Context, table string, rows []reflect.Value, meta columnMetaData, driver string) error {
	if err := orm.validateUniqueBulkUpdateKeys(rows, meta); err != nil {
		return err
	}

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
		return orm.setError(
			"Bulk update contains primary keys that do not exist: "+strings.Join(missing, ", "),
			fmt.Errorf("update: records not found: %s", strings.Join(missing, ", ")),
			http.StatusNotFound,
		)
	}

	return nil
}

func (orm *ORM) validateBulkUpdateRowsMongo(ctx context.Context, coll *mongo.Collection, rows []reflect.Value, meta columnMetaData) error {
	if err := orm.validateUniqueBulkUpdateKeys(rows, meta); err != nil {
		return err
	}

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

func (orm *ORM) validateUniqueBulkUpdateKeys(rows []reflect.Value, meta columnMetaData) error {
	seen := make(map[string]bool, len(rows))
	duplicates := make(map[string]bool)
	var duplicateLabels []string
	for _, row := range rows {
		values := make([]any, len(meta.primaryKeyIndex))
		for i, idx := range meta.primaryKeyIndex {
			values[i] = row.Field(idx).Interface()
		}
		key := primaryKeyTuple(values)
		if seen[key] && !duplicates[key] {
			duplicateLabels = append(duplicateLabels, primaryKeyLabel(values))
			duplicates[key] = true
		}
		seen[key] = true
	}
	if len(duplicateLabels) == 0 {
		return nil
	}
	message := "Bulk update contains duplicate primary keys: " + strings.Join(duplicateLabels, ", ")
	return orm.setError(message, errors.New("update: duplicate primary keys: "+strings.Join(duplicateLabels, ", ")), http.StatusBadRequest)
}

// validateTaggedUniqueBulkUpdateValues rejects repeated values in the request
// for fields explicitly marked unique in the gorest struct tag. It is a request
// validation guard and does not replace a database uniqueness constraint.
func (orm *ORM) validateTaggedUniqueBulkUpdateValues(rows []reflect.Value, meta columnMetaData) error {
	if len(rows) < 2 {
		return nil
	}

	structType := rows[0].Type()
	var conflicts []string
	for fieldIndex := 0; fieldIndex < structType.NumField(); fieldIndex++ {
		field := structType.Field(fieldIndex)
		if !hasGorestTagOption(field.Tag.Get("gorest"), "unique") {
			continue
		}
		includedInUpdate := false
		isPrimaryKey := false
		for _, index := range meta.columnIndex {
			if index == fieldIndex {
				includedInUpdate = true
				break
			}
		}
		for _, index := range meta.primaryKeyIndex {
			if index == fieldIndex {
				isPrimaryKey = true
				break
			}
		}
		if !includedInUpdate || isPrimaryKey {
			continue
		}
		column := strings.TrimSpace(strings.Split(field.Tag.Get("gorest"), ",")[0])
		if column == "" || column == "-" {
			continue
		}

		rowsByValue := make(map[string][]string, len(rows))
		valuesByKey := make(map[string]any, len(rows))
		for _, row := range rows {
			fieldValue := row.Field(fieldIndex)
			for fieldValue.Kind() == reflect.Pointer || fieldValue.Kind() == reflect.Interface {
				if fieldValue.IsNil() {
					break
				}
				fieldValue = fieldValue.Elem()
			}
			if !fieldValue.IsValid() || ((fieldValue.Kind() == reflect.Pointer || fieldValue.Kind() == reflect.Interface) && fieldValue.IsNil()) {
				continue
			}
			value := fieldValue.Interface()
			key := primaryKeyTuple([]any{value})
			valuesByKey[key] = value
			primaryKey := make([]any, len(meta.primaryKeyIndex))
			for i, primaryKeyIndex := range meta.primaryKeyIndex {
				primaryKey[i] = row.Field(primaryKeyIndex).Interface()
			}
			rowsByValue[key] = append(rowsByValue[key], primaryKeyLabel(primaryKey))
		}

		for key, primaryKeys := range rowsByValue {
			if len(primaryKeys) < 2 {
				continue
			}
			conflicts = append(conflicts, fmt.Sprintf("%s=%v (IDs: %s)", column, valuesByKey[key], strings.Join(primaryKeys, ", ")))
		}
	}

	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	message := "Bulk update contains duplicate values for unique field(s): " + strings.Join(conflicts, "; ")
	return orm.setError(message, errors.New("update: duplicate unique values in request: "+strings.Join(conflicts, "; ")), http.StatusConflict)
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
