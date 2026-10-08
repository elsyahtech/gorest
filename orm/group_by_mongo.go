package orm

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var (
	mongoGroupAggregateExpr = regexp.MustCompile(`(?i)^\s*(COUNT|SUM|AVG|MIN|MAX)\s*\(\s*(\*|[\w.]+)\s*\)\s*(?:AS\s+)?([\w]+)?\s*$`)
	mongoHavingExpr         = regexp.MustCompile(`(?i)^\s*(COUNT\s*\(\s*(?:\*|[\w.]+)\s*\)|SUM\s*\(\s*[\w.]+\s*\)|AVG\s*\(\s*[\w.]+\s*\)|MIN\s*\(\s*[\w.]+\s*\)|MAX\s*\(\s*[\w.]+\s*\)|[\w.]+)\s*(=|<>|!=|>=|>|<=|<)\s*\?\s*$`)
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
	isSlice bool,
) (mongo.Pipeline, error) {
	if len(orm.Preloads) > 0 {
		return nil, orm.setError(
			"Preload is not supported with MongoDB grouped results.",
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

	for _, col := range groupColumns {
		field := mongoFieldName(col, collection)

		groupKeys[strings.ToLower(field)] = field

		groupID[field] = "$" + field
	}

	groupDoc := bson.M{"_id": groupID}
	project := bson.M{"_id": 0}

	for field := range groupID {
		project[field] = "$_id." + field
	}

	accumulators := make(map[string]mongoGroupAccumulator)
	aliases := make(map[string]string)

	addAccumulator := func(expression, alias string) (mongoGroupAccumulator, error) {
		match := mongoGroupAggregateExpr.FindStringSubmatch(expression)

		if match == nil {
			return mongoGroupAccumulator{}, fmt.Errorf("unsupported aggregate expression %q", expression)
		}

		operation := strings.ToUpper(match[1])
		field := mongoFieldName(match[2], collection)
		identity := strings.ToLower(operation + "(" + field + ")")

		if field == "*" {
			identity = strings.ToLower(operation + "(*)")
		}

		if alias == "" {
			alias = strings.TrimSpace(match[3])
		}

		if alias == "" {
			return mongoGroupAccumulator{}, fmt.Errorf("aggregate expression %q needs an alias, e.g. COUNT(*) AS total", expression)
		}

		acc, ok := accumulators[identity]
		if !ok {
			acc = mongoGroupAccumulator{key: fmt.Sprintf("__gorest_agg_%d", len(accumulators)), operation: operation, field: field}
			accumulators[identity] = acc

			var accumulator any

			switch operation {
			case "COUNT":
				if field == "*" {
					accumulator = bson.M{"$sum": 1}
				} else {
					accumulator = bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$ne": bson.A{"$" + field, nil}}, 1, 0}}}
				}
			case "SUM":
				accumulator = bson.M{"$sum": "$" + field}
			case "AVG":
				accumulator = bson.M{"$avg": "$" + field}
			case "MIN":
				accumulator = bson.M{"$min": "$" + field}
			case "MAX":
				accumulator = bson.M{"$max": "$" + field}
			}

			groupDoc[acc.key] = accumulator
		}

		aliases[strings.ToLower(alias)] = acc.key

		project[alias] = "$" + acc.key

		return acc, nil
	}

	selected := splitMongoSelectColumns(orm.SelectedCols)

	for _, expression := range selected {
		if strings.TrimSpace(expression) == "*" {
			return nil, orm.mongoGroupInputError("Select(\"*\") is not valid for grouped MongoDB results; select group columns and aggregate expressions.", "wildcard projection with GroupBy")
		}

		if match := mongoGroupAggregateExpr.FindStringSubmatch(expression); match != nil {
			if _, err := addAccumulator(expression, match[3]); err != nil {
				return nil, orm.mongoGroupInputError(err.Error(), "invalid aggregate selection")
			}

			continue
		}

		field := mongoFieldName(expression, collection)

		if _, isGroupKey := groupKeys[strings.ToLower(field)]; !isGroupKey {
			return nil, orm.mongoGroupInputError(fmt.Sprintf("selected field %q must be grouped or aggregated", expression), "ungrouped selected field")
		}
	}

	var pipeline mongo.Pipeline

	if len(filter) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: filter}})
	}

	var havingStages mongo.Pipeline

	for idx, clause := range orm.HavingClauses {
		match := mongoHavingExpr.FindStringSubmatch(clause)

		if match == nil || idx >= len(orm.HavingArgs) {
			return nil, orm.mongoGroupInputError("MongoDB Having() supports one ? placeholder and a simple comparison per call, such as Having(\"COUNT(*) > ?\", 2).", "unsupported having expression")
		}

		expression := strings.TrimSpace(match[1])

		var target string

		if aggregate := mongoGroupAggregateExpr.FindStringSubmatch(expression); aggregate != nil {
			acc, err := addAccumulator(expression, aggregate[3])
			if err != nil {
				// Having expressions may omit an output alias; use the canonical accumulator.
				acc, err = addAccumulator(expression, "__having_"+fmt.Sprint(idx))
			}

			if err != nil {
				return nil, orm.mongoGroupInputError(err.Error(), "invalid aggregate in Having")
			}

			target = acc.key
		} else if alias, ok := aliases[strings.ToLower(expression)]; ok {
			target = alias
		} else if field, ok := groupKeys[strings.ToLower(mongoFieldName(expression, collection))]; ok {
			target = "_id." + field
		} else {
			return nil, orm.mongoGroupInputError(fmt.Sprintf("Having() field %q must be a group column or selected aggregate alias", expression), "unknown having field")
		}

		operator := map[string]string{"=": "$eq", "!=": "$ne", "<>": "$ne", ">": "$gt", ">=": "$gte", "<": "$lt", "<=": "$lte"}[match[2]]
		havingStages = append(havingStages, bson.D{{Key: "$match", Value: bson.M{target: bson.M{operator: orm.HavingArgs[idx]}}}})
	}

	if len(orm.HavingArgs) != len(orm.HavingClauses) {
		return nil, orm.mongoGroupInputError("MongoDB Having() requires exactly one argument for each condition.", "mismatched Having arguments")
	}

	pipeline = append(pipeline, bson.D{{Key: "$group", Value: groupDoc}})
	// MongoDB applies post-group filters after $group. Move the generated $match stages
	// after grouping and before the projection that exposes result aliases.
	groupStage := pipeline[len(pipeline)-1]
	preGroup := pipeline[:len(pipeline)-1]
	postGroup := make(mongo.Pipeline, 0, len(preGroup)+2)

	postGroup = append(postGroup, preGroup...)
	postGroup = append(postGroup, groupStage)
	postGroup = append(postGroup, havingStages...)
	postGroup = append(postGroup, bson.D{{Key: "$project", Value: project}})

	if len(sort) > 0 {
		postGroup = append(postGroup, bson.D{{Key: "$sort", Value: sort}})
	}

	if orm.OffsetVal > 0 {
		postGroup = append(postGroup, bson.D{{Key: "$skip", Value: int64(orm.OffsetVal)}})
	}

	limit := orm.LimitVal

	if !isSlice && limit <= 0 {
		limit = 1
	}

	if limit > 0 {
		postGroup = append(postGroup, bson.D{{Key: "$limit", Value: int64(limit)}})
	}

	return postGroup, nil
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
}
