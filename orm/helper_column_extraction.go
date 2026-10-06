package orm

import (
	"reflect"
	"strings"
)

type columnMetaData struct {
	allColumnPrimaryKeyIdx map[string]int
	columns                []string
	primaryKeyColumns      []string
	columnIndex            []int
	primaryKeyIndex        []int
}

func (orm *ORM) extractColumnMetaData(rowsVal []reflect.Value, opName string) columnMetaData {
	firstElem := rowsVal[0]
	typ := firstElem.Type()
	data := &strucTextractColumnMetaData{}

	// Normalization of SelectedCols: Supports both .Select("id, email") and .Select("id", "email")
	data = orm.normalizedSelectedCols(data)

	for idx := 0; idx < typ.NumField(); idx++ {
		columnName, isPK, isOk := findPKExtractColumnMetaData(typ, idx)
		if !isOk {
			continue
		}

		if isPK {
			data = defineDataPKExtractColumnMetaData(data, opName, columnName, rowsVal, idx)
		} else {
			// If it is not a PK, ignore it during a pure Delete operation (usually, Delete only requires the PK)
			//nolint:staticcheck
			data, isOk = defineDataNonPKExtractColumnMetaData(data, opName, columnName, idx)
			if !isOk {
				continue
			}
		}
	}

	allColFieldIdx := buildColFieldIndex(typ)

	return columnMetaData{
		columns:                data.columnsValue,
		primaryKeyColumns:      data.primaryKeyColumnsValue,
		columnIndex:            data.columnIndexValue,
		primaryKeyIndex:        data.primaryKeyIndexValue,
		allColumnPrimaryKeyIdx: allColFieldIdx,
	}
}

func buildColFieldIndex(typ reflect.Type) map[string]int {
	index := make(map[string]int)

	for idx := 0; idx < typ.NumField(); idx++ {
		field := typ.Field(idx)

		fieldType := field.Type

		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Struct {
			continue
		}

		tag := field.Tag.Get("gorest")

		if tag == "" || tag == "-" {
			continue
		}

		colName := strings.TrimSpace(strings.Split(tag, ",")[0])

		index[colName] = idx
	}

	return index
}

func defineDataPKExtractColumnMetaData(
	req *strucTextractColumnMetaData,
	opName, columnName string,
	rowsVal []reflect.Value,
	idx int,
) *strucTextractColumnMetaData {
	data := req

	data.primaryKeyIndexValue = append(data.primaryKeyIndexValue, idx)
	data.primaryKeyColumnsValue = append(data.primaryKeyColumnsValue, columnName)

	if opName == create {
		fieldVal := rowsVal[0].Field(idx)

		// If not empty (0 or ""), include in the insert column (for auto-increment or custom ID)
		if !fieldVal.IsZero() {
			data.columnsValue = append(data.columnsValue, columnName)
			data.columnIndexValue = append(data.columnIndexValue, idx)
		}
	} else {
		// For Update, Delete, or Read operations, the PK is usually required in the WHERE clause.
		data.columnsValue = append(data.columnsValue, columnName)
		data.columnIndexValue = append(data.columnIndexValue, idx)
	}

	return data
}

func defineDataNonPKExtractColumnMetaData(
	req *strucTextractColumnMetaData,
	opName, columnName string,
	idx int,
) (*strucTextractColumnMetaData, bool) {
	data := req

	// If it is not a PK, ignore it during a pure Delete operation (usually, Delete only requires the PK)
	if opName == "Delete" {
		return data, false
	}

	// Validate the SelectedCols filter (if present)
	if len(data.normalizedSelectedCols) > 0 {
		data, found := validateSelectedColsExtractColumnMetaData(data, columnName)
		if !found {
			return data, false
		}
	}

	data.columnsValue = append(data.columnsValue, columnName)
	data.columnIndexValue = append(data.columnIndexValue, idx)

	return data, true
}

func validateSelectedColsExtractColumnMetaData(req *strucTextractColumnMetaData, columnName string) (*strucTextractColumnMetaData, bool) {
	data := req

	found := false

	for _, selectedCol := range data.normalizedSelectedCols {
		if selectedCol == columnName {
			found = true

			break
		}
	}

	if !found {
		return data, false
	}

	return data, true
}

func findPKExtractColumnMetaData(typ reflect.Type, idx int) (columnName string, isPK, isOk bool) {
	field := typ.Field(idx)
	tag := field.Tag.Get("gorest")

	if tag == "" || tag == "-" {
		return "", isPK, false
	}

	parts := strings.Split(tag, ",")
	columnName = strings.TrimSpace(parts[0])

	isPK = false

	// Check if the field is a primary key
	for _, part := range parts[1:] {
		cleanedPart := strings.TrimSpace(strings.ToLower(part))

		if cleanedPart == "primary_key" {
			isPK = true

			break
		}
	}

	return columnName, isPK, true
}
