package orm

import (
	"strings"

	"github.com/elsyahtech/gorest/database"
)

func normalizeWherePlaceholders(clause string, activeDriver string, startArgCounter int) (string, int) {
	var normalized strings.Builder
	normalized.Grow(len(clause))

	counter := startArgCounter
	for idx := 0; idx < len(clause); {
		if activeDriver == database.ORACLE && idx+1 < len(clause) &&
			(clause[idx] == 'q' || clause[idx] == 'Q') && clause[idx+1] == '\'' {
			end := skipOracleQuotedString(clause, idx)
			normalized.WriteString(clause[idx:end])
			idx = end
			continue
		}

		if end, ok := skipSQLQuotedSection(clause, idx, activeDriver); ok {
			normalized.WriteString(clause[idx:end])
			idx = end
			continue
		}

		if startsSQLLineComment(clause, idx, activeDriver) {
			end := strings.IndexByte(clause[idx:], '\n')
			if end < 0 {
				normalized.WriteString(clause[idx:])
				break
			}

			end += idx
			normalized.WriteString(clause[idx:end])
			idx = end
			continue
		}

		if strings.HasPrefix(clause[idx:], "/*") {
			end := skipSQLBlockComment(clause, idx, activeDriver)
			normalized.WriteString(clause[idx:end])
			idx = end
			continue
		}

		if clause[idx] == '?' && idx+1 < len(clause) && clause[idx+1] == '?' {
			end := idx + 2
			if end < len(clause) && (clause[end] == '|' || clause[end] == '&') {
				end++
			}

			normalized.WriteByte('?')
			normalized.WriteString(clause[idx+2 : end])
			idx = end
			continue
		}

		if clause[idx] == '$' && activeDriver == database.POSTGRES {
			if delimiter, ok := postgresDollarQuoteDelimiter(clause, idx); ok {
				end := strings.Index(clause[idx+len(delimiter):], delimiter)
				if end < 0 {
					normalized.WriteString(clause[idx:])
					break
				}

				end += idx + len(delimiter)*2
				normalized.WriteString(clause[idx:end])
				idx = end
				continue
			}
		}

		if end, ok := sqlPlaceholderEnd(clause, idx); ok {
			normalized.WriteString(getPlaceholder(activeDriver, counter))
			counter++
			idx = end
			continue
		}

		normalized.WriteByte(clause[idx])
		idx++
	}

	return normalized.String(), counter
}

func skipSQLQuotedSection(clause string, start int, activeDriver string) (int, bool) {
	quote := clause[start]
	var closing byte
	switch quote {
	case '\'', '"':
		closing = quote
	case '`':
		if activeDriver != database.MYSQL && activeDriver != database.SQLITE {
			return start, false
		}
		closing = quote
	case '[':
		if activeDriver != database.SQLSERVER {
			return start, false
		}
		closing = ']'
	default:
		return start, false
	}

	backslashEscapes := activeDriver == database.MYSQL && (quote == '\'' || quote == '"')
	if activeDriver == database.POSTGRES && quote == '\'' && start > 0 &&
		(clause[start-1] == 'e' || clause[start-1] == 'E') &&
		(start == 1 || !isSQLIdentifierByte(clause[start-2])) {
		backslashEscapes = true
	}

	for idx := start + 1; idx < len(clause); idx++ {
		if backslashEscapes && clause[idx] == '\\' {
			idx++
			continue
		}

		if clause[idx] != closing {
			continue
		}

		if idx+1 < len(clause) && clause[idx+1] == closing {
			idx++
			continue
		}

		return idx + 1, true
	}

	return len(clause), true
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
		}
	}

	return len(clause)
}

func postgresDollarQuoteDelimiter(clause string, start int) (string, bool) {
	if clause[start] != '$' {
		return "", false
	}

	for idx := start + 1; idx < len(clause); idx++ {
		ch := clause[idx]
		if ch == '$' {
			return clause[start : idx+1], true
		}
		if idx == start+1 && !isSQLIdentifierStartByte(ch) {
			return "", false
		}
		if !isSQLIdentifierByte(ch) {
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
	}

	for idx := start + 3; idx+1 < len(clause); idx++ {
		if clause[idx] == closing && clause[idx+1] == '\'' {
			return idx + 2
		}
	}

	return len(clause)
}
