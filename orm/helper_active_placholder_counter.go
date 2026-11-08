package orm

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func countActivePlaceholders(clause string, activeDriver string) int {
	_, nextCounter := normalizeWherePlaceholders(clause, activeDriver, 0)

	return nextCounter
}

func countClausePlaceholders(clauses []string, activeDriver string) int {
	count := 0

	for _, clause := range clauses {
		count += countActivePlaceholders(clause, activeDriver)
	}

	return count
}

func (orm *ORM) validatePlaceholderArguments(operation, clauseName string, clauses []string, args []any, activeDriver string) error {
	expected := countClausePlaceholders(clauses, activeDriver)
	if expected == len(args) {
		return nil
	}

	return orm.setError(
		fmt.Sprintf(
			"Provide exactly one argument per placeholder in %s (expected %d, got %d).",
			clauseName,
			expected,
			len(args),
		),
		errors.New(strings.ToLower(operation)+": mismatched "+strings.ToLower(clauseName)+" placeholders and arguments"),
		http.StatusBadRequest,
	)
}
