package orm

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

// validateGroupByColumns parses GroupBy input as a list of tagged column
// identifiers. SQL fragments, expressions, and clauses such as HAVING are
// deliberately rejected; values belong in Having() arguments instead.
func (orm *ORM) validateGroupByColumns(structType reflect.Type, tableName string) ([]string, error) {
	columns, _ := taggedColumns(structType)
	knownColumns := make(map[string]string, len(columns))
	for _, column := range columns {
		knownColumns[strings.ToLower(column)] = column
	}

	var normalized []string

	for _, clause := range orm.GroupByClauses {
		for _, rawColumn := range strings.Split(clause, ",") {
			parts := strings.Split(strings.TrimSpace(rawColumn), ".")
			if len(parts) == 0 || len(parts) > 2 {
				return nil, orm.invalidGroupBy(fmt.Sprintf("invalid column expression %q", rawColumn))
			}

			for _, part := range parts {
				if !isSafeOrderByIdentifier(part) {
					return nil, orm.invalidGroupBy(fmt.Sprintf("invalid column expression %q", rawColumn))
				}
			}

			columnName := parts[len(parts)-1]
			if len(parts) == 2 && !strings.EqualFold(parts[0], tableName) {
				return nil, orm.invalidGroupBy(fmt.Sprintf("table qualifier %q does not match %q", parts[0], tableName))
			}

			canonical, ok := knownColumns[strings.ToLower(columnName)]
			if !ok {
				return nil, orm.invalidGroupBy(fmt.Sprintf("column %q is not tagged with gorest on %s", columnName, structType.Name()))
			}

			normalized = append(normalized, canonical)
		}
	}

	return normalized, nil
}

func (orm *ORM) invalidGroupBy(detail string) error {
	return orm.setError(
		"Ensure GroupBy() contains only columns tagged with gorest. Use Having() for aggregate filters and pass values with placeholders.",
		fmt.Errorf("find: invalid GroupBy clause: %s", detail),
		http.StatusBadRequest,
	)
}

func (orm *ORM) validateGroupBySyntax(clause string) error {
	for _, rawColumn := range strings.Split(clause, ",") {
		parts := strings.Split(strings.TrimSpace(rawColumn), ".")
		if len(parts) == 0 || len(parts) > 2 {
			return orm.invalidGroupBy(fmt.Sprintf("invalid column expression %q", rawColumn))
		}
		for _, part := range parts {
			if !isSafeOrderByIdentifier(part) {
				return orm.invalidGroupBy(fmt.Sprintf("invalid column expression %q", rawColumn))
			}
		}
	}
	return nil
}
