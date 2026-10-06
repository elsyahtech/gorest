package orm

import (
	"fmt"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// translateWhereToMongoFilter converts simple "field = ?" WHERE clauses (as used by
// the SQL side of .Where()) into a MongoDB filter document. Only the "=" operator is
// supported — each clause must be exactly "field = ?" and consume exactly one arg.
var mongoWhereEqPattern = regexp.MustCompile(`^\s*([a-zA-Z0-9_.]+)\s*=\s*\?\s*$`)

func convertWhereToMongoFilter(clauses []string, args []any) (bson.M, string, error) {
	if len(clauses) != len(args) {
		message := fmt.Sprintf("only simple `field = ?` conditions are supported for MongoDB deletes; " +
			"operators like >, <, LIKE, IN are not yet translated")

		return nil, message, fmt.Errorf(
			"delete: expected %d WHERE arg(s) for %d clause(s), got %d",
			len(clauses), len(clauses), len(args),
		)
	}

	filter := bson.M{}

	for cls, clause := range clauses {
		matches := mongoWhereEqPattern.FindStringSubmatch(clause)

		if matches == nil {
			message := fmt.Sprintf("only simple `field = ?` conditions are supported for MongoDB deletes; " +
				"operators like >, <, LIKE, IN are not yet translated")

			return nil, message, fmt.Errorf("delete: unsupported WHERE clause for MongoDB: %q", clause)
		}

		fieldName := matches[1]
		filter[fieldName] = args[cls]
	}

	return filter, "", nil
}
