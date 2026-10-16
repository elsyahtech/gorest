package orm

import "strings"

// qualifyUpsertClause adds a table prefix to columns to prevent ambiguity in
// ON CONFLICT / MERGE clauses (Postgres & SQL Server).
func (orm *ORM) upsertClause(clause, table string) string {
	last := 0
	var builder strings.Builder

	for _, loc := range upsertColRe.FindAllStringSubmatchIndex(clause, -1) {
		start, end := loc[2], loc[3] // grup 1 = nama kolom
		word := clause[start:end]

		if _, isKeyword := upsertKeywords[strings.ToUpper(word)]; isKeyword {
			continue
		}

		// Sudah berprefix (t.email)
		if start > 0 && clause[start-1] == '.' {
			continue
		}

		builder.WriteString(clause[last:start])
		builder.WriteString(table)
		builder.WriteByte('.')
		builder.WriteString(word)

		last = end
	}

	builder.WriteString(clause[last:])

	return builder.String()
}
