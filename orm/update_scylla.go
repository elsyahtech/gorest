package orm

import (
	"errors"
)

func (orm *ORM) updateScylla(data any) error {
	activeDriver := orm.DatabaseConfig.Driver

	// 1. Validate the DB instance (connection available)
	tableName, err := orm.validateDBInstance(data, activeDriver, "Update")
	if err != nil {
		return orm.Error
	}

	// 2. Validate Struct (Ensure the struct has valid tags & primary_key)
	fieldReq := fieldRequirement{
		requirePrimaryKey: true,
		requireNonEmpty:   true,
	}

	rowsVal, err := orm.validateData(data, fieldReq, "Update")
	if err != nil {
		return orm.Error
	}

	// 3. Verify that the processed data list contains at least one valid record to update
	if len(rowsVal) == 0 {
		const message = "ensure that the data list is not empty"

		return orm.setError(message, errors.New("update: data list cannot be empty"))
	}

	// 4. Extract updatable columns, their struct field index, and the primary key column(s)/index from the struct tags.
	meta := orm.extractColumnMetaData(rowsVal, "Update")

	if len(meta.primaryKeyIndex) == 0 {
		const message = "Ensure your struct has a primary_key tag designated for updates."

		return orm.setError(message, errors.New("update: missing primary key for update"))
	}

	// 5. Set context timeout
	execCtx, cancel := orm.newContext()
	defer cancel()

	isBatch := len(rowsVal) > 1

	// SINGLE UPDATE
	if !isBatch {
		return orm.updateSingleScylla(execCtx, tableName, rowsVal, meta, activeDriver)
	}

	// BULK UPDATE
	return orm.updateBulkScylla(execCtx, tableName, rowsVal, meta, activeDriver)
}
