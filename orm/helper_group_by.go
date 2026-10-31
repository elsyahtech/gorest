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
func (orm *ORM) validateGroupByColumns(structType reflect.Type, tableName string) error {
	if len(orm.GroupByClauses) == 0 {
		return nil
	}

	columns, _ := taggedColumns(structType)

	knownColumns := make(map[string]string, len(columns))

	for _, column := range columns {
		knownColumns[strings.ToLower(column)] = column
	}

	var normalized []string

	for _, clause := range orm.GroupByClauses {
		for _, rawColumn := range strings.Split(clause, ",") {
			canonical, err := orm.normalizeGroupByColumn(rawColumn, tableName, structType, knownColumns)
			if err != nil {
				return err
			}

			normalized = append(normalized, canonical)
		}
	}

	orm.GroupByClauses = normalized

	return nil
}

func (orm *ORM) normalizeGroupByColumn(
	rawColumn string,
	tableName string,
	structType reflect.Type,
	knownColumns map[string]string,
) (string, error) {
	parts := strings.Split(strings.TrimSpace(rawColumn), ".")
	if len(parts) == 0 || len(parts) > 2 {
		return "", orm.invalidGroupBy(fmt.Sprintf("invalid column expression %q", rawColumn))
	}

	for _, part := range parts {
		if !isSafeOrderByIdentifier(part) {
			return "", orm.invalidGroupBy(fmt.Sprintf("invalid column expression %q", rawColumn))
		}
	}

	columnName := parts[len(parts)-1]
	if len(parts) == 2 && !strings.EqualFold(parts[0], tableName) {
		return "", orm.invalidGroupBy(fmt.Sprintf("table qualifier %q does not match %q", parts[0], tableName))
	}

	canonical, ok := knownColumns[strings.ToLower(columnName)]
	if !ok {
		return "", orm.invalidGroupBy(fmt.Sprintf("column %q is not tagged with gorest on %s", columnName, structType.Name()))
	}

	return canonical, nil
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
