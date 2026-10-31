package orm

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

//nolint:revive,lll
var safeJoinClause = regexp.MustCompile(`(?i)^\s*(?:(?:LEFT|RIGHT|FULL)(?:\s+OUTER)?|INNER|CROSS)?\s+JOIN\s+[A-Za-z_][A-Za-z0-9_$#]*(?:\.[A-Za-z_][A-Za-z0-9_$#]*)?\s+ON\s+[A-Za-z_][A-Za-z0-9_$#]*(?:\.[A-Za-z_][A-Za-z0-9_$#]*)?\s*=\s*[A-Za-z_][A-Za-z0-9_$#]*(?:\.[A-Za-z_][A-Za-z0-9_$#]*)?\s*$`)

func (orm *ORM) validateJoinClause(clause string) error {
	if !safeJoinClause.MatchString(clause) {
		return orm.setError(
			"Join() accepts a simple JOIN [table] ON table.column = table.column clause. Use a native query for complex joins.",
			fmt.Errorf("find: invalid Join clause %q", strings.TrimSpace(clause)),
			http.StatusBadRequest,
		)
	}

	return nil
}
