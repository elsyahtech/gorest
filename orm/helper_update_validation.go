package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"slices"
	"strconv"
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

	query, args := buildSQLBulkUpdateKeyQuery(table, rows, meta, driver)

	result, _, _, err := orm.Database.QuerySQL(ctx, orm.DatabaseConfig, query, args...)
	if err != nil {
		return orm.setDatabaseOperationError("Update", err)
	}

	defer func() {
		if err := result.Close(); err != nil {
			log.Print(err)
		}
	}()

	found, err := scanSQLBulkUpdateKeys(result, rows, meta)
	if err != nil {
		return orm.setDatabaseOperationError("Update", err)
	}

	missing := missingSQLBulkUpdatePrimaryKeys(rows, meta, found)

	if len(missing) > 0 {
		return orm.setError(
			"The requested records were not found. "+
				"Check these primary keys: "+strings.Join(missing, ", ")+". "+
				"Reference: https://github.com/elsyahtech/gorest/blob/main/docs/guide/orm.md#shared-database-error-contract",
			fmt.Errorf("update: records not found: %s", strings.Join(missing, ", ")),
			http.StatusNotFound,
		)
	}

	return nil
}

func buildSQLBulkUpdateKeyQuery(table string, rows []reflect.Value, meta columnMetaData, driver string) (string, []any) {
	var (
		clauses = make([]string, 0, len(rows))
		args    = make([]any, 0, len(meta.primaryKeyIndex)*len(rows))
	)

	for _, row := range rows {
		terms := make([]string, 0, len(meta.primaryKeyIndex))

		for pos, fieldIdx := range meta.primaryKeyIndex {
			terms = append(terms, meta.primaryKeyColumns[pos]+" = "+getPlaceholder(driver, len(args)+1))
			args = append(args, row.Field(fieldIdx).Interface())
		}

		clauses = append(clauses, "("+strings.Join(terms, " AND ")+")")
	}

	query := "SELECT " + strings.Join(meta.primaryKeyColumns, ", ") + " FROM " + table + " WHERE " + strings.Join(clauses, " OR ")

	return query, args
}

func scanSQLBulkUpdateKeys(result *sql.Rows, rows []reflect.Value, meta columnMetaData) (map[string]bool, error) {
	found := make(map[string]bool)

	for result.Next() {
		values := make([]any, len(meta.primaryKeyIndex))
		dest := make([]any, len(values))

		for i := range values {
			dest[i] = &values[i]
		}

		if err := result.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan SQL bulk update primary keys: %w", err)
		}

		for idx, fieldIndex := range meta.primaryKeyIndex {
			normalized, err := normalizeScannedSQLPrimaryKey(values[idx], rows[0].Field(fieldIndex))
			if err != nil {
				return nil, err
			}

			values[idx] = normalized
		}

		found[primaryKeyTuple(values)] = true
	}

	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("iterate SQL bulk update primary keys: %w", err)
	}

	return found, nil
}

func missingSQLBulkUpdatePrimaryKeys(rows []reflect.Value, meta columnMetaData, found map[string]bool) []string {
	var missing []string

	seen := make(map[string]bool)

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

	return missing
}

func normalizeScannedSQLPrimaryKey(raw any, input reflect.Value) (any, error) {
	target, ok := nonNilReflectValue(input)
	if !ok {
		return raw, nil
	}

	source := reflect.ValueOf(raw)
	if !source.IsValid() {
		return raw, nil
	}

	var text string
	//nolint:exhaustive
	switch source.Kind() {
	case reflect.String:
		text = source.String()
	case reflect.Slice:
		if source.Type().Elem().Kind() != reflect.Uint8 {
			return raw, nil
		}

		text = string(source.Bytes())
	default:
		return raw, nil
	}

	//nolint:exhaustive
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(text, 10, target.Type().Bits())
		if err != nil {
			return nil, fmt.Errorf("cannot convert scanned SQL primary key to %s", target.Type())
		}

		normalized := reflect.New(target.Type()).Elem()
		normalized.SetInt(value)

		return normalized.Interface(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(text, 10, target.Type().Bits())
		if err != nil {
			return nil, fmt.Errorf("cannot convert scanned SQL primary key to %s", target.Type())
		}

		normalized := reflect.New(target.Type()).Elem()
		normalized.SetUint(value)

		return normalized.Interface(), nil
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(text, target.Type().Bits())
		if err != nil {
			return nil, fmt.Errorf("cannot convert scanned SQL primary key to %s", target.Type())
		}

		normalized := reflect.New(target.Type()).Elem()
		normalized.SetFloat(value)

		return normalized.Interface(), nil
	case reflect.String:
		normalized := reflect.New(target.Type()).Elem()
		normalized.SetString(text)

		return normalized.Interface(), nil
	default:
		return raw, nil
	}
}

func (orm *ORM) validateBulkUpdateRowsMongo(ctx context.Context, coll *mongo.Collection, rows []reflect.Value, meta columnMetaData) error {
	if err := orm.validateUniqueBulkUpdateKeys(rows, meta); err != nil {
		return err
	}

	cursor, err := coll.Find(
		ctx,
		bson.M{"$or": buildMongoBulkUpdateFilters(rows, meta)},
		options.Find().SetProjection(buildMongoBulkUpdateProjection(meta)),
	)
	if err != nil {
		return orm.setMongoWriteError("Update", err)
	}

	defer func() {
		if err := cursor.Close(ctx); err != nil {
			log.Print(err)
		}
	}()

	found, err := scanMongoBulkUpdatePrimaryKeys(ctx, cursor, meta)
	if err != nil {
		return orm.setMongoWriteError("Update", err)
	}

	missing := missingBulkUpdatePrimaryKeys(rows, meta, found)
	if len(missing) > 0 {
		return orm.setError("Bulk update contains primary keys that do not "+
			"exist: "+strings.Join(missing, ", "), errors.New("update: records not found: "+strings.Join(missing, ", ")), http.StatusNotFound)
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

	message := "Remove duplicate primary keys from the bulk update " +
		"request: " + strings.Join(duplicateLabels, ", ") + ". " +
		"Reference: https://github.com/elsyahtech/gorest/blob/main/docs/guide/orm.md#shared-database-error-contract"

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

		column, applicable := taggedUniqueUpdateColumn(fieldIndex, field, meta)
		if !applicable {
			continue
		}

		conflicts = append(conflicts, taggedUniqueValueConflicts(rows, meta, fieldIndex, column)...)
	}

	if len(conflicts) == 0 {
		return nil
	}

	slices.Sort(conflicts)

	message := "The bulk update contains values that conflict with a unique " +
		"field: " + strings.Join(conflicts, "; ") + ". Reference: " + databaseErrorReference(orm.DatabaseConfig.Driver, databaseConflictError)

	return orm.setError(message, errors.New("update: duplicate unique values in request: "+strings.Join(conflicts, "; ")), http.StatusConflict)
}

func taggedUniqueUpdateColumn(fieldIndex int, field reflect.StructField, meta columnMetaData) (string, bool) {
	if !hasGorestTagOption(field.Tag.Get("gorest"), "unique") ||
		!containsFieldIndex(meta.columnIndex, fieldIndex) ||
		containsFieldIndex(meta.primaryKeyIndex, fieldIndex) {
		return "", false
	}

	column := strings.TrimSpace(strings.Split(field.Tag.Get("gorest"), ",")[0])

	return column, column != "" && column != "-"
}

func containsFieldIndex(indices []int, fieldIndex int) bool {
	for _, index := range indices {
		if index == fieldIndex {
			return true
		}
	}

	return false
}

func taggedUniqueValueConflicts(rows []reflect.Value, meta columnMetaData, fieldIndex int, column string) []string {
	rowsByValue := make(map[string][]string, len(rows))
	valuesByKey := make(map[string]any, len(rows))

	for _, row := range rows {
		fieldValue, present := nonNilReflectValue(row.Field(fieldIndex))
		if !present {
			continue
		}

		value := fieldValue.Interface()
		key := primaryKeyTuple([]any{value})
		valuesByKey[key] = value
		rowsByValue[key] = append(rowsByValue[key], primaryKeyLabel(bulkUpdatePrimaryKey(row, meta)))
	}

	var conflicts []string

	for key, primaryKeys := range rowsByValue {
		if len(primaryKeys) < 2 {
			continue
		}

		conflicts = append(conflicts, fmt.Sprintf("%s=%v (IDs: %s)", column, valuesByKey[key], strings.Join(primaryKeys, ", ")))
	}

	return conflicts
}

func nonNilReflectValue(valueParam reflect.Value) (reflect.Value, bool) {
	value := valueParam

	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return reflect.Value{}, false
		}

		value = value.Elem()
	}

	if !value.IsValid() {
		return reflect.Value{}, false
	}

	return value, true
}

func bulkUpdatePrimaryKey(row reflect.Value, meta columnMetaData) []any {
	values := make([]any, len(meta.primaryKeyIndex))
	for i, primaryKeyIndex := range meta.primaryKeyIndex {
		values[i] = row.Field(primaryKeyIndex).Interface()
	}

	return values
}

func primaryKeyTuple(values []any) string {
	parts := make([]string, len(values))
	for idx, val := range values {
		if b, ok := val.([]byte); ok {
			val = string(b)
		}

		value := reflect.ValueOf(val)

		if value.IsValid() {
			parts[idx] = fmt.Sprintf("%T:%v", val, val)

			switch value.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				parts[idx] = "number:" + fmt.Sprint(value.Int())

				continue
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				parts[idx] = "number:" + fmt.Sprint(value.Uint())

				continue
			case reflect.String:
				parts[idx] = "string:" + value.String()

				continue
			case reflect.Invalid,
				reflect.Bool,
				reflect.Uintptr,
				reflect.Float32,
				reflect.Float64,
				reflect.Complex64,
				reflect.Complex128,
				reflect.Array,
				reflect.Chan,
				reflect.Func,
				reflect.Interface,
				reflect.Map,
				reflect.Pointer,
				reflect.Slice,
				reflect.Struct,
				reflect.UnsafePointer:
			default:
				continue
			}

			continue
		}

		parts[idx] = fmt.Sprintf("%T:%v", val, val)
	}

	return strings.Join(parts, "\x00")
}

func primaryKeyLabel(values []any) string {
	parts := make([]string, len(values))

	for idx, val := range values {
		parts[idx] = fmt.Sprint(val)
	}

	return strings.Join(parts, "/")
} //nolint:revive
