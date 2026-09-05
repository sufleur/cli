// Package toolschema checks a JSON Schema against the subset the code
// generators can express.
//
// The server only validates that a tool's input schema is an object at the
// root. Everything else it accepts, but constructs the generators cannot model
// — $ref, allOf, tuple-typed items — silently degrade to `unknown` in
// TypeScript and `Any` in Python. That failure is invisible at authoring time
// and only shows up much later as untyped generated code, so it is worth
// catching before the schema is ever pushed.
package toolschema

import (
	"fmt"
	"sort"
	"strings"
)

// Issue is one unsupported construct, located by JSON Pointer.
type Issue struct {
	Path    string // e.g. "/properties/query"
	Message string
}

func (i Issue) String() string {
	path := i.Path
	if path == "" {
		path = "/"
	}
	return fmt.Sprintf("  %s: %s", path, i.Message)
}

// supportedTypes are the JSON Schema types both generators can express.
var supportedTypes = map[string]bool{
	"string": true, "integer": true, "number": true,
	"boolean": true, "null": true, "object": true, "array": true,
}

// unsupportedKeywords are constructs neither generator models, mapped to the
// reason, so the message says what to do instead rather than only what is wrong.
var unsupportedKeywords = map[string]string{
	"$ref":  "$ref is not resolved; inline the referenced schema",
	"allOf": "allOf is not merged; write the combined object directly",
	"not":   "not has no type to generate from; describe the allowed shape instead",
	"if":    "conditional schemas are not generated; split into a oneOf",
	"then":  "conditional schemas are not generated; split into a oneOf",
	"else":  "conditional schemas are not generated; split into a oneOf",
	"$defs": "$defs is only useful with $ref, which is not resolved; inline instead",
	"patternProperties": "patternProperties has no generatable key type; " +
		"declare the properties you expect",
}

// ValidateInput checks a tool's argument schema. The root must be an object
// schema: it describes the arguments the model emits, which providers require
// to be a JSON object.
func ValidateInput(schema map[string]any) []Issue {
	if schema == nil {
		return []Issue{{Message: "input schema is required"}}
	}
	issues := []Issue{}
	if t, _ := schema["type"].(string); t != "object" {
		issues = append(issues, Issue{
			Message: `input schema must have "type": "object" at the root — it describes the arguments the model emits`,
		})
	}
	return append(issues, walk(schema, "")...)
}

// ValidateOutput checks a tool's result schema. Any schema is allowed at the
// root; the result is typed statically from whatever shape it describes.
func ValidateOutput(schema map[string]any) []Issue {
	if schema == nil {
		return nil
	}
	return walk(schema, "")
}

func walk(schema map[string]any, path string) []Issue {
	var issues []Issue

	for _, keyword := range sortedKeys(schema) {
		if reason, bad := unsupportedKeywords[keyword]; bad {
			issues = append(issues, Issue{Path: path, Message: reason})
		}
	}

	if t, ok := schema["type"]; ok {
		switch v := t.(type) {
		case string:
			if !supportedTypes[v] {
				issues = append(issues, Issue{Path: path, Message: fmt.Sprintf("unknown type %q", v)})
			}
		case []any:
			// A type union is spelled oneOf/anyOf in the shape the generators read.
			issues = append(issues, Issue{
				Path:    path,
				Message: "a list of types is not generated; use anyOf with one schema per type",
			})
		default:
			issues = append(issues, Issue{Path: path, Message: "type must be a string"})
		}
	}

	issues = append(issues, checkEnum(schema, path)...)

	if props, ok := schema["properties"]; ok {
		asMap, isMap := props.(map[string]any)
		if !isMap {
			issues = append(issues, Issue{Path: join(path, "properties"), Message: "properties must be an object"})
		} else {
			for _, name := range sortedKeys(asMap) {
				child, isChildMap := asMap[name].(map[string]any)
				if !isChildMap {
					issues = append(issues, Issue{Path: join(path, "properties", name), Message: "property schema must be an object"})
					continue
				}
				issues = append(issues, walk(child, join(path, "properties", name))...)
			}
		}
	}

	if additional, ok := schema["additionalProperties"]; ok {
		if _, isBool := additional.(bool); !isBool {
			issues = append(issues, Issue{
				Path:    join(path, "additionalProperties"),
				Message: "a schema-valued additionalProperties is not generated; use true or false, or declare the properties",
			})
		}
	}

	if items, ok := schema["items"]; ok {
		switch v := items.(type) {
		case map[string]any:
			issues = append(issues, walk(v, join(path, "items"))...)
		case []any:
			issues = append(issues, Issue{
				Path:    join(path, "items"),
				Message: "tuple-typed items are not generated; use a single schema for every element",
			})
		default:
			issues = append(issues, Issue{Path: join(path, "items"), Message: "items must be a schema"})
		}
	}

	for _, keyword := range []string{"oneOf", "anyOf"} {
		branches, ok := schema[keyword]
		if !ok {
			continue
		}
		list, isList := branches.([]any)
		if !isList {
			issues = append(issues, Issue{Path: join(path, keyword), Message: keyword + " must be a list of schemas"})
			continue
		}
		for i, branch := range list {
			child, isMap := branch.(map[string]any)
			if !isMap {
				issues = append(issues, Issue{Path: join(path, keyword, fmt.Sprint(i)), Message: "branch must be a schema"})
				continue
			}
			issues = append(issues, walk(child, join(path, keyword, fmt.Sprint(i)))...)
		}
	}

	issues = append(issues, checkRequired(schema, path)...)
	return issues
}

// checkEnum rejects a mixed-type enum: both generators pick one literal kind
// per enum and cannot express a union of kinds.
func checkEnum(schema map[string]any, path string) []Issue {
	raw, ok := schema["enum"]
	if !ok {
		return nil
	}
	values, isList := raw.([]any)
	if !isList {
		return []Issue{{Path: join(path, "enum"), Message: "enum must be a list"}}
	}
	if len(values) == 0 {
		return []Issue{{Path: join(path, "enum"), Message: "enum must have at least one value"}}
	}

	kinds := map[string]bool{}
	for _, v := range values {
		switch v.(type) {
		case string:
			kinds["string"] = true
		case float64, int:
			kinds["number"] = true
		case bool:
			kinds["boolean"] = true
		default:
			kinds["other"] = true
		}
	}
	if kinds["other"] {
		return []Issue{{Path: join(path, "enum"), Message: "enum values must be strings, numbers or booleans"}}
	}
	if len(kinds) > 1 {
		return []Issue{{Path: join(path, "enum"), Message: "enum values must all be the same type"}}
	}
	return nil
}

// checkRequired catches a required entry with no matching property — the field
// would be generated as absent yet mandatory, which no caller can satisfy.
func checkRequired(schema map[string]any, path string) []Issue {
	raw, ok := schema["required"]
	if !ok {
		return nil
	}
	names, isList := raw.([]any)
	if !isList {
		return []Issue{{Path: join(path, "required"), Message: "required must be a list of property names"}}
	}
	props, _ := schema["properties"].(map[string]any)

	var issues []Issue
	for _, n := range names {
		name, isString := n.(string)
		if !isString {
			issues = append(issues, Issue{Path: join(path, "required"), Message: "required entries must be strings"})
			continue
		}
		if _, declared := props[name]; !declared {
			issues = append(issues, Issue{
				Path:    join(path, "required"),
				Message: fmt.Sprintf("%q is required but not declared in properties", name),
			})
		}
	}
	return issues
}

func join(path string, segments ...string) string {
	var b strings.Builder
	b.WriteString(path)
	for _, s := range segments {
		b.WriteString("/")
		b.WriteString(escapePointer(s))
	}
	return b.String()
}

// escapePointer applies RFC 6901 escaping so a property named "a/b" does not
// read as two path segments.
func escapePointer(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
