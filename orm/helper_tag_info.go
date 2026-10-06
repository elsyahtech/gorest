package orm

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

type tagInfo struct {
	objectIDColumns  map[string]bool
	err              error
	primaryKeyColumn string
	hasValidTag      bool
	hasPrimaryKey    bool
}

var tagCache sync.Map // map[reflect.Type]structTagInfo

func resolveTagInfo(structType reflect.Type) tagInfo {
	if cached, ok := tagCache.Load(structType); ok {
		if info, typeOk := cached.(tagInfo); typeOk {
			return info
		}
	}

	var info tagInfo

	for idx := 0; idx < structType.NumField(); idx++ {
		field := structType.Field(idx)

		value, ok := field.Tag.Lookup("gorest")
		if !ok {
			continue
		}

		trimed, trimOk := trimTagInfo(value, &info, field, structType)
		if !trimOk {
			continue
		}

		info.hasValidTag = true

		splitInspectGorestTags(value, &info, trimed)
	}

	tagCache.Store(structType, info)

	return info
}

func trimTagInfo(value string, info *tagInfo, field reflect.StructField, structType reflect.Type) (string, bool) {
	trimmed := strings.TrimSpace(strings.Split(value, ",")[0])

	if trimmed == "" {
		if info.err == nil {
			info.err = fmt.Errorf(
				`field %q on struct %s has an empty gorest tag; `+
					`use gorest:"-" to exclude it explicitly, or provide a column name`,
				field.Name, structType.Name(),
			)
		}

		return trimmed, false
	}

	if trimmed == "-" {
		return trimmed, false
	}

	return trimmed, true
}

func splitInspectGorestTags(value string, info *tagInfo, trimed string) {
	for _, part := range strings.Split(value, ",") {
		trimPart := strings.TrimSpace(part)

		if strings.EqualFold(trimPart, "primary_key") {
			info.hasPrimaryKey = true
			info.primaryKeyColumn = trimed
		}

		if strings.EqualFold(trimPart, "object_id") {
			if info.objectIDColumns == nil {
				info.objectIDColumns = make(map[string]bool)
			}

			info.objectIDColumns[strings.ToLower(trimed)] = true
		}
	}
}
