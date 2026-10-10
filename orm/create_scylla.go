package orm

import (
	"errors"
	"net/http"
)

func (orm *ORM) createScylla(data any) error {
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

	rowsVal, err := orm.validateData(data, fieldReq, "Create")
	if err != nil {
		return orm.Error
	}

	// 3. Verify that the processed data list contains at least one valid record
	if len(rowsVal) == 0 {
		return orm.setError("ensure that the data list is not empty", errors.New("create: data list cannot be empty"))
	}

	// 4. Extract updatable columns, their struct field index, and the primary key column(s)/index from the struct tags.
	meta := orm.extractColumnMetaData(rowsVal, "Create")

	// 5. Build Upsert
	buildRow, err := orm.buildUpsertCreateScylla(meta, tableName)
	if err != nil {
		return orm.Error
	}

	// 6. Build CQL INSERT Statement (ScyllaDB compatible using ? placeholders)
	resultArg := buildQueryStatement(rowsVal, buildRow)

	// 7. Set context timeout
	execCtx, cancel := orm.newContext()
	defer cancel()

	// 8. Execute Query DB
	message, httpCode, err := orm.Database.ExecCQL(execCtx, resultArg.queryStr, resultArg.valueArgs...)
	if err != nil {
		return orm.setError(message, err, httpCode)
	}

	orm.Message = "Data created successfully"
	orm.HTTPCode = http.StatusCreated

	// 9. Assign final result to ORM instance for handler consumption
	orm.extractResultCreateScylla(rowsVal, meta.primaryKeyIndex)

	return nil
}
