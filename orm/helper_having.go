package orm

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var (
	havingIdentifier = `(?:[A-Za-z_][A-Za-z0-9_$#]*)(?:\.(?:[A-Za-z_][A-Za-z0-9_$#]*))?`
	havingOperand    = `(?:COUNT\s*\(\s*(?:\*|` + havingIdentifier + `)\s*\)|(?:SUM|AVG|MIN|MAX)\s*\(\s*` + havingIdentifier + `\s*\)|` + havingIdentifier + `)`
	havingPredicate  = regexp.MustCompile(`(?i)^\s*` + havingOperand + `\s*(?:=|<>|!=|>=|>|<=|<)\s*\?\s*$`)
	havingAndSplit   = regexp.MustCompile(`(?i)\s+AND\s+`)
)

// validateHavingClause allows only simple column/aggregate comparisons with a
// bound value. Multiple predicates may be joined with AND. Keeping this grammar
// small prevents raw SQL fragments from being appended to SQL HAVING clauses.
func (orm *ORM) validateHavingClause(clauseParam string) error {
	clause := strings.TrimSpace(clauseParam)

	if clause == "" {
		return orm.invalidHaving("condition cannot be empty")
	}

	for _, predicate := range havingAndSplit.Split(clause, -1) {
		if !havingPredicate.MatchString(predicate) {
			return orm.invalidHaving(fmt.Sprintf("unsupported condition %q", predicate))
		}
	}

	return nil
}

func (orm *ORM) invalidHaving(detail string) error {
	return orm.setError(
		"Use simple column or aggregate comparisons with '?' placeholders in Having(). Join multiple conditions with AND and pass values as arguments.",
		fmt.Errorf("find: invalid Having clause: %s", detail),
		http.StatusBadRequest,
	)
}
