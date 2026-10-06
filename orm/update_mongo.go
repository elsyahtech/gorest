package orm

import (
	"errors"
)

func (orm *ORM) updateMongo(data any) error {
	activeDriver := orm.DatabaseConfig.Driver

	// 1. Validate DB instance
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

	// 6. Dapatkan instance Collection MongoDB terlebih dahulu
	collection, message, err := orm.Database.Collection(orm.DatabaseConfig, tableName)
	if err != nil {
		return orm.setError(message, err)
	}

	isBatch := len(rowsVal) > 1

	// SINGLE UPDATE
	if !isBatch {
		return orm.updateSingleMongo(execCtx, collection, rowsVal, meta)
	}

	// BULK UPDATE
	return orm.updateBulkMongo(execCtx, collection, rowsVal, meta)
}
