package orm

import (
	"fmt"

	"github.com/elsyahtech/gorest/database"
)

func getPlaceholder(activeDriver string, counter int) string {
	switch activeDriver {
	case database.POSTGRES:
		return fmt.Sprintf("$%d", counter)
	case database.SQLSERVER:
		return fmt.Sprintf("@p%d", counter)
	case database.ORACLE:
		return fmt.Sprintf(":%d", counter)
	default:
		// MYSQL, SQLITE
		return "?"
	}
}
