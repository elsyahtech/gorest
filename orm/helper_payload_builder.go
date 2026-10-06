package orm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// fieldUpdateData holds the extracted non-PK column data ready to be set for a single-row update query.
type resultBuildUpdatePayload struct {
	columns    []string
	values     []any
	setClauses []string
}

// Extract Single Update Fields: separates PK and non-PK columns for single update purposes.
func buildUpdatePayload(row reflect.Value, meta columnMetaData, activeDriver string, startArgCounter int) *resultBuildUpdatePayload {
	var (
		setClauses []string
		columns    []string
		values     []any
	)

	argCounter := startArgCounter

	for colIndex, colIdx := range meta.columnIndex {
		colName := meta.columns[colIndex]
		isPK := false

		for _, pkIdx := range meta.primaryKeyIndex {
			if pkIdx == colIdx {
				isPK = true

				break
			}
		}

		if isPK {
			continue
		}

		fieldVal := row.Field(colIdx).Interface()

		columns = append(columns, colName)
		values = append(values, fieldVal)

		// If the driver requires placeholders (SQL/Scylla)
		if activeDriver != "mongodb" && activeDriver != "mongo" {
			ph := getPlaceholder(activeDriver, argCounter)

			setClauses = append(setClauses, fmt.Sprintf("%s = %s", colName, ph))

			argCounter++
		} else {
			// For MongoDB, a standard key-value mapping suffices.
			setClauses = append(setClauses, colName)
		}
	}

	return &resultBuildUpdatePayload{
		columns:    columns,
		values:     values,
		setClauses: setClauses,
	}
}

func buildUpdatePayloadMongo(row reflect.Value, meta columnMetaData) bson.M {
	updateFields := bson.M{}

	for colIndex, colIdx := range meta.columnIndex {
		isPK := false

		for _, pkIdx := range meta.primaryKeyIndex {
			if pkIdx == colIdx {
				isPK = true

				break
			}
		}

		if !isPK {
			updateFields[meta.columns[colIndex]] = row.Field(colIdx).Interface()
		}
	}

	return updateFields
}

type resultBuildDeletePayload struct {
	whereClause string
	whereArgs   []any
}

func (orm *ORM) buildDeletePayload(rowsVal []reflect.Value, meta columnMetaData, activeDriver string) (*resultBuildDeletePayload, error) {
	isBatch := len(rowsVal) > 1

	var (
		err         error
		whereClause string
		whereArgs   []any
	)

	switch {
	case isBatch:
		if len(meta.primaryKeyIndex) == 0 {
			message := "ensure the struct has at least one field tagged " +
				"`gorest:\"col_name,primary_key\"` to perform a batch delete"

			return nil, orm.setError(message, errors.New("delete: batch delete requires a primary key column"))
		}

		if len(meta.primaryKeyIndex) == 1 {
			// Single-column PK -> WHERE pk IN (?, ?, ?, ...)
			whereClause, whereArgs, err = orm.pkMarkBuildDeletePayload(
				&reqPKMarkBuildDeletePayload{
					rowsVal:      rowsVal,
					meta:         meta,
					whereArgs:    whereArgs,
					activeDriver: activeDriver,
				},
			)
			if err != nil {
				return nil, orm.Error
			}
		} else {
			whereClause, whereArgs, err = orm.pkGroupBuildDeletePayload(
				&reqPKkGroupBuildDeletePayload{
					rowsVal:      rowsVal,
					meta:         meta,
					whereArgs:    whereArgs,
					activeDriver: activeDriver,
				},
			)
			if err != nil {
				return nil, orm.Error
			}
		}

	default:
		// Single row delete: prefer explicit .Where(), fallback to primary key.
		whereClause, whereArgs, err = orm.buildDeletePayloadByDefault(whereClause, activeDriver, whereArgs, meta, rowsVal)
		if err != nil {
			return nil, orm.Error
		}
	}

	return &resultBuildDeletePayload{
		whereClause: whereClause,
		whereArgs:   whereArgs,
	}, nil
}

func (orm *ORM) buildDeletePayloadByDefault(
	currentWhereClause, activeDriver string,
	currentWhereArgs []any,
	meta columnMetaData,
	rowsVal []reflect.Value,
) (whereClause string, whereArgs []any, err error) {
	whereClause = currentWhereClause
	whereArgs = currentWhereArgs

	switch {
	case len(orm.WhereClauses) > 0:
		whereClause, whereArgs, err = orm.buildDeletePayloadByWhereClause(
			&reqBuildDeletePayloadByWhereClause{
				whereArgs:    whereArgs,
				whereClause:  whereClause,
				activeDriver: activeDriver,
			},
		)
		if err != nil {
			return "", nil, orm.Error
		}
	case len(meta.primaryKeyIndex) > 0:
		whereClause, whereArgs, err = orm.buildDeletePayloadByPK(
			&reqBuildDeletePayloadByPK{
				rowsVal:      rowsVal,
				meta:         meta,
				whereArgs:    whereArgs,
				whereClause:  whereClause,
				activeDriver: activeDriver,
			},
		)
		if err != nil {
			return "", nil, orm.Error
		}
	default:
		return "", nil, orm.setError(
			"call .Where(\"condition\", args...) or ensure the struct has a "+
				"`gorest:\"col_name,primary_key\"` tagged field before calling Delete()",
			errors.New("delete: no WHERE condition and no primary key available"),
		)
	}

	return whereClause, whereArgs, nil
}

type reqPKMarkBuildDeletePayload struct {
	meta         columnMetaData
	activeDriver string
	rowsVal      []reflect.Value
	whereArgs    []any
}

func (orm *ORM) pkMarkBuildDeletePayload(req *reqPKMarkBuildDeletePayload) (whereClause string, whereArgs []any, err error) {
	var sqlPKMarks []string

	whereArgs = req.whereArgs

	for rowIdx, rowVal := range req.rowsVal {
		primaryKeyVal := rowVal.Field(req.meta.primaryKeyIndex[0])

		if primaryKeyVal.IsZero() {
			return "", nil, orm.setError(
				fmt.Sprintf(
					"element at index %d has a zero value for primary key column `%s`; "+
						"ensure every row in the batch has its primary key set", rowIdx, req.meta.columns[0],
				),
				errors.New("delete: primary key value is zero, refusing to delete"),
			)
		}

		whereArgs = append(whereArgs, primaryKeyVal.Interface())
		sqlPKMarks = append(sqlPKMarks, "?")
	}

	rawWhereClause := fmt.Sprintf("%s IN (%s)", req.meta.columns[0], strings.Join(sqlPKMarks, ", "))

	whereClause, _ = normalizeWherePlaceholders(rawWhereClause, req.activeDriver, 1)

	return whereClause, whereArgs, nil
}

type reqPKkGroupBuildDeletePayload struct {
	meta         columnMetaData
	activeDriver string
	rowsVal      []reflect.Value
	whereArgs    []any
}

func (orm *ORM) pkGroupBuildDeletePayload(req *reqPKkGroupBuildDeletePayload) (whereClause string, whereArgs []any, err error) {
	var pkGroups []string

	whereArgs = req.whereArgs

	for rowIdx, rowVal := range req.rowsVal {
		var conds []string

		for idx, primaryKeyIdx := range req.meta.primaryKeyIndex {
			pkVal := rowVal.Field(primaryKeyIdx)

			if pkVal.IsZero() {
				return "", nil, orm.setError(
					fmt.Sprintf(
						"element at index %d has a zero value for primary key column `%s`; "+
							"ensure every row in the batch has its primary key set", rowIdx, req.meta.columns[idx],
					),
					errors.New("delete: primary key value is zero, refusing to delete"),
				)
			}

			whereArgs = append(whereArgs, pkVal.Interface())
			conds = append(conds, fmt.Sprintf("%s = ?", req.meta.columns[idx]))
		}

		pkGroups = append(pkGroups, fmt.Sprintf("(%s)", strings.Join(conds, " AND ")))
	}

	rawWhereClause := strings.Join(pkGroups, " OR ")

	whereClause, _ = normalizeWherePlaceholders(rawWhereClause, req.activeDriver, 1)

	return whereClause, whereArgs, nil
}

type reqBuildDeletePayloadByWhereClause struct {
	whereClause  string
	activeDriver string
	whereArgs    []any
}

func (orm *ORM) buildDeletePayloadByWhereClause(req *reqBuildDeletePayloadByWhereClause) (whereClause string, whereArgs []any, err error) {
	rawWhereClause := strings.Join(orm.WhereClauses, " AND ")

	// Sanity check
	if expected := countActivePlaceholders(rawWhereClause, req.activeDriver); expected != len(orm.WhereArgs) {
		return "", nil, orm.setError(
			fmt.Sprintf(
				"WHERE clause expects %d bound value(s) but %d were provided via .Where(...) args",
				expected, len(orm.WhereArgs),
			),
			errors.New("delete: mismatched WHERE placeholder count"),
		)
	}

	whereClause, _ = normalizeWherePlaceholders(rawWhereClause, req.activeDriver, 1)

	whereArgs = orm.WhereArgs

	return whereClause, whereArgs, nil
}

type reqBuildDeletePayloadByPK struct {
	meta         columnMetaData
	whereClause  string
	activeDriver string
	rowsVal      []reflect.Value
	whereArgs    []any
}

func (orm *ORM) buildDeletePayloadByPK(req *reqBuildDeletePayloadByPK) (whereClause string, whereArgs []any, err error) {
	var conds []string

	for primaryKey, fieldIdx := range req.meta.primaryKeyIndex {
		pkVal := req.rowsVal[0].Field(fieldIdx)

		if pkVal.IsZero() {
			return "", nil, orm.setError(
				fmt.Sprintf(
					"primary key column `%s` has a zero value; either set it explicitly on the struct "+
						"or use .Where(...) to specify the delete condition", req.meta.columns[primaryKey],
				),
				errors.New("delete: primary key value is zero, refusing to delete"),
			)
		}

		whereArgs = append(whereArgs, pkVal.Interface())
		conds = append(conds, fmt.Sprintf("%s = ?", req.meta.columns[primaryKey]))
	}

	rawClause := strings.Join(conds, " AND ")

	whereClause, _ = normalizeWherePlaceholders(rawClause, req.activeDriver, 1)

	return whereClause, whereArgs, nil
}

func (orm *ORM) buildDeletePayloadMongo(rowsVal []reflect.Value, meta columnMetaData) (map[string]any, error) {
	isBatch := len(rowsVal) > 1

	var (
		err    error
		filter bson.M
	)

	switch {
	case isBatch:
		if len(meta.primaryKeyIndex) == 0 {
			return nil, orm.setError(
				"ensure the struct has at least one field tagged "+
					"`gorest:\"col_name,primary_key\"` to perform a batch delete",
				errors.New("delete: batch delete requires a primary key column"),
			)
		}

		if len(meta.primaryKeyIndex) == 1 {
			// Single-column PK -> WHERE pk IN (?, ?, ?, ...)
			filter, err = orm.pkMarkBuildDeletePayloadMongo(rowsVal, meta)
			if err != nil {
				return nil, orm.Error
			}
		} else {
			// Composite PK -> (pk1 = ? AND pk2 = ?) OR (pk1 = ? AND pk2 = ?) OR ...
			filter, err = orm.pkGroupBuildDeletePayloadMongo(rowsVal, meta)
			if err != nil {
				return nil, orm.Error
			}
		}
	default:
		// Single row delete: prefer explicit .Where(), fallback to primary key.
		switch {
		case len(orm.WhereClauses) > 0:
			translated, message, err := convertWhereToMongoFilter(orm.WhereClauses, orm.WhereArgs)
			if err != nil {
				return nil, orm.setError(message, err)
			}

			filter = translated
		case len(meta.primaryKeyIndex) > 0:
			filter, err = orm.buildDeletePayloadByPKMongo(rowsVal, meta)
			if err != nil {
				return nil, orm.Error
			}
		default:
			return nil, orm.setError(
				"call .Where(\"condition\", args...) or ensure the struct has a "+
					"`gorest:\"col_name,primary_key\"` tagged field before calling Delete()",
				errors.New("delete: no WHERE condition and no primary key available"),
			)
		}
	}

	return filter, nil
}

func (orm *ORM) buildDeletePayloadByPKMongo(rowsVal []reflect.Value, meta columnMetaData) (bson.M, error) {
	filter := bson.M{}

	for primaryKey, fieldIdx := range meta.primaryKeyIndex {
		pkVal := rowsVal[0].Field(fieldIdx)

		if pkVal.IsZero() {
			return nil, orm.setError(
				fmt.Sprintf(
					"primary key field `%s` has a zero value; either set it explicitly on the struct "+
						"or use .Where(...) to specify the delete condition", meta.columns[primaryKey],
				),
				errors.New("delete: primary key value is zero, refusing to delete"),
			)
		}

		filter[meta.columns[primaryKey]] = pkVal.Interface()
	}

	return filter, nil
}

func (orm *ORM) pkMarkBuildDeletePayloadMongo(rowsVal []reflect.Value, meta columnMetaData) (bson.M, error) {
	var mongoPKMarks []any

	for rowIdx, rowVal := range rowsVal {
		pkVal := rowVal.Field(meta.primaryKeyIndex[0])

		if pkVal.IsZero() {
			return nil, orm.setError(
				fmt.Sprintf(
					"element at index %d has a zero value for primary key field `%s`; "+
						"ensure every document in the batch has its primary key set", rowIdx, meta.columns[0],
				),
				errors.New("delete: primary key value is zero, refusing to delete"),
			)
		}

		mongoPKMarks = append(mongoPKMarks, pkVal.Interface())
	}

	filter := bson.M{meta.columns[0]: bson.M{
		//nolint:goconst
		"$in": mongoPKMarks,
	}}

	return filter, nil
}

func (orm *ORM) pkGroupBuildDeletePayloadMongo(rowsVal []reflect.Value, meta columnMetaData) (bson.M, error) {
	var pkGroups []bson.M

	for rowIdx, rowVal := range rowsVal {
		conds := bson.M{}

		for primaryKey, fieldIdx := range meta.primaryKeyIndex {
			pkVal := rowVal.Field(fieldIdx)

			if pkVal.IsZero() {
				return nil, orm.setError(
					fmt.Sprintf(
						"element at index %d has a zero value for primary key field `%s`; "+
							"ensure every document in the batch has its primary key set", rowIdx, meta.columns[fieldIdx],
					),
					errors.New("delete: primary key value is zero, refusing to delete"),
				)
			}

			conds[meta.columns[primaryKey]] = pkVal.Interface()
		}

		pkGroups = append(pkGroups, conds)
	}

	filter := bson.M{"$or": pkGroups}

	return filter, nil
} //nolint:revive
