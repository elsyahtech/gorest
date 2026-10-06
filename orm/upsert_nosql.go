package orm

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// MongoDB.

// mongoUpsertOp = a single updateOne(upsert: true) operation.
type mongoUpsertOp struct {
	filter bson.M
	update bson.M
	id     any
}

const (
	dolarne  = "$ne"
	dolarlt  = "$lt"
	dolarlte = "$lte"
	dolargt  = "$gt"
	dolargte = "$gte"
)

var (
	mongoUpsertAndRe  = regexp.MustCompile(`(?i)\s+AND\s+`)
	mongoUpsertCondRe = regexp.MustCompile(`(?i)^\s*([A-Za-z_][\w.]*)\s*(?:(!=|<>|<=|>=|=|<|>)\s*\?|(IS\s+NOT\s+NULL|IS\s+NULL))\s*$`)
	mongoUpsertCmpOps = map[string]string{
		"!=": dolarne,
		"<>": dolarne,
		"<":  dolarlt,
		"<=": dolarlte,
		">":  dolargt,
		">=": dolargte,
	}
)

// translateUpsertClausesToMongo translates a simple subset of SQL into BSON filter conditions.
// Supported: field = ?, !=, <>, <, <=, >, >=, IS NULL, IS NOT NULL, combined with AND.
func translateUpsertClausesToMongo(clauses []string, args []any) ([]bson.M, error) {
	var (
		conds  []bson.M
		argIdx int
	)

	for _, clause := range clauses {
		for _, part := range mongoUpsertAndRe.Split(clause, -1) {
			partStr := mongoUpsertCondRe.FindStringSubmatch(part)
			if partStr == nil {
				return nil, fmt.Errorf(
					"unsupported condition %q (MongoDB upsert supports: field = ?, !=, <, <=, >, >=, IS NULL, IS NOT NULL joined by AND)",
					strings.TrimSpace(part),
				)
			}

			field := partStr[1]

			if partStr[3] != "" {
				if strings.Contains(strings.ToUpper(partStr[3]), "NOT") {
					conds = append(conds, bson.M{field: bson.M{dolarne: nil}})
				} else {
					conds = append(conds, bson.M{field: nil})
				}

				continue
			}

			if argIdx >= len(args) {
				return nil, errors.New("not enough arguments for the placeholders in the Upsert() clause")
			}

			val := args[argIdx]
			argIdx++

			if partStr[2] == "=" {
				conds = append(conds, bson.M{field: val})
			} else {
				conds = append(conds, bson.M{field: bson.M{mongoUpsertCmpOps[partStr[2]]: val}})
			}
		}
	}

	if argIdx != len(args) {
		return nil, fmt.Errorf("upsert() clause consumed %d argument(s) but %d were provided", argIdx, len(args))
	}

	return conds, nil
}

// buildMongoUpsertOps constructs the filter and update for each document (mapping columns to values ​​from createMongo).
func (orm *ORM) buildMongoUpsertOps(rowsVal []any) ([]mongoUpsertOp, error) {
	conds, err := translateUpsertClausesToMongo(orm.UpsertClauses, orm.UpsertArgs)
	if err != nil {
		const message = "Use Upsert(\"field\") or simple conditions such as Upsert(\"email = ? AND deleted_at IS NULL\", value) on MongoDB."

		return nil, orm.setError(message, fmt.Errorf("create: %w", err))
	}

	ops := make([]mongoUpsertOp, 0, len(rowsVal))

	for _, raw := range rowsVal {
		doc, ok := raw.(map[string]any)
		if !ok {
			return nil, orm.setError("unexpected document type for upsert", errors.New("create: upsert document is not a map"))
		}

		op, err := orm.buildMongoUpsertOp(doc, conds)
		if err != nil {
			return nil, err
		}

		ops = append(ops, op)
	}

	return ops, nil
}

func (orm *ORM) buildMongoUpsertOp(doc map[string]any, conds []bson.M) (mongoUpsertOp, error) {
	notFound := func(col string) error {
		message := fmt.Sprintf("Upsert column %q was not found in the struct tags (or it was filtered out by Select()).", col)

		return orm.setError(message, fmt.Errorf("create: upsert column %q not found in document", col))
	}

	// 1. Conflict columns -> actual key names in the document
	conflictKeys, err := orm.buildConflictKeysInbuildMongoUpsertOp(doc, notFound)
	if err != nil {
		return mongoUpsertOp{}, orm.Error
	}

	conflictSet, filter := buildConflictSetInbuildMongoUpsertOp(conflictKeys, doc, conds)

	// 2. Columns to be updated (optional, from Insert Update Cos)
	explicit := len(orm.UpsertUpdateCols) > 0

	updateSet, err := orm.updateColumnsInBuildMongoUpsertOp(doc, notFound)
	if err != nil {
		return mongoUpsertOp{}, orm.setError("_id cannot be updated in MongoDB.", errors.New("create: upsert update column _id"))
	}

	// 3. Separate $set (used during matching) and $setOnInsert (only when a new document is created)
	setDoc, setOnInsert, docID := setDocInbuildMongoUpsertOp(doc, conflictSet, updateSet, explicit)

	update := bson.M{}

	if len(setDoc) > 0 {
		update["$set"] = setDoc
	}

	if len(setOnInsert) > 0 {
		update["$setOnInsert"] = setOnInsert
	}

	// Document only contains conflict columns: update cannot be empty -> insert-if-not-exists.
	if len(update) == 0 {
		update["$setOnInsert"] = bson.M{conflictKeys[0]: doc[conflictKeys[0]]}
	}

	return mongoUpsertOp{filter: filter, update: update, id: docID}, nil
}

func (orm *ORM) buildConflictKeysInbuildMongoUpsertOp(doc map[string]any, notFound func(col string) error) ([]string, error) {
	var conflictKeys []string

	if len(orm.UpsertConflictCols) == 0 {
		key, ok := lookupDocKey(doc, "_id")
		if !ok || isZeroValue(doc[key]) {
			return nil, orm.setError(
				"Upsert() without columns uses the primary key (_id), so _id must be set on the struct. Or use Upsert(\"field\").",
				errors.New("create: Upsert without conflict columns and empty _id"),
			)
		}

		conflictKeys = []string{key}
	} else {
		for _, col := range orm.UpsertConflictCols {
			key, ok := lookupDocKey(doc, col)
			if !ok {
				return nil, notFound(col)
			}

			conflictKeys = append(conflictKeys, key)
		}
	}

	return conflictKeys, nil
}

func buildConflictSetInbuildMongoUpsertOp(conflictKeys []string, doc map[string]any, conds []bson.M) (map[string]struct{}, bson.M) {
	conflictSet := make(map[string]struct{}, len(conflictKeys))
	filter := bson.M{}

	for _, key := range conflictKeys {
		conflictSet[key] = struct{}{}
		filter[key] = doc[key]
	}

	// Additional conditions (from the clause) are added to $and to avoid conflicts with the keys above.
	// Unlike SQL: if the conditions don't match, Mongo attempts an INSERT (which may fail due to a duplicate key) rather than skipping the update.
	if len(conds) > 0 {
		and := make(bson.A, 0, len(conds))

		for _, cond := range conds {
			and = append(and, cond)
		}

		filter["$and"] = and
	}

	return conflictSet, filter
}

func (orm *ORM) updateColumnsInBuildMongoUpsertOp(doc map[string]any, notFound func(col string) error) (map[string]struct{}, error) {
	updateSet := make(map[string]struct{}, len(orm.UpsertUpdateCols))

	for _, col := range orm.UpsertUpdateCols {
		key, ok := lookupDocKey(doc, col)
		if !ok {
			return nil, notFound(col)
		}

		const _id = "_id"

		if key == _id {
			return nil, orm.setError("_id cannot be updated in MongoDB.", errors.New("create: upsert update column _id"))
		}

		updateSet[key] = struct{}{}
	}

	return updateSet, nil
}

//nolint:revive
func setDocInbuildMongoUpsertOp(
	doc map[string]any,
	conflictSet map[string]struct{},
	updateSet map[string]struct{},
	explicit bool,
) (setDoc bson.M, setOnInsert bson.M, docID any) {
	setDoc = bson.M{}
	setOnInsert = bson.M{}

	for key, val := range doc {
		if _, isConflict := conflictSet[key]; isConflict {
			continue // handled by the filter during insertion
		}

		if key == "_id" {
			if !isZeroValue(val) {
				setOnInsert["_id"] = val
				docID = val
			}

			continue
		}

		if _, isUpdate := updateSet[key]; explicit && !isUpdate {
			setOnInsert[key] = val

			continue
		}

		setDoc[key] = val
	}

	if _, ok := conflictSet["_id"]; ok {
		docID = doc["_id"]
	}

	return setDoc, setOnInsert, docID
}

func lookupDocKey(doc map[string]any, name string) (string, bool) {
	if _, ok := doc[name]; ok {
		return name, true
	}

	for key := range doc {
		if strings.EqualFold(key, name) {
			return key, true
		}
	}

	return "", false
}

func isZeroValue(val any) bool {
	if val == nil {
		return true
	}

	return reflect.ValueOf(val).IsZero()
}

// ScyllaDB.

// prepareScyllaUpsert validates Upsert() for ScyllaDB.
// INSERT in CQL already behaves as an upsert on the primary key, so no additional syntax is required.
func (orm *ORM) prepareScyllaUpsert(meta columnMetaData) error {
	if len(orm.UpsertClauses) > 0 {
		return orm.setError(
			"conditional upsert is not supported on ScyllaDB (it would require lightweight transactions)",
			errors.New("create: Upsert clause is not supported on Scylla"),
		)
	}

	// The CQL conflict target is always the full primary key, so the provided columns must match it.
	if len(orm.UpsertConflictCols) > 0 {
		if err := orm.validateColumnNamesUpsert(orm.UpsertConflictCols, meta.allColumnPrimaryKeyIdx, "UpsertConflictCols"); err != nil {
			return err
		}

		matches := len(orm.UpsertConflictCols) == len(meta.primaryKeyColumns)

		for _, col := range orm.UpsertConflictCols {
			matches = matches && containsFold(meta.primaryKeyColumns, col)
		}

		if !matches {
			message := fmt.Sprintf(
				"ScyllaDB upsert always conflicts on the full primary key (%s). Use Upsert() without arguments or list exactly those columns.",
				strings.Join(meta.primaryKeyColumns, ", "),
			)

			return orm.setError(message, errors.New("create: Scylla upsert columns must equal the primary key"))
		}
	}

	if len(orm.UpsertUpdateCols) > 0 {
		if err := orm.validateColumnNamesUpsert(orm.UpsertUpdateCols, meta.allColumnPrimaryKeyIdx, "UpsertUpdateCols"); err != nil {
			return err
		}

		for _, col := range orm.UpsertUpdateCols {
			if containsFold(meta.primaryKeyColumns, col) {
				return orm.setError(
					fmt.Sprintf("primary key column %q cannot be updated in ScyllaDB.", col),
					fmt.Errorf("create: upsert update column %q is part of the primary key", col),
				)
			}

			if !containsFold(meta.columns, col) {
				return orm.setError(
					fmt.Sprintf("upsert update column %q is not part of the columns being written (check Select()).", col),
					fmt.Errorf("create: upsert update column %q is not in the write columns", col),
				)
			}
		}
	}

	return nil
}

// buildScyllaUpsertUpdate: UPDATE ... SET (selected columns) WHERE (primary key). Used when UpsertUpdateCols is populated,
// because INSERT overwrites all columns. In CQL, UPDATE also functions as an upsert: the row is created if it does not yet exist.
func buildScyllaUpsertUpdate(tableName string, rowVal reflect.Value, meta columnMetaData, updateCols []string) (string, []any) {
	fieldIdx := make(map[string]int, len(meta.columns))

	for i, col := range meta.columns {
		fieldIdx[strings.ToLower(col)] = meta.columnIndex[i]
	}

	sets := make([]string, 0, len(updateCols))
	wheres := make([]string, 0, len(meta.primaryKeyColumns))
	args := make([]any, 0, len(updateCols)+len(meta.primaryKeyColumns))

	for _, col := range updateCols {
		sets = append(sets, col+" = ?")
		args = append(args, rowVal.Field(fieldIdx[strings.ToLower(col)]).Interface())
	}

	for i, pk := range meta.primaryKeyColumns {
		wheres = append(wheres, pk+" = ?")
		args = append(args, rowVal.Field(meta.primaryKeyIndex[i]).Interface())
	}

	return fmt.Sprintf("UPDATE %s SET %s WHERE %s", tableName, strings.Join(sets, ", "), strings.Join(wheres, " AND ")), args
} //nolint:revive
