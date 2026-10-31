package orm

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var safeTableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$#]*(?:\.[A-Za-z_][A-Za-z0-9_$#]*){0,2}$`)

func (orm *ORM) validateTableIdentifier(table string) error {
	if table != strings.TrimSpace(table) || !safeTableName.MatchString(table) {
		return orm.setError(
			"Table() accepts a table or collection identifier, optionally qualified by schema/database; use a fixed application value.",
			fmt.Errorf("query: invalid Table identifier %q", table),
			http.StatusBadRequest,
		)
	}

	return nil
}
