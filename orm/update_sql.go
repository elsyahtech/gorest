package orm

import (
	"errors"
	"net/http"

	"github.com/elsyahtech/gorest/database"
)

func (orm *ORM) updateSQL(data any) error {
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

	// 5. Guard: there must be at least one primary_key-tagged field, since
	// every row in the batch is matched (and safely scoped) by its Primary Key.
	if len(meta.primaryKeyIndex) == 0 {
		const message = "Ensure your struct has a primary_key tag designated for updates."

		return orm.setError(message, errors.New("update: missing primary key for update"))
	}

	// 6. Guard: there must be at least one non-PK column to avoid an empty SET clause.
	if err := orm.validateUpdateColumns(meta); err != nil {
		return err
	}

	var retPlan *returnPlan

	if orm.IsReturn {
		if activeDriver == database.ORACLE && len(orm.ReturnCols) == 0 {
			orm.ReturnCols = []string{"*"}
		}

		retPlan, err = orm.planReturn(data, rowsVal, meta, activeDriver)
		if err != nil {
			return err
		}
	}

	// 7. Set context timeout
	execCtx, cancel := orm.newContext()
	defer cancel()

	if retPlan != nil && activeDriver == database.ORACLE {
		if len(rowsVal) != 1 {
			return orm.setError(
				"Gorest ORM Oracle Update().Return() currently handles one record per call. "+
					"Update records individually or omit Return() for a bulk update. See the ORM driver "+
					"matrix: https://github.com/elsyahtech/gorest/blob/main/docs/guide/orm.md#4-return-values-from-writes",
				errors.New("update: bulk Update().Return() is not implemented by the Gorest Oracle ORM path"),
				http.StatusBadRequest,
			)
		}

		return orm.updateOracleWithReturn(execCtx, tableName, rowsVal, meta, retPlan)
	}

	// Set single or bulk update
	isBatch := len(rowsVal) > 1

	// SINGLE UPDATE
	if !isBatch {
		return orm.updateSingleSQL(execCtx, tableName, rowsVal, meta, activeDriver, retPlan)
	}

	// BULK UPDATE
	return orm.updateBulkSQL(execCtx, tableName, rowsVal, meta, activeDriver, retPlan)
}
