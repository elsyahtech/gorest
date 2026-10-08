package orm

import (
	"fmt"
	"strings"
)

func (orm *ORM) findScylla(data any) error {
	activeDriver := orm.DatabaseConfig.Driver

	// 1. Validate the environment (DB instance and struct)
	tableName, err := orm.validateEnvironmentFind(data, activeDriver)
	if err != nil {
		return orm.Error
	}

	// 2. Limit join and offset clause
	if err := orm.bannedJoinAndOffsetFindScylla(); err != nil {
		return orm.Error
	}

	// 3. Reflection: Validate Pointer to Struct or Slice of Structs
	valElem, err := orm.extractReflectionValue(data, "Find")
	if err != nil {
		return orm.Error
	}

	// 4. Resolve struct info (type, slice value, and slice flag)
	structInfo, isSlice := resolveStructInfo(valElem)

	// 5. Build grouping select clauses
	column := buildSelectedColumnsByTable(orm.SelectedCols, tableName)

	// 6. Build preloads metadata
	preload, err := orm.planScyllaPreloads(structInfo.structType, column.table)
	if err != nil {
		return orm.Error
	}

	// 7. Validate select table prefix
	if len(column.table) > 0 {
		if err := orm.validateSelectTablePrefixes(column.table, tableName, preload); err != nil {
			return orm.Error
		}
	}

	columns := buildColumnsFindScylla(column, tableName, preload)

	// 8. Build base query string
	orm.buildQueryStringFindScylla(columns, tableName)

	// 9. Build WHERE clauses and implicit LIMIT 1 logic
	prefix := tableName + "."

	valueArgs, limitVal, err := orm.buildWhereClauseFindScylla(isSlice, prefix)
	if err != nil {
		return orm.Error
	}
	if len(orm.GroupByClauses) > 0 {
		orm.safeWriteString(" GROUP BY ")
		orm.safeWriteString(strings.Join(orm.GroupByClauses, ", "))
	}

	// 10. Build ORDER BY clauses (valid only for clustering keys when the partition key is restricted)
	if len(orm.OrderByClauses) > 0 {
		orderBy := strings.ReplaceAll(strings.Join(orm.OrderByClauses, ", "), prefix, "")

		orm.safeWriteString(" ORDER BY ")
		orm.safeWriteString(orderBy)
	}

	// 11. Build LIMIT
	if limitVal > 0 {
		orm.safeWriteString(" LIMIT ?")

		valueArgs = append(valueArgs, limitVal)
	}

	// 12. Build allow filtering
	if orm.AllowFilteringFlag {
		orm.safeWriteString(" ALLOW FILTERING")
	}

	// 13. Generate final query string
	queryStr := orm.StringBuilder.String()

	// 14. Set context timeout for execution
	execCtx, cancel := orm.newContext()
	defer cancel()

	// 15. Execute database query
	iter, message, err := orm.Database.QueryCQL(execCtx, queryStr, valueArgs...)
	if err != nil {
		return orm.setError(message, err)
	}

	defer func() {
		if err := iter.Close(); err == nil {
			return
		}

		orm.Message = "Ensure the context has sufficient timeout for " +
			"cleanup operations and check network stability to the database server."
		orm.Error = fmt.Errorf("failed to iter close: %w", err)
	}()

	// 16. Build field lookup map for struct
	fieldLookupMap := buildStructFieldMapByTag(structInfo.structType)

	// 17. Process iter scanning and relationship mapping
	if err := orm.buildIterScanFindScylla(iter, structInfo.structType, fieldLookupMap, isSlice, valElem, structInfo.structValue); err != nil {
		return orm.Error
	}
	if !isSlice && orm.RowsAffected == 0 {
		return orm.setNotFound("find")
	}

	// 18. Load Find Scylla Preload/has-many relationships preload data
	if err := orm.loadFindScyllaPreloadData(execCtx, preload, valElem, isSlice); err != nil {
		return orm.Error
	}

	// 19. Assign final result to ORM instance for handler consumption
	orm.Result = data

	return nil
}
