package orm

import (
	"errors"
	"fmt"
	"net/http"
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
		return orm.setError("Ensure the request contains at least one record.", errors.New("delete: data list cannot be empty"), http.StatusBadRequest)
	}

	// 4. Extract updatable columns, their struct field index, and the primary key column(s)/index from the struct tags.
	meta := orm.extractColumnMetaData(rowsVal, "Delete")

	// 5. Set context timeout
	execCtx, cancel := orm.newContext()
	defer cancel()

	var deleted int64

	for _, row := range rowsVal {
		var (
			whereClause string
			whereArgs   []any
		)

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
			return orm.setError(
				"refusing to run DELETE without a WHERE clause to avoid wiping the entire table",
				errors.New("delete: missing WHERE condition"),
				http.StatusBadRequest,
			)
		}

		queryStr := fmt.Sprintf("DELETE FROM %s WHERE %s IF EXISTS", tableName, whereClause)

		applied, _, _, err := orm.Database.MapScanCAS(execCtx, orm.DatabaseConfig, queryStr, whereArgs...)
		if err != nil {
			return orm.setScyllaWriteError("Delete", err)
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

	orm.Message = "Data deleted successfully"

	return nil
}
