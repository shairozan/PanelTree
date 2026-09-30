package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Explicit references preserve recursive authoring/layout types that the SDK's
// default schema inference rejects. The SDK still validates these schemas.
func schema[T any]() map[string]any {
	defs := map[string]any{}
	seen := map[reflect.Type]string{}
	var walk func(reflect.Type) map[string]any
	walk = func(t reflect.Type) map[string]any {
		if t.Kind() == reflect.Pointer {
			return map[string]any{"anyOf": []any{walk(t.Elem()), map[string]any{"type": "null"}}}
		}
		if t.Implements(reflect.TypeFor[json.Marshaler]()) {
			return map[string]any{"type": "object"}
		}
		switch t.Kind() {
		case reflect.Struct:
			if id, ok := seen[t]; ok {
				return map[string]any{"$ref": "#/$defs/" + id}
			}
			id := fmt.Sprintf("type%d", len(seen))
			seen[t] = id
			props := map[string]any{}
			required := []string{}
			var fields func(reflect.Type)
			fields = func(st reflect.Type) {
				for i := 0; i < st.NumField(); i++ {
					f := st.Field(i)
					if !f.IsExported() {
						continue
					}
					tag := strings.Split(f.Tag.Get("json"), ",")
					if tag[0] == "-" {
						continue
					}
					if f.Anonymous && tag[0] == "" {
						et := f.Type
						if et.Kind() == reflect.Pointer {
							et = et.Elem()
						}
						if et.Kind() == reflect.Struct {
							fields(et)
							continue
						}
					}
					name := tag[0]
					if name == "" {
						name = f.Name
					}
					props[name] = walk(f.Type)
					if !strings.Contains(f.Tag.Get("json"), "omitempty") {
						required = append(required, name)
					}
				}
			}
			fields(t)
			defs[id] = map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
			return map[string]any{"$ref": "#/$defs/" + id}
		case reflect.Slice:
			return map[string]any{"anyOf": []any{map[string]any{"type": "array", "items": walk(t.Elem())}, map[string]any{"type": "null"}}}
		case reflect.Array:
			return map[string]any{"type": "array", "items": walk(t.Elem())}
		case reflect.Map:
			return map[string]any{"anyOf": []any{map[string]any{"type": "object", "additionalProperties": walk(t.Elem())}, map[string]any{"type": "null"}}}
		case reflect.String:
			return map[string]any{"type": "string"}
		case reflect.Bool:
			return map[string]any{"type": "boolean"}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return map[string]any{"type": "integer"}
		case reflect.Float32, reflect.Float64:
			return map[string]any{"type": "number"}
		default:
			return map[string]any{}
		}
	}
	root := walk(reflect.TypeFor[T]())
	root["$defs"] = defs
	root["type"] = "object"
	return root
}
