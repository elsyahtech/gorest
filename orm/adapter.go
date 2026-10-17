package orm

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (orm *ORM) Create(data any) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Create()",
			Error:   errors.New("create: orm is not running"),
		}
	}

	return orm.execOrm(data, "Create", orm.createSQL, orm.createMongo, orm.createScylla)
}

func (orm *ORM) Update(data any) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Update()",
			Error:   errors.New("update: orm is not running"),
		}
	}

	return orm.execOrm(data, "Update", orm.updateSQL, orm.updateMongo, orm.updateScylla)
}

func (orm *ORM) Delete(data any) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Delete()",
			Error:   errors.New("delete: orm is not running"),
		}
	}

	return orm.execOrm(data, "Delete", orm.deleteSQL, orm.deleteMongo, orm.deleteScylla)
}

func (orm *ORM) Find(data any) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Find()",
			Error:   errors.New("find: orm is not running"),
		}
	}

	if err := orm.validateLimitOffset(); err != nil {
		return orm
	}

	if orm.orderByInvalid {
		return orm
	}

	if err := orm.validateSelectColumns(); err != nil {
		return orm
	}

	if len(orm.DistinctCols) > 0 {
		if len(orm.SelectedCols) > 0 {
			orm.Message = "Use either Select() or Distinct(columns), not both. Distinct(columns) selects the distinct columns."
			orm.Error = orm.setError(
				orm.Message,
				fmt.Errorf("find: Select() conflicts with Distinct(columns)"),
				http.StatusBadRequest,
			)

			return orm
		}

		orm.SelectedCols = orm.DistinctCols
	}

	return orm.execOrm(data, "Find", orm.findSQL, orm.findMongo, orm.findScylla)
}

func (orm *ORM) Select(cols ...string) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Select()",
			Error:   errors.New("select: orm is not running"),
		}
	}

	orm.SelectedCols = cols

	_ = orm.validateSelectColumns()

	return orm
}

func (orm *ORM) validateSelectColumns() error {
	if orm == nil {
		return errors.New("validateSelectColumns: orm is not running")
	}

	for _, column := range orm.SelectedCols {
		for _, expression := range strings.Split(column, ",") {
			if strings.TrimSpace(expression) == "" {
				return orm.setError(
					"Use Select('*') to select all columns, or provide one or more valid column names.",
					errors.New("invalid SELECT clause: column expression cannot be empty"),
					http.StatusBadRequest,
				)
			}
		}
	}

	return nil
}

func (orm *ORM) Where(query string, args ...any) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Where()",
			Error:   errors.New("where: orm is not running"),
		}
	}

	orm.WhereClauses = append(orm.WhereClauses, query)

	if len(args) > 0 {
		orm.WhereArgs = append(orm.WhereArgs, args...)
	}

	return orm
}

func (orm *ORM) AllowFiltering() *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call AllowFiltering()",
			Error:   errors.New("allowFiltering: orm is not running"),
		}
	}

	orm.AllowFilteringFlag = true

	return orm
}

func (orm *ORM) Distinct(cols ...string) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Distinct()",
			Error:   errors.New("distinct: orm is not running"),
		}
	}

	orm.IsDistinct = true
	orm.DistinctCols = cols

	return orm
}

func (orm *ORM) Join(clause string) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Join()",
			Error:   errors.New("join: orm is not running"),
		}
	}

	orm.JoinClauses = append(orm.JoinClauses, clause)

	return orm
}

func (orm *ORM) Limit(limit int) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Limit()",
			Error:   errors.New("limit: orm is not running"),
		}
	}

	orm.LimitVal = limit
	orm.limitSet = true
	_ = orm.validateLimitOffset()

	return orm
}

func (orm *ORM) Offset(offset int) *ORM {
	if orm == nil {
		return &ORM{
			Message: "Ensure that you have call Table() before call Offset()",
			Error:   errors.New("offset: orm is not running"),
		}
	}

	orm.OffsetVal = offset
	_ = orm.validateLimitOffset()

	return orm
}
