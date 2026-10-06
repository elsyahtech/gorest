package orm

import "regexp"

var (
	// The first argument to Upsert() is treated as a clause (rather than a column name)
	// if it contains an operator, "?", or an SQL keyword.
	upsertClauseRe = regexp.MustCompile(`(?i)(\?|!=|<>|<=|>=|=|<|>|\s(IS|LIKE|IN|AND|OR)\s)`)

	// "column = ?" -> conflict column. Other conditions (IS NULL, <, LIKE, ...) are
	// purely update criteria, not conflict targets.
	upsertEqColRe = regexp.MustCompile(`(?i)\b([A-Za-z_]\w*)\s*=\s*\?`)

	// All columns compared within the clause; used for table name qualification.
	upsertColRe = regexp.MustCompile(`(?i)\b([A-Za-z_]\w*)(\s*(?:!=|<>|<=|>=|=|<|>)|\s+(?:NOT\s+)?(?:IS|LIKE|ILIKE|IN)\b)`)

	upsertKeywords = map[string]struct{}{"NOT": {}, "AND": {}, "OR": {}}
)

func isUpsertClause(s string) bool {
	return upsertClauseRe.MatchString(s)
}
