package orm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

func (orm *ORM) deleteScylla(data any) error {
	activeDriver := orm.DatabaseConfig.Driver

	// 1. Validate DB instance.
	tableName, err := orm.validateDBInstance(data, activeDriver, "Delete")
	if err != nil {
		return orm.Error
	}

	// 2. Validate Struct (Ensure the struct has valid tags & primary_key)
	fieldReq := fieldRequirement{
		requirePrimaryKey: true,
		requireNonEmpty:   true,
	}

	rowsVal, err := orm.validateData(data, fieldReq, "Delete")
	if err != nil {
		return orm.Error
	}

	// 3. Verify that the processed data list contains at least one valid record to delete.
	if len(rowsVal) == 0 {
		const message = "ensure that the data list is not empty"

		return orm.setError(message, errors.New("delete: data list cannot be empty"))
	}

	// 4. Extract updatable columns, their struct field index, and the primary key column(s)/index from the struct tags.
	meta := orm.extractColumnMetaData(rowsVal, "Delete")

	// 5. Set context timeout
	execCtx, cancel := orm.newContext()
	defer cancel()

	var deleted int64
	for _, row := range rowsVal {
		var whereClause string
		var whereArgs []any
		if len(rowsVal) > 1 {
			whereClause, whereArgs, err = orm.buildDeletePayloadByPK(&reqBuildDeletePayloadByPK{
				rowsVal:      []reflect.Value{row},
				meta:         meta,
				activeDriver: activeDriver,
			})
		} else {
			var payload *resultBuildDeletePayload
			payload, err = orm.buildDeletePayload([]reflect.Value{row}, meta, activeDriver)
			if payload != nil {
				whereClause = payload.whereClause
				whereArgs = payload.whereArgs
			}
		}
		if err != nil {
			return orm.Error
		}
		if strings.TrimSpace(whereClause) == "" {
			const message = "refusing to run DELETE without a WHERE clause to avoid wiping the entire table"

			return orm.setError(message, errors.New("delete: missing WHERE condition"))
		}

		queryStr := fmt.Sprintf("DELETE FROM %s WHERE %s IF EXISTS", tableName, whereClause)
		applied, message, err := orm.Database.ScanCQL(execCtx, queryStr, whereArgs...)
		if err != nil {
			return orm.setError(message, err)
		}
		if applied {
			deleted++
		}
	}
	if deleted == 0 {
		return orm.setNotFound("delete")
	}
	orm.RowsAffected = deleted

	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: deleted,
	}

	return nil
}
