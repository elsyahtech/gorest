package orm

import (
	"fmt"
	"net/http"
	"reflect"
)

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
