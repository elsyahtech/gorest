package orm

import (
	"errors"
	"fmt"
	"strings"
)

func (orm *ORM) deleteSQL(data any) error {
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

	resDelPayload, err := orm.buildDeletePayload(rowsVal, meta, activeDriver)
	if err != nil {
		return orm.Error
	}

	// 6. Safety guard: NEVER allow a DELETE without a WHERE clause or without tag primary key.
	if strings.TrimSpace(resDelPayload.whereClause) == "" {
		const message = "refusing to run DELETE without a WHERE clause to avoid wiping the entire table"

		return orm.setError(message, errors.New("delete: missing WHERE condition"))
	}

	// 7. Build DELETE Statement
	orm.safeWriteString(fmt.Sprintf("DELETE FROM %s WHERE %s", tableName, resDelPayload.whereClause))

	// 8. Set context timeout
	execCtx, cancel := orm.newContext()
	defer cancel()

	// 9. Execute Query DB
	queryStr := orm.StringBuilder.String()

	result, message, err := orm.Database.ExecSQL(execCtx, queryStr, resDelPayload.whereArgs...)
	if err != nil {
		return orm.setError(message, err)
	}

	orm.extractResultDeleteSQL(result)
	if orm.RowsAffected == 0 {
		return orm.setNotFound("delete")
	}

	return nil
}
