package database

import (
	"strconv"
	"strings"
)

// ===============================================================================================================
// normalizeDatabaseDriver converts various driver aliases into standardized driver constant names.
// ===============================================================================================================.
func NormalizeDatabaseDriver(driver string) string {
	normalized := strings.ToLower(strings.TrimSpace(driver))

	switch normalized {
	case MYSQL, MARIADB:
		return MYSQL
	case POSTGRES:
		return POSTGRES
	case SQLSERVER:
		return SQLSERVER
	case SQLITE:
		return SQLITE
	case ORACLE:
		return ORACLE
	case MONGO:
		return MONGO
	case SCYLLA:
		return SCYLLA
	default:
		return normalized
	}
}

func stringToInt(val string) int {
	if val == "" {
		return 0
	}

	num, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}

	return num
}
