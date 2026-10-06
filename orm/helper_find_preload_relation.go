package orm

import (
	"fmt"
	"reflect"
	"strings"
)

type resultFindPreloadRelation struct {
	relatedStructType reflect.Type
	foreignKey        string
	targetKey         string
	found             bool
}

type structFindPreloadRelation struct {
	relatedStructType reflect.Type
	foundFKField      *reflect.StructField
	foundStructField  *reflect.StructField
	foreignKey        string
	targetKey         string
	found             bool
}

// findPreloadRelation detects the preload relation and extracts FK, target key, and related struct type.
// Requires EXPLICIT foreign key definition when relation struct exists.
//
// Returns error if:
// - Relation struct found but FK field not explicitly defined
// - Explicit DB tag indicates developer intent, convention fallback is not allowed.
func (orm *ORM) findPreloadRelation(structType reflect.Type, relationName string) (*resultFindPreloadRelation, error) {
	var (
		err  error
		isOk bool
	)

	data := &structFindPreloadRelation{}

	// Normalize baseName to lowercase and singular form
	baseName := strings.ToLower(pluralismNormalization(relationName))

	// Scan all fields to find FK and relation struct
	for j := 0; j < structType.NumField(); j++ {
		field := structType.Field(j)
		fieldName := strings.ToLower(field.Name)

		// Foreign Key detection (Case-Insensitive & Flexible Pattern)
		data = findFKPreloadRelation(data, baseName, fieldName, field)

		// Relation struct detection (Case-Insensitive)
		data, isOk, err = orm.findRelationStructPreloadRelation(
			data,
			baseName, relationName,
			field,
		)
		if err != nil {
			return nil, orm.Error
		}

		if !isOk {
			continue
		}

		if data.foundFKField != nil && data.foundStructField != nil {
			break
		}
	}

	data, err = orm.verifStructFieldFoundedPreloadRelation(data, relationName, structType)
	if err != nil {
		return &resultFindPreloadRelation{
			relatedStructType: data.relatedStructType,
			foreignKey:        data.foreignKey,
			targetKey:         data.targetKey,
			found:             data.found,
		}, orm.Error
	}

	return &resultFindPreloadRelation{
		relatedStructType: data.relatedStructType,
		foreignKey:        data.foreignKey,
		targetKey:         data.targetKey,
		found:             data.found,
	}, nil
}

func (orm *ORM) verifStructFieldFoundedPreloadRelation(
	req *structFindPreloadRelation,
	relationName string,
	structType reflect.Type,
) (*structFindPreloadRelation, error) {
	data := req

	if data.foundStructField != nil {
		if data.foundFKField == nil {
			message := fmt.Sprintf("ensure that a foreign key field is explicitly defined for the preload relation %q", relationName)

			data.found = false

			return data, orm.setError(message, fmt.Errorf("preload error: "+
				"relation struct %q found but foreign key field not explicitly defined on struct %s", data.foundStructField.Name, structType.Name()))
		}

		data.found = true

		return data, nil
	}

	data.found = false

	message := fmt.Sprintf("preload relation %q not found on struct %s", relationName, structType.Name())

	return data, orm.setError(message, fmt.Errorf("preload error: relation struct for %q not found on struct %s", relationName, structType.Name()))
}

func findFKPreloadRelation(req *structFindPreloadRelation, baseName, fieldName string, field reflect.StructField) *structFindPreloadRelation {
	data := req

	if data.foundFKField == nil {
		expectedFK1 := baseName + "id"
		expectedFK2 := baseName + "_id"

		if fieldName == expectedFK1 || fieldName == expectedFK2 {
			dbTag := field.Tag.Get("gorest")
			// Clean the 'gorest' tag of commas or modifiers, if present.
			parts := strings.Split(dbTag, ",")
			cleanTag := strings.TrimSpace(parts[0])

			if cleanTag != "" && cleanTag != "-" {
				data.foundFKField = &field
				data.foreignKey = cleanTag
			}
		}
	}

	return data
}

func (orm *ORM) findRelationStructPreloadRelation(req *structFindPreloadRelation, baseName, relationName string, field reflect.StructField) (
	data *structFindPreloadRelation,
	isOk bool,
	err error,
) {
	data = req

	isOk = true

	if data.foundStructField != nil {
		return data, isOk, nil
	}

	fieldBaseName := strings.ToLower(pluralismNormalization(field.Name))

	if fieldBaseName != baseName {
		isOk = false

		return data, isOk, nil
	}

	fieldType := field.Type
	if fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}

	if fieldType.Kind() != reflect.Struct {
		isOk = false

		return data, isOk, nil
	}

	data.relatedStructType = fieldType
	data.foundStructField = &field

	if pkCol, ok := getPrimaryKeyColumn(fieldType); ok {
		data.targetKey = pkCol
	}

	if refTag := field.Tag.Get("references"); refTag != "" && refTag != "-" {
		data, isOk, err = orm.refTagfindPreloadRelation(data, refTag, relationName, fieldType)
		if err != nil {
			return data, isOk, orm.Error
		}
	}

	return data, isOk, nil
}

func (orm *ORM) refTagfindPreloadRelation(
	req *structFindPreloadRelation,
	refTag, relationName string,
	fieldType reflect.Type,
) (*structFindPreloadRelation, bool, error) {
	data := req

	if trimmed := strings.TrimSpace(refTag); trimmed != "" {
		col, err := orm.findRelatedDBColumn(fieldType, trimmed, relationName)
		if err != nil {
			data.found = false

			return data, false, orm.Error
		}

		data.targetKey = col
	}

	return data, true, nil
}
