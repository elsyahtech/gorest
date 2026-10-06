package orm

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoHasMany: One-to-Many (parent punya []Child), contoh Result.Orders.
type mongoHasMany struct {
	rel   *hasManyRelation // reuse metadata dari versi SQL (references, orderby, PK parent)
	extra any              // filter tambahan dari handler (nil = tidak ada)
}

// mongoBelongsTo: One-to-One / Many-to-One (parent menyimpan FK), contoh Result.Department.
type mongoBelongsTo struct {
	relatedType  reflect.Type
	extra        any
	relationName string
	collection   string
	parentFKCol  string
	targetCol    string
	fieldIndex   int
	parentFKIdx  int
	targetIdx    int
	fieldIsPtr   bool
}

type mongoPreloadPlan struct {
	hasMany   []*mongoHasMany
	belongsTo []*mongoBelongsTo
}

func (p *mongoPreloadPlan) isEmpty() bool {
	return p == nil || (len(p.hasMany) == 0 && len(p.belongsTo) == 0)
}

// requiredParentColumns: kolom parent yang wajib ikut ter-select supaya preload bisa jalan.
func (p *mongoPreloadPlan) requiredParentColumns() []string {
	cols := make([]string, 0, len(p.hasMany)+len(p.belongsTo))

	for _, h := range p.hasMany {
		cols = append(cols, h.rel.parentKeyCol)
	}

	for _, b := range p.belongsTo {
		cols = append(cols, b.parentFKCol)
	}

	return cols
}

// planMongoPreloads memvalidasi semua Preload SEBELUM query utama dijalankan.
func (orm *ORM) planMongoPreloads(structType reflect.Type) (*mongoPreloadPlan, error) {
	plan := &mongoPreloadPlan{}

	if len(orm.Preloads) == 0 {
		return plan, nil
	}

	parentLookup := buildStructFieldMapByTag(structType)

	for relationName, args := range orm.Preloads {
		hasManyRelation, err := orm.findHasManyRelation(structType, relationName)
		if err != nil {
			return nil, orm.Error
		}

		if hasManyRelation != nil {
			childObjectIDCols := resolveTagInfo(hasManyRelation.elemType).objectIDColumns

			extra, err := orm.parseMongoPreloadFilter(relationName, args, hasManyRelation.table, childObjectIDCols)
			if err != nil {
				return nil, orm.Error
			}

			plan.hasMany = append(plan.hasMany, &mongoHasMany{rel: hasManyRelation, extra: extra})

			continue
		}

		belongTo, err := orm.planMongoBelongsTo(structType, parentLookup, relationName, args)
		if err != nil {
			return nil, orm.Error
		}

		plan.belongsTo = append(plan.belongsTo, belongTo)
	}

	return plan, nil
}

const mongoPreloadFilterHint = "Preload condition must be a string " +
	"like \"orders.deleted_at IS NULL\" (optionally with ? args) or a bson.M / bson.D filter."

func (orm *ORM) planMongoBelongsTo(
	structType reflect.Type,
	parentLookup map[string]*structFieldInfo,
	relationName string,
	args []any,
) (*mongoBelongsTo, error) {
	result, err := orm.findPreloadRelation(structType, relationName)
	if err != nil {
		return nil, orm.Error
	}

	if !result.found {
		return nil, orm.setError(
			"ensure that the preload relation exists on the main model",
			fmt.Errorf("preload error: "+
				"relation %q was not found on struct %s", relationName, structType.Name()),
		)
	}

	fieldIdx := -1

	var fieldType reflect.Type

	for idx := 0; idx < structType.NumField(); idx++ {
		field := structType.Field(idx)

		if !strings.EqualFold(field.Name, relationName) {
			continue
		}

		fieldIdx = idx
		fieldType = field.Type

		break
	}

	if fieldIdx < 0 {
		return nil, orm.setError(
			"ensure that the relation field name matches the Preload() name",
			fmt.Errorf("preload error: "+
				"field for relation %q was not found on struct %s", relationName, structType.Name()),
		)
	}

	isPtr := fieldType.Kind() == reflect.Pointer
	relatedType := fieldType

	if isPtr {
		relatedType = fieldType.Elem()
	}

	if relatedType.Kind() != reflect.Struct {
		return nil, orm.setError(
			"ensure that the relation field is a struct or a pointer to a struct",
			fmt.Errorf("preload error: "+
				"relation %q must be a struct, got %s", relationName, fieldType.String()),
		)
	}

	targetCol, err := orm.findRelatedDBColumn(relatedType, result.targetKey, relationName)
	if err != nil {
		return nil, orm.setError(
			"ensure that the references field exists on the related struct",
			fmt.Errorf("preload error: "+
				"reference %q for relation %q was not found", result.targetKey, relationName),
		)
	}

	target, targetOk := buildStructFieldMapByTag(relatedType)[strings.ToLower(targetCol)]
	if !targetOk {
		return nil, orm.setError(
			"ensure that the references field is mappable on the related struct",
			fmt.Errorf("preload error: "+
				"column %q could not be mapped on struct %s", targetCol, relatedType.Name()),
		)
	}

	forKey, forKeyOk := parentLookup[strings.ToLower(result.foreignKey)]
	if !forKeyOk {
		return nil, orm.setError(
			"ensure that the foreign key field exists on the main model",
			fmt.Errorf("preload error: "+
				"foreign key %q for relation %q was not found on struct %s", result.foreignKey, relationName, structType.Name()),
		)
	}

	collection := pluralizeForm(pluralismNormalization(relationName))
	relatedObjectIDCols := resolveTagInfo(relatedType).objectIDColumns

	extra, err := orm.parseMongoPreloadFilter(relationName, args, collection, relatedObjectIDCols)
	if err != nil {
		return nil, orm.setError(mongoPreloadFilterHint, err)
	}

	return &mongoBelongsTo{
		relationName: relationName,
		fieldIndex:   fieldIdx,
		fieldIsPtr:   isPtr,
		relatedType:  relatedType,
		collection:   collection,
		parentFKCol:  result.foreignKey,
		parentFKIdx:  forKey.index,
		targetCol:    targetCol,
		targetIdx:    target.index,
		extra:        extra,
	}, nil
}

// parseMongoPreloadFilter parses Preload(relation, args...) arguments:
//
//	Preload("Orders", "orders.deleted_at IS NULL")
//	Preload("Orders", "orders.status = ? AND orders.deleted_at IS NULL", "COMPLETED")
//	Preload("Orders", bson.M{"deleted_at": nil})
//
// Strings use the same Mongo .Where() parser (buildMongoFilter), so the handler code
// is identical to the SQL version.
func (orm *ORM) parseMongoPreloadFilter(relation string, args []any, collectionName string, objectIDCols map[string]bool) (any, error) {
	if len(args) == 0 {
		return nil, nil
	}

	switch typeArg := args[0].(type) {
	case string:
		clause := strings.TrimSpace(typeArg)

		if clause == "" {
			return nil, orm.setError(
				fmt.Sprintf("Ensure the condition string passed to Preload(%q, ...) is not empty", relation),
				fmt.Errorf("preload error: condition for Preload(%q, ...) cannot be empty", relation),
			)
		}

		filter, err := orm.buildMongoFilter([]string{clause}, args[1:], collectionName, objectIDCols)
		if err != nil {
			return nil, orm.Error
		}

		return filter, nil
	case bson.M:
		if len(args) > 1 {
			return nil, orm.setError(
				fmt.Sprintf("Ensure no extra arguments are passed when using bson.M in Preload(%q, ...)", relation),
				fmt.Errorf("preload error: Preload(%q, bson.M) does not accept extra arguments", relation),
			)
		}

		return typeArg, nil
	case bson.D:
		if len(args) > 1 {
			return nil, orm.setError(
				fmt.Sprintf("Ensure no extra arguments are passed when using bson.D in Preload(%q, ...)", relation),
				fmt.Errorf("preload error: Preload(%q, bson.D) does not accept extra arguments", relation),
			)
		}

		return typeArg, nil
	default:
		return nil, orm.setError(
			"Ensure a valid condition type is used (supported types: string, bson.M, or bson.D)",
			errors.New("preload error: unsupported condition type, use string, bson.M, or bson.D"),
		)
	}
}

func (orm *ORM) buildMongoFilter(clauses []string, args []any, collectionName string, objectIDCols map[string]bool) (bson.M, error) {
	if collectionName == "" {
		return nil, orm.setError(
			`Ensure that your call to Table("collectionName") is correct and match with your exists mongoDB.`,
			errors.New("collection name is empty"),
		)
	}

	if len(clauses) == 0 {
		if len(args) > 0 {
			return nil, orm.setError(
				"Ensure .Where() uses supported syntax: "+
					"field op ? (=, !=, <>, >, >=, <, <=, LIKE, NOT LIKE, IN, NOT IN, IS NULL, IS NOT NULL) joined by AND / OR, "+
					"without parentheses.",
				errors.New("unused arguments provided to .Where()"),
			)
		}

		return bson.M{}, nil
	}

	argIndex := 0
	andFilters := make([]bson.M, 0, len(clauses))

	for _, clause := range clauses {
		orParts := mongoOrSplit.Split(clause, -1)
		orFilters := make([]bson.M, 0, len(orParts))

		for _, orPart := range orParts {
			andParts := mongoAndSplit.Split(orPart, -1)
			conds := make([]bson.M, 0, len(andParts))

			for _, cond := range andParts {
				cnd, err := orm.parseMongoCondition(cond, args, &argIndex, collectionName, objectIDCols)
				if err != nil {
					return nil, orm.Error
				}

				conds = append(conds, cnd)
			}

			orFilters = append(orFilters, combineMongo("$and", conds))
		}

		andFilters = append(andFilters, combineMongo("$or", orFilters))
	}

	if argIndex != len(args) {
		return nil, orm.setError(
			"Ensure the number of provided arguments matches the placeholders consumed by all WHERE clauses",
			errors.New("unused arguments provided to .Where()"),
		)
	}

	return combineMongo("$and", andFilters), nil
}

func (orm *ORM) buildFieldAndBundleInparseMongoCondition(
	stringSubmatch []string,
	collectionName, cond string,
	argIndex *int,
	args []any,
) (field string, bundle string, countSbm int, err error) {
	field = mongoFieldName(stringSubmatch[1], collectionName)
	bundle = strings.Join(strings.Fields(strings.ToUpper(stringSubmatch[2])), " ")

	sbm := stringSubmatch[3]

	if bundle == isNull || bundle == iSNotNull {
		return field, bundle, countSbm, errors.New("error")
	}

	countSbm = strings.Count(sbm, "?")

	if countSbm == 0 {
		return field, bundle, countSbm, orm.setError(
			"Ensure the where condition uses a '?' placeholder for dynamic values",
			fmt.Errorf("where condition %q must use a ? placeholder", strings.TrimSpace(cond)),
		)
	}

	if *argIndex+countSbm > len(args) {
		return field, bundle, countSbm, orm.setError(
			"Ensure the number of provided arguments matches the placeholders required by the where clause",
			errors.New("mismatched where clause placeholders and arguments"),
		)
	}

	return field, bundle, countSbm, nil
}

func (orm *ORM) execParseMongoCondition(
	args []any,
	argIndex *int,
	countSbm int,
	bundle string,
	field string,
	objectIDCols map[string]bool,
) (result bson.M, err error) {
	condArgs := args[*argIndex : *argIndex+countSbm]

	*argIndex += countSbm

	switch bundle {
	case "=":
		result = bson.M{field: mongoValue(field, condArgs[0], objectIDCols)}

		return result, nil
	case "!=", "<>":
		result = bson.M{field: bson.M{"$ne": mongoValue(field, condArgs[0], objectIDCols)}}

		return result, nil
	case ">":
		result = bson.M{field: bson.M{"$gt": mongoValue(field, condArgs[0], objectIDCols)}}

		return result, nil
	case ">=":
		result = bson.M{field: bson.M{"$gte": mongoValue(field, condArgs[0], objectIDCols)}}

		return result, nil
	case "<":
		result = bson.M{field: bson.M{"$lt": mongoValue(field, condArgs[0], objectIDCols)}}

		return result, nil
	case "<=":
		result = bson.M{field: bson.M{"$lte": mongoValue(field, condArgs[0], objectIDCols)}}

		return result, nil
	case like, notLike:
		pattern, ok := condArgs[0].(string)
		if !ok {
			return nil, orm.setError(
				"Ensure the argument passed to the LIKE operator is a valid string type",
				fmt.Errorf("LIKE on field %q requires a string argument", field),
			)
		}

		toRegex := bson.Regex{Pattern: likeToRegex(pattern)}

		if bundle == notLike {
			result = bson.M{field: bson.M{"$not": toRegex}}

			return result, nil
		}

		result = bson.M{field: toRegex}

		return result, nil
	case "IN", "NOT IN":
		values := flattenMongoArgs(field, condArgs, objectIDCols)

		if bundle == "NOT IN" {
			result = bson.M{field: bson.M{"$nin": values}}

			return result, nil
		}

		//nolint:goconst
		result = bson.M{field: bson.M{"$in": values}}

		return result, nil
	default:
		return nil, fmt.Errorf("unsupported operator %q", bundle)
	}
}

//nolint:revive
func (orm *ORM) parseMongoCondition(cond string, args []any, argIndex *int, collectionName string, objectIDCols map[string]bool) (bson.M, error) {
	if collectionName == "" {
		return nil, orm.setError(
			`Ensure that your call to Table("collectionName") is correct and match with your exists mongoDB.`,
			fmt.Errorf("collection name is empty"),
		)
	}

	stringSubmatch := mongoCondExpr.FindStringSubmatch(cond)
	if stringSubmatch == nil {
		return nil, orm.setError(
			fmt.Sprintf("Ensure the where condition strictly follows the supported SQL-to-Mongo expression format, "+
				"got: %q", strings.TrimSpace(cond)),
			fmt.Errorf("unsupported where condition: %q", strings.TrimSpace(cond)),
		)
	}

	field, bundle, countSbm, err := orm.buildFieldAndBundleInparseMongoCondition(stringSubmatch, collectionName, cond, argIndex, args)
	if err != nil {
		return nil, orm.Error
	}

	return orm.execParseMongoCondition(args, argIndex, countSbm, bundle, field, objectIDCols)
}

// mongoKeyVariants: key string 24-hex dicari sebagai string DAN ObjectID,
// karena FK bisa tersimpan di salah satu bentuk itu.
func mongoKeyVariants(v any) []any {
	out := []any{v}

	if s, ok := v.(string); ok {
		if oid, err := bson.ObjectIDFromHex(s); err == nil {
			out = append(out, oid)
		}
	}

	return out
}

func andMongoFilter(base bson.M, extra any) any {
	if extra == nil {
		return base
	}

	return bson.M{"$and": []any{base, extra}}
}

func groupMongoParents(parents []reflect.Value, fieldIdx int) (map[string][]reflect.Value, []any) {
	byKey := make(map[string][]reflect.Value)

	var keys []any

	for _, parent := range parents {
		dKey, ok := derefKey(parent.Field(fieldIdx))
		if !ok {
			continue
		}

		kebelet := fmt.Sprint(dKey.Interface())

		if _, seen := byKey[kebelet]; !seen {
			keys = append(keys, dKey.Interface())
		}

		byKey[kebelet] = append(byKey[kebelet], parent)
	}

	return byKey, keys
}

// mapMongoDoc maps a single document to a new struct (rules are the same as the mapping in findMongo).
func mapMongoDoc(doc bson.M, structType reflect.Type, lookup map[string]*structFieldInfo) (reflect.Value, error) {
	ptr := reflect.New(structType)
	val := ptr.Elem()

	for key, raw := range doc {
		if raw == nil {
			continue
		}

		fi, found := lookup[strings.ToLower(key)]
		if !found {
			continue
		}

		fv := val.Field(fi.index)
		if !fv.CanSet() {
			continue
		}

		if err := assignReflectValue(fv, normalizeBSONValue(raw)); err != nil {
			return reflect.Value{}, fmt.Errorf("mapping error for field %q: %w", key, err)
		}
	}

	return ptr, nil
}

// eachMongoDoc executes a Find operation on another collection and calls each() for each document.
func (orm *ORM) eachMongoDoc(ctx context.Context, collection string, filter any, sort bson.D, each func(doc bson.M) error) error {
	coll, message, err := orm.Database.Collection(orm.DatabaseConfig, collection)
	if err != nil {
		return orm.setError(message, err)
	}

	opts := options.Find()
	if len(sort) > 0 {
		opts.SetSort(sort)
	}

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return orm.setError(message, err)
	}

	defer func() {
		if err := cursor.Close(ctx); err != nil {
			log.Print("failed to close cursor")
		}
	}()

	for cursor.Next(ctx) {
		var doc bson.M

		if err := cursor.Decode(&doc); err != nil {
			orm.Message = "ensure that the MongoDB document can be decoded"

			return orm.setError(message, err)
		}

		if err := each(doc); err != nil {
			return err
		}
	}

	if err := cursor.Err(); err != nil {
		orm.Message = "ensure that database result stream is valid and complete"

		return orm.setError(message, err)
	}

	return nil
}

// loadMongoPreloads is executed AFTER the main query completes.
func (orm *ORM) loadMongoPreloads(ctx context.Context, plan *mongoPreloadPlan, parents []reflect.Value) error {
	for _, h := range plan.hasMany {
		if err := orm.loadMongoHasMany(ctx, parents, h); err != nil {
			return orm.Error
		}
	}

	for _, b := range plan.belongsTo {
		if err := orm.loadMongoBelongsTo(ctx, parents, b); err != nil {
			return orm.Error
		}
	}

	return nil
}

func (orm *ORM) loadMongoHasMany(ctx context.Context, parents []reflect.Value, hashMany *mongoHasMany) error {
	rel := hashMany.rel

	parentsByKey, keys := groupMongoParents(parents, rel.parentKeyIdx)

	if len(keys) == 0 {
		return nil
	}

	var sortSpec bson.D

	if rel.orderBy != "" {
		str, err := orm.buildMongoSort([]string{rel.orderBy}, rel.table)
		if err != nil {
			return orm.Error
		}

		sortSpec = str
	}

	childLookup := buildStructFieldMapByTag(rel.elemType)
	grouped := make(map[string][]reflect.Value)

	var err error

	for start := 0; start < len(keys); start += hasManyChunkSize {
		grouped, err = orm.loadGroupedMongoHasMany(
			ctx,
			start,
			keys,
			rel,
			hashMany,
			sortSpec,
			childLookup,
			grouped,
		)
		if err != nil {
			return orm.Error
		}
	}

	for prnt, parents := range parentsByKey {
		loadParentMongoHasMany(grouped, prnt, parents, rel)
	}

	return nil
}

//nolint:revive
func (orm *ORM) loadGroupedMongoHasMany(
	ctx context.Context,
	start int,
	keys []any,
	rel *hasManyRelation,
	hashMany *mongoHasMany,
	sortSpec bson.D,
	childLookup map[string]*structFieldInfo,
	grouped map[string][]reflect.Value,
) (map[string][]reflect.Value, error) {
	end := start + hasManyChunkSize
	if end > len(keys) {
		end = len(keys)
	}

	variants := make([]any, 0, (end-start)*2)
	for _, k := range keys[start:end] {
		variants = append(variants, mongoKeyVariants(k)...)
	}

	filter := andMongoFilter(bson.M{rel.childFKCol: bson.M{"$in": variants}}, hashMany.extra)

	if err := orm.eachMongoDoc(ctx, rel.table, filter, sortSpec, func(doc bson.M) error {
		elemPtr, err := mapMongoDoc(doc, rel.elemType, childLookup)
		if err != nil {
			return orm.setError(
				"ensure that document field types can be converted to struct field types",
				err,
			)
		}

		elem := elemPtr.Elem()

		foreignKey, ok := derefKey(elem.Field(rel.childFKIdx))
		if !ok {
			return nil
		}

		fkInterface := fmt.Sprint(foreignKey.Interface())

		if rel.elemIsPtr {
			grouped[fkInterface] = append(grouped[fkInterface], elemPtr)
		} else {
			grouped[fkInterface] = append(grouped[fkInterface], elem)
		}

		return nil
	}); err != nil {
		return nil, orm.Error
	}

	return grouped, nil
}

func loadParentMongoHasMany(
	grouped map[string][]reflect.Value,
	prnt string,
	parents []reflect.Value,
	rel *hasManyRelation,
) {
	items := grouped[prnt]

	for _, parent := range parents {
		field := parent.Field(rel.fieldIndex)

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

func (orm *ORM) loadMongoBelongsTo(ctx context.Context, parents []reflect.Value, belong *mongoBelongsTo) error {
	parentsByFK, keys := groupMongoParents(parents, belong.parentFKIdx)
	if len(keys) == 0 {
		return nil
	}

	relatedLookup := buildStructFieldMapByTag(belong.relatedType)
	found := make(map[string]reflect.Value)

	for start := 0; start < len(keys); start += hasManyChunkSize {
		if err := orm.loadFilterMongoBelongsTo(
			ctx,
			start,
			keys,
			belong,
			relatedLookup,
			found,
		); err != nil {
			return orm.Error
		}
	}

	for parent, parents := range parentsByFK {
		loadParentBelongsTo(found, parent, parents, belong)
	}

	return nil
}

func (orm *ORM) loadFilterMongoBelongsTo(
	ctx context.Context,
	start int,
	keys []any,
	belong *mongoBelongsTo,
	relatedLookup map[string]*structFieldInfo,
	found map[string]reflect.Value,
) error {
	end := start + hasManyChunkSize
	if end > len(keys) {
		end = len(keys)
	}

	variants := make([]any, 0, (end-start)*2)
	for _, k := range keys[start:end] {
		variants = append(variants, mongoKeyVariants(k)...)
	}

	filter := andMongoFilter(bson.M{belong.targetCol: bson.M{"$in": variants}}, belong.extra)

	err := orm.eachMongoDoc(ctx, belong.collection, filter, nil, func(doc bson.M) error {
		ptr, err := mapMongoDoc(doc, belong.relatedType, relatedLookup)
		if err != nil {
			return orm.setError(
				"ensure that document field types can be converted to struct field types",
				err,
			)
		}

		key, ok := derefKey(ptr.Elem().Field(belong.targetIdx))

		if !ok {
			return nil
		}

		found[fmt.Sprint(key.Interface())] = ptr

		return nil
	})
	if err != nil {
		return orm.Error
	}

	return nil
} //nolint:revive
