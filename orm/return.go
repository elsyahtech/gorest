package orm

import (
	"errors"
	"fmt"
	"strings"
)

// Return assigns the values ​​of the newly written row (INSERT/UPSERT) to the struct or slice.
//
//	Return()                        -> all tagged columns, to the input (&user / &users)
//	Return("id, created_at")        -> only those columns, to the input
//	Return(&other)                  -> all tagged columns of another struct, to &other / &others
//	Return(&other, "id, created_at") -> only those columns, to &other / &others
//
// Only the returned fields are overwritten; other fields in the destination struct remain untouched.
func (orm *ORM) Return(args ...any) *ORM {
	orm.IsReturn = true

	for _, arg := range args {
		if str, isString := arg.(string); isString {
			for _, col := range strings.Split(str, ",") {
				if col = strings.TrimSpace(col); col != "" {
					orm.ReturnCols = appendUnique(orm.ReturnCols, col)
				}
			}

			continue
		}

		if orm.ReturnDest != nil {
			orm.Message = "Return() accepts at most one destination"
			orm.Error = errors.New("return: more than one destination")

			return orm
		}

		if !isReturnDest(arg) {
			orm.Message = "Return() destination must be a pointer to a struct or to a slice of structs, e.g. Return(&user)"
			orm.Error = fmt.Errorf("return: invalid destination %T", arg)

			return orm
		}

		orm.ReturnDest = arg
	}

	return orm
}
