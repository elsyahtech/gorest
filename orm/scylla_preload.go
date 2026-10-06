package orm

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
)

// IN pada partition key di CQL adalah multi-partition query: makin besar makin berat
// bagi coordinator, jadi chunk-nya lebih kecil daripada SQL/Mongo.
const scyllaInChunkSize = 100

const scyllaPreloadCondHint = "Preload condition must be a plain AND-only CQL " +
	"condition like \"orders.status = ?\" (no OR, LIKE, IS NULL, or parentheses). Non-key columns need .AllowFiltering()."

var cqlUnsupportedIsNull = regexp.MustCompile(`(?i)\sIS\s+(NOT\s+)?NULL\b`)

// scyllaCond: kondisi tambahan dari handler, ditempel ke query preload dengan AND.
type scyllaCond struct {
	clause string
	args   []any
}

type childSortKey struct {
	fieldIdx int
	desc     bool
}

type scyllaHasMany struct {
	rel        *hasManyRelation
	cond       scyllaCond
	sortKeys   []childSortKey
	selectCols []string
}

type scyllaBelongsTo struct {
	// metadata relasi dipakai ulang dari planMongoBelongsTo (murni reflect, tidak ada yang spesifik Mongo)
	core       *mongoBelongsTo
	cond       scyllaCond
	selectCols []string
}

type scyllaPreloadPlan struct {
	hasMany   []*scyllaHasMany
	belongsTo []*scyllaBelongsTo
}

func (p *scyllaPreloadPlan) isEmpty() bool {
	return p == nil || (len(p.hasMany) == 0 && len(p.belongsTo) == 0)
}

// requiredParentColumns: kolom parent yang wajib ikut ter-select agar preload bisa jalan.
func (p *scyllaPreloadPlan) requiredParentColumns() []string {
	cols := make([]string, 0, len(p.hasMany)+len(p.belongsTo))

	for _, h := range p.hasMany {
		cols = append(cols, h.rel.parentKeyCol)
	}

	for _, b := range p.belongsTo {
		cols = append(cols, b.core.parentFKCol)
	}

	return cols
}

// appendMissingColumns adds mandatory columns to the SELECT list if they are not already present (case-insensitive).
func appendMissingColumns(currentCols []string, needs []string) []string {
	cols := currentCols

	for _, need := range needs {
		exists := false

		for _, col := range cols {
			if strings.EqualFold(strings.TrimSpace(col), need) {
				exists = true

				break
			}
		}

		if !exists {
			cols = append(cols, need)
		}
	}

	return cols
}

// planScyllaPreloads memvalidasi semua Preload SEBELUM query utama dijalankan.
func (orm *ORM) planScyllaPreloads(structType reflect.Type, byTable map[string][]string) (*scyllaPreloadPlan, error) {
	plan := &scyllaPreloadPlan{}

	if len(orm.Preloads) == 0 {
		return plan, nil
	}

	parentLookup := buildStructFieldMapByTag(structType)

	for relationName, args := range orm.Preloads {
		hasMany, err := orm.planHasManyScyllaPreloads(relationName, args, structType, byTable, plan)
		if err != nil {
			return nil, err
		}

		if hasMany {
			continue
		}

		if err := orm.planBelongsToScyllaPreloads(relationName, args, structType, byTable, plan, parentLookup); err != nil {
			return nil, err
		}
	}

	return plan, nil
}

func (orm *ORM) planHasManyScyllaPreloads(
	relName string,
	args []any,
	structType reflect.Type,
	byTable map[string][]string,
	preload *scyllaPreloadPlan,
) (bool, error) {
	hasMany, err := orm.findHasManyRelation(structType, relName)
	if err != nil {
		return false, orm.Error
	}

	if hasMany == nil {
		return false, nil
	}

	cond, err := parseScyllaPreloadCondition(relName, args, hasMany.table)
	if err != nil {
		return false, orm.setError(scyllaPreloadCondHint, err)
	}

	keys, err := buildChildSortKeys(hasMany.orderByRaw, hasMany.elemType)
	if err != nil {
		const message = "ensure the `orderby` tag uses the format \"column [ASC|DESC]\" with columns that exist on the child struct"

		return false, orm.setError(message, err)
	}

	var selectCols []string

	if cols, ok := byTable[strings.ToLower(hasMany.table)]; ok && len(cols) > 0 {
		selectCols = appendMissingColumns(cols, []string{hasMany.childFKCol})
	}

	preload.hasMany = append(preload.hasMany, &scyllaHasMany{rel: hasMany, cond: cond, sortKeys: keys, selectCols: selectCols})

	return true, nil
}

func (orm *ORM) planBelongsToScyllaPreloads(
	relationName string,
	args []any,
	structType reflect.Type,
	byTable map[string][]string,
	plan *scyllaPreloadPlan,
	parentLookup map[string]*structFieldInfo,
) error {
	belongTo, err := orm.planMongoBelongsTo(structType, parentLookup, relationName, nil)
	if err != nil {
		return orm.Error
	}

	cond, err := parseScyllaPreloadCondition(relationName, args, belongTo.collection)
	if err != nil {
		return orm.setError(scyllaPreloadCondHint, err)
	}

	var selectCols []string

	if cols, ok := byTable[strings.ToLower(belongTo.collection)]; ok && len(cols) > 0 {
		selectCols = appendMissingColumns(cols, []string{belongTo.targetCol})
	}

	plan.belongsTo = append(plan.belongsTo, &scyllaBelongsTo{core: belongTo, cond: cond, selectCols: selectCols})

	return nil
}

// parseScyllaPreloadCondition membaca Preload(relation, "cond", args...) untuk CQL.
// CQL tidak mendukung OR, LIKE, IS [NOT] NULL, maupun tanda kurung di WHERE, jadi ditolak lebih awal.
func parseScyllaPreloadCondition(relation string, args []any, table string) (scyllaCond, error) {
	if len(args) == 0 {
		return scyllaCond{}, nil
	}

	clause, ok := args[0].(string)
	clause = strings.TrimSpace(clause)

	if !ok || clause == "" {
		return scyllaCond{}, fmt.Errorf("preload error: first extra argument of Preload(%q, ...) must be a non-empty condition string", relation)
	}

	padded := " " + clause + " "

	switch {
	case cqlUnsupportedOr.MatchString(padded):
		return scyllaCond{}, errors.New("preload error: OR is not supported by CQL")
	case cqlUnsupportedLike.MatchString(padded):
		return scyllaCond{}, errors.New("preload error: LIKE is not supported by CQL without a SASI/SAI index")
	case cqlUnsupportedIsNull.MatchString(padded):
		return scyllaCond{}, errors.New("preload error: IS NULL / IS NOT NULL is not supported by CQL in WHERE; " +
			"use a boolean column (e.g. is_deleted = ?) or filter in Go")
	case strings.ContainsAny(clause, "()"):
		return scyllaCond{}, errors.New("preload error: parentheses are not supported in CQL WHERE")
	}

	condArgs := args[1:]

	if n := strings.Count(clause, "?"); n != len(condArgs) {
		return scyllaCond{}, fmt.Errorf("preload error: condition for %q has %d placeholder(s) but %d argument(s) were provided", relation, n, len(condArgs))
	}

	return scyllaCond{clause: strings.ReplaceAll(clause, table+".", ""), args: condArgs}, nil
}

// buildChildSortKeys translates the `orderby` tag into in-memory sort keys.
// CQL only allows ORDER BY on clustering columns and requires a restricted partition key,
// so child sorting is performed in Go after the data is loaded.
func buildChildSortKeys(currentRaw string, elemType reflect.Type) ([]childSortKey, error) {
	raw := currentRaw

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	lookup := buildStructFieldMapByTag(elemType)

	var keys []childSortKey

	for _, item := range strings.Split(raw, ",") {
		tokens := strings.Fields(item)
		if len(tokens) == 0 || len(tokens) > 2 {
			return nil, fmt.Errorf("invalid order item %q, expected \"column [ASC|DESC]\"", strings.TrimSpace(item))
		}

		look, ok := lookup[strings.ToLower(tokens[0])]
		if !ok {
			return nil, fmt.Errorf("column %q was not found on struct %s", tokens[0], elemType.Name())
		}

		descending := false

		if len(tokens) == 2 {
			switch strings.ToUpper(tokens[1]) {
			case asc:
			case desc:
				descending = true
			default:
				return nil, fmt.Errorf("invalid direction %q for column %q, use ASC or DESC", tokens[1], tokens[0])
			}
		}

		keys = append(keys, childSortKey{fieldIdx: look.index, desc: descending})
	}

	return keys, nil
}

func derefForSort(val reflect.Value) (reflect.Value, bool) {
	currentVal := val

	for currentVal.Kind() == reflect.Pointer || currentVal.Kind() == reflect.Interface {
		curr := currentVal.Kind()

		if (curr == reflect.Pointer || curr == reflect.Interface) && currentVal.IsNil() {
			return reflect.Value{}, false
		}

		currentVal = currentVal.Elem()
	}

	return currentVal, currentVal.IsValid()
}

// compareSortValues: -1 / 0 / 1. Null values ​​are sorted first.
func compareTimeInCompareSortValues(goyang, joget reflect.Value) (int, bool) {
	if goyang.Type() != reflect.TypeOf(time.Time{}) {
		return 0, false
	}

	waktuGoyang, ok1 := goyang.Interface().(time.Time)

	waktuJoget, ok2 := joget.Interface().(time.Time)

	if !ok1 || !ok2 {
		return 0, true
	}

	if waktuGoyang.Before(waktuJoget) {
		return -1, true
	}

	if waktuGoyang.After(waktuJoget) {
		return 1, true
	}

	return 0, true
}

func compareSortValues(valGoy, valJoget reflect.Value) int {
	goyang, goyangOk := derefForSort(valGoy)
	joget, jogetOk := derefForSort(valJoget)

	if !goyangOk || !jogetOk {
		if goyangOk == jogetOk {
			return 0
		}

		if !goyangOk {
			return -1
		}

		return 1
	}

	if goyang.Kind() != joget.Kind() {
		return 0
	}

	if res, handled := compareTimeInCompareSortValues(goyang, joget); handled {
		return res
	}

	//nolint:exhaustive
	switch goyang.Kind() {
	case reflect.String:
		return strings.Compare(goyang.String(), joget.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return compareInt(goyang.Int(), joget.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return compareUint(goyang.Uint(), joget.Uint())
	case reflect.Float32, reflect.Float64:
		return compareFloat(goyang.Float(), joget.Float())
	case reflect.Bool:
		return compareBool(goyang.Bool(), joget.Bool())
	default:
		return 0
	}
}

func compareInt(a, b int64) int {
	if a < b {
		return -1
	}

	if a > b {
		return 1
	}

	return 0
}

func compareUint(anak, bapak uint64) int {
	if anak < bapak {
		return -1
	}

	if anak > bapak {
		return 1
	}

	return 0
}

func compareFloat(anak, bapak float64) int {
	if anak < bapak {
		return -1
	}

	if anak > bapak {
		return 1
	}

	return 0
}

//nolint:revive
func compareBool(anak, bapak bool) int {
	if !anak && bapak {
		return -1
	}

	if anak && !bapak {
		return 1
	}

	return 0
}

// mapScyllaRow maps a single MapScan row to a new struct (the same mapping rules as in findScylla apply).
func (orm *ORM) mapScyllaRow(row map[string]any, structType reflect.Type, lookup map[string]*structFieldInfo) (reflect.Value, error) {
	ptr := reflect.New(structType)
	val := ptr.Elem()

	for key, raw := range row {
		normalized := normalizeScyllaValue(raw)

		if normalized == nil {
			continue
		}

		toLower, found := lookup[strings.ToLower(key)]

		if !found {
			continue
		}

		val := val.Field(toLower.index)

		if !val.CanSet() {
			continue
		}

		if err := assignReflectValue(val, normalized); err != nil {
			message := fmt.Sprintf("Ensure the struct field type matches the ScyllaDB column type for %q", key)

			return reflect.Value{}, orm.setError(message, err)
		}
	}

	return ptr, nil
}

// buildScyllaInQuery: SELECT * FROM table WHERE col IN (?, ...) [AND cond] [ALLOW FILTERING].
func (orm *ORM) buildScyllaInQuery(table, col, currentSelectCols string, keys []any, cond scyllaCond) (string, []any) {
	selectCols := currentSelectCols

	if selectCols == "" {
		selectCols = "*"
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	cql := fmt.Sprintf("SELECT %s FROM %s WHERE %s IN (%s)", selectCols, table, col, placeholders)

	args := append(make([]any, 0), keys...)

	if cond.clause != "" {
		cql += " AND " + cond.clause

		args = append(args, cond.args...)
	}

	if orm.AllowFilteringFlag {
		cql += " ALLOW FILTERING"
	}

	return cql, args
}

func (orm *ORM) eachScyllaRow(ctx context.Context, cql string, args []any, each func(row map[string]any) error) error {
	iter, message, err := orm.Database.QueryCQL(ctx, cql, args...)
	if err != nil {
		return orm.setError(message, err)
	}

	defer func() {
		if err := iter.Close(); err != nil {
			log.Printf("failed to close iterator: %v", err) // Atau pakai logger project-mu
		}
	}()

	for {
		row := make(map[string]any)
		if !iter.MapScan(row) {
			break
		}

		if err := each(row); err != nil {
			message = "Ensure the row data processing logic and type casting inside the callback " +
				"handles the ScyllaDB column values correctly"

			return orm.setError(message, err)
		}
	}

	if err := iter.Close(); err != nil {
		const message = "ensure the related key column is the partition key (or is filterable with .AllowFiltering()) " +
			"and the condition uses CQL-supported operators"

		return orm.setError(message, err)
	}

	return nil
}

// loadScyllaPreloads dijalankan SETELAH query utama selesai.
func (orm *ORM) loadScyllaPreloads(ctx context.Context, plan *scyllaPreloadPlan, parents []reflect.Value) error {
	for _, h := range plan.hasMany {
		if err := orm.loadScyllaHasMany(ctx, parents, h); err != nil {
			return orm.Error
		}
	}

	for _, b := range plan.belongsTo {
		if err := orm.loadScyllaBelongsTo(ctx, parents, b); err != nil {
			return err
		}
	}

	return nil
}

func (orm *ORM) loadScyllaHasMany(ctx context.Context, parents []reflect.Value, hashMany *scyllaHasMany) error {
	var err error

	rel := hashMany.rel

	parentsByKey, keys := groupMongoParents(parents, rel.parentKeyIdx)

	selectCols := strings.Join(hashMany.selectCols, ", ")

	if len(keys) == 0 {
		return nil
	}

	childLookup := buildStructFieldMapByTag(rel.elemType)
	grouped := make(map[string][]reflect.Value)

	for start := 0; start < len(keys); start += scyllaInChunkSize {
		if err = orm.loadQueryScyllaHasMany(&structLoadQueryScyllaHasMany{
			ctx:         ctx,
			start:       start,
			keys:        keys,
			rel:         rel,
			hashMany:    hashMany,
			childLookup: childLookup,
			grouped:     grouped,
			selectCols:  selectCols,
		}); err != nil {
			return orm.Error
		}
	}

	if len(hashMany.sortKeys) > 0 {
		for _, items := range grouped {
			sortChildren(items, hashMany.sortKeys)
		}
	}

	for parent, parents := range parentsByKey {
		loadParentScyllaHasMany(
			grouped,
			parent,
			parents,
			rel,
		)
	}

	return nil
}

func loadParentScyllaHasMany(
	grouped map[string][]reflect.Value,
	parent string,
	parents []reflect.Value,
	rel *hasManyRelation,
) {
	items := grouped[parent]

	for _, prnt := range parents {
		field := prnt.Field(rel.fieldIndex)

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

type structLoadQueryScyllaHasMany struct {
	ctx         context.Context
	rel         *hasManyRelation
	hashMany    *scyllaHasMany
	childLookup map[string]*structFieldInfo
	grouped     map[string][]reflect.Value
	selectCols  string
	keys        []any
	start       int
}

func (orm *ORM) loadQueryScyllaHasMany(req *structLoadQueryScyllaHasMany) error {
	end := req.start + scyllaInChunkSize

	if end > len(req.keys) {
		end = len(req.keys)
	}

	cql, args := orm.buildScyllaInQuery(req.rel.table, req.rel.childFKCol, req.selectCols, req.keys[req.start:end], req.hashMany.cond)

	err := orm.eachScyllaRow(req.ctx, cql, args, func(row map[string]any) error {
		elemPtr, err := orm.mapScyllaRow(row, req.rel.elemType, req.childLookup)
		if err != nil {
			return orm.Error
		}

		elem := elemPtr.Elem()

		foreignKey, ok := derefKey(elem.Field(req.rel.childFKIdx))
		if !ok {
			return nil
		}

		printForeignKey := fmt.Sprint(foreignKey.Interface())

		if req.rel.elemIsPtr {
			req.grouped[printForeignKey] = append(req.grouped[printForeignKey], elemPtr)
		} else {
			req.grouped[printForeignKey] = append(req.grouped[printForeignKey], elem)
		}

		return nil
	})
	if err != nil {
		return orm.Error
	}

	return nil
}

func sortChildren(items []reflect.Value, keys []childSortKey) {
	structOf := func(val reflect.Value) reflect.Value {
		if val.Kind() == reflect.Pointer {
			return val.Elem()
		}

		return val
	}

	slices.SortStableFunc(items, func(goyang, joget reflect.Value) int {
		gue, loe := structOf(goyang), structOf(joget)

		for _, kita := range keys {
			kumpul := compareSortValues(gue.Field(kita.fieldIdx), loe.Field(kita.fieldIdx))

			if kumpul == 0 {
				continue
			}

			if kita.desc {
				return -kumpul
			}

			return kumpul
		}

		return 0
	})
}

func (orm *ORM) loadScyllaBelongsTo(ctx context.Context, parents []reflect.Value, belong *scyllaBelongsTo) error {
	core := belong.core

	parentsByFK, keys := groupMongoParents(parents, core.parentFKIdx)
	if len(keys) == 0 {
		return nil
	}

	relatedLookup := buildStructFieldMapByTag(core.relatedType)
	found := make(map[string]reflect.Value)

	selectCols := strings.Join(belong.selectCols, ", ")

	for start := 0; start < len(keys); start += scyllaInChunkSize {
		if err := orm.loadQueryScyllaBelongsTo(&structLoadQueryScyllaBelongsTo{
			ctx:           ctx,
			start:         start,
			keys:          keys,
			core:          core,
			belong:        belong,
			found:         found,
			relatedLookup: relatedLookup,
			selectCols:    selectCols,
		}); err != nil {
			return orm.Error
		}
	}

	for parent, parents := range parentsByFK {
		loadParentBelongsTo(found, parent, parents, core)
	}

	return nil
}

type structLoadQueryScyllaBelongsTo struct {
	ctx           context.Context
	core          *mongoBelongsTo
	belong        *scyllaBelongsTo
	found         map[string]reflect.Value
	relatedLookup map[string]*structFieldInfo
	selectCols    string
	keys          []any
	start         int
}

func (orm *ORM) loadQueryScyllaBelongsTo(req *structLoadQueryScyllaBelongsTo) error {
	end := req.start + scyllaInChunkSize

	if end > len(req.keys) {
		end = len(req.keys)
	}

	cql, args := orm.buildScyllaInQuery(req.core.collection, req.core.targetCol, req.selectCols, req.keys[req.start:end], req.belong.cond)

	err := orm.eachScyllaRow(req.ctx, cql, args, func(row map[string]any) error {
		ptr, err := orm.mapScyllaRow(row, req.core.relatedType, req.relatedLookup)
		if err != nil {
			orm.Message = "ensure that column types can be converted to struct field types"

			return orm.Error
		}

		key, ok := derefKey(ptr.Elem().Field(req.core.targetIdx))
		if !ok {
			return nil
		}

		req.found[fmt.Sprint(key.Interface())] = ptr

		return nil
	})
	if err != nil {
		return orm.Error
	}

	return nil
} //nolint:revive
