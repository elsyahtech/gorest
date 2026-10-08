package orm

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/elsyahtech/gorest/database"
)

type fieldRequirement struct {
	requirePrimaryKey bool
	requireNonEmpty   bool
}

func (orm *ORM) validateData(data any, fieldReq fieldRequirement, opName string) ([]reflect.Value, error) {
	if err := orm.validateStructTags(data, fieldReq, opName); err != nil {
		return nil, orm.Error
	}

	valElem, err := orm.extractReflectionValue(data, opName)
	if err != nil {
		return nil, orm.Error
	}

	rowsVal, err := orm.normalizeStructInput(*valElem, opName)
	if err != nil {
		return nil, orm.Error
	}

	return rowsVal, nil
}

func (orm *ORM) validateDataMongo(data any, fieldReq fieldRequirement, opName string) ([]any, error) {
	if err := orm.validateStructTags(data, fieldReq, opName); err != nil {
		return nil, orm.Error
	}

	valElem, err := orm.extractReflectionValue(data, opName)
	if err != nil {
		return nil, orm.Error
	}

	rowsVal, err := orm.normalizeStructInputMongo(*valElem, opName)
	if err != nil {
		return nil, orm.Error
	}

	return rowsVal, nil
}

func (orm *ORM) validateStructTags(data any, structRequirement fieldRequirement, opName string) error {
	activeDriver := orm.DatabaseConfig.Driver

	val, err := orm.buildValueValidateStructTags(data, opName)
	if err != nil {
		return orm.Error
	}

	var structType reflect.Type

	//nolint:exhaustive
	switch val.Kind() {
	case reflect.Slice:
		if err := orm.verifFieldRequirementValidateStructTags(structRequirement, val, opName); err != nil {
			return orm.Error
		}

		structType, err = orm.buildStructTypeByElemInValidateStructTags(val, opName)
		if err != nil {
			return orm.Error
		}
	case reflect.Struct:
		structType = val.Type()

	default:
		message := fmt.Sprintf("Ensure you pass a struct or a slice of structs, but got type '%s'.", val.Kind())

		return orm.setError(message, fmt.Errorf("%s - validateStructTags: InvalidDataType: "+
			"expected struct or slice of structs, got %s", opName, val.Kind()), http.StatusBadRequest)
	}

	tagInfo, err := orm.buildTagInfoInvalidateStructTags(structType, structRequirement, opName)
	if err != nil {
		return orm.Error
	}

	if activeDriver == database.MONGO && tagInfo.hasPrimaryKey && tagInfo.primaryKeyColumn != _id {
		hint := ""

		if strings.EqualFold(strings.TrimPrefix(tagInfo.primaryKeyColumn, "_"), "id") {
			hint = " (did you forget the underscore? use \"_id\")"
		}

		message := fmt.Sprintf(
			"MongoDB primary_key must be gorest:\"_id, primary_key\", got %q%s. "+
				"This field always decodes empty, breaking Find results, Update/Delete by ID, and Preload.",
			tagInfo.primaryKeyColumn, hint,
		)

		return orm.setError(message, fmt.Errorf(
			"%s - validateStructTags: InvalidMongoPrimaryKeyColumn: struct %s got %q, want \"_id\"",
			opName, structType.Name(), tagInfo.primaryKeyColumn,
		), http.StatusBadRequest)
	}

	return nil
}

func (orm *ORM) buildValueValidateStructTags(data any, opName string) (reflect.Value, error) {
	val := reflect.ValueOf(data)
	if !val.IsValid() {
		return val, orm.setError(
			"Ensure the payload data passed is a struct, a slice of structs, or a valid pointer.",
			fmt.Errorf("%s - validateStructTags: InvalidReflectValue", opName),
			http.StatusBadRequest,
		)
	}

	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return val, orm.setError(
				fmt.Sprintf("Ensure the pointer passed to the %s operation is not nil; initialize it before passing.", opName),
				fmt.Errorf("%s - validateStructTags: NilPointerDereference", opName),
				http.StatusBadRequest,
			)
		}

		val = val.Elem()
	}

	return val, nil
}

func (orm *ORM) verifFieldRequirementValidateStructTags(requirement fieldRequirement, val reflect.Value, opName string) error {
	if requirement.requireNonEmpty {
		if val.Len() == 0 {
			return orm.setError(
				"Ensure the provided slice contains at least one record/element to perform this operation.",
				fmt.Errorf("%s - validateStructTags: EmptySliceProvided", opName),
				http.StatusBadRequest,
			)
		}

		for idx := 0; idx < val.Len(); idx++ {
			if val.Index(idx).Kind() == reflect.Pointer && val.Index(idx).IsNil() {
				return orm.setError(
					fmt.Sprintf("Ensure element at index %d in the slice is not a nil pointer.", idx),
					fmt.Errorf("%s - validateStructTags: NilElementInSlice", opName),
					http.StatusBadRequest,
				)
			}
		}
	}

	return nil
}

func (orm *ORM) buildStructTypeByElemInValidateStructTags(val reflect.Value, opName string) (reflect.Type, error) {
	elemType := val.Type().Elem()

	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}

	if elemType.Kind() != reflect.Struct {
		return nil, orm.setError(
			fmt.Sprintf("Ensure the slice contains structs, but found a slice of type '%s'.", elemType.Kind()),
			fmt.Errorf("%s - validateStructTags: "+
				"InvalidSliceElementType: expected slice of structs, got slice of %s", opName, elemType.Kind()),
			http.StatusBadRequest,
		)
	}

	structType := elemType

	return structType, nil
}

func (orm *ORM) buildTagInfoInvalidateStructTags(structType reflect.Type, structRequirement fieldRequirement, opName string) (tagInfo, error) {
	tagInfo := resolveTagInfo(structType)

	if tagInfo.err != nil {
		return tagInfo, orm.setError(
			fmt.Sprintf(
				`Struct %s has a field with an empty gorest tag. `+
					`Use gorest:"-" to exclude it explicitly, or provide a column name. Detail: %v`,
				structType.Name(), tagInfo.err,
			),
			fmt.Errorf("%s - validateStructTags: EmptyGorestTag: %w", opName, tagInfo.err),
			http.StatusBadRequest,
		)
	}

	if !tagInfo.hasValidTag {
		return tagInfo, orm.setError(
			"Ensure your struct fields have valid 'gorest' tags mapped to columns (e.g., gorest:\"column_name\").",
			fmt.Errorf("%s - validateStructTags: MissingGorestTags", opName),
			http.StatusBadRequest,
		)
	}

	if structRequirement.requirePrimaryKey && !tagInfo.hasPrimaryKey {
		return tagInfo, orm.setError(
			"Ensure that at least one field in your struct is designated using "+
				"the 'primary_key' flag (e.g., gorest:\"id, primary_key\").",
			fmt.Errorf("%s - validateStructTags: MissingPrimaryKeyTag", opName),
			http.StatusBadRequest,
		)
	}

	return tagInfo, nil
}
