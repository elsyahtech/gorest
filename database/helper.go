package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func splitStatements(content string) []string {
	rawStatements := strings.Split(content, ";")

	statements := make([]string, 0, len(rawStatements))

	for _, statement := range rawStatements {
		if statement = strings.TrimSpace(statement); statement != "" {
			statements = append(statements, statement)
		}
	}

	return statements
}

func decodeCompensationJSON(content string, result any) error {
	decoder := json.NewDecoder(strings.NewReader(content))

	decoder.DisallowUnknownFields()

	if err := decoder.Decode(result); err != nil {
		return fmt.Errorf("decode compensation JSON: %w", err)
	}

	var trailing any

	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("compensation JSON must contain a single value")
		}

		return fmt.Errorf("decode compensation JSON: %w", err)
	}

	return nil
}
