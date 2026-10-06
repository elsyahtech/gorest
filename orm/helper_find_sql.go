package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/elsyahtech/gorest/database"
)

func (orm *ORM) validateEnvironmentFind(data any, activeDriver string) (string, error) {
	// 1. Validate the DB instance (connection available)
	tableName, err := orm.validateDBInstance(data, activeDriver, "Find")
	if err != nil {
		return "", orm.Error
	}

	// 3. Validate Struct (Ensure the struct has valid tags & primary_key)
	fieldReq := fieldRequirement{
		requirePrimaryKey: false,
		requireNonEmpty:   false,
	}

	if err := orm.validateStructTags(data, fieldReq, "Find"); err != nil {
		return "", orm.Error
	}

	return tableName, nil
}

func (orm *ORM) loadFindSQLPreloadData(
	execCtx context.Context,
	activeDriver string,
	valElem *reflect.Value,
	isSlice bool,
	hasManyRels []*hasManyRelation,
	cols []string,
) error {
	if len(hasManyRels) == 0 {
		return nil
	}

	parents, err := orm.collectParents(*valElem, isSlice)
	if err != nil {
		return orm.Error
	}

	for _, rel := range hasManyRels {
		if err := orm.loadHasMany(execCtx, activeDriver, parents, cols, rel); err != nil {
			return orm.Error
		}
	}

	return nil
}

func (orm *ORM) buildSelectColumnFindSQL(tableName string) []string {
	var columnsToSelect []string

	if len(orm.SelectedCols) > 0 {
		for _, col := range orm.SelectedCols {
			if strings.TrimSpace(col) == "*" {
				col = tableName + ".*"
			}

			columnsToSelect = append(columnsToSelect, col)
		}
	} else {
		columnsToSelect = append(columnsToSelect, fmt.Sprintf("%s.*", tableName))
	}

	return columnsToSelect
}

func (orm *ORM) buildSQLPreloads(structType reflect.Type, tableName, activeDriver string) (*sqlPreloadPlan, error) {
	plan := &sqlPreloadPlan{}

	if len(orm.Preloads) == 0 {
		plan.columnsToSelect = orm.buildSelectColumnFindSQL(tableName)

		return plan, nil
	}

	joinArgCounter := 1
	columnsToSelect := orm.buildSelectColumnFindSQL(tableName)

	plan.hasManyNames = make(map[string]struct{})
	plan.preloadReferences = make(map[string]string)

	for relationName := range orm.Preloads {
		hmRel, hmErr := orm.findHasManyRelation(structType, relationName)
		if hmErr != nil {
			return nil, orm.Error
		}

		if hmRel != nil {
			plan.hasManyRels = append(plan.hasManyRels, hmRel)
			plan.hasManyNames[relationName] = struct{}{}

			continue // has-many is not included in the JOIN.
		}

		baseName := pluralismNormalization(relationName)
		tableRef := pluralizeForm(baseName)

		if err := orm.verifJoinClausesPreloadFindSQL(tableRef, relationName); err != nil {
			return nil, orm.Error
		}

		// Find preload relation
		resultFindPreloadRelation, findPreloadRelationErr := orm.findPreloadRelationFindSQL(structType, relationName)
		if findPreloadRelationErr != nil {
			return nil, orm.Error
		}

		targetColumn, targetColumnErr := orm.findRelatedDBColumn(
			resultFindPreloadRelation.relatedStructType,
			resultFindPreloadRelation.targetKey,
			relationName,
		)
		if targetColumnErr != nil {
			return nil, orm.Error
		}

		plan.preloadReferences[baseName] = targetColumn

		columnsToSelect = buildPreloadColumns(resultFindPreloadRelation.relatedStructType, tableRef, columnsToSelect)

		condSQL, condArgs, condErr := orm.parsePreloadCondition(relationName, orm.Preloads[relationName], activeDriver)
		if condErr != nil {
			return nil, orm.Error
		}

		joinSQL := fmt.Sprintf(" LEFT JOIN %s ON %s.%s = %s.%s",
			tableRef, tableRef, targetColumn, tableName, resultFindPreloadRelation.foreignKey,
		)

		if condSQL != "" {
			normalized, next := normalizeWherePlaceholders(condSQL, activeDriver, joinArgCounter)

			joinArgCounter = next

			joinSQL += " AND (" + normalized + ")"

			plan.joinArgs = append(plan.joinArgs, condArgs...)
		}

		plan.dynamicJoins = append(plan.dynamicJoins, joinSQL)
	}

	plan.columnsToSelect = columnsToSelect
	plan.joinArgCounter = joinArgCounter

	return plan, nil
}

func (orm *ORM) verifJoinClausesPreloadFindSQL(tableRef string, relationName string) error {
	for _, joinClause := range orm.JoinClauses {
		if joinTargetsTable(joinClause, tableRef) {
			return orm.setError(
				fmt.Sprintf("Table %q is already handled by Preload. Remove the manual .Join() or the .Preload().", tableRef),
				fmt.Errorf("find error: manual join conflicts with active preload for relation %q", relationName),
			)
		}
	}

	return nil
}

func joinTargetsTable(joinClause, table string) bool {
	pattern := "(?i)\\bjoin\\s+[\"`\\[]?(?:\\w+\\.)?" + regexp.QuoteMeta(table) + "[\"`\\]]?(\\s|$)"

	return regexp.MustCompile(pattern).MatchString(joinClause)
}

type resFindPreloadRelationFindSQL struct {
	relatedStructType reflect.Type
	foreignKey        string
	targetKey         string
}

func (orm *ORM) findPreloadRelationFindSQL(structType reflect.Type, relationName string) (*resFindPreloadRelationFindSQL, error) {
	result, err := orm.findPreloadRelation(structType, relationName)
	if err != nil {
		return nil, orm.Error
	}

	if !result.found {
		return nil, orm.setError(
			"ensure that the preload relation exists on the main model",
			fmt.Errorf("preload error: relation %q was not found on struct %s", relationName, structType.Name()),
		)
	}

	return &resFindPreloadRelationFindSQL{
		relatedStructType: result.relatedStructType,
		foreignKey:        result.foreignKey,
		targetKey:         result.targetKey,
	}, nil
}

func (orm *ORM) buildQueryStringFindSQL(columnsToSelect []string, tableName string, dynamicJoins []string) {
	orm.safeWriteString("SELECT ")

	if orm.IsDistinct {
		orm.safeWriteString("DISTINCT ")
	}

	orm.safeWriteString(strings.Join(columnsToSelect, ", "))
	orm.safeWriteString(fmt.Sprintf(" FROM %s", tableName))

	if len(orm.JoinClauses) > 0 {
		for _, join := range orm.JoinClauses {
			orm.safeWriteString(fmt.Sprintf(" %s", join))
		}
	}

	for _, join := range dynamicJoins {
		orm.safeWriteString(join)
	}
}

type reqBuildWhereClauseFindSQL struct {
	activeDriver   string
	joinArgs       []any
	joinArgCounter int
	isSlice        bool
}

type resBuildWhereClauseFindSQL struct {
	valueArgs  []any
	argCounter int
	limitVal   int
}

func (orm *ORM) buildWhereClauseFindSQL(req *reqBuildWhereClauseFindSQL) (*resBuildWhereClauseFindSQL, error) {
	var (
		whereParts []string
		argCounter int
	)

	if req.joinArgCounter == 0 {
		argCounter = 1
	} else {
		argCounter = req.joinArgCounter
	}

	valueArgs := req.joinArgs
	limitVal := orm.LimitVal

	if len(orm.WhereClauses) > 0 {
		argIndex := 0

		for _, clause := range orm.WhereClauses {
			placeholderCount := countActivePlaceholders(clause, req.activeDriver)

			if argIndex+placeholderCount > len(orm.WhereArgs) {
				return nil, orm.setError(
					"Ensure the number of placeholders in .Where() matches the number of arguments provided.",
					errors.New("find: mismatched where clause placeholders and arguments"),
				)
			}

			normalized, nextCounter := normalizeWherePlaceholders(clause, req.activeDriver, argCounter)

			argCounter = nextCounter

			whereParts = append(whereParts, normalized)
			valueArgs = append(valueArgs, orm.WhereArgs[argIndex:argIndex+placeholderCount]...)

			argIndex += placeholderCount
		}

		if argIndex != len(orm.WhereArgs) {
			return nil, orm.setError(
				"Ensure the number of arguments matches the number of placeholders across all .Where() clauses.",
				errors.New("find: unused arguments provided to .Where()"),
			)
		}
	} else if !req.isSlice {
		if orm.LimitVal <= 0 {
			limitVal = 1
		}
	}

	if len(whereParts) > 0 {
		orm.safeWriteString(" WHERE ")
		orm.safeWriteString(strings.Join(whereParts, " AND "))
	}

	return &resBuildWhereClauseFindSQL{
		valueArgs:  valueArgs,
		argCounter: argCounter,
		limitVal:   limitVal,
	}, nil
}

type structBuildLimitOffsetFindSQL struct {
	activeDriver string
	valueArgs    []any
	limitVal     int
	argCounter   int
}

func (orm *ORM) buildLimitOffsetFindSQL(req *structBuildLimitOffsetFindSQL) []any {
	if req.limitVal > 0 || orm.OffsetVal > 0 {
		switch req.activeDriver {
		case database.SQLSERVER:
			orm.buildLimitOffsetForSQLSRVInFindSQL(req)
		case database.ORACLE:
			orm.buildLimitOffsetForORACLEInFindSQL(req)
		default: // MySQL, Postgres, SQLite
			orm.buildLimitOffsetForDefaultInFindSQL(req)
		}
	}

	return req.valueArgs
}

func (orm *ORM) buildLimitOffsetForSQLSRVInFindSQL(req *structBuildLimitOffsetFindSQL) {
	if len(orm.OrderByClauses) == 0 {
		orm.safeWriteString(" ORDER BY (SELECT NULL)")
	}

	orm.safeWriteString(fmt.Sprintf(" OFFSET @p%d ROWS", req.argCounter))

	req.valueArgs = append(req.valueArgs, orm.OffsetVal)

	req.argCounter++

	if req.limitVal <= 0 {
		return
	}

	orm.safeWriteString(fmt.Sprintf(" FETCH NEXT @p%d ROWS ONLY", req.argCounter))

	req.valueArgs = append(req.valueArgs, req.limitVal)

	req.argCounter++
}

func (orm *ORM) buildLimitOffsetForORACLEInFindSQL(req *structBuildLimitOffsetFindSQL) {
	orm.safeWriteString(fmt.Sprintf(" OFFSET :%d ROWS", req.argCounter))

	req.valueArgs = append(req.valueArgs, orm.OffsetVal)

	req.argCounter++

	if req.limitVal <= 0 {
		return
	}

	orm.safeWriteString(fmt.Sprintf(" FETCH NEXT :%d ROWS ONLY", req.argCounter))

	req.valueArgs = append(req.valueArgs, req.limitVal)

	req.argCounter++
}

func (orm *ORM) buildLimitOffsetForDefaultInFindSQL(req *structBuildLimitOffsetFindSQL) {
	placeholder := func() string {
		if req.activeDriver == database.POSTGRES {
			puyeng := fmt.Sprintf("$%d", req.argCounter)

			req.argCounter++

			return puyeng
		}

		return "?"
	}

	switch {
	case req.limitVal > 0:
		orm.safeWriteString(" LIMIT ")
		orm.safeWriteString(placeholder())

		req.valueArgs = append(req.valueArgs, req.limitVal)

	case orm.OffsetVal > 0 && req.activeDriver == database.MYSQL:
		orm.safeWriteString(" LIMIT 18446744073709551615")

	case orm.OffsetVal > 0 && req.activeDriver == database.SQLITE:
		orm.safeWriteString(" LIMIT -1")
	default:
	}

	if orm.OffsetVal <= 0 {
		return
	}

	orm.safeWriteString(" OFFSET ")
	orm.safeWriteString(placeholder())

	req.valueArgs = append(req.valueArgs, orm.OffsetVal)
}

func (orm *ORM) mappingSQLRowsColumnResult(rows *sql.Rows) ([]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, orm.setError("ensure that database columns can be retrieved correctly", err)
	}

	return cols, nil
}

func (orm *ORM) buildPreloadPrefixMapFindSQL(hasManyNames map[string]struct{}) map[string]struct{} {
	preloadPrefixMap := make(map[string]struct{})

	if len(orm.Preloads) > 0 {
		for relName := range orm.Preloads {
			if _, isMany := hasManyNames[relName]; isMany {
				continue
			}

			baseName := pluralismNormalization(relName)
			tableRef := pluralizeForm(baseName)
			prefix := strings.ToLower(tableRef + "_")

			preloadPrefixMap[prefix] = struct{}{}
		}
	}

	return preloadPrefixMap
}

type structBuildRowsScanFindSQL struct {
	structType        reflect.Type
	rows              *sql.Rows
	preloadPrefixMap  map[string]struct{}
	fieldLookupMap    map[string]*structFieldInfo
	hasManyNames      map[string]struct{}
	preloadReferences map[string]string
	valElem           *reflect.Value
	sliceVal          *reflect.Value
	cols              []string
	isSlice           bool
}

func (orm *ORM) buildRowsScanFindSQL(req *structBuildRowsScanFindSQL) error {
	for req.rows.Next() {
		newStructPtr := reflect.New(req.structType)
		newStructVal := newStructPtr.Elem()

		resStructScanColumnRows, err := orm.scanColumnRowsScanFindSQL(
			&reqStructScanColumnRowsScanFindSQL{
				rows:             req.rows,
				cols:             req.cols,
				preloadPrefixMap: req.preloadPrefixMap,
				fieldLookupMap:   req.fieldLookupMap,
				newStructVal:     newStructVal,
			},
		)
		if err != nil {
			return orm.Error
		}

		fieldMapRowsScanFindSQL(resStructScanColumnRows.fieldMappings, resStructScanColumnRows.fieldHolders)

		// Preload Mapping (if active)
		if len(orm.Preloads) > 0 {
			orm.preloadScanFindSQL(
				&structPreloadScanFindSQL{
					req.hasManyNames,
					req.structType,
					newStructVal,
					req.preloadReferences,
					req.cols,
					resStructScanColumnRows.scanArgs,
				},
			)
		}

		if !req.isSlice {
			req.valElem.Set(newStructVal)

			break
		}

		if req.valElem.Type().Elem().Kind() == reflect.Pointer {
			*req.sliceVal = reflect.Append(*req.sliceVal, newStructPtr)
		} else {
			*req.sliceVal = reflect.Append(*req.sliceVal, newStructVal)
		}
	}

	if req.isSlice {
		req.valElem.Set(*req.sliceVal)
	}

	if err := req.rows.Err(); err != nil {
		return orm.setError(
			"ensure that database result stream is valid and complete",
			err,
		)
	}

	if err := req.rows.Close(); err != nil {
		return orm.setError(
			"ensure SQLDB is reachable and the query is valid.",
			err,
		)
	}

	return nil
}

type reqStructScanColumnRowsScanFindSQL struct {
	rows             *sql.Rows
	preloadPrefixMap map[string]struct{}
	fieldLookupMap   map[string]*structFieldInfo
	newStructVal     reflect.Value
	cols             []string
}

type resStructScanColumnRowsScanFindSQL struct {
	fieldHolders  map[int]reflect.Value
	fieldMappings map[int]reflect.Value
	scanArgs      []any
}

func (orm *ORM) scanColumnRowsScanFindSQL(req *reqStructScanColumnRowsScanFindSQL) (*resStructScanColumnRowsScanFindSQL, error) {
	scanArgs := make([]any, len(req.cols))
	fieldMappings := make(map[int]reflect.Value)
	fieldHolders := make(map[int]reflect.Value)
	usedFields := make(map[int]struct{})

	for idx, colName := range req.cols {
		res := prefixScanColumnRowsScanFindSQL(
			&reqPrefixScanColumnRowsScan{
				scanArgs:         scanArgs,
				fieldMappings:    fieldMappings,
				fieldHolders:     fieldHolders,
				colName:          colName,
				preloadPrefixMap: req.preloadPrefixMap,
				idx:              idx,
				fieldLookupMap:   req.fieldLookupMap,
				newStructVal:     req.newStructVal,
				usedFields:       usedFields,
			})

		scanArgs = res.scanArgs
		fieldHolders = res.fieldHolders
		fieldMappings = res.fieldMappings
	}

	if err := req.rows.Scan(scanArgs...); err != nil {
		return nil, orm.setError(
			"ensure that database column types match your struct field types",
			err,
		)
	}

	return &resStructScanColumnRowsScanFindSQL{
		scanArgs:      scanArgs,
		fieldHolders:  fieldHolders,
		fieldMappings: fieldMappings,
	}, nil
}

type reqPrefixScanColumnRowsScan struct {
	fieldMappings    map[int]reflect.Value
	fieldHolders     map[int]reflect.Value
	preloadPrefixMap map[string]struct{}
	usedFields       map[int]struct{}
	fieldLookupMap   map[string]*structFieldInfo
	newStructVal     reflect.Value
	colName          string
	scanArgs         []any
	idx              int
}

type resPrefixScanColumnRowsScan struct {
	fieldHolders  map[int]reflect.Value
	fieldMappings map[int]reflect.Value
	scanArgs      []any
}

func prefixScanColumnRowsScanFindSQL(req *reqPrefixScanColumnRowsScan) *resPrefixScanColumnRowsScan {
	colLower := strings.ToLower(req.colName)
	matched := false

	for prefix := range req.preloadPrefixMap {
		if !strings.HasPrefix(colLower, prefix) {
			continue
		}

		var rawVal any

		req.scanArgs[req.idx] = &rawVal

		matched = true

		break
	}

	if !matched {
		matched = fieldInfoPrefixScanColumnRowsScanFindSQL(
			&structFieldInfoPrefixScanColumnRowsScanFindSQL{
				scanArgs:       req.scanArgs,
				fieldLookupMap: req.fieldLookupMap,
				colLower:       colLower,
				usedFields:     req.usedFields,
				newStructVal:   req.newStructVal,
				idx:            req.idx,
				fieldHolders:   req.fieldHolders,
				fieldMappings:  req.fieldMappings,
				matched:        matched,
			},
		)
	}

	if !matched {
		var dummy any

		req.scanArgs[req.idx] = &dummy
	}

	return &resPrefixScanColumnRowsScan{
		scanArgs:      req.scanArgs,
		fieldHolders:  req.fieldHolders,
		fieldMappings: req.fieldMappings,
	}
}

type structFieldInfoPrefixScanColumnRowsScanFindSQL struct {
	fieldLookupMap map[string]*structFieldInfo
	usedFields     map[int]struct{}
	fieldHolders   map[int]reflect.Value
	fieldMappings  map[int]reflect.Value
	newStructVal   reflect.Value
	colLower       string
	scanArgs       []any
	idx            int
	matched        bool
}

func fieldInfoPrefixScanColumnRowsScanFindSQL(req *structFieldInfoPrefixScanColumnRowsScanFindSQL) bool {
	if fieldInfo, found := req.fieldLookupMap[req.colLower]; found {
		req.matched = takenFieldInfoPrefixScanColumnRowsScanFindSQL(
			&structTakenFieldInfoPrefixScanColumnRowsScanFindSQL{
				scanArgs:      req.scanArgs,
				fieldInfo:     fieldInfo,
				usedFields:    req.usedFields,
				newStructVal:  req.newStructVal,
				idx:           req.idx,
				fieldHolders:  req.fieldHolders,
				fieldMappings: req.fieldMappings,
				matched:       req.matched,
			},
		)
	}

	return req.matched
}

type structTakenFieldInfoPrefixScanColumnRowsScanFindSQL struct {
	fieldInfo     *structFieldInfo
	usedFields    map[int]struct{}
	fieldHolders  map[int]reflect.Value
	fieldMappings map[int]reflect.Value
	newStructVal  reflect.Value
	scanArgs      []any
	idx           int
	matched       bool
}

func takenFieldInfoPrefixScanColumnRowsScanFindSQL(req *structTakenFieldInfoPrefixScanColumnRowsScanFindSQL) bool {
	if _, taken := req.usedFields[req.fieldInfo.index]; !taken {
		fieldVal := req.newStructVal.Field(req.fieldInfo.index)

		req.matched = fieldValtakenFieldInfoPrefixScanColumnRowsScanFindSQL(
			&structieldValtakenFieldInfoPrefixScanColumnRowsScanFindSQL{
				fieldVal:      fieldVal,
				scanArgs:      req.scanArgs,
				idx:           req.idx,
				fieldHolders:  req.fieldHolders,
				fieldMappings: req.fieldMappings,
				usedFields:    req.usedFields,
				fieldInfo:     req.fieldInfo,
				matched:       req.matched,
			},
		)
	}

	return req.matched
}

type structieldValtakenFieldInfoPrefixScanColumnRowsScanFindSQL struct {
	fieldHolders  map[int]reflect.Value
	fieldMappings map[int]reflect.Value
	usedFields    map[int]struct{}
	fieldInfo     *structFieldInfo
	fieldVal      reflect.Value
	scanArgs      []any
	idx           int
	matched       bool
}

func fieldValtakenFieldInfoPrefixScanColumnRowsScanFindSQL(req *structieldValtakenFieldInfoPrefixScanColumnRowsScanFindSQL) bool {
	if req.fieldVal.CanAddr() && req.fieldVal.CanSet() {
		holder := reflect.New(reflect.PointerTo(req.fieldVal.Type()))

		req.scanArgs[req.idx] = holder.Interface()
		req.fieldHolders[req.idx] = holder
		req.fieldMappings[req.idx] = req.fieldVal
		req.usedFields[req.fieldInfo.index] = struct{}{}

		req.matched = true
	}

	return req.matched
}

func fieldMapRowsScanFindSQL(fieldMappings map[int]reflect.Value, fieldHolders map[int]reflect.Value) {
	for fieldMap, fieldVal := range fieldMappings {
		ptr := fieldHolders[fieldMap].Elem()

		if !ptr.IsNil() {
			fieldVal.Set(ptr.Elem())
		}
	}
}

type structPreloadScanFindSQL struct {
	hasManyNames      map[string]struct{}
	structType        reflect.Type
	newStructVal      reflect.Value
	preloadReferences map[string]string
	cols              []string
	scanArgs          []any
}

func (orm *ORM) preloadScanFindSQL(req *structPreloadScanFindSQL) {
	for relName := range orm.Preloads {
		if _, isMany := req.hasManyNames[relName]; isMany {
			continue
		}

		baseName := pluralismNormalization(relName)
		tableRef := pluralizeForm(baseName)
		prefix := strings.ToLower(tableRef + "_")

		fieldPreloadScanFindSQL(
			&structFieldPreloadScanFindSQL{
				structType:        req.structType,
				baseName:          baseName,
				newStructVal:      req.newStructVal,
				preloadReferences: req.preloadReferences,
				tableRef:          tableRef,
				cols:              req.cols,
				scanArgs:          req.scanArgs,
				prefix:            prefix,
			},
		)
	}
}

type structFieldPreloadScanFindSQL struct {
	structType        reflect.Type
	preloadReferences map[string]string
	newStructVal      reflect.Value
	baseName          string
	tableRef          string
	prefix            string
	cols              []string
	scanArgs          []any
}

func fieldPreloadScanFindSQL(req *structFieldPreloadScanFindSQL) {
	for idx := 0; idx < req.structType.NumField(); idx++ {
		field := req.structType.Field(idx)
		fieldLower := strings.ToLower(field.Name)
		fieldBase := strings.TrimSuffix(fieldLower, "s")

		if fieldBase != req.baseName {
			continue
		}

		relVal, isRelValOk := relValFieldPreloadScanFindSQL(
			&structRelValFieldPreloadScanFindSQL{
				newStructVal:      req.newStructVal,
				idx:               idx,
				preloadReferences: req.preloadReferences,
				baseName:          req.baseName,
				tableRef:          req.tableRef,
				cols:              req.cols,
				scanArgs:          req.scanArgs,
			},
		)
		if !isRelValOk {
			continue
		}

		colLowerFieldPreloadScanFindSQL(
			relVal,
			req.cols,
			req.prefix,
			req.scanArgs,
		)

		break
	}
}

func colLowerFieldPreloadScanFindSQL(
	relVal reflect.Value,
	cols []string,
	prefix string,
	scanArgs []any,
) {
	relStructType := relVal.Type()

	if relStructType.Kind() == reflect.Pointer {
		return
	}

	for col, colName := range cols {
		colLower := strings.ToLower(colName)

		if !strings.HasPrefix(colLower, prefix) {
			continue
		}

		relFieldColLowerFieldPreloadScanFindSQL(
			&structRelFieldColLowerFieldPreloadScanFindSQL{
				relStructType: relStructType,
				prefix:        prefix,
				colLower:      colLower,
				scanArgs:      scanArgs,
				col:           col,
				relVal:        relVal,
			},
		)
	}
}

type structRelFieldColLowerFieldPreloadScanFindSQL struct {
	relStructType reflect.Type
	relVal        reflect.Value
	prefix        string
	colLower      string
	scanArgs      []any
	col           int
}

func relFieldColLowerFieldPreloadScanFindSQL(req *structRelFieldColLowerFieldPreloadScanFindSQL) {
	rawColName := strings.TrimPrefix(req.colLower, req.prefix)

	for idx := 0; idx < req.relStructType.NumField(); idx++ {
		relField := req.relStructType.Field(idx)
		dbTag := relField.Tag.Get("gorest")

		if dbTag == "-" {
			continue
		}

		parts := strings.Split(dbTag, ",")
		cleanTag := strings.TrimSpace(parts[0])

		if cleanTag != rawColName && !strings.EqualFold(relField.Name, rawColName) {
			continue
		}

		valPtr, ok := req.scanArgs[req.col].(*any)
		if !ok || valPtr == nil || *valPtr == nil {
			break
		}

		targetField := req.relVal.Field(idx)
		if targetField.CanSet() {
			if err := assignReflectValue(targetField, *valPtr); err != nil {
				break
			}
		}

		break
	}
}

type structRelValFieldPreloadScanFindSQL struct {
	preloadReferences map[string]string
	newStructVal      reflect.Value
	baseName          string
	tableRef          string
	cols              []string
	scanArgs          []any
	idx               int
}

func relValFieldPreloadScanFindSQL(req *structRelValFieldPreloadScanFindSQL) (reflect.Value, bool) {
	relVal := req.newStructVal.Field(req.idx)

	if !relVal.CanSet() {
		return relVal, false
	}

	targetKey := req.preloadReferences[req.baseName]
	targetRefCol := req.tableRef + "_" + targetKey

	resRefCol := targetRefColRelValFieldPreloadScanFindSQL(
		&reqStructTargetRefColRelValFieldPreloadScanFindSQL{
			cols:         req.cols,
			targetRefCol: targetRefCol,
			scanArgs:     req.scanArgs,
		},
	)

	if !resRefCol.referenceFound || resRefCol.referenceIsNil {
		relVal.Set(reflect.Zero(relVal.Type()))

		return relVal, false
	}

	if relVal.Kind() == reflect.Pointer {
		if relVal.IsNil() {
			relVal.Set(reflect.New(relVal.Type().Elem()))
		}

		relVal = relVal.Elem()
	}

	return relVal, true
}

type reqStructTargetRefColRelValFieldPreloadScanFindSQL struct {
	cols         []string
	targetRefCol string
	scanArgs     []any
}

type resStructTargetRefColRelValFieldPreloadScanFindSQL struct {
	referenceFound bool
	referenceIsNil bool
}

func targetRefColRelValFieldPreloadScanFindSQL(
	req *reqStructTargetRefColRelValFieldPreloadScanFindSQL,
) *resStructTargetRefColRelValFieldPreloadScanFindSQL {
	referenceFound := false
	referenceIsNil := true

	for col, colName := range req.cols {
		if !strings.EqualFold(colName, req.targetRefCol) {
			continue
		}

		referenceFound = true

		valPtr, ok := req.scanArgs[col].(*any)

		if ok && valPtr != nil && *valPtr != nil {
			referenceIsNil = false
		}

		break
	}

	return &resStructTargetRefColRelValFieldPreloadScanFindSQL{
		referenceFound: referenceFound,
		referenceIsNil: referenceIsNil,
	}
}

// validateDuplicateColumnsFindSQL rejects result sets where two non-preload columns
// resolve to the same parent struct field (e.g. users.id + departments.id -> "id"),
// because the mapping would be ambiguous.
func (orm *ORM) validateDuplicateColumnsFindSQL(
	cols []string,
	preloadPrefixMap map[string]struct{},
	fieldLookupMap map[string]*structFieldInfo,
) error {
	seen := make(map[string]int, len(cols))

	for idx, col := range cols {
		lower := strings.ToLower(col)

		isPreloadCol := false

		for prefix := range preloadPrefixMap {
			if strings.HasPrefix(lower, prefix) {
				isPreloadCol = true

				break
			}
		}

		if isPreloadCol {
			continue
		}

		if _, mapped := fieldLookupMap[lower]; !mapped {
			continue
		}

		if first, dup := seen[lower]; dup {
			return orm.setError(
				fmt.Sprintf(
					"column %q is selected more than once. "+
						"Preload columns are auto-selected: "+
						"remove the manual columns of the preloaded table from Select(), "+
						"or give them an alias (e.g. departments.id AS dept_id)",
					col,
				),
				fmt.Errorf("find error: duplicate column %q at positions %d and %d in result set", col, first, idx),
			)
		}

		seen[lower] = idx
	}

	return nil
} //nolint:revive
