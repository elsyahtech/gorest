package orm

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

type fieldRequirement struct {
	requirePrimaryKey bool
	requireNonEmpty   bool
	validateRequired  bool
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

	_, err = orm.buildTagInfoInvalidateStructTags(structType, structRequirement, opName)
	if err != nil {
		return orm.Error
	}

	if structRequirement.validateRequired {
		if err := orm.validateRequiredFields(val, structType, opName); err != nil {
			return err
		}
	}

	return nil
}

func hasGorestTagOption(tag, option string) bool {
	for _, part := range strings.Split(tag, ",")[1:] {
		if strings.EqualFold(strings.TrimSpace(part), option) {
			return true
		}
	}

	return false
}
