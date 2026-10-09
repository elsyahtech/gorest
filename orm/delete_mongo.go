package orm

import (
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (orm *ORM) deleteMongo(data any) error {
	activeDriver := orm.DatabaseConfig.Driver

	// 1. Validate the DB instance (connection available)
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

	// 3. Verify that the processed data list contains at least one valid record to insert
	if len(rowsVal) == 0 {
		const message = "ensure that the data list is not empty"

		return orm.setError(message, errors.New("delete: data list cannot be empty"))
	}

	// 4. Extract updatable columns, their struct field index, and the primary key column(s)/index from the struct tags.
	meta := orm.extractColumnMetaData(rowsVal, "Delete")

	isBatch := len(rowsVal) > 1

	filter, err := orm.buildDeletePayloadMongo(rowsVal, meta)
	if err != nil {
		return orm.Error
	}

	// 12. Safety guard: NEVER allow a delete with an empty filter (that would wipe the whole collection).
	if len(filter) == 0 {
		const message = "refusing to run Delete without a filter to avoid wiping the entire collection"

		return orm.setError(message, errors.New("delete: missing filter condition"))
	}

	// 13. Set context timeout
	execCtx, cancel := orm.newContext()
	defer cancel()

	// 14. Execute delete against the target collection
	collection, message, httpCode, err := orm.Database.Collection(orm.DatabaseConfig, tableName)
	if err != nil {
		return orm.setError(message, err, httpCode)
	}

	var deletedCount int64

	if isBatch {
		var res *mongo.DeleteResult

		res, err = collection.DeleteMany(execCtx, filter)

		if res != nil {
			deletedCount = res.DeletedCount
		}
	} else {
		var res *mongo.DeleteResult

		res, err = collection.DeleteOne(execCtx, filter)

		if res != nil {
			deletedCount = res.DeletedCount
		}
	}

	if err != nil {
		const message = "Ensure that the MongoDB connection is active and the collection query filter is valid."

		return orm.setError(message, err)
	}

	orm.RowsAffected = deletedCount
	if deletedCount == 0 {
		return orm.setNotFound("delete")
	}
	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: deletedCount,
	}

	return nil
}
