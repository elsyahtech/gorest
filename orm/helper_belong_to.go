package orm

import "reflect"

func loadParentBelongsTo(found map[string]reflect.Value, parent string, parents []reflect.Value, core *mongoBelongsTo) {
	related, relOk := found[parent]

	for _, prnt := range parents {
		field := prnt.Field(core.fieldIndex)

		if !field.CanSet() {
			continue
		}

		if !relOk {
			continue
		}

		// copy per parent to avoid sharing the same pointer
		cpy := reflect.New(core.relatedType)

		cpy.Elem().Set(related.Elem())

		if core.fieldIsPtr {
			field.Set(cpy)
		} else {
			field.Set(cpy.Elem())
		}
	}
}
