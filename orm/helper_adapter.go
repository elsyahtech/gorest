package orm

import (
	"fmt"

	"github.com/elsyahtech/gorest/database"
)

func (orm *ORM) execOrm(data any, opName string, sqlFn, mongoFn, scyllaFn func(any) error) *ORM {
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
