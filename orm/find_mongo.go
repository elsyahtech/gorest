package orm

import (
	"errors"
	"fmt"
	"net/http"

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

	// 2. Block fitur join and disctinc for mongoDB to avoid overhead.
	// Performance consideration
	if len(orm.JoinClauses) > 0 || orm.IsDistinct {
		const message = "Join and Distinct are not supported by Mongo Find."

		return orm.setError(message, errors.New("find: unsupported clause for MongoDB"), http.StatusBadRequest)
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

	// 8. Build Sort clauses
	sort, err := orm.buildMongoSort(orm.OrderByClauses, tableName)
	if err != nil {
		return orm.Error
	}

	grouped := len(orm.GroupByClauses) > 0 || len(orm.HavingClauses) > 0
	var (
		findOpts *options.FindOptionsBuilder
		pipeline mongo.Pipeline
	)
	if grouped {
		pipeline, err = orm.buildMongoGroupPipeline(tableName, filter, sort, isSlice)
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
	coll, message, err := orm.Database.Collection(orm.DatabaseConfig, tableName)
	if err != nil {
		return orm.setError(message, err)
	}

	// 12. Execute Mongo find or aggregation query.
	var cursor *mongo.Cursor
	if grouped {
		cursor, err = coll.Aggregate(execCtx, pipeline)
	} else {
		cursor, err = coll.Find(execCtx, filter, findOpts)
	}
	if err != nil {
		const findMessage = "Check your filter, sort, or index configuration."

		return orm.setError(findMessage, err)
	}

	defer func() {
		if err := cursor.Close(execCtx); err == nil {
			return
		}

		orm.Message = "Ensure the context has sufficient timeout for " +
			"cleanup operations and check network stability to the database server."
		orm.Error = fmt.Errorf("failed to cursor.Close: %w", err)
	}()

	// 13. Document Mapping (Mapping Documents to Struct/Slice)
	fieldLookupMap := buildStructFieldMapByTag(structInfo.structType)

	// 14. Process rows scanning and relationship mapping
	cursor, err = orm.buildRowsScanFindMongo(
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
	if err := orm.loadFindMogoPreloadData(execCtx, preload, valElem, isSlice); err != nil {
		return orm.Error
	}

	// 16. Assign final result to ORM instance for handler consumption
	orm.Result = data

	return nil
}
