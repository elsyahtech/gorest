package orm

import (
	"errors"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (orm *ORM) findMongo(data any) error {
	activeDriver := orm.DatabaseConfig.Driver

	// 1. Validate the DB instance (connection available)
	tableName, err := orm.validateDBInstance(data, activeDriver, "Find")
	if err != nil {
		return orm.Error
	}

	// 3. Validate Struct (Ensure the struct has valid tags & primary_key)
	fieldReq := fieldRequirement{
		requirePrimaryKey: false,
		requireNonEmpty:   false,
	}

	if err := orm.validateStructTags(data, fieldReq, "Find"); err != nil {
		return orm.Error
	}

	// 4. Reflection: Extract and validate pointer to struct or slice of structs
	valElem, err := orm.extractReflectionValue(data, "Find")
	if err != nil {
		return orm.Error
	}

	// 5. Resolve struct info (type, slice value, and slice flag)
	structInfo, isSlice := resolveStructInfo(valElem)

	// 6. Build preloads metadata
	preload, err := orm.planMongoPreloads(structInfo.structType)
	if err != nil {
		return orm.Error
	}

	tagInfo := resolveTagInfo(structInfo.structType)

	// 7. Build Filter
	filter, err := orm.buildMongoFilter(orm.WhereClauses, orm.WhereArgs, tableName, tagInfo.objectIDColumns)
	if err != nil {
		return orm.Error
	}

	if orm.limitSet && orm.LimitVal == 0 {
		alwaysFalse := bson.M{"$expr": bson.M{"$eq": bson.A{1, 0}}}

		if len(filter) == 0 {
			filter = alwaysFalse
		} else {
			filter = bson.M{"$and": bson.A{filter, alwaysFalse}}
		}
	}

	// 8. Build Sort clauses
	sort, err := orm.buildMongoSort(orm.OrderByClauses, tableName, structInfo.structType)
	if err != nil {
		return orm.Error
	}

	grouped := len(orm.GroupByClauses) > 0 || len(orm.HavingClauses) > 0
	joining := len(orm.JoinClauses) > 0

	if (orm.IsDistinct || joining) && grouped {
		return orm.setError(
			"MongoDB Find() cannot combine Join() or Distinct() with GroupBy() or Having(); choose one aggregation shape for this query.",
			errors.New("find: Join()/Distinct() conflict with GroupBy()/Having() for MongoDB"),
			http.StatusBadRequest,
		)
	}

	var (
		findOpts *options.FindOptionsBuilder
		pipeline mongo.Pipeline
	)

	if orm.IsDistinct {
		pipeline, err = orm.buildMongoDistinctPipeline(filter, sort, tableName)
	} else if grouped {
		pipeline, err = orm.buildMongoGroupPipeline(tableName, filter, sort, isSlice, structInfo.structType)
	} else if joining {
		pipeline, err = orm.buildMongoJoinPipeline(tableName, filter, sort, structInfo.structType)
	} else {
		findOpts, err = orm.buildMongoOptionFindMongo(tableName, preload, sort, isSlice)
	}

	if err != nil {
		return orm.Error
	}

	// 10. Set context timeout for execution
	execCtx, cancel := orm.newContext()
	defer cancel()

	// 11. Execute mongo collection
	coll, _, _, err := orm.Database.Collection(orm.DatabaseConfig, tableName)
	if err != nil {
		return orm.setDatabaseOperationError("Find", err)
	}

	// 12. Execute Mongo find or aggregation query.
	var cursor *mongo.Cursor

	if grouped || orm.IsDistinct || joining {
		cursor, err = coll.Aggregate(execCtx, pipeline)
	} else {
		cursor, err = coll.Find(execCtx, filter, findOpts)
	}

	if err != nil {
		if grouped || orm.IsDistinct || joining {
			return orm.setDatabaseOperationError("Find", err)
		}

		return orm.setDatabaseOperationError("Find", err)
	}

	// Keep the original cursor for cleanup. The scanner returns a nil cursor
	// when decoding or mapping fails, so do not overwrite the cursor captured
	// by the deferred Close call.
	cursorToClose := cursor

	defer func() {
		if cursorToClose == nil {
			return
		}

		if err := cursorToClose.Close(execCtx); err == nil {
			return
		}

		if orm.Error == nil {
			orm.setDatabaseOperationError("Find", err)
		}
	}()

	// 13. Document Mapping (Mapping Documents to Struct/Slice)
	fieldLookupMap := buildStructFieldMapByTag(structInfo.structType)

	// 14. Process rows scanning and relationship mapping
	_, err = orm.buildRowsScanFindMongo(
		execCtx,
		cursor,
		structInfo.structType,
		fieldLookupMap,
		isSlice,
		valElem,
		structInfo.structValue,
	)
	if err != nil {
		return orm.Error
	}

	if !isSlice && orm.RowsAffected == 0 {
		return orm.setNotFound("find")
	}

	// 15. Load Find Mongo Preload/has-many relationships preload data
	if !orm.IsDistinct && !joining {
		if err := orm.loadFindMogoPreloadData(execCtx, preload, valElem, isSlice); err != nil {
			return orm.Error
		}
	}

	// 16. Assign final result to ORM instance for handler consumption
	orm.Result = data
	orm.Message = "Data retrieved successfully"

	return nil
}
