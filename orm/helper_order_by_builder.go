package orm

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

type orderByTerm struct {
	Column    string
	Direction string
	Qualifier string
}

// parseOrderByTerms validates ORDER BY input against the destination's gorest
// tags and returns normalized terms for each database driver's query format.
func (orm *ORM) parseOrderByTerms(structType reflect.Type, tableName string, clauses []string) ([]orderByTerm, error) {
	columns, _ := taggedColumns(structType)

	knownColumns := make(map[string]string, len(columns))

	for _, column := range columns {
		knownColumns[strings.ToLower(column)] = column
	}

	var terms []orderByTerm

	for _, rawClause := range clauses {
		var err error

		terms, err = parseOrderByTerms(
			terms,
			rawClause,
			tableName,
			knownColumns,
			structType,
		)
		if err != nil {
			return nil, orm.invalidOrderBy(err.Error())
		}
	}

	return terms, nil
}

func parseOrderByTerms(
	termsParam []orderByTerm,
	rawClause string,
	tableName string,
	knownColumns map[string]string,
	structType reflect.Type,
) ([]orderByTerm, error) {
	terms := termsParam

	for _, rawTerm := range strings.Split(rawClause, ",") {
		direction, identifierParts, columnName, err := partsOnParseOrderByTerms(rawTerm)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}

		qualifier, err := validateOrderByQualifier(identifierParts, tableName)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}

		canonicalColumn, found := knownColumns[strings.ToLower(columnName)]
		if !found {
			msg := fmt.Sprintf("ORDER BY column %q is not tagged with gorest on %s", columnName, structType.Name())

			return nil, errors.New(msg)
		}

		terms = append(terms, orderByTerm{Column: canonicalColumn, Direction: direction, Qualifier: qualifier})
	}

	return terms, nil
}

func partsOnParseOrderByTerms(rawTerm string) (direction string, identifierParts []string, columnName string, err error) {
	parts := strings.Fields(strings.TrimSpace(rawTerm))

	if len(parts) == 0 || len(parts) > 2 {
		return "", nil, "", fmt.Errorf("invalid ORDER BY expression %q", rawTerm)
	}

	direction = ""

	if len(parts) == 2 {
		direction = strings.ToUpper(parts[1])
		if direction != asc && direction != desc {
			return "", nil, "", fmt.Errorf("invalid ORDER BY direction %q", parts[1])
		}
	}

	identifierParts = strings.Split(parts[0], ".")

	columnName = identifierParts[len(identifierParts)-1]

	for _, identifier := range identifierParts {
		if !isSafeOrderByIdentifier(identifier) {
			return "", nil, "", fmt.Errorf("invalid ORDER BY column %q", parts[0])
		}
	}

	return direction, identifierParts, columnName, nil
}

func validateOrderByQualifier(identifierParts []string, tableName string) (string, error) {
	qualifier := ""

	if len(identifierParts) > 1 {
		qualifier = strings.Join(identifierParts[:len(identifierParts)-1], ".")

		if !strings.EqualFold(qualifier, tableName) {
			return "", fmt.Errorf("ORDER BY qualifier %q does not match table %q", qualifier, tableName)
		}
	}

	return qualifier, nil
}

func (orm *ORM) buildOrderByClause(structType reflect.Type, tableName string) (string, error) {
	terms, err := orm.parseOrderByTerms(structType, tableName, orm.OrderByClauses)
	if err != nil || len(terms) == 0 {
		return "", err
	}

	orderTerms := make([]string, 0, len(terms))
	for _, term := range terms {
		column := term.Column

		if term.Qualifier != "" {
			column = term.Qualifier + "." + column
		}

		if term.Direction != "" {
			column += " " + term.Direction
		}

		orderTerms = append(orderTerms, column)
	}

	return " ORDER BY " + strings.Join(orderTerms, ", "), nil
}

func isSafeOrderByIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}

	for idx, char := range identifier {
		valid := char == '_' || char == '$' || char == '#' ||
			(char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(idx > 0 && char >= '0' && char <= '9')
		if !valid {
			return false
		}
	}

	return true
}

func (orm *ORM) invalidOrderBy(detail string) error {
	return orm.setError(
		"Ensure OrderBy() uses a column tagged with gorest and an optional ASC or DESC direction.",
		fmt.Errorf("find: invalid OrderBy clause: %s", detail),
		http.StatusBadRequest,
	)
}
