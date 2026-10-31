package orm

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var (
	selectIdentifier = `[A-Za-z_][A-Za-z0-9_$#]*(?:\.[A-Za-z_][A-Za-z0-9_$#]*)*`
	selectColumn     = regexp.MustCompile(`(?i)^\s*(?:\*|` + selectIdentifier + `)(?:\s+AS\s+[A-Za-z_][A-Za-z0-9_$#]*)?\s*$`)
	selectWildcard   = regexp.MustCompile(`(?i)^\s*` + selectIdentifier + `\.\*\s*$`)
	selectAggregate  = regexp.MustCompile(`(?i)^\s*(?:COUNT\s*\(\s*(?:\*|` + selectIdentifier + `)\s*\)|(?:SUM|AVG|MIN|MAX)\s*\(\s*` + selectIdentifier + `\s*\))(?:\s+AS\s+[A-Za-z_][A-Za-z0-9_$#]*)?\s*$`) //nolint:revive,lll
)

func (orm *ORM) validateSelectExpressions(columns []string) error {
	for _, entry := range columns {
		for _, expression := range strings.Split(entry, ",") {
			expression = strings.TrimSpace(expression)
			if expression == "" ||
				(!selectColumn.MatchString(expression) &&
					!selectWildcard.MatchString(expression) &&
					!selectAggregate.MatchString(expression)) {
				return orm.setError(
					"Select() accepts column identifiers and supported aggregate expressions with an optional AS alias.",
					fmt.Errorf("find: invalid Select expression %q", expression),
					http.StatusBadRequest,
				)
			}
		}
	}

	return nil
}
