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
		validateRequired:  true,
	}

	rowsVal, err := orm.validateData(data, fieldReq, "Create")
	if err != nil {
		return orm.Error
	}

	// 3. Verify that the processed data list contains at least one valid record
	if len(rowsVal) == 0 {
		return orm.setError("Ensure the request contains at least one record.", errors.New("create: data list cannot be empty"), http.StatusBadRequest)
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
	if !orm.IsUpsert {
		var created int64

		for _, row := range rowsVal {
			queryStr, valueArgs := buildRow(row)

			queryStr += " IF NOT EXISTS"

			applied, _, _, execErr := orm.Database.MapScanCAS(
				execCtx,
				orm.DatabaseConfig,
				queryStr,
				valueArgs...,
			)
			if execErr != nil {
				orm.RowsAffected = created

				return orm.setScyllaWriteError("Create", execErr)
			}

			if !applied {
				orm.RowsAffected = created

				return orm.setDatabaseOperationErrorKind(
					"Create",
					errors.New("data violates a primary-key or unique constraint"),
					databaseConflictError,
					http.StatusConflict,
				)
			}

			created++
		}
	} else {
		_, _, err = orm.Database.ExecCQL(execCtx, orm.DatabaseConfig, resultArg.queryStr, resultArg.valueArgs...)
		if err != nil {
			return orm.setScyllaWriteError("Create", err)
		}
	}

	orm.Message = "Data created successfully"

	// 9. Assign final result to ORM instance for handler consumption
	orm.extractResultCreateScylla(rowsVal, meta.primaryKeyIndex)

	return nil
}
