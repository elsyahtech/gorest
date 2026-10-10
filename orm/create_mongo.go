package orm

import (
	"errors"
)

func (orm *ORM) createMongo(data any) error {
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

	rowsVal, err := orm.validateDataMongo(data, fieldReq, "Create")
	if err != nil {
		return orm.Error
	}

	// 3. Verify that the processed data list contains at least one valid record to insert
	if len(rowsVal) == 0 {
		const message = "ensure that the data list is not empty"

		return orm.setError(message, errors.New("create: data list cannot be empty"))
	}

	// 4. Set context timeout if configured
	execCtx, cancel := orm.newContext()
	defer cancel()

	// 5. Execute InsertOne or InsertMany based on payload count
	if err := orm.execCreateMongo(execCtx, rowsVal, tableName); err != nil {
		return orm.Error
	}

	orm.Message = "Data created successfully"

	return nil
}
