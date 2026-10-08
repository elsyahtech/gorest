package orm

import (
	"fmt"
	"strings"
)

func (orm *ORM) findSQL(data any) error {
	activeDriver := orm.DatabaseConfig.Driver

	// 1. Validate the environment (DB instance and struct)
	tableName, err := orm.validateEnvironmentFind(data, activeDriver)
	if err != nil {
		return orm.Error
	}

	// 3. Reflection: Extract and validate pointer to struct or slice of structs
	valElem, err := orm.extractReflectionValue(data, "Find")
	if err != nil {
		return orm.Error
	}

	// 4. Resolve struct info (type, slice value, and slice flag)
	structInfo, isSlice := resolveStructInfo(valElem)

	// 5. Build preloads metadata
	preload, err := orm.buildSQLPreloads(structInfo.structType, tableName, activeDriver)
	if err != nil {
		return orm.Error
	}

	// 6. Build base query string
	orm.buildQueryStringFindSQL(preload.columnsToSelect, tableName, preload.dynamicJoins)

	// 7. Build WHERE clauses and implicit LIMIT 1 logic
	whereClause, err := orm.buildWhereClauseFindSQL(
		&reqBuildWhereClauseFindSQL{
			joinArgs:       preload.joinArgs,
			joinArgCounter: preload.joinArgCounter,
			activeDriver:   activeDriver,
			isSlice:        isSlice,
		},
	)
	if err != nil {
		return orm.Error
	}

	// 8. Build ORDER BY clauses
	if len(orm.OrderByClauses) > 0 {
		orm.safeWriteString(fmt.Sprintf(" ORDER BY %s", strings.Join(orm.OrderByClauses, ", ")))
	}

	// 9. Build LIMIT and OFFSET clauses
	whereClause.valueArgs = orm.buildLimitOffsetFindSQL(
		&structBuildLimitOffsetFindSQL{
			activeDriver: activeDriver,
			limitVal:     whereClause.limitVal,
			argCounter:   whereClause.argCounter,
			valueArgs:    whereClause.valueArgs,
		},
	)

	// 10. Generate final query string
	queryStr := orm.StringBuilder.String()

	// 11. Set context timeout for execution
	execCtx, cancel := orm.newContext()
	defer cancel()

	// 12. Execute database query
	rows, message, err := orm.Database.QuerySQL(execCtx, queryStr, whereClause.valueArgs...)
	if err != nil {
		return orm.setError(message, err)
	}

	defer func() {
		if err := rows.Close(); err == nil {
			return
		}

		orm.Message = "Ensure the context has sufficient timeout for " +
			"cleanup operations and check network stability to the database server."
		orm.Error = fmt.Errorf("failed to rows.Close: %w", err)
	}()

	// 13. Map SQL result columns
	cols, err := orm.mappingSQLRowsColumnResult(rows)
	if err != nil {
		return orm.Error
	}

	// 14. Build preload prefix mapping
	preloadPrefixMap := orm.buildPreloadPrefixMapFindSQL(preload.hasManyNames)

	// 15. Build field lookup map for struct
	fieldLookupMap := buildStructFieldMap(structInfo.structType)

	// 15b. Guard: reject ambiguous duplicate columns
	if err := orm.validateDuplicateColumnsFindSQL(cols, preloadPrefixMap, fieldLookupMap); err != nil {
		return orm.Error
	}

	// 16. Process rows scanning and relationship mapping
	if err = orm.buildRowsScanFindSQL(
		&structBuildRowsScanFindSQL{
			rows:              rows,
			structType:        structInfo.structType,
			cols:              cols,
			preloadPrefixMap:  preloadPrefixMap,
			fieldLookupMap:    fieldLookupMap,
			hasManyNames:      preload.hasManyNames,
			preloadReferences: preload.preloadReferences,
			isSlice:           isSlice,
			valElem:           valElem,
			sliceVal:          structInfo.structValue,
		},
	); err != nil {
		return orm.Error
	}
	if !isSlice && orm.RowsAffected == 0 {
		return orm.setNotFound("find")
	}

	// 17. Load Find SQL Preload/has-many relationships preload data
	if err := orm.loadFindSQLPreloadData(execCtx, activeDriver, valElem, isSlice, preload.hasManyRels, cols); err != nil {
		return orm.Error
	}

	// 18. Assign final result to ORM instance for handler consumption
	orm.Result = data

	return nil
}
