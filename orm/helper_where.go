package orm

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var (
	whereIdentifier = `(?:[A-Za-z_][A-Za-z0-9_$#]*)(?:\.[A-Za-z_][A-Za-z0-9_$#]*)*`
	wherePredicate  = regexp.MustCompile(`(?i)^\s*` + whereIdentifier + `\s*(?:(?:=|<>|!=|>=|<=|>|<|LIKE|NOT\s+LIKE)\s+\?|(?:NOT\s+)?IN\s*\(\s*\?(?:\s*,\s*\?)*\s*\)|IS\s+(?:NOT\s+)?NULL)\s*$`)
	whereJoiner     = regexp.MustCompile(`(?i)\s+(?:AND|OR)\s+`)
)

func (orm *ORM) validateWhereClause(clause string) error {
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return orm.invalidWhere("condition cannot be empty")
	}

	for _, predicate := range whereJoiner.Split(clause, -1) {
		if !wherePredicate.MatchString(predicate) {
			return orm.invalidWhere(fmt.Sprintf("unsupported condition %q", predicate))
		}
	}

	return nil
}

func (orm *ORM) invalidWhere(detail string) error {
	return orm.setError(
		"Use a column comparison with '?' placeholders, IN placeholders, or IS [NOT] NULL; join conditions with AND/OR.",
		fmt.Errorf("query: invalid Where clause: %s", detail),
		http.StatusBadRequest,
	)
}
