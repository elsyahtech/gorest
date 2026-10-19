package orm

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (orm *ORM) execCreateMongo(execCtx context.Context, rowsVal []any, tableName string) error {
	collection, message, httpCode, err := orm.Database.Collection(orm.DatabaseConfig, tableName)
	if err != nil {
		return orm.setDatabaseOperationError("Create", err)
	}

	orm.Message = message
	orm.HTTPCode = httpCode

	if orm.IsUpsert {
		if err := orm.execCreateMongoUpsert(execCtx, rowsVal, collection); err != nil {
			return orm.Error
		}
	} else {
		if err := orm.execCreateMongoNormal(execCtx, rowsVal, collection); err != nil {
			return orm.Error
		}
	}

	return nil
}

func (orm *ORM) execCreateMongoNormal(ctx context.Context, rowsVal []any, coll *mongo.Collection) error {
	switch {
	case len(rowsVal) == 0:
		return nil
	case len(rowsVal) == 1:
		res, err := coll.InsertOne(ctx, rowsVal[0])
		if err != nil {
			return orm.setMongoWriteError("Create", err)
		}

		orm.RowsAffected = 1
		orm.Result = res

		if res.InsertedID != nil {
			orm.LastInsertId = fmt.Sprintf("%v", res.InsertedID)
		} else {
			orm.LastInsertId = ""
		}
	default:
		// Continue inserting independent documents when one write fails. MongoDB's
		// default is ordered, which stops the batch at the first failed document.
		res, err := coll.InsertMany(ctx, rowsVal, options.InsertMany().SetOrdered(false))
		if res != nil {
			orm.RowsAffected = int64(len(res.InsertedIDs))
			var idStrs []string
			for _, id := range res.InsertedIDs {
				idStrs = append(idStrs, fmt.Sprintf("%v", id))
			}
			orm.LastInsertId = strings.Join(idStrs, ",")
		}
		if err != nil {
			fmt.Printf("ERROR MONGO INSERT MANY: %+v\n", err)
			return orm.setMongoWriteError("Create", err)
		}
		if res == nil {
			return orm.setDatabaseOperationErrorKind(
				"Create",
				fmt.Errorf("MongoDB InsertMany returned no result for %d submitted documents", len(rowsVal)),
				databaseExecutionError,
				http.StatusInternalServerError,
			)
		}
		if len(res.InsertedIDs) != len(rowsVal) {
			return orm.setDatabaseOperationErrorKind(
				"Create",
				fmt.Errorf("MongoDB InsertMany reported %d inserted documents out of %d submitted documents", len(res.InsertedIDs), len(rowsVal)),
				databaseExecutionError,
				http.StatusInternalServerError,
			)
		}

		orm.Result = res
	}

	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: orm.RowsAffected,
		lastInsertID: orm.LastInsertId,
	}

	return nil
}

func (orm *ORM) execCreateMongoUpsert(ctx context.Context, rowsVal []any, coll *mongo.Collection) error {
	ops, err := orm.buildMongoUpsertOps(rowsVal)
	if err != nil {
		return orm.Error
	}

	var allIDs []string

	for _, op := range ops {
		if op.id != nil {
			allIDs = append(allIDs, fmt.Sprintf("%v", op.id))
		}
	}

	orm.LastInsertId = strings.Join(allIDs, ", ")

	if len(ops) == 1 {
		res, err := coll.UpdateOne(ctx, ops[0].filter, ops[0].update, options.UpdateOne().SetUpsert(true))
		if err != nil {
			return orm.setMongoWriteError("Create", err)
		}

		orm.RowsAffected = res.MatchedCount + res.UpsertedCount

		orm.Result = map[string]any{
			isSuccess:    true,
			rowsAffected: orm.RowsAffected,
			upsertedID:   orm.LastInsertId,
		}

		orm.Message = "Data updated successfully"
		orm.HTTPCode = http.StatusOK

		return nil
	}

	models := make([]mongo.WriteModel, 0, len(ops))

	for _, op := range ops {
		models = append(models, mongo.NewUpdateOneModel().SetFilter(op.filter).SetUpdate(op.update).SetUpsert(true))
	}

	res, err := coll.BulkWrite(ctx, models)
	if err != nil {
		return orm.setMongoWriteError("Create", err)
	}

	orm.RowsAffected = res.MatchedCount + res.UpsertedCount

	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: orm.RowsAffected,
		upsertedID:   orm.LastInsertId,
	}

	orm.Message = "Data created successfully"
	orm.HTTPCode = http.StatusOK

	return nil
}
