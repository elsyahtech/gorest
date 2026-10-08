package orm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"gopkg.in/inf.v0"
)

var (
	cqlUnsupportedOr   = regexp.MustCompile(`(?i)\sOR\s`)
	cqlUnsupportedLike = regexp.MustCompile(`(?i)\s(NOT\s+)?LIKE\s`)
)

func (orm *ORM) buildQueryStringFindScylla(columns, tableName string) {
	orm.safeWriteString("SELECT ")

	if orm.IsDistinct {
		orm.safeWriteString("DISTINCT ")
	}

	orm.safeWriteString(columns)
	orm.safeWriteString(" FROM ")
	orm.safeWriteString(tableName)
}

func buildColumnsFindScylla(column *resultBuildSelectedColumnsByTable, tableName string, preload *scyllaPreloadPlan) string {
	columns := "*"

	if !column.isSelect {
		if cols, ok := column.table[strings.ToLower(tableName)]; ok && len(cols) > 0 {
			cols = appendMissingColumns(cols, preload.requiredParentColumns())

			columns = strings.Join(cols, ", ")
		}
	}

	return columns
}

type structBuildWhereClauseFindScylla struct {
	valueArgs  []any
	whereParts []string
}

func (orm *ORM) buildWhereClauseFindScylla(currrentIsSlice bool, prefix string) ([]any, int, error) {
	data := &structBuildWhereClauseFindScylla{}
	isSlice := currrentIsSlice
	limitVal := orm.LimitVal

	var err error

	if len(orm.WhereClauses) > 0 {
		argIndex := 0

		data, argIndex, err = orm.execBuildWhereClauseFindScylla(data, prefix)
		if err != nil {
			return nil, 0, orm.Error
		}

		if argIndex != len(orm.WhereArgs) {
			return nil, 0, orm.setError(
				"Ensure the number of arguments matches the number of placeholders across all .Where() clauses.",
				errors.New("find: unused arguments provided to .Where()"),
			)
		}
	} else if !isSlice && limitVal <= 0 {
		// Single struct tanpa Where: implicit LIMIT 1
		limitVal = 1
	}

	if len(data.whereParts) > 0 {
		orm.safeWriteString(" WHERE ")
		orm.safeWriteString(strings.Join(data.whereParts, " AND "))
	}

	return data.valueArgs, limitVal, nil
}

func (orm *ORM) execBuildWhereClauseFindScylla(
	req *structBuildWhereClauseFindScylla,
	prefix string,
) (data *structBuildWhereClauseFindScylla, argIndex int, err error) {
	argIndex = 0
	data = req

	for _, clause := range orm.WhereClauses {
		if cqlUnsupportedOr.MatchString(clause) {
			return nil, 0, orm.setError(
				"CQL does not support OR in WHERE. Use IN (?) or run separate queries.",
				errors.New("find: OR is not supported by ScyllaDB"),
			)
		}

		if cqlUnsupportedLike.MatchString(clause) {
			return nil, 0, orm.setError(
				"CQL does not support LIKE without a SASI/SAI index. Use =, IN, or range operators.",
				errors.New("find: LIKE is not supported by ScyllaDB"),
			)
		}

		placeholderCount := strings.Count(clause, "?")

		if argIndex+placeholderCount > len(orm.WhereArgs) {
			return nil, 0, orm.setError(
				"Ensure the number of placeholders in .Where() matches the number of arguments provided.",
				errors.New("find: mismatched where clause placeholders and arguments"),
			)
		}

		data.whereParts = append(data.whereParts, strings.ReplaceAll(clause, prefix, ""))
		data.valueArgs = append(data.valueArgs, orm.WhereArgs[argIndex:argIndex+placeholderCount]...)

		argIndex += placeholderCount
	}

	return data, argIndex, nil
}

func (orm *ORM) bannedJoinAndOffsetFindScylla() error {
	if len(orm.HavingClauses) > 0 {
		return orm.setError(
			"ScyllaDB CQL does not support HAVING. Filter aggregate results in application code or use another query strategy. Ref: https://docs.scylladb.com/manual/stable/cql/dml/select.html",
			errors.New("find: unsupported Having clause for ScyllaDB"),
			http.StatusBadRequest,
		)
	}

	if len(orm.JoinClauses) > 0 {
		return orm.setError(
			"ScyllaDB CQL does not support JOIN; a SELECT query can read from only one table. Use Preload or another query strategy. Ref: https://docs.scylladb.com/manual/stable/cql/dml/select.html",
			errors.New("find: unsupported JOIN clause for ScyllaDB"),
			http.StatusBadRequest,
		)
	}

	if orm.OffsetVal > 0 {
		return orm.setError(
			"CQL does not support OFFSET. Use a clustering-key condition (e.g. .Where(\"created_at < ?\", last)) for pagination. Ref: https://docs.scylladb.com/manual/stable/cql/dml/select.html",
			errors.New("find: OFFSET is not supported by ScyllaDB"),
		)
	}

	return nil
}

//nolint:revive
func (orm *ORM) buildIterScanFindScylla(
	iter *gocql.Iter,
	structType reflect.Type,
	fieldLookupMap map[string]*structFieldInfo,
	isSlice bool,
	valElem *reflect.Value,
	sliceVal *reflect.Value,
) (scanErr error) {
	defer func() {
		if closeErr := iter.Close(); closeErr != nil && scanErr == nil {
			scanErr = orm.setScyllaFindError(
				"ensure ScyllaDB is reachable and the query is valid (non-key columns need .AllowFiltering())",
				closeErr,
			)
		}
	}()

	for {
		row := make(map[string]any)
		if !iter.MapScan(row) {
			break
		}

		newStructPtr := reflect.New(structType)
		newStructVal := newStructPtr.Elem()

		for key, raw := range row {
			normalized := normalizeScyllaValue(raw)
			if normalized == nil {
				continue
			}

			fieldInfo, found := fieldLookupMap[strings.ToLower(key)]
			if !found {
				continue
			}

			fieldVal := newStructVal.Field(fieldInfo.index)
			if !fieldVal.CanSet() {
				continue
			}

			if err := assignReflectValue(fieldVal, normalized); err != nil {
				return orm.setError(
					"ensure that column types can be converted to struct field types",
					fmt.Errorf("mapping error for column %q: %w", key, err),
				)
			}
		}

		orm.RowsAffected++

		if !isSlice {
			valElem.Set(newStructVal)

			break
		}

		if valElem.Type().Elem().Kind() == reflect.Pointer {
			*sliceVal = reflect.Append(*sliceVal, newStructPtr)
		} else {
			*sliceVal = reflect.Append(*sliceVal, newStructVal)
		}
	}

	if isSlice {
		valElem.Set(*sliceVal)
	}

	return nil
}

func (orm *ORM) loadFindScyllaPreloadData(execCtx context.Context, preload *scyllaPreloadPlan, valElem *reflect.Value, isSlice bool) error {
	if !preload.isEmpty() {
		parents, err := orm.collectParents(*valElem, isSlice)
		if err != nil {
			return orm.Error
		}

		if err := orm.loadScyllaPreloads(execCtx, preload, parents); err != nil { // 👈 tanpa parameter columns
			return orm.Error
		}
	}

	return nil
}

// Convert the specific gocql type to a standard Go type so it can be assigned to a struct.
func normalizeScyllaValue(val any) any {
	switch typ := val.(type) {
	case gocql.UUID:
		if typ == (gocql.UUID{}) {
			return nil
		}

		return typ.String()
	case time.Time:
		// gocql returns a zero time for NULL columns
		if typ.IsZero() {
			return nil
		}

		return typ.UTC()
	case net.IP:
		return typ.String()
	case *inf.Dec:
		if typ == nil {
			return 0.0 // or return nil if the field is a pointer (*float64)
		}

		// Convert the ScyllaDB decimal type to float64 for your Go struct
		basic, _ := typ.Unscaled() // get the base value

		decVal := float64(basic)

		for idx := 0; idx < int(typ.Scale()); idx++ {
			decVal /= 10
		}

		return decVal
	default:
		return val
	}
} //nolint:revive
