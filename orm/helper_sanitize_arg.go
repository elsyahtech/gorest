package orm

import "github.com/elsyahtech/gorest/database"

func (orm *ORM) sanitizeArg(val any) any {
	if orm.DatabaseConfig.Driver == database.ORACLE {
		switch valType := val.(type) {
		case bool:
			if valType {
				return 1
			}

			return 0
		case *bool:
			if valType == nil {
				return nil
			}

			if *valType {
				return 1
			}

			return 0
		}
	}

	return val
}
