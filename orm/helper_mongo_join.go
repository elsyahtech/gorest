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
var mongoJoinClausePattern = regexp.MustCompile(`(?i)^\s*(?:(LEFT|INNER)\s+(?:OUTER\s+)?)?JOIN\s+([a-z_][a-z0-9_]*)\s+ON\s+([a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*)\s*=\s*([a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*)\s*$`)

func (orm *ORM) buildMongoJoinPipeline(
	table string,
	filter bson.M,
	sort bson.D,
	structType reflect.Type,
) (mongo.Pipeline, error) {
	if len(orm.Preloads) > 0 {
		return nil, orm.mongoJoinError("MongoDB Join() cannot be combined with Preload(); " +
			"use one relation-loading strategy per Find().")
	}

	if len(orm.JoinClauses) != 1 {
		return nil, orm.mongoJoinError("MongoDB Join() currently accepts one simple JOIN clause per Find().")
	}

	match := mongoJoinClausePattern.FindStringSubmatch(orm.JoinClauses[0])
	if len(match) != 5 {
		return nil, orm.mongoJoinError("MongoDB Join() supports `JOIN collection ON " +
			"collection.field = base.field` and `LEFT JOIN ...` equality clauses. Use Preload() for other relation shapes.")
	}

	joinKind, joinedCollection := strings.ToUpper(match[1]), match[2]
	leftTable, leftField := splitMongoQualifiedField(match[3])
	rightTable, rightField := splitMongoQualifiedField(match[4])
	localField, foreignField := "", ""

	switch {
	case strings.EqualFold(leftTable, table) && strings.EqualFold(rightTable, joinedCollection):
		localField, foreignField = leftField, rightField
	case strings.EqualFold(rightTable, table) && strings.EqualFold(leftTable, joinedCollection):
		localField, foreignField = rightField, leftField
	default:
		return nil, orm.mongoJoinError("MongoDB Join() ON fields must reference the base collection and " +
			"the joined collection named in the clause.")
	}

	projection, err := orm.mongoJoinProjection(table, structType)
	if err != nil {
		return nil, err
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: joinedCollection},
			{Key: "localField", Value: localField},
			{Key: "foreignField", Value: foreignField},
			{Key: "as", Value: joinedCollection},
		}}},
	}

	unwind := bson.D{{Key: "path", Value: "$" + joinedCollection}}

	if joinKind == "LEFT" {
		unwind = append(unwind, bson.E{Key: "preserveNullAndEmptyArrays", Value: true})
	}

	pipeline = append(pipeline, bson.D{{Key: "$unwind", Value: unwind}})

	if len(filter) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: filter}})
	}

	if len(sort) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$sort", Value: sort}})
	}

	pipeline = append(pipeline, bson.D{{Key: "$project", Value: projection}})

	if orm.OffsetVal > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$skip", Value: orm.OffsetVal}})
	}

	if orm.LimitVal > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: orm.LimitVal}})
	}

	return pipeline, nil
}

func (orm *ORM) mongoJoinProjection(table string, structType reflect.Type) (bson.D, error) {
	if len(orm.SelectedCols) == 0 {
		return nil, orm.mongoJoinError("MongoDB Join() requires Select() with explicit fields. " +
			"Alias duplicate field names to unique gorest tags in the result struct.")
	}

	lookup := buildStructFieldMapByTag(structType)
	projection := bson.D{{Key: "_id", Value: 0}}
	seen := map[string]struct{}{}

	for _, selection := range orm.SelectedCols {
		for _, raw := range strings.Split(selection, ",") {
			tokens := strings.Fields(raw)
			if len(tokens) == 0 || (len(tokens) != 1 && len(tokens) != 3) || (len(tokens) == 3 && !strings.EqualFold(tokens[1], "AS")) {
				return nil, orm.mongoJoinError("MongoDB Join() Select() supports `collection.field [AS result_tag]` expressions only.")
			}

			if tokens[0] == "*" || strings.HasSuffix(tokens[0], ".*") {
				return nil, orm.mongoJoinError("MongoDB Join() requires explicit selected fields; wildcard projection cannot disambiguate joined documents.")
			}

			qualified := strings.Split(tokens[0], ".")
			if len(qualified) != 2 || !mongoIdentifier.MatchString(tokens[0]) {
				return nil, orm.mongoJoinError("Qualify MongoDB Join() selected fields with their collection name, for example `users.id AS user_id`.")
			}

			output := qualified[1]
			if len(tokens) == 3 {
				output = tokens[2]
			}

			output = strings.TrimSpace(output)
			if !mongoIdentifier.MatchString(output) || strings.Contains(output, ".") {
				return nil, orm.mongoJoinError("MongoDB Join() output aliases must be simple gorest tag names.")
			}

			if _, ok := lookup[strings.ToLower(output)]; !ok {
				return nil, orm.setError(
					fmt.Sprintf("Select() output %q must match a gorest tag on the Find destination struct.", output),
					fmt.Errorf("find: MongoDB Join() destination has no gorest tag %q", output),
					http.StatusBadRequest,
				)
			}

			if _, ok := seen[strings.ToLower(output)]; ok {
				return nil, orm.mongoJoinError("MongoDB Join() selected output names must be unique; alias duplicate columns to distinct gorest tags.")
			}

			seen[strings.ToLower(output)] = struct{}{}

			path := "$" + qualified[1]

			if !strings.EqualFold(qualified[0], table) {
				path = "$" + qualified[0] + "." + qualified[1]
			}

			projection = append(projection, bson.E{Key: output, Value: path})
		}
	}

	return projection, nil
}

func splitMongoQualifiedField(field string) (res string, res2 string) {
	parts := strings.SplitN(field, ".", 2)

	return parts[0], parts[1]
}

func (orm *ORM) mongoJoinError(message string) error {
	return orm.setError(message+" Reference: https://www.mongodb.com/docs/manual/reference/operator/aggregation/lookup/", errors.New("find: unsupported MongoDB Join() shape"), http.StatusBadRequest)
}
