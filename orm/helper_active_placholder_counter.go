package orm

func countActivePlaceholders(clause string, activeDriver string) int {
	_, nextCounter := normalizeWherePlaceholders(clause, activeDriver, 0)

	return nextCounter - 0
}
