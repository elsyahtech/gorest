package orm

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

//nolint:revive
var (
	mongoGroupAggregateExpr = regexp.MustCompile(`(?i)^\s*(COUNT|SUM|AVG|MIN|MAX)\s*\(\s*(\*|[\w.]+)\s*\)\s*(?:AS\s+)?([\w]+)?\s*$`)
	mongoHavingExpr         = regexp.MustCompile(`(?i)^\s*(COUNT\s*\(\s*(?:\*|[\w.]+)\s*\)|SUM\s*\(\s*[\w.]+\s*\)|AVG\s*\(\s*[\w.]+\s*\)|MIN\s*\(\s*[\w.]+\s*\)|MAX\s*\(\s*[\w.]+\s*\)|[\w.]+)\s*(=|<>|!=|>=|>|<=|<)\s*\?\s*$`) //nolint:lll
)

type mongoGroupAccumulator struct {
	key       string
	operation string
	field     string
}

func (orm *ORM) buildMongoGroupPipeline(
	collection string,
	filter bson.M,
	sort bson.D,
	structType reflect.Type,
) (mongo.Pipeline, error) {
	if len(orm.Preloads) > 0 {
		return nil, orm.setError(
			"Gorest ORM does not support combining Preload() with MongoDB GroupBy() aggregation. "+
				"Run the aggregation and load related records in a separate query, or build a native pipeline. "+
				"Reference: https://www.mongodb.com/docs/manual/core/aggregation-pipeline/",
			errors.New("find: preload is unsupported with MongoDB aggregation"),
			http.StatusBadRequest,
		)
	}

	groupColumns := splitMongoSelectColumns(orm.GroupByClauses)

	if len(groupColumns) == 0 {
		return nil, orm.setError("GroupBy() requires at least one column for MongoDB aggregation.",
			errors.New("find: MongoDB aggregation requires group columns"), http.StatusBadRequest)
	}

	groupKeys := make(map[string]string, len(groupColumns))
	groupID := bson.M{}
	knownColumns := make(map[string]string)

	columns, _ := taggedColumns(structType)

	for _, column := range columns {
		knownColumns[strings.ToLower(column)] = column
	}

	for _, col := range groupColumns {
		field := mongoFieldName(col, collection)

		canonical, ok := knownColumns[strings.ToLower(field)]
		if !ok {
			return nil, orm.mongoGroupInputError(
				fmt.Sprintf("GroupBy() column %q is not tagged with gorest on %s.", col, structType.Name()),
				"unknown MongoDB group column",
			)
		}

		field = canonical

		groupKeys[strings.ToLower(field)] = field

		groupID[field] = "$" + field
	}

	groupDoc := bson.M{_id: groupID}
	project := bson.M{_id: 0}

	for field := range groupID {
		project[field] = dolarId + field
	}

	accumulators := make(map[string]mongoGroupAccumulator)
	aliases := make(map[string]string)

	selected := splitMongoSelectColumns(orm.SelectedCols)

	for _, expression := range selected {
		if strings.TrimSpace(expression) == "*" {
			return nil, orm.mongoGroupInputError(
				"Select(\"*\") is not valid for grouped MongoDB results; select group columns and aggregate expressions.",
				"wildcard projection with GroupBy",
			)
		}

		if match := mongoGroupAggregateExpr.FindStringSubmatch(expression); match != nil {
			_, err := addAccumulator(&structAddAccumulator{
				expression:   expression,
				alias:        match[3],
				collection:   collection,
				knownColumns: knownColumns,
				structType:   structType,
				accumulators: accumulators,
				groupDoc:     groupDoc,
				aliases:      aliases,
				project:      project,
			})
			if err != nil {
				return nil, orm.mongoGroupInputError(err.Error(), "invalid aggregate selection")
			}

			continue
		}

		field := mongoFieldName(expression, collection)

		if _, isGroupKey := groupKeys[strings.ToLower(field)]; !isGroupKey {
			return nil, orm.mongoGroupInputError(
				fmt.Sprintf("selected field %q must be grouped or aggregated", expression),
				"ungrouped selected field",
			)
		}
	}

	pipeline, havingStages, message, err := orm.pipelinepOnbuildMongoGroupPipeline(
		&structPipelinepOnbuildMongoGroupPipeline{
			filter:       filter,
			collection:   collection,
			knownColumns: knownColumns,
			structType:   structType,
			accumulators: accumulators,
			groupDoc:     groupDoc,
			aliases:      aliases,
			project:      project,
			groupKeys:    groupKeys,
		})
	if err != nil {
		return nil, orm.mongoGroupInputError(
			message,
			err.Error(),
		)
	}

	if len(orm.HavingArgs) != len(orm.HavingClauses) {
		return nil, orm.mongoGroupInputError(
			"MongoDB Having() requires exactly one argument for each condition.",
			"mismatched Having arguments",
		)
	}

	pipeline = append(pipeline, bson.D{{Key: "$group", Value: groupDoc}})
	// MongoDB applies post-group filters after $group. Move the generated $match stages
	// after grouping and before the projection that exposes result aliases.
	return orm.postGroupSlicePipeline(pipeline, havingStages, project, sort), nil
}

func (orm *ORM) buildMongoSingleGroupPipeline(
	collection string,
	filter bson.M,
	sort bson.D,
	structType reflect.Type,
) (mongo.Pipeline, error) {
	pipeline, err := orm.buildMongoGroupPipeline(collection, filter, sort, structType)
	if err != nil {
		return nil, err
	}

	return applyMongoSingleResultDefaultLimit(pipeline, orm.LimitVal), nil
}

func applyMongoSingleResultDefaultLimit(pipeline mongo.Pipeline, limit int) mongo.Pipeline {
	if limit > 0 {
		return pipeline
	}

	return append(pipeline, bson.D{{Key: dolarLimit, Value: int64(1)}})
}

type structAddAccumulator struct {
	structType   reflect.Type
	knownColumns map[string]string
	accumulators map[string]mongoGroupAccumulator
	groupDoc     bson.M
	aliases      map[string]string
	project      bson.M
	expression   string
	alias        string
	collection   string
}

func addAccumulator(req *structAddAccumulator) (mongoGroupAccumulator, error) {
	match := mongoGroupAggregateExpr.FindStringSubmatch(req.expression)

	if match == nil {
		return mongoGroupAccumulator{}, fmt.Errorf("unsupported aggregate expression %q", req.expression)
	}

	operation := strings.ToUpper(match[1])
	field := mongoFieldName(match[2], req.collection)

	if field != "*" {
		canonical, ok := req.knownColumns[strings.ToLower(field)]
		if !ok {
			return mongoGroupAccumulator{}, fmt.Errorf("aggregate field %q is not tagged with gorest on %s", field, req.structType.Name())
		}

		field = canonical
	}

	identity := strings.ToLower(operation + "(" + field + ")")

	if field == "*" {
		identity = strings.ToLower(operation + "(*)")
	}

	if req.alias == "" {
		req.alias = strings.TrimSpace(match[3])
	}

	if req.alias == "" {
		return mongoGroupAccumulator{}, fmt.Errorf("aggregate expression %q needs an alias, e.g. COUNT(*) AS total", req.expression)
	}

	acc, ok := req.accumulators[identity]
	if !ok {
		acc = mongoGroupAccumulator{key: fmt.Sprintf("__gorest_agg_%d", len(req.accumulators)), operation: operation, field: field}
		req.accumulators[identity] = acc

		var accumulator any

		switch operation {
		case "COUNT":
			if field == "*" {
				accumulator = bson.M{dolarSum: 1}
			} else {
				accumulator = bson.M{dolarSum: bson.M{dolarCond: bson.A{bson.M{dolarNe: bson.A{dolar + field, nil}}, 1, 0}}}
			}
		case "SUM":
			accumulator = bson.M{dolarSum: "$" + field}
		case "AVG":
			accumulator = bson.M{dolarAvg: "$" + field}
		case "MIN":
			accumulator = bson.M{dolarMin: "$" + field}
		case "MAX":
			accumulator = bson.M{dolarMax: "$" + field}
		default:
		}

		req.groupDoc[acc.key] = accumulator
	}

	req.aliases[strings.ToLower(req.alias)] = acc.key

	req.project[req.alias] = "$" + acc.key

	return acc, nil
}

type structPipelinepOnbuildMongoGroupPipeline struct {
	structType   reflect.Type
	filter       bson.M
	knownColumns map[string]string
	accumulators map[string]mongoGroupAccumulator
	groupDoc     bson.M
	aliases      map[string]string
	project      bson.M
	groupKeys    map[string]string
	collection   string
}

func (orm *ORM) pipelinepOnbuildMongoGroupPipeline(
	req *structPipelinepOnbuildMongoGroupPipeline,
) (pipeline mongo.Pipeline, havingStages mongo.Pipeline, message string, err error) {
	if len(req.filter) > 0 {
		pipeline = append(pipeline, bson.D{{Key: dolarMatch, Value: req.filter}})
	}

	for idx, clause := range orm.HavingClauses {
		match := mongoHavingExpr.FindStringSubmatch(clause)

		if match == nil || idx >= len(orm.HavingArgs) {
			const message = "MongoDB Having() supports one ? placeholder and a simple comparison per call, such as Having(\"COUNT(*) > ?\", 2)."

			return nil, nil, message, errors.New("unsupported having expression")
		}

		expression := strings.TrimSpace(match[1])

		target, isAggregate, err := mongoHavingTarget(expression, idx, req)
		if err != nil {
			if isAggregate {
				return nil, nil, err.Error(), errors.New("invalid aggregate in Having")
			}

			return nil, nil, err.Error(), errors.New("unknown having field")
		}

		operator := map[string]string{"=": dolarEq, "!=": dolarNe, "<>": dolarNe, ">": dolarGt, ">=": dolarGte, "<": dolarLt, "<=": dolarLte}[match[2]]

		havingStages = append(havingStages, bson.D{{Key: "$match", Value: bson.M{target: bson.M{operator: orm.HavingArgs[idx]}}}})
	}

	return pipeline, havingStages, "", nil
}

func mongoHavingTarget(
	expression string,
	index int,
	req *structPipelinepOnbuildMongoGroupPipeline,
) (string, bool, error) {
	if aggregate := mongoGroupAggregateExpr.FindStringSubmatch(expression); aggregate != nil {
		target, err := mongoAggregateHavingTarget(expression, index, aggregate[3], req)

		return target, true, err
	}

	if alias, ok := req.aliases[strings.ToLower(expression)]; ok {
		return alias, false, nil
	}

	fieldName := strings.ToLower(mongoFieldName(expression, req.collection))
	if field, ok := req.groupKeys[fieldName]; ok {
		return "_id." + field, false, nil
	}

	msg := fmt.Sprintf("Having() field %q must be a group column or selected aggregate alias. "+
		"Group columns: %v, aggregate aliases: %v", expression, req.groupKeys, req.aliases)

	return "", false, errors.New(msg)
}

func mongoAggregateHavingTarget(
	expression string,
	index int,
	alias string,
	req *structPipelinepOnbuildMongoGroupPipeline,
) (string, error) {
	accumulatorReq := &structAddAccumulator{
		expression:   expression,
		alias:        alias,
		collection:   req.collection,
		knownColumns: req.knownColumns,
		structType:   req.structType,
		accumulators: req.accumulators,
		groupDoc:     req.groupDoc,
		aliases:      req.aliases,
		project:      req.project,
	}

	acc, err := addAccumulator(accumulatorReq)
	if err != nil {
		// Having expressions may omit an output alias; use the canonical accumulator.
		accumulatorReq.alias = "__having_" + fmt.Sprint(index)
		acc, err = addAccumulator(accumulatorReq)
	}

	if err != nil {
		return "", err
	}

	return acc.key, nil
}

func (orm *ORM) postGroupSlicePipeline(
	pipeline mongo.Pipeline,
	havingStages mongo.Pipeline,
	project bson.M,
	sort bson.D,
) mongo.Pipeline {
	return orm.postGroupPipeline(pipeline, havingStages, project, sort, orm.LimitVal)
}

func (orm *ORM) postGroupPipeline(
	pipeline mongo.Pipeline,
	havingStages mongo.Pipeline,
	project bson.M,
	sort bson.D,
	limit int,
) mongo.Pipeline {
	groupStage := pipeline[len(pipeline)-1]
	preGroup := pipeline[:len(pipeline)-1]
	postGroup := make(mongo.Pipeline, 0, len(preGroup)+2)

	postGroup = append(postGroup, preGroup...)
	postGroup = append(postGroup, groupStage)
	postGroup = append(postGroup, havingStages...)
	postGroup = append(postGroup, bson.D{{Key: dolarProject, Value: project}})

	if len(sort) > 0 {
		postGroup = append(postGroup, bson.D{{Key: dolarSort, Value: sort}})
	}

	if orm.OffsetVal > 0 {
		postGroup = append(postGroup, bson.D{{Key: dolarSkip, Value: int64(orm.OffsetVal)}})
	}

	if limit > 0 {
		postGroup = append(postGroup, bson.D{{Key: dolarLimit, Value: int64(limit)}})
	}

	return postGroup
}

func (orm *ORM) mongoGroupInputError(message, cause string) error {
	return orm.setError(message, errors.New("find: MongoDB aggregation: "+cause), http.StatusBadRequest)
}

func splitMongoSelectColumns(columns []string) []string {
	var fields []string

	for _, column := range columns {
		for _, field := range strings.Split(column, ",") {
			if field = strings.TrimSpace(field); field != "" {
				fields = append(fields, field)
			}
		}
	}

	return fields
} //nolint:revive
