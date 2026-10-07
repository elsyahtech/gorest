package orm

import (
	"fmt"
	golog "log"

	"github.com/elsyahtech/gorest/database"
)

func (orm *ORM) execOrm(data any, opName string, sqlFn, mongoFn, scyllaFn func(any) error) *ORM {
	if orm.Database == nil || orm.DatabaseConfig == nil {
		const message = "Ensure that you have run database.Run(database.Config{...}) in your app. " +
			"To use ORM, the application must use one of the following databases: " +
			"ORACLE, SQLSERVER, MYSQL, POSTGRES, SQLITE, MONGO or SCYLLA"

		golog.Fatalf("Database is not running. Message: %s", message)

		return orm
	}

	if data == nil {
		orm.Message = fmt.Sprintf("%s: ensure the payload data passed is a struct, slice of structs (array), or pointers", opName)
		orm.Error = fmt.Errorf("%s: struct not found/nil", opName)

		return orm
	}

	driver := orm.DatabaseConfig.Driver

	switch driver {
	case database.SQLSERVER, database.MYSQL, database.POSTGRES, database.SQLITE, database.ORACLE:
		orm.Error = sqlFn(data)
	case database.MONGO:
		orm.Error = mongoFn(data)
	case database.SCYLLA:
		orm.Error = scyllaFn(data)
	default:
		orm.Error = fmt.Errorf("unsupported database driver: %s", driver)
	}

	return orm
}
