package orm

import (
	"errors"
	"fmt"
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

	resDelPayload, err := orm.buildDeletePayload(rowsVal, meta, activeDriver)
	if err != nil {
		return orm.Error
	}

	// 12. Safety guard: NEVER allow a DELETE without a WHERE clause.
	if strings.TrimSpace(resDelPayload.whereClause) == "" {
		const message = "refusing to run DELETE without a WHERE clause to avoid wiping the entire table"

		return orm.setError(message, errors.New("delete: missing WHERE condition"))
	}

	// 13. Build DELETE Statement
	orm.safeWriteString(fmt.Sprintf("DELETE FROM %s WHERE %s", tableName, resDelPayload.whereClause))

	// 14. Set context timeout
	execCtx, cancel := orm.newContext()
	defer cancel()

	// 15. Execute Query with Context Timeout
	queryStr := orm.StringBuilder.String()

	message, err := orm.Database.ExecCQL(execCtx, queryStr, resDelPayload.whereArgs...)
	if err != nil {
		return orm.setError(message, err)
	}

	const note = "note"

	orm.Result = map[string]any{
		isSuccess: true,
		note:      "CQL execution completed successfully (rows affected count is not supported by Scylla)",
	}

	return nil
}
