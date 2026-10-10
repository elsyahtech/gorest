package orm

import (
	"errors"
	"fmt"
	"strings"
)

func (orm *ORM) createSQL(data any) error {
	activeDriver := orm.DatabaseConfig.Driver

	// 1. Validate the DB instance (connection available)
	tableName, err := orm.validateDBInstance(data, activeDriver, "Create")
	if err != nil {
		return orm.Error
	}

	// 2. Validate Struct (Ensure the struct has valid tags & primary_key)
	fieldReq := fieldRequirement{
		requirePrimaryKey: true,
		requireNonEmpty:   true,
	}

	// Nama method nya nyambung ngga? membuat developer bingung ngga?
	rowsVal, err := orm.validateData(data, fieldReq, "Create")
	if err != nil {
		return orm.Error
	}

	// 3. Verify that the processed data list contains at least one valid record to insert
	if len(rowsVal) == 0 {
		return orm.setError("ensure that the data list is not empty", errors.New("create: data list cannot be empty"))
	}

	// 4. Extract updatable columns, their struct field index, and the primary key column(s)/index from the struct tags.
	meta := orm.extractColumnMetaData(rowsVal, "Create")

	// 5. Build Values & Placeholders
	resultArg := buildValueArgCreateSQL(rowsVal, meta.columnIndex, activeDriver)

	// 6. Build base query string
	orm.safeWriteString(fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", tableName, strings.Join(meta.columns, ", "), strings.Join(resultArg.placeholders, ", ")))

	// 7. Build Upsert
	if err = orm.buildUpsertCreateSQL(meta, tableName, activeDriver, resultArg); err != nil {
		return orm.Error
	}

	// 8. Build RETURNING
	retPlan, err := orm.buildReturnCreateSQL(data, rowsVal, meta, activeDriver, tableName, resultArg)
	if err != nil {
		return orm.Error
	}

	// 8. Set context timeout
	execCtx, cancel := orm.newContext()
	defer cancel()

	// 9. Execute Query DB
	queryStr := orm.StringBuilder.String()

	for idx, arg := range resultArg.valueArgs {
		resultArg.valueArgs[idx] = orm.sanitizeArg(arg)
	}

	if retPlan != nil {
		return orm.execReturn(execCtx, queryStr, resultArg.valueArgs, retPlan, rowsVal, meta)
	}

	result, message, httpCode, err := orm.Database.ExecSQL(execCtx, queryStr, resultArg.valueArgs...)
	if err != nil {
		return orm.setError(message, err, httpCode)
	}

	orm.Result = result
	orm.Message = "Data created successfully"

	// 10. Assign final result to ORM instance for handler consumption
	if result != nil {
		orm.extractResultCreateSQL(result, meta.primaryKeyIndex, rowsVal)
	}

	return nil
}
