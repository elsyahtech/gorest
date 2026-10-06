package orm

import (
	"fmt"

	"github.com/elsyahtech/gorest/database"
)

func (orm *ORM) validateDBInstance(data any, targetDriver string, opName string) (string, error) {
	if data == nil {
		message := "Ensure that the payload data passed to " + opName + "() is not configured nil"

		return "", orm.setError(message, fmt.Errorf("%s: payload data is nil", opName))
	}

	if orm.Database == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app."

		return "", orm.setError(message, fmt.Errorf("%s: database service is not running", opName))
	}

	switch targetDriver {
	case database.SQLSERVER, database.MYSQL, database.POSTGRES, database.SQLITE, database.ORACLE:
		if orm.Database.SQL == nil {
			const message = "Ensure the application running must use one of the following databases: " +
				"MYSQL, POSTGRES, SQLSERVER, ORACLE, or SQLITE."

			return "", orm.setError(message, fmt.Errorf("%s: current the app running in database [%s]", opName, targetDriver))
		}
	case database.MONGO:
		if orm.Database.Mongo == nil {
			const message = "Ensure that MongoDB is properly configured and connected in your database configuration."

			return "", orm.setError(message, fmt.Errorf("%s: current the app running in database [%s]", opName, targetDriver))
		}
	case database.SCYLLA:
		if orm.Database.Scylla == nil {
			const message = "Ensure that ScyllaDB is properly configured and connected in your database configuration."

			return "", orm.setError(message, fmt.Errorf("%s: current the app running in database [%s], but Scylla connection is nil", opName, targetDriver))
		}
	default:
		message := "The application must use one of the following databases: " +
			"MySQL, PostgreSQL, SQL Server, SQLite, MongoDB, and Scylla. " +
			"Gorest does not currently support any database types other than these."

		return message, fmt.Errorf("database driver %q is not supported", targetDriver)
	}

	var tableName string

	if orm.Table == nil || *orm.Table == "" {
		message := fmt.Sprintf("Ensure that you specify the target table using .Table(\"table_name\") before calling %s().", opName)

		return "", orm.setError(message, fmt.Errorf("%s: table name is empty", opName))
	}

	tableName = *orm.Table

	return tableName, nil
}
