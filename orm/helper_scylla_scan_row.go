package orm

import (
	"reflect"
	"strings"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
)

func scyllaScanRow(columns []gocql.ColumnInfo, structType reflect.Type, lookup map[string]*structFieldInfo) map[string]any {
	row := make(map[string]any)

	for _, column := range columns {
		fieldInfo, found := lookup[strings.ToLower(column.Name)]
		if !found {
			continue
		}

		fieldType := structType.Field(fieldInfo.index).Type
		if fieldType.Kind() == reflect.Pointer {
			row[column.Name] = reflect.New(fieldType).Interface()
		}
	}

	return row
}
