package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	mongoOrSplit  = regexp.MustCompile(`(?i)\s+OR\s+`)
	mongoAndSplit = regexp.MustCompile(`(?i)\s+AND\s+`)
	mongoCondExpr = regexp.MustCompile(`(?is)^\s*([\w\.]+)\s*(=|!=|<>|>=|<=|>|<|NOT\s+LIKE|LIKE|NOT\s+IN|IN|IS\s+NOT\s+NULL|IS\s+NULL)\s*(.*?)\s*$`)
)

//nolint:revive
func (orm *ORM) buildMongoOptionFindMongo(
	collectionName string,
	preload *mongoPreloadPlan,
	sort bson.D,
	isSlice bool,
) (*options.FindOptionsBuilder, error) {
	if collectionName == "" {
		return nil, orm.setError(
			`Ensure that your call to Table("collectionName") is correct and match with your exists mongoDB.`,
			errors.New("collection name is empty"),
		)
	}

	if preload == nil {
		return nil, orm.setError(
			"Ensure your struct includs gorest tag and primary_key.",
			errors.New("find: preload is nil"),
		)
	}

	findOpts := options.Find()

	if projection := buildMongoProjection(orm.SelectedCols, collectionName); projection != nil {
		for _, col := range preload.requiredParentColumns() {
			projection[col] = 1
		}

		findOpts.SetProjection(projection)
	}

	// 10. Build Sort clauses
	if len(sort) > 0 {
		findOpts.SetSort(sort)
	}

	// 11. Build LIMIT clauses
	limitVal := orm.LimitVal

	if !isSlice && limitVal <= 0 {
		limitVal = 1
	}

	if limitVal > 0 {
		findOpts.SetLimit(int64(limitVal))
	}

	// 12. Build OFFSET clauses
	if orm.OffsetVal > 0 {
		findOpts.SetSkip(int64(orm.OffsetVal))
	}

	return findOpts, nil
}

func mongoFieldName(name, table string) string {
	return strings.TrimPrefix(name, table+".")
}

// String 24-hex pada field _id otomatis jadi ObjectID.
func mongoValue(field string, val any, objectIDCols map[string]bool) any {
	if objectIDCols[strings.ToLower(field)] {
		if str, ok := val.(string); ok {
			if oid, err := bson.ObjectIDFromHex(str); err == nil {
				return oid
			}
		}
	}

	return val
}

func combineMongo(op string, list []bson.M) bson.M {
	if len(list) == 1 {
		return list[0]
	}

	return bson.M{op: list}
}

// ========================
// "%john_" -> "^.*john.$"
// ========================.
func likeToRegex(pattern string) string {
	escaped := regexp.QuoteMeta(pattern)
	escaped = strings.ReplaceAll(escaped, "%", ".*")
	escaped = strings.ReplaceAll(escaped, "_", ".")

	return "^" + escaped + "$"
}

// Flatten slice arguments for IN: IN (?) with []int{1,2} or IN (?, ?)
func flattenMongoArgs(field string, args []any, objectIDCols map[string]bool) []any {
	var out []any

	for _, arg := range args {
		refVal := reflect.ValueOf(arg)

		if refVal.IsValid() && (refVal.Kind() == reflect.Slice || refVal.Kind() == reflect.Array) {
			if _, isBytes := arg.([]byte); !isBytes {
				for idx := 0; idx < refVal.Len(); idx++ {
					out = append(out, mongoValue(field, refVal.Index(idx).Interface(), objectIDCols))
				}

				continue
			}
		}

		out = append(out, mongoValue(field, arg, objectIDCols))
	}

	return out
}

func (orm *ORM) buildMongoSort(clauses []string, collectionName string) (bson.D, error) {
	if collectionName == "" {
		return nil, orm.setError(
			`Ensure that your call to Table("collectionName") is correct and match with your exists mongoDB.`,
			errors.New("collection name is empty"),
		)
	}

	var sort bson.D

	const message = "Ensure .OrderBy() uses the format \"field [ASC|DESC]\"."

	for _, clause := range clauses {
		for _, part := range strings.Split(clause, ",") {
			fields := strings.Fields(part)

			if len(fields) == 0 {
				continue
			}

			if len(fields) > 2 {
				return nil, orm.setError(message, fmt.Errorf("unsupported order by expression: %q", strings.TrimSpace(part)))
			}

			dir := 1

			if len(fields) == 2 {
				switch strings.ToUpper(fields[1]) {
				case asc:
				case desc:
					dir = -1
				default:
					return nil, orm.setError(message, fmt.Errorf("unsupported order direction: %q", fields[1]))
				}
			}

			sort = append(sort, bson.E{Key: mongoFieldName(fields[0], collectionName), Value: dir})
		}
	}

	return sort, nil
}

// nil = get all fields.
func buildMongoProjection(cols []string, table string) bson.M {
	projection := bson.M{}

	for _, col := range cols {
		for _, coll := range strings.Split(col, ",") {
			coll = strings.TrimSpace(coll)
			if coll == "" {
				continue
			}

			if coll == "*" || strings.HasSuffix(coll, ".*") {
				return nil
			}

			projection[mongoFieldName(coll, table)] = 1
		}
	}

	if len(projection) == 0 {
		return nil
	}

	return projection
}

func normalizeBSONValue(val any) any {
	switch typ := val.(type) {
	case bson.ObjectID:
		return typ.Hex()
	case bson.DateTime:
		return typ.Time().UTC()
	case bson.Binary:
		return typ.Data
	case bson.A:
		out := make([]any, len(typ))

		for idx, item := range typ {
			out[idx] = normalizeBSONValue(item)
		}

		return out
	case bson.M:
		out := make(map[string]any, len(typ))

		for kTyp, item := range typ {
			out[kTyp] = normalizeBSONValue(item)
		}

		return out
	case bson.D:
		out := make(map[string]any, len(typ))

		for _, e := range typ {
			out[e.Key] = normalizeBSONValue(e.Value)
		}

		return out
	default:
		return val
	}
}

//nolint:revive
func (orm *ORM) buildRowsScanFindMongo(
	execCtx context.Context,
	cursor *mongo.Cursor,
	structType reflect.Type,
	fieldLookupMap map[string]*structFieldInfo,
	isSlice bool,
	valElem *reflect.Value,
	sliceVal *reflect.Value,
) (*mongo.Cursor, error) {
	for cursor.Next(execCtx) {
		var doc bson.M

		if err := cursor.Decode(&doc); err != nil {
			return nil, orm.setError(
				"ensure that the MongoDB document can be decoded",
				err,
			)
		}

		newStructPtr := reflect.New(structType)
		newStructVal := newStructPtr.Elem()

		for key, raw := range doc {
			if raw == nil {
				continue
			}

			keyLower := strings.ToLower(key)
			fieldInfo, found := fieldLookupMap[keyLower]

			if !found {
				continue
			}

			fieldVal := newStructVal.Field(fieldInfo.index)
			if !fieldVal.CanSet() {
				continue
			}

			if err := assignReflectValue(fieldVal, normalizeBSONValue(raw)); err != nil {
				return nil, orm.setError(
					"ensure that document field types can be converted to struct field types",
					fmt.Errorf("mapping error for field %q: %w", key, err),
				)
			}
		}

		orm.RowsAffected++

		if !isSlice {
			valElem.Set(newStructVal)

			break
		}

		if valElem.Type().Elem().Kind() == reflect.Pointer {
			*sliceVal = reflect.Append(*sliceVal, newStructPtr)
		} else {
			*sliceVal = reflect.Append(*sliceVal, newStructVal)
		}
	}

	if isSlice {
		valElem.Set(*sliceVal)
	}

	if err := cursor.Err(); err != nil {
		return nil, orm.setError(
			"ensure that database result stream is valid and complete",
			err,
		)
	}

	return cursor, nil
}

func (orm *ORM) loadFindMogoPreloadData(
	execCtx context.Context,
	preload *mongoPreloadPlan,
	valElem *reflect.Value,
	isSlice bool,
) error {
	if !preload.isEmpty() {
		parents, err := orm.collectParents(*valElem, isSlice)
		if err != nil {
			return orm.Error
		}

		if err := orm.loadMongoPreloads(execCtx, preload, parents); err != nil {
			return orm.Error
		}
	}

	return nil
} //nolint:revive
