package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Maximum number of keys per IN (...) query.
// Safe for SQL Server (2100) and Oracle (1000 items per IN) parameter limits.
const hasManyChunkSize = 500

// hasManyRelation stores One-to-Many relationship metadata (parent has []Child).
//
// Result.Orders []Order `gorest:"orders" references:"UserID"`
//
// - references -> FK fields in CHILD (Order.UserID -> user_id)
// - parent key -> field tagged primary_key in PARENT (Result.ID -> id).
type hasManyRelation struct {
	sliceType    reflect.Type
	elemType     reflect.Type
	relationName string
	table        string
	childFKCol   string
	parentKeyCol string
	orderBy      string
	orderByRaw   string
	fieldIndex   int
	childFKIdx   int
	parentKeyIdx int
	elemIsPtr    bool
}

// findHasManyRelation detects whether relationName is a slice relation (One-to-Many).
// Returns (nil, "", nil) if it is NOT a has-many relation, so that the legacy flow (JOIN) continues to work.

func (orm *ORM) findHasManyRelation(parentType reflect.Type, relationName string) (*hasManyRelation, error) {
	for idx := 0; idx < parentType.NumField(); idx++ {
		field, elemType, elemIsPtr, isFieldOk, err := buildFieldInFindHasManyRelation(parentType, idx, relationName)
		if err != nil {
			return nil, nil //nolint:ineffassign,nilerr,nolintlint
		}

		if !isFieldOk {
			continue
		}

		// 1. FK in the child table, derived from the `references` tag
		childFKCol, err := orm.buildRefTagAndChildFKColInfindHasManyRelation(field, relationName, elemType)
		if err != nil {
			return nil, orm.Error
		}

		childLookup := buildStructFieldMap(elemType)

		childFK, childFKOk := childLookup[strings.ToLower(childFKCol)]
		if !childFKOk {
			const message = "ensure that the foreign key field on the child struct is mappable"

			return nil, orm.setError(message, fmt.Errorf("preload error: "+
				"foreign key column %q could not be mapped on struct %s", childFKCol, elemType.Name()))
		}

		// 2. Primary key in the parent
		parentKeyCol, parentKeyIdx, err := orm.buildParentKeyColInfindHasManyRelation(parentType, relationName)
		if err != nil {
			return nil, orm.Error
		}

		table := pluralizeForm(pluralismNormalization(relationName))

		// 3. ORDER BY child: from the `orderby` tag, the child's default primary key (ASC) is deterministic
		orderBy, err := buildChildOrderBy(field.Tag.Get("orderby"), table, elemType, childLookup)
		if err != nil {
			message := fmt.Sprintf("check the `orderby` tag on field %s (e.g. orderby:\"created_at DESC, id ASC\")", field.Name)

			return nil, orm.setError(message, fmt.Errorf("preload error: invalid orderby for relation %q: %w", relationName, err))
		}

		return &hasManyRelation{
			relationName: relationName,
			fieldIndex:   idx,
			sliceType:    field.Type,
			elemType:     elemType,
			elemIsPtr:    elemIsPtr,
			table:        table,
			childFKCol:   childFKCol,
			childFKIdx:   childFK.index,
			parentKeyCol: parentKeyCol,
			parentKeyIdx: parentKeyIdx,
			orderBy:      orderBy,
			orderByRaw:   strings.TrimSpace(field.Tag.Get("orderby")),
		}, nil
	}

	return nil, nil
}

func buildFieldInFindHasManyRelation(
	parentType reflect.Type,
	idx int,
	relationName string,
) (field reflect.StructField, elemType reflect.Type, elemIsPtr bool, isOk bool, err error) {
	field = parentType.Field(idx)

	if !strings.EqualFold(field.Name, relationName) {
		return field, nil, false, false, nil
	}

	if field.Type.Kind() != reflect.Slice {
		return field, nil, false, true, errors.New("error")
	}

	elemType = field.Type.Elem()
	elemIsPtr = elemType.Kind() == reflect.Pointer

	if elemIsPtr {
		elemType = elemType.Elem()
	}

	if elemType.Kind() != reflect.Struct {
		return field, nil, false, true, errors.New("error")
	}

	return field, elemType, elemIsPtr, true, nil
}

func (orm *ORM) buildRefTagAndChildFKColInfindHasManyRelation(
	field reflect.StructField,
	relationName string,
	elemType reflect.Type,
) (childFKCol string, err error) {
	refTag := strings.TrimSpace(field.Tag.Get("references"))
	if refTag == "" {
		message := fmt.Sprintf("add a `references:\"<ChildFKField>\"` tag to field %s (e.g. references:\"UserID\")", field.Name)

		return "", orm.setError(message, fmt.Errorf("preload error: "+
			"one-to-many relation %q requires a references tag pointing to the foreign key on %s", relationName, elemType.Name()))
	}

	childFKCol, err = orm.findRelatedDBColumn(elemType, refTag, relationName)
	if err != nil {
		const message = "ensure that the references field exists on the related (child) struct"

		return "", orm.setError(message, fmt.Errorf("preload error: "+
			"reference %q for relation %q was not found on struct %s", refTag, relationName, elemType.Name()))
	}

	return childFKCol, nil
}

func (orm *ORM) buildParentKeyColInfindHasManyRelation(parentType reflect.Type, name string) (string, int, error) {
	parentKeyCol, parentKeyIdx := "", -1

	for idx := 0; idx < parentType.NumField(); idx++ {
		tag := parentType.Field(idx).Tag.Get("gorest")

		if tag == "" || tag == "-" {
			continue
		}

		parts := strings.Split(tag, ",")
		for _, p := range parts[1:] {
			if strings.EqualFold(strings.TrimSpace(p), "primary_key") {
				parentKeyCol = strings.TrimSpace(parts[0])
				parentKeyIdx = idx
			}
		}
	}

	if parentKeyIdx < 0 {
		const message = "ensure that the parent struct has a primary_key field (e.g. gorest:\"id, primary_key\")"

		return parentKeyCol, parentKeyIdx, orm.setError(message, fmt.Errorf("preload error: "+
			"parent struct %s has no primary_key, required by one-to-many relation %q", parentType.Name(), name))
	}

	return parentKeyCol, parentKeyIdx, nil
}

// derefKey dereferences a pointer/interface and rejects nil/zero values.
func derefKey(currentVal reflect.Value) (reflect.Value, bool) {
	val := currentVal

	for val.Kind() == reflect.Pointer || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return reflect.Value{}, false
		}

		val = val.Elem()
	}

	if !val.IsValid() || val.IsZero() {
		return reflect.Value{}, false
	}

	return val, true
}

// loadHasMany executes one additional query (per chunk) to populate the has-many relationship
// for all parent records at once. No N+1 issues, no duplicate parent rows.

func (orm *ORM) loadHasMany(ctx context.Context, driver string, parents []reflect.Value, cols []string, rel *hasManyRelation) error {
	// The parent PK column must also be selected; otherwise, we won't have the key for the IN (...) clause.
	if err := orm.isHasKeyColInLoadHasMany(cols, rel); err != nil {
		return orm.Error
	}

	// Grouping parents by key
	keys, parentsByKey := buildKeysInLoadHasMany(parents, rel)

	if len(keys) == 0 {
		return nil
	}

	// Additional condition from the handler: Preload("Orders", "orders.deleted_at IS NULL")
	condSQL, condArgs, err := orm.parsePreloadCondition(rel.relationName, orm.Preloads[rel.relationName], driver)
	if err != nil {
		return orm.Error
	}

	grouped, err := orm.buildGroupInloadHasMany(ctx, rel, keys, condSQL, condArgs, driver)
	if err != nil {
		return orm.Error
	}

	// Attach to each parent (parents without children get empty slices, not nil)
	for ks, ps := range parentsByKey {
		items := grouped[ks]

		for _, p := range ps {
			field := p.Field(rel.fieldIndex)
			if !field.CanSet() {
				continue
			}

			slice := reflect.MakeSlice(rel.sliceType, 0, len(items))
			for _, it := range items {
				slice = reflect.Append(slice, it)
			}

			field.Set(slice)
		}
	}

	return nil
}

func (orm *ORM) isHasKeyColInLoadHasMany(cols []string, rel *hasManyRelation) error {
	hasKeyCol := false

	for _, col := range cols {
		name := col
		if idx := strings.LastIndex(col, "."); idx != -1 {
			name = col[idx+1:]
		}

		if strings.EqualFold(name, rel.parentKeyCol) {
			hasKeyCol = true

			break
		}
	}

	if !hasKeyCol {
		message := fmt.Sprintf("ensure column %q is included in Select() when using Preload(%q)", rel.parentKeyCol, rel.relationName)

		return orm.setError(message, fmt.Errorf("preload error: parent key column %q is not selected", rel.parentKeyCol))
	}

	return nil
}

func buildKeysInLoadHasMany(parents []reflect.Value, rel *hasManyRelation) (keys []any, parentsByKey map[string][]reflect.Value) {
	parentsByKey = make(map[string][]reflect.Value)

	for _, parent := range parents {
		derefFieldParent, ok := derefKey(parent.Field(rel.parentKeyIdx))
		if !ok {
			continue
		}

		fieldParent := fmt.Sprint(derefFieldParent.Interface())

		if _, seen := parentsByKey[fieldParent]; !seen {
			keys = append(keys, derefFieldParent.Interface())
		}

		parentsByKey[fieldParent] = append(parentsByKey[fieldParent], parent)
	}

	return keys, parentsByKey
}

func (orm *ORM) execLoadHasMany(
	ctx context.Context,
	query string,
	args []any,
	rel *hasManyRelation,
	childLookup map[string]*structFieldInfo,
	grouped map[string][]reflect.Value,
) error {
	rows, message, err := orm.Database.QuerySQL(ctx, query, args...)
	if err != nil {
		return orm.setError(message, err)
	}

	defer func() {
		if err := rows.Close(); err != nil {
			orm.Message = "Ensure the context has sufficient timeout for " +
				"cleanup operations and check network stability to the database server."
			orm.Error = fmt.Errorf("failed to rows.Close: %w", err)
		}
	}()

	rCols, err := rows.Columns()
	if err != nil {
		return orm.setError(message, err)
	}

	err = orm.rowsNextInExecLoadHasMany(rows, rel, rCols, childLookup, grouped)
	if err != nil {
		return orm.Error
	}

	if err := rows.Err(); err != nil {
		orm.Message = "ensure that database result stream is valid and complete"

		return orm.setError(message, err)
	}

	return nil
}

func scanInRowsNextInExecLoadHasMany(
	rCols []string,
	childLookup map[string]*structFieldInfo,
	elem reflect.Value,
) (scanArgs []any, holders []reflect.Value, targets []reflect.Value) {
	scanArgs = make([]any, len(rCols))
	holders = make([]reflect.Value, len(rCols))
	targets = make([]reflect.Value, len(rCols))

	for idx, colName := range rCols {
		if lookup, found := childLookup[strings.ToLower(colName)]; found {
			fieldLookup := elem.Field(lookup.index)

			if fieldLookup.CanSet() {
				typ := reflect.New(reflect.PointerTo(fieldLookup.Type())) // **T -> save for NULL

				scanArgs[idx] = typ.Interface()
				holders[idx] = typ
				targets[idx] = fieldLookup

				continue
			}
		}

		var dummy any

		scanArgs[idx] = &dummy
	}

	return scanArgs, holders, targets
}

func (orm *ORM) rowsNextInExecLoadHasMany(
	rows *sql.Rows,
	rel *hasManyRelation,
	rCols []string,
	childLookup map[string]*structFieldInfo,
	grouped map[string][]reflect.Value,
) error {
	for rows.Next() {
		elemPtr := reflect.New(rel.elemType)
		elem := elemPtr.Elem()

		scanArgs, holders, targets := scanInRowsNextInExecLoadHasMany(rCols, childLookup, elem)

		if err := rows.Scan(scanArgs...); err != nil {
			const message = "ensure that database column types match your struct field types"

			return orm.setError(message, err)
		}

		for hld, hlds := range holders {
			if !hlds.IsValid() {
				continue
			}

			if janda := hlds.Elem(); !janda.IsNil() {
				targets[hld].Set(janda.Elem())
			}
		}

		derefFieldElem, ok := derefKey(elem.Field(rel.childFKIdx))
		if !ok {
			continue
		}

		fieldElem := fmt.Sprint(derefFieldElem.Interface())

		if rel.elemIsPtr {
			grouped[fieldElem] = append(grouped[fieldElem], elemPtr)
		} else {
			grouped[fieldElem] = append(grouped[fieldElem], elem)
		}
	}

	return nil
}

func (orm *ORM) buildGroupInloadHasMany(
	ctx context.Context,
	rel *hasManyRelation,
	keys []any,
	condSQL string,
	condArgs []any,
	activeDriver string,
) (grouped map[string][]reflect.Value, err error) {
	grouped = make(map[string][]reflect.Value)

	childLookup := buildStructFieldMap(rel.elemType)

	for start := 0; start < len(keys); start += hasManyChunkSize {
		end := start + hasManyChunkSize
		if end > len(keys) {
			end = len(keys)
		}

		chunk := keys[start:end]

		// Construct the WHERE clause with "?" placeholders first, then normalize it once
		// automatically converts to ?, $n, @pN, or :n depending on the driver (sequential argument numbering)
		rawPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		whereSQL := fmt.Sprintf("%s.%s IN (%s)", rel.table, rel.childFKCol, rawPlaceholders)

		args := append(make([]any, 0, len(chunk)), chunk...)

		if condSQL != "" {
			whereSQL += " AND (" + condSQL + ")"

			args = append(args, condArgs...)
		}

		whereSQL, _ = normalizeWherePlaceholders(whereSQL, activeDriver, 1)

		query := fmt.Sprintf("SELECT %s.* FROM %s WHERE %s", rel.table, rel.table, whereSQL)

		if rel.orderBy != "" {
			query += " ORDER BY " + rel.orderBy
		}

		if err := orm.execLoadHasMany(ctx, query, args, rel, childLookup, grouped); err != nil {
			return nil, orm.Error
		}
	}

	return grouped, nil
}

// collect Parents gathers strict parents (addressable) from the Find results.
//
//nolint:revive
func (orm *ORM) collectParents(valElem reflect.Value, isSlice bool) ([]reflect.Value, error) {
	if !isSlice {
		return []reflect.Value{valElem}, nil
	}

	parents := make([]reflect.Value, 0, valElem.Len())

	for idx := 0; idx < valElem.Len(); idx++ {
		val := valElem.Index(idx)

		if val.Kind() == reflect.Pointer {
			if val.IsNil() {
				continue
			}

			val = val.Elem()
		}

		if val.Kind() != reflect.Struct {
			const message = "Ensure each element in the query result slice is a struct or a pointer to a struct"

			return nil, orm.setError(message, errors.New("preload error: parent element is not a struct"))
		}

		parents = append(parents, val)
	}

	return parents, nil
}

// primaryKeyColumn returns the name of the column tagged with primary_key ("" if none).
func primaryKeyColumn(t reflect.Type) string {
	for j := 0; j < t.NumField(); j++ {
		tag := t.Field(j).Tag.Get("gorest")

		if tag == "" || tag == "-" {
			continue
		}

		parts := strings.Split(tag, ",")

		for _, p := range parts[1:] {
			if strings.EqualFold(strings.TrimSpace(p), "primary_key") {
				return strings.TrimSpace(parts[0])
			}
		}
	}

	return ""
}

// buildChildOrderBy validates the `orderby` tag against child columns (preventing typos/injection)
// and generates a "table.col DIR, table.col DIR" string.
// Without the tag: sorts by the child's primary key (ASC) to ensure deterministic results.
func buildChildOrderBy(currentRaw, table string, elemType reflect.Type, lookup map[string]*structFieldInfo) (string, error) {
	raw := strings.TrimSpace(currentRaw)

	if raw == "" {
		if pk := primaryKeyColumn(elemType); pk != "" {
			return fmt.Sprintf("%s.%s ASC", table, pk), nil
		}

		return "", nil
	}

	var parts []string

	for _, item := range strings.Split(raw, ",") {
		tokens := strings.Fields(item)
		if len(tokens) == 0 || len(tokens) > 2 {
			return "", fmt.Errorf("invalid order item %q, expected \"column [ASC|DESC]\"", strings.TrimSpace(item))
		}

		fi, ok := lookup[strings.ToLower(tokens[0])]
		if !ok {
			return "", fmt.Errorf("column %q was not found on struct %s", tokens[0], elemType.Name())
		}

		col := strings.TrimSpace(strings.Split(fi.dbTag, ",")[0])
		if col == "" {
			return "", fmt.Errorf("field %q has no gorest column tag", tokens[0])
		}

		dir := asc

		if len(tokens) == 2 {
			switch strings.ToUpper(tokens[1]) {
			case asc:
				dir = asc
			case desc:
				dir = desc
			default:
				return "", fmt.Errorf("invalid direction %q for column %q, use ASC or DESC", tokens[1], tokens[0])
			}
		}

		parts = append(parts, fmt.Sprintf("%s.%s %s", table, col, dir))
	}

	return strings.Join(parts, ", "), nil
}

// parsePreloadCondition parses GORM-style Preload(relation, args...) arguments:
//
//	Preload("Orders", "orders.deleted_at IS NULL")
//	Preload("Orders", "orders.status = ? AND orders.deleted_at IS NULL", "COMPLETED")
//
// args[0] = condition (string), args[1:] = placeholder arguments.
// The number of placeholders must match the number of arguments (same rule as .Where()).
func (orm *ORM) parsePreloadCondition(relation string, args []any, activeDriver string) (string, []any, error) {
	const message = "ensure Preload(relation, \"condition\", args...) has a valid condition string and matching number of arguments"

	if len(args) == 0 {
		return "", nil, nil
	}

	clause, ok := args[0].(string)
	clause = strings.TrimSpace(clause)

	if !ok || clause == "" {
		return "", nil, orm.setError(
			message,
			fmt.Errorf("preload error: first extra argument of Preload(%q, ...) must be a non-empty condition string", relation),
		)
	}

	condArgs := args[1:]

	if n := countActivePlaceholders(clause, activeDriver); n != len(condArgs) {
		return "", nil, orm.setError(
			message,
			fmt.Errorf("preload error: condition for %q has %d placeholder(s) but %d argument(s) were provided", relation, n, len(condArgs)),
		)
	}

	return clause, condArgs, nil
} //nolint:revive
