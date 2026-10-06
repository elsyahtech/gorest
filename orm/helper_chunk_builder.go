package orm

import "github.com/elsyahtech/gorest/database"

func buildChunkSize(driver string, columnsCount, pkCount int) int {
	paramsPerRow := columnsCount*(pkCount+1) + pkCount
	if paramsPerRow < 1 {
		paramsPerRow = 1
	}

	// Set parameter database limitation
	paramLimit := getDriverParamLimit(driver)

	// Calculate the number of rows that fit in a single query before hitting the database parameter limit
	// Subtract a small amount (e.g., -10) as a safety margin for any additional clauses
	maxRowsByParam := (paramLimit - 10) / paramsPerRow

	if maxRowsByParam < 1 {
		maxRowsByParam = 1
	}

	// Set a maximum limit (cap) to prevent the payload/query size
	// from becoming excessively large (e.g., max 1,000 rows per batch)
	if maxRowsByParam > 1000 {
		maxRowsByParam = 1000
	}

	return maxRowsByParam
}

func getDriverParamLimit(driver string) int {
	switch driver {
	case database.SQLSERVER:
		return 2100 // Limit SQL Server
	case database.POSTGRES, database.MYSQL:
		return 65535 // Safe for Postgres/MySQL
	default:
		// SQLITE, ORACLE
		return 1000
	}
}
