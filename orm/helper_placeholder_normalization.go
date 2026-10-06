package orm

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	universalPlaceholderPattern     = regexp.MustCompile(`\?|\$\d+|@p\d+|:\d+`)
	escapePattern                   = regexp.MustCompile(`\?\?[|&]?`)
	escapedJsonbOperatorPlaceholder = "\x00ESCAPED_JSONB_OP_%d\x00"
)

func normalizeWherePlaceholders(clause string, activeDriver string, startArgCounter int) (string, int) {
	var escapedOperators []string

	workingClause := escapePattern.ReplaceAllStringFunc(clause, func(match string) string {
		literal := strings.TrimPrefix(match, "?") // "??" -> "?", "??|" -> "?|", "??&" -> "?&"
		token := fmt.Sprintf(escapedJsonbOperatorPlaceholder, len(escapedOperators))

		escapedOperators = append(escapedOperators, literal)

		return token
	})

	counter := startArgCounter

	normalizedClause := universalPlaceholderPattern.ReplaceAllStringFunc(workingClause, func(_ string) string {
		formatted := getPlaceholder(activeDriver, counter)

		counter++

		return formatted
	})

	for i, literal := range escapedOperators {
		token := fmt.Sprintf(escapedJsonbOperatorPlaceholder, i)

		normalizedClause = strings.ReplaceAll(normalizedClause, token, literal)
	}

	return normalizedClause, counter
}
