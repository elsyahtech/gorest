package orm

import (
	"errors"
	"fmt"
	"strings"
)

// OrderBy to specify sorting columns and direction (e.g., db.OrderBy("created_at", "DESC")).
func (orm *ORM) OrderBy(args ...string) *ORM {
	switch len(args) {
	case 0:
		return orm
	case 1:
		orm.OrderByClauses = append(orm.OrderByClauses, strings.TrimSpace(args[0]))
	case 2:
		dir := strings.ToUpper(strings.TrimSpace(args[1]))

		if dir != asc && dir != desc {
			orm.Error = fmt.Errorf("OrderBy: invalid direction %q, use ASC or DESC", args[1])
			orm.Message = "Ensure the second argument of OrderBy() is ASC or DESC."

			return orm
		}

		orm.OrderByClauses = append(orm.OrderByClauses, fmt.Sprintf("%s %s", strings.TrimSpace(args[0]), dir))
	default:
		orm.Error = errors.New("OrderBy: too many arguments, use OrderBy(\"column DESC\") or OrderBy(\"column\", \"DESC\")")
		orm.Message = "Pass at most two arguments to OrderBy()."

		return orm
	}

	return orm
}
