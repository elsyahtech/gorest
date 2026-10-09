package orm

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (orm *ORM) execCreateMongo(execCtx context.Context, rowsVal []any, tableName string) error {
	collection, message, httpCode, err := orm.Database.Collection(orm.DatabaseConfig, tableName)
	if err != nil {
		return orm.setError(message, err, httpCode)
	}

	buildMessage := func() string {
		return "MongoDB error detected. " +
			"Check your collection schema, document structure, duplicate keys (unique index violation), or network connection."
	}

	if orm.IsUpsert {
		if err := orm.execCreateMongoUpsert(execCtx, rowsVal, collection, buildMessage); err != nil {
			return orm.Error
		}
	} else {
		if err := orm.execCreateMongoNormal(execCtx, rowsVal, collection, buildMessage); err != nil {
			return orm.Error
		}
	}

	return nil
}

func (orm *ORM) execCreateMongoNormal(ctx context.Context, rowsVal []any, coll *mongo.Collection, message func() string) error {
	switch {
	case len(rowsVal) == 0:
		return nil
	case len(rowsVal) == 1:
		res, err := coll.InsertOne(ctx, rowsVal[0])
		if err != nil {
			return orm.setError(message(), err)
		}

		orm.RowsAffected = 1
		orm.Result = res

		if res.InsertedID != nil {
			orm.LastInsertId = fmt.Sprintf("%v", res.InsertedID)
		} else {
			orm.LastInsertId = ""
		}
	default:
		res, err := coll.InsertMany(orm.Context, rowsVal)
		if err != nil {
			return orm.setError(message(), err)
		}

		orm.RowsAffected = int64(len(res.InsertedIDs))
		orm.Result = res

		if len(res.InsertedIDs) > 0 {
			var idStrs []string

			for _, id := range res.InsertedIDs {
				idStrs = append(idStrs, fmt.Sprintf("%v", id))
			}

			orm.LastInsertId = strings.Join(idStrs, ",")
		} else {
			orm.LastInsertId = ""
		}
	}

	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: orm.RowsAffected,
		lastInsertID: orm.LastInsertId,
	}

	return nil
}

func (orm *ORM) execCreateMongoUpsert(ctx context.Context, rowsVal []any, coll *mongo.Collection, message func() string) error {
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
			return orm.setError(message(), err)
		}

		orm.RowsAffected = res.MatchedCount + res.UpsertedCount

		orm.Result = map[string]any{
			isSuccess:    true,
			rowsAffected: orm.RowsAffected,
			upsertedID:   orm.LastInsertId,
		}

		return nil
	}

	models := make([]mongo.WriteModel, 0, len(ops))

	for _, op := range ops {
		models = append(models, mongo.NewUpdateOneModel().SetFilter(op.filter).SetUpdate(op.update).SetUpsert(true))
	}

	res, err := coll.BulkWrite(ctx, models)
	if err != nil {
		return orm.setError(message(), err)
	}

	orm.RowsAffected = res.MatchedCount + res.UpsertedCount

	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: orm.RowsAffected,
		upsertedID:   orm.LastInsertId,
	}

	return nil
}
