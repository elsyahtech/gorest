package orm

import (
	"strings"

	"github.com/elsyahtech/gorest/database"
)

func normalizeWherePlaceholders(clause string, activeDriver string, startArgCounter int) (string, int) {
	var normalized strings.Builder

	writer := &ORM{StringBuilder: &normalized}

	normalized.Grow(len(clause))

	counter := startArgCounter

	for idx := 0; idx < len(clause); {
		if next, handled := normalizeSQLSyntaxAt(clause, idx, activeDriver, writer); handled {
			idx = next
			continue
		}

		if end, ok := sqlPlaceholderEnd(clause, idx); ok {
			writer.safeWriteString(getPlaceholder(activeDriver, counter))
			counter++
			idx = end

			continue
		}

		writer.safeWriteByte(clause[idx])
		idx++
	}

	return normalized.String(), counter
}

func normalizeSQLSyntaxAt(clause string, idx int, activeDriver string, writer *ORM) (int, bool) {
	if end, ok := oracleQuotedStringEnd(clause, idx, activeDriver); ok {
		writer.safeWriteString(clause[idx:end])

		return end, true
	}

	if end, ok := skipSQLQuotedSection(clause, idx, activeDriver); ok {
		writer.safeWriteString(clause[idx:end])

		return end, true
	}

	if end, ok := sqlLineCommentEnd(clause, idx, activeDriver); ok {
		writer.safeWriteString(clause[idx:end])

		return end, true
	}

	if strings.HasPrefix(clause[idx:], "/*") {
		end := skipSQLBlockComment(clause, idx, activeDriver)
		writer.safeWriteString(clause[idx:end])

		return end, true
	}

	if end, ok := escapedQuestionOperatorEnd(clause, idx); ok {
		writer.safeWriteByte('?')
		writer.safeWriteString(clause[idx+2 : end])

		return end, true
	}

	if end, ok := postgresDollarQuotedSectionEnd(clause, idx, activeDriver); ok {
		writer.safeWriteString(clause[idx:end])

		return end, true
	}

	return idx, false
}

func oracleQuotedStringEnd(clause string, idx int, activeDriver string) (int, bool) {
	if activeDriver != database.ORACLE || idx+1 >= len(clause) ||
		(clause[idx] != 'q' && clause[idx] != 'Q') || clause[idx+1] != '\'' {
		return idx, false
	}

	return skipOracleQuotedString(clause, idx), true
}

func sqlLineCommentEnd(clause string, idx int, activeDriver string) (int, bool) {
	if !startsSQLLineComment(clause, idx, activeDriver) {
		return idx, false
	}

	end := strings.IndexByte(clause[idx:], '\n')
	if end < 0 {
		return len(clause), true
	}

	return idx + end, true
}

func escapedQuestionOperatorEnd(clause string, idx int) (int, bool) {
	if idx+1 >= len(clause) || clause[idx] != '?' || clause[idx+1] != '?' {
		return idx, false
	}

	end := idx + 2
	if end < len(clause) && (clause[end] == '|' || clause[end] == '&') {
		end++
	}

	return end, true
}

func postgresDollarQuotedSectionEnd(clause string, start int, activeDriver string) (int, bool) {
	if activeDriver != database.POSTGRES {
		return start, false
	}

	delimiter, ok := postgresDollarQuoteDelimiter(clause, start)
	if !ok {
		return start, false
	}

	closingOffset := strings.Index(clause[start+len(delimiter):], delimiter)
	if closingOffset < 0 {
		return len(clause), true
	}

	return start + len(delimiter)*2 + closingOffset, true
}

func skipSQLQuotedSection(clause string, start int, activeDriver string) (int, bool) {
	quote, closing, ok := quoteAndClosingOnSkipSQLQuotedSection(clause, start, activeDriver)
	if !ok {
		return start, false
	}

	return scanSQLQuotedSection(clause, start, quote, closing, activeDriver), true
}

func sqlQuotedSectionUsesBackslashEscapes(clause string, start int, quote byte, activeDriver string) bool {
	if activeDriver == database.MYSQL && (quote == '\'' || quote == '"') {
		return true
	}

	return isPostgresEscapeString(clause, start, quote, activeDriver)
}

func isPostgresEscapeString(clause string, start int, quote byte, activeDriver string) bool {
	if activeDriver != database.POSTGRES || quote != '\'' || start == 0 {
		return false
	}

	if clause[start-1] != 'e' && clause[start-1] != 'E' {
		return false
	}

	return start == 1 || !isSQLIdentifierByte(clause[start-2])
}

func scanSQLQuotedSection(clause string, start int, quote, closing byte, activeDriver string) int {
	backslashEscapes := sqlQuotedSectionUsesBackslashEscapes(clause, start, quote, activeDriver)

	for idx := start + 1; idx < len(clause); idx++ {
		if backslashEscapes && clause[idx] == '\\' {
			idx++

			continue
		}

		if clause[idx] == closing && isDoubledSQLQuote(clause, idx, closing) {
			idx++

			continue
		}

		if clause[idx] == closing {
			return idx + 1
		}
	}

	return len(clause)
}

func isDoubledSQLQuote(clause string, idx int, closing byte) bool {
	return idx+1 < len(clause) && clause[idx+1] == closing
}

func quoteAndClosingOnSkipSQLQuotedSection(clause string, start int, activeDriver string) (quote byte, closing byte, isOk bool) {
	quote = clause[start]

	switch quote {
	case '\'', '"':
		closing = quote
	case '`':
		if activeDriver != database.MYSQL && activeDriver != database.SQLITE {
			return quote, closing, false
		}

		closing = quote
	case '[':
		if activeDriver != database.SQLSERVER {
			return quote, closing, false
		}

		closing = ']'
	default:
		return quote, closing, false
	}

	return quote, closing, true
}

func startsSQLLineComment(clause string, idx int, activeDriver string) bool {
	if activeDriver == database.MYSQL && clause[idx] == '#' {
		return true
	}

	if !strings.HasPrefix(clause[idx:], "--") {
		return false
	}

	if activeDriver != database.MYSQL || idx+2 == len(clause) {
		return true
	}

	next := clause[idx+2]

	return next == ' ' || next == '\t' || next == '\n' || next == '\r' || next == '\f'
}

func skipSQLBlockComment(clause string, start int, activeDriver string) int {
	depth := 1

	for idx := start + 2; idx < len(clause); idx++ {
		switch {
		case activeDriver == database.POSTGRES && strings.HasPrefix(clause[idx:], "/*"):
			depth++
			idx++
		case strings.HasPrefix(clause[idx:], "*/"):
			depth--

			idx++

			if depth == 0 {
				return idx + 1
			}
		default:
		}
	}

	return len(clause)
}

func postgresDollarQuoteDelimiter(clause string, start int) (string, bool) {
	if clause[start] != '$' {
		return "", false
	}

	for idx := start + 1; idx < len(clause); idx++ {
		cls := clause[idx]

		if cls == '$' {
			return clause[start : idx+1], true
		}

		if idx == start+1 && !isSQLIdentifierStartByte(cls) {
			return "", false
		}

		if !isSQLIdentifierByte(cls) {
			return "", false
		}
	}

	return "", false
}

func sqlPlaceholderEnd(clause string, start int) (int, bool) {
	switch clause[start] {
	case '?':
		return start + 1, true
	case '$', ':':
		idx := start + 1
		for idx < len(clause) && isASCIIDigit(clause[idx]) {
			idx++
		}

		return idx, idx > start+1
	case '@':
		if start+2 >= len(clause) || (clause[start+1] != 'p' && clause[start+1] != 'P') {
			return start, false
		}

		idx := start + 2
		for idx < len(clause) && isASCIIDigit(clause[idx]) {
			idx++
		}

		return idx, idx > start+2
	default:
		return start, false
	}
}

func isASCIIDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isSQLIdentifierByte(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || isASCIIDigit(ch)
}

func isSQLIdentifierStartByte(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func skipOracleQuotedString(clause string, start int) int {
	if start+2 >= len(clause) {
		return len(clause)
	}

	opening := clause[start+2]
	closing := opening

	switch opening {
	case '[':
		closing = ']'
	case '{':
		closing = '}'
	case '(':
		closing = ')'
	case '<':
		closing = '>'
	default:
	}

	for idx := start + 3; idx+1 < len(clause); idx++ {
		if clause[idx] == closing && clause[idx+1] == '\'' {
			return idx + 2
		}
	}

	return len(clause)
} //nolint:revive
