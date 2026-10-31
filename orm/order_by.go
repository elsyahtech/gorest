package orm

import (
	"errors"
	"fmt"
	"net/http"
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
			orm.orderByInvalid = true

			//nolint:errcheck,revive
			orm.setError(
				"Ensure the OrderBy() direction is ASC or DESC.",
				fmt.Errorf("OrderBy: invalid direction %q, use ASC or DESC", args[1]),
				http.StatusBadRequest,
			)

			return orm
		}

		orm.OrderByClauses = append(orm.OrderByClauses, fmt.Sprintf("%s %s", strings.TrimSpace(args[0]), dir))
	default:
		orm.orderByInvalid = true

		//nolint:errcheck,revive
		orm.setError(
			"Pass at most two arguments to OrderBy(). Use OrderBy(\"column DESC\") or OrderBy(\"column\", \"DESC\").",
			errors.New("OrderBy: too many arguments"),
			http.StatusBadRequest,
		)

		return orm
	}

	return orm
}
