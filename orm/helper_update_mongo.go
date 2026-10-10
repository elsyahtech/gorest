package orm

import (
	"context"
	"errors"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	matchedCount  = "matchedCount"
	modifiedCount = "modifiedCount"
)

func (orm *ORM) updateSingleMongo(execCtx context.Context, collection *mongo.Collection, rowsVal []reflect.Value, meta columnMetaData) error {
	row := rowsVal[0]

	field := buildUpdatePayloadMongo(row, meta)

	if len(field) == 0 {
		return orm.setError(
			"Ensure that there are fields to update. Specify fields using .Select() or ensure your struct has non-primary key fields.",
			errors.New("update: no fields to update"),
		)
	}

	filter, err := orm.buildSingleMongoFilter(row, &meta)
	if err != nil {
		return orm.Error
	}

	updateDoc := bson.M{"$set": field}

	res, err := collection.UpdateOne(execCtx, filter, updateDoc)
	if err != nil {
		return orm.setError(
			"Ensure that your MongoDB connection is active and your filter/update document syntax is correct.",
			err,
		)
	}

	var matched, modified int64

	if res != nil {
		matched = res.MatchedCount
		modified = res.ModifiedCount
	}
	if matched == 0 {
		return orm.setNotFound("update")
	}

	orm.Message = "Data updated successfully"
	orm.RowsAffected = modified
	orm.Result = map[string]any{
		isSuccess:     true,
		matchedCount:  matched,
		modifiedCount: modified,
	}

	return nil
}

func (orm *ORM) updateBulkMongo(execCtx context.Context, collection *mongo.Collection, rowsVal []reflect.Value, meta columnMetaData) error {
	var (
		writeModels                 []mongo.WriteModel
		totalMatched, totalModified int64
	)

	for _, row := range rowsVal {
		updateFields := buildUpdatePayloadMongo(row, meta)

		if len(updateFields) == 0 {
			return orm.setError(
				"Ensure that there are fields to update. Specify fields using .Select() or ensure your struct has non-primary key fields.",
				errors.New("update: no fields to update"),
			)
		}

		filter, err := orm.buildBulkMongoFilter(row, &meta)
		if err != nil {
			return orm.Error
		}

		updateDoc := bson.M{"$set": updateFields}

		writeModels = append(writeModels, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(updateDoc))
	}

	result, err := collection.BulkWrite(execCtx, writeModels, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return orm.setError(
			"Ensure that your MongoDB connection is active and your filter/update document syntax is correct.",
			err,
		)
	}

	if result != nil {
		totalMatched = result.MatchedCount
		totalModified = result.ModifiedCount
	}
	if totalMatched == 0 {
		return orm.setNotFound("update")
	}

	orm.Message = "Data updated successfully"
	orm.RowsAffected = totalModified
	orm.Result = map[string]any{
		isSuccess:     true,
		matchedCount:  totalMatched,
		modifiedCount: totalModified,
	}

	return nil
}
