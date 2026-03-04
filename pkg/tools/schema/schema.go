// Package schema generates JSON Schema from Go struct types using reflection.
// Field names come from json tags, descriptions from "desc" struct tags, and
// required/optional status from the presence of omitempty in the json tag.
package schema

import (
	"encoding/json"
	"reflect"
	"strings"
)

// Generate returns a JSON Schema (as json.RawMessage) for the given Go struct
// type T. The schema is derived from struct field tags:
//
//   - json:"name"         → property name; omitempty means the field is optional
//   - desc:"..."          → property description
//
// Supported field types: string, int/int64, bool, []string, []struct,
// map[string]string, and nested structs.
func Generate[T any]() json.RawMessage {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	s := buildObject(t)

	b, err := json.Marshal(s)
	if err != nil {
		panic("schema: marshal failed: " + err.Error())
	}

	return json.RawMessage(b)
}

type objectSchema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	Required   []string       `json:"required,omitempty"`
}

func buildObject(t reflect.Type) objectSchema {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	s := objectSchema{
		Type:       "object",
		Properties: make(map[string]any),
	}

	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		jsonTag := f.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		name, opts := parseJSONTag(jsonTag)
		if name == "" {
			continue
		}

		prop := buildProperty(f.Type)

		if desc := f.Tag.Get("desc"); desc != "" {
			prop["description"] = desc
		}

		s.Properties[name] = prop

		if !opts.contains("omitempty") {
			s.Required = append(s.Required, name)
		}
	}

	if len(s.Properties) == 0 {
		s.Properties = nil
	}

	return s
}

func buildProperty(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}

	case reflect.Bool:
		return map[string]any{"type": "boolean"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}

	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}

	case reflect.Slice:
		items := buildProperty(t.Elem())
		return map[string]any{"type": "array", "items": items}

	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			valProp := buildProperty(t.Elem())
			return map[string]any{"type": "object", "additionalProperties": valProp}
		}

		return map[string]any{"type": "object"}

	case reflect.Struct:
		obj := buildObject(t)
		m := map[string]any{"type": "object"}
		if len(obj.Properties) > 0 {
			m["properties"] = obj.Properties
		}
		if len(obj.Required) > 0 {
			m["required"] = obj.Required
		}

		return m

	default:
		return map[string]any{}
	}
}

type tagOptions string

func (o tagOptions) contains(name string) bool {
	s := string(o)
	for s != "" {
		var part string
		if before, after, found := strings.Cut(s, ","); found {
			part = before
			s = after
		} else {
			part = s
			s = ""
		}

		if part == name {
			return true
		}
	}

	return false
}

func parseJSONTag(tag string) (string, tagOptions) {
	if before, after, found := strings.Cut(tag, ","); found {
		return before, tagOptions(after)
	}

	return tag, ""
}
