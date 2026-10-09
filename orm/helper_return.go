package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/elsyahtech/gorest/database"
)

func isReturnDest(arg any) bool {
	val := reflect.ValueOf(arg)

	if val.Kind() != reflect.Pointer || val.IsNil() {
		return false
	}

	elem := val.Elem()

	//nolint:exhaustive
	switch elem.Kind() {
	case reflect.Struct:
		return true
	case reflect.Slice:
		itemType := elem.Type().Elem()

		if itemType.Kind() == reflect.Pointer {
			itemType = itemType.Elem()
		}

		return itemType.Kind() == reflect.Struct
	default:
		return false
	}
}

type returnPlan struct {
	destPtr      any
	destType     reflect.Type
	destVal      reflect.Value
	fillIdx      []int
	queryCols    []string
	matchIdx     []int
	pkIdx        []int
	destIsSlice  bool
	elemIsPtr    bool
	sameAsInput  bool
	inputIsSlice bool
	projection   bool
}

func (plan *returnPlan) returningClause() string {
	return " RETURNING " + strings.Join(plan.queryCols, ", ")
}

func (plan *returnPlan) outputClause() string {
	cols := make([]string, 0, len(plan.queryCols))

	for _, col := range plan.queryCols {
		cols = append(cols, "INSERTED."+col)
	}

	return "OUTPUT " + strings.Join(cols, ", ")
}

// taggedColumns: tagged struct fields (ordered according to the fields).
func taggedColumns(structType reflect.Type) ([]string, map[string]int) {
	var ordered []string

	fieldIdx := make(map[string]int)

	for idx := 0; idx < structType.NumField(); idx++ {
		tag := structType.Field(idx).Tag.Get("gorest")

		if tag == "" || tag == "-" {
			continue
		}

		name := strings.TrimSpace(strings.Split(tag, ",")[0])

		if name == "" || name == "-" {
			continue
		}

		lower := strings.ToLower(name)

		if _, dup := fieldIdx[lower]; dup {
			continue
		}

		ordered = append(ordered, name)
		fieldIdx[lower] = idx
	}

	return ordered, fieldIdx
}

func derefValue(val reflect.Value) reflect.Value {
	for val.Kind() == reflect.Pointer && !val.IsNil() {
		//nolint:revive
		val = val.Elem()
	}

	return val
}

// planReturn specifies the target, the columns to be populated, and the RETURNING/OUTPUT columns.
func (orm *ORM) planReturn(data any, rowsVal []reflect.Value, meta columnMetaData, driver string) (*returnPlan, error) {
	switch driver {
	case database.POSTGRES, database.SQLITE, database.SQLSERVER:
	default:
		return nil, orm.setError(
			"Return() is not supported on this database driver yet",
			fmt.Errorf("create: Return is not supported on driver %s", driver),
		)
	}

	inputVal := derefValue(reflect.ValueOf(data))
	inputType := rowsVal[0].Type()

	plan := &returnPlan{
		inputIsSlice: inputVal.Kind() == reflect.Slice,
		projection:   len(orm.ReturnCols) > 0,
	}

	// 1. Destination
	if err := orm.destinationInPlanReturn(data, rowsVal, plan, inputType); err != nil {
		return nil, orm.Error
	}

	// 2. Fields to populate: user selection, or all tagged fields of the target struct
	destCols, destFieldIdx := taggedColumns(plan.destType)
	if len(destCols) == 0 {
		return nil, orm.setError(
			fmt.Sprintf("struct %s has no field tagged with 'gorest', so Return() has nothing to fill", plan.destType.Name()),
			errors.New("create: Return destination has no tagged columns"),
		)
	}

	var (
		queryCols []string
		seen      = make(map[string]struct{})
	)

	addQueryCol := func(name string) {
		lower := strings.ToLower(name)

		if _, dup := seen[lower]; !dup {
			seen[lower] = struct{}{}

			queryCols = append(queryCols, name)
		}
	}

	if err := orm.buildColInPlanReturn(addQueryCol, destCols, destFieldIdx, plan); err != nil {
		return nil, orm.Error
	}

	// 3. Matching key & primary key (only if destination = input, since the types are the same)
	orm.compareColToPKInPlanReturn(addQueryCol, meta, plan, rowsVal, destFieldIdx)

	plan.queryCols = queryCols

	return plan, nil
}

func (orm *ORM) destinationInPlanReturn(data any, rowsVal []reflect.Value, plan *returnPlan, inputType reflect.Type) error {
	sameAsInput := orm.ReturnDest == nil

	if !sameAsInput {
		destPtr := reflect.ValueOf(orm.ReturnDest)
		dataPtr := reflect.ValueOf(data)

		sameAsInput = dataPtr.Kind() == reflect.Pointer && dataPtr.Type() == destPtr.Type() && dataPtr.Pointer() == destPtr.Pointer()
	}

	if sameAsInput {
		if !rowsVal[0].CanAddr() {
			return orm.setError(
				"Return() writes the results back into the input, so pass a pointer, e.g. Create(&user)",
				errors.New("create: Return needs an addressable input"),
			)
		}

		plan.sameAsInput = true
		plan.destPtr = data
		plan.destType = inputType
		plan.destIsSlice = plan.inputIsSlice
	} else {
		destPtr := reflect.ValueOf(orm.ReturnDest)

		plan.destPtr = orm.ReturnDest
		plan.destVal = destPtr.Elem()

		verifIsReflectSliceInPlanReturn(plan)
	}

	return nil
}

func verifIsReflectSliceInPlanReturn(plan *returnPlan) {
	if plan.destVal.Kind() == reflect.Slice {
		plan.destIsSlice = true
		plan.destType = plan.destVal.Type().Elem()

		if plan.destType.Kind() == reflect.Pointer {
			plan.elemIsPtr = true
			plan.destType = plan.destType.Elem()
		}

		return
	}

	plan.destType = plan.destVal.Type()
}

func (orm *ORM) buildColInPlanReturn(addQueryCol func(name string), destCols []string, destFieldIdx map[string]int, plan *returnPlan) (err error) {
	userCols := destCols

	if len(orm.ReturnCols) > 0 {
		userCols = nil

		for _, col := range orm.ReturnCols {
			colToCheck := col

			if dot := strings.LastIndex(col, "."); dot != -1 {
				colToCheck = col[dot+1:]
			}

			if _, ok := destFieldIdx[strings.ToLower(colToCheck)]; !ok {
				message := fmt.Sprintf("Ensure the column name '%s' in Return() matches a field tagged with 'gorest' in struct %s.", col, plan.destType.Name())

				return orm.setError(message, fmt.Errorf("create: Return invalid column %q for %s", col, plan.destType.Name()))
			}

			userCols = append(userCols, colToCheck)
		}
	}

	filled := make(map[int]struct{})

	for _, col := range userCols {
		idx := destFieldIdx[strings.ToLower(col)]

		if _, dup := filled[idx]; dup {
			continue
		}

		filled[idx] = struct{}{}

		plan.fillIdx = append(plan.fillIdx, idx)

		addQueryCol(col)
	}

	return nil
}

func (orm *ORM) compareColToPKInPlanReturn(
	addQueryCol func(name string), meta columnMetaData, plan *returnPlan, rowsVal []reflect.Value, destFieldIdx map[string]int,
) {
	for _, primaryKey := range meta.primaryKeyColumns {
		addQueryCol(primaryKey)
	}

	plan.pkIdx = meta.primaryKeyIndex

	if !plan.inputIsSlice {
		return
	}

	matchCols := orm.matchColumnsForReturn(rowsVal, meta)

	for _, col := range matchCols {
		if idx, ok := destFieldIdx[strings.ToLower(col)]; ok {
			plan.matchIdx = append(plan.matchIdx, idx)

			addQueryCol(col)
		}
	}

	if len(plan.matchIdx) != len(matchCols) {
		plan.matchIdx = nil // kunci tidak lengkap -> pencocokan lewat urutan
	}
}

// matchColumnsForReturn: key columns to match result rows to input slice elements.
// Upsert -> conflict columns (database PK may differ from struct PK); others -> PK if fully populated.
func (orm *ORM) matchColumnsForReturn(rowsVal []reflect.Value, meta columnMetaData) []string {
	if orm.IsUpsert && len(orm.UpsertConflictCols) > 0 {
		return orm.UpsertConflictCols
	}

	if len(meta.primaryKeyColumns) == 0 || len(meta.primaryKeyColumns) != len(meta.primaryKeyIndex) {
		return nil
	}

	for _, row := range rowsVal {
		for _, idx := range meta.primaryKeyIndex {
			if row.Field(idx).IsZero() {
				return nil
			}
		}
	}

	return meta.primaryKeyColumns
}

// execReturn executes a query containing RETURNING/OUTPUT and populates the result into the destination.
func (orm *ORM) execReturn(
	execCtx context.Context,
	queryStr string,
	args []any,
	plan *returnPlan,
	inputRows []reflect.Value,
	meta columnMetaData,
) error {
	scanned, err := orm.queryReturnRows(execCtx, queryStr, args, plan)
	if err != nil {
		return err
	}

	if err = orm.finishReturn(plan, scanned, inputRows); err != nil {
		return err
	}

	orm.LastInsertId = lastInsertIDFromReturn(plan, scanned, inputRows, meta)

	return nil
}

func (orm *ORM) queryReturnRows(execCtx context.Context, queryStr string, args []any, plan *returnPlan) ([]reflect.Value, error) {
	rows, message, httpCode, err := orm.Database.QuerySQL(execCtx, queryStr, args...)
	if err != nil {
		return nil, orm.setError(message, err, httpCode)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil && orm.Error == nil {
			orm.Message = "Ensure the context has sufficient timeout for " +
				"cleanup operations and check network stability to the database server."
			orm.Error = fmt.Errorf("failed to rows.Close: %w", closeErr)
		}
	}()

	return orm.scanReturnRows(rows, plan)
}

func (orm *ORM) finishReturn(plan *returnPlan, scanned, inputRows []reflect.Value) error {
	if err := orm.applyReturnRows(plan, scanned, inputRows); err != nil {
		return err
	}

	orm.RowsAffected = int64(len(scanned))
	orm.Result = plan.destPtr

	return nil
}

func buildUpdateStatement(tableName, setClause, whereClause string, plan *returnPlan, driver string) string {
	switch {
	case plan == nil:
		return fmt.Sprintf("UPDATE %s SET %s WHERE %s", tableName, setClause, whereClause)
	case driver == database.SQLSERVER:
		return fmt.Sprintf("UPDATE %s SET %s %s WHERE %s", tableName, setClause, plan.outputClause(), whereClause)
	default:
		return fmt.Sprintf("UPDATE %s SET %s WHERE %s%s", tableName, setClause, whereClause, plan.returningClause())
	}
}

// scanReturnRows reuses the Find scan engine: each row is scanned into a struct of the target type.
func (orm *ORM) scanReturnRows(rows *sql.Rows, plan *returnPlan) ([]reflect.Value, error) {
	cols, err := orm.mappingSQLRowsColumnResult(rows)
	if err != nil {
		return nil, orm.Error
	}

	fieldLookupMap := buildStructFieldMap(plan.destType)

	var scanned []reflect.Value

	for rows.Next() {
		structVal := reflect.New(plan.destType).Elem()

		res, scanErr := orm.scanColumnRowsScanFindSQL(
			&reqStructScanColumnRowsScanFindSQL{
				rows:             rows,
				cols:             cols,
				preloadPrefixMap: make(map[string]struct{}),
				fieldLookupMap:   fieldLookupMap,
				newStructVal:     structVal,
			},
		)
		if scanErr != nil {
			return nil, orm.Error
		}

		fieldMapRowsScanFindSQL(res.fieldMappings, res.fieldHolders)

		scanned = append(scanned, structVal)
	}

	if err = rows.Err(); err != nil {
		return nil, orm.setError("ensure that database result stream is valid and complete", err)
	}

	return scanned, nil
}

// fillReturnedFields populates the destination from the result row. With a projection (list of columns),
// the destination is cleared first so that only the requested columns are filled; without a projection,
// other fields remain untouched.
func fillReturnedFields(target, src reflect.Value, plan *returnPlan) {
	if plan.projection && target.CanSet() {
		target.Set(reflect.Zero(target.Type()))
	}

	for _, idx := range plan.fillIdx {
		field := target.Field(idx)

		if field.CanSet() {
			field.Set(src.Field(idx))
		}
	}
}

func (orm *ORM) applyReturnRows(plan *returnPlan, scanned, inputRows []reflect.Value) error {
	// No rows returned (upsert DO NOTHING, condition not met): target remains unchanged.
	if len(scanned) == 0 {
		return nil
	}

	switch {
	case plan.sameAsInput:
		return orm.applyReturnToInput(plan, scanned, inputRows)
	case plan.destIsSlice:
		sliceType := plan.destVal.Type()
		result := reflect.MakeSlice(sliceType, 0, len(scanned))

		for _, item := range scanned {
			if plan.elemIsPtr {
				ptr := reflect.New(plan.destType)
				ptr.Elem().Set(item)

				result = reflect.Append(result, ptr)
			} else {
				result = reflect.Append(result, item)
			}
		}

		plan.destVal.Set(result)

		return nil
	default:
		if len(scanned) > 1 {
			return orm.setError(
				fmt.Sprintf("Return() received %d rows but the destination is a single struct. Use a slice destination, e.g. Return(&items).", len(scanned)),
				errors.New("create: Return destination is a struct but multiple rows were returned"),
			)
		}

		fillReturnedFields(plan.destVal, scanned[0], plan)

		return nil
	}
}

func (orm *ORM) applyReturnToInput(plan *returnPlan, scanned, inputRows []reflect.Value) error {
	if !plan.inputIsSlice {
		fillReturnedFields(inputRows[0], scanned[0], plan)

		return nil
	}

	// Slice: match by key if present, otherwise by order.
	if len(plan.matchIdx) > 0 {
		byKey := make(map[string]reflect.Value, len(scanned))

		for _, item := range scanned {
			byKey[returnKey(item, plan.matchIdx)] = item
		}

		for _, row := range inputRows {
			if item, ok := byKey[returnKey(row, plan.matchIdx)]; ok {
				fillReturnedFields(row, item, plan)
			}
		}

		return nil
	}

	if len(scanned) != len(inputRows) {
		return orm.setError(
			fmt.Sprintf("return() got %d rows for %d items and cannot map them without a unique key. "+
				"Set the primary key on each item or use Upsert(\"column\").", len(scanned), len(inputRows)),
			errors.New("create: Return cannot map returned rows to input items"),
		)
	}

	for idx, row := range inputRows {
		fillReturnedFields(row, scanned[idx], plan)
	}

	return nil
}

func returnKey(row reflect.Value, idxs []int) string {
	parts := make([]string, 0, len(idxs))

	for _, idx := range idxs {
		val := row.Field(idx)

		for val.Kind() == reflect.Pointer {
			if val.IsNil() {
				break
			}

			val = val.Elem()
		}

		if val.Kind() == reflect.Pointer {
			parts = append(parts, "<nil>")

			continue
		}

		parts = append(parts, fmt.Sprint(val.Interface()))
	}

	return strings.Join(parts, "\x00")
}

// lastInsertIDFromReturn: PK from the result row if available; otherwise, from the input struct.
func lastInsertIDFromReturn(plan *returnPlan, scanned, inputRows []reflect.Value, meta columnMetaData) string {
	if len(meta.primaryKeyIndex) == 0 {
		return ""
	}

	source := inputRows[0]

	if plan.sameAsInput && len(scanned) > 0 {
		source = scanned[0]
	}

	values := make([]string, 0, len(meta.primaryKeyIndex))

	for _, idx := range meta.primaryKeyIndex {
		values = append(values, fmt.Sprintf("%v", source.Field(idx).Interface()))
	}

	return strings.Join(values, "-")
} //nolint:revive
