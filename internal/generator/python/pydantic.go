package python

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// pydanticClass represents a Pydantic BaseModel class to be emitted.
type pydanticClass struct {
	Name   string
	Fields []pydanticField
}

// pydanticField represents a single field in a Pydantic model.
type pydanticField struct {
	Name     string
	Type     string
	Required bool
}

// jsonSchemaToPydantic converts a JSON Schema (as a map) into Pydantic model
// class definitions. It returns the concatenated class definitions (leaf-first)
// and the top-level class name (or inline type like "str" for primitives).
func jsonSchemaToPydantic(schema map[string]interface{}, className string) (models string, topClassName string) {
	if len(schema) == 0 {
		return "", "dict[str, Any]"
	}

	var classes []pydanticClass
	topType := schemaToPydanticType(schema, className, &classes)

	if len(classes) == 0 {
		return "", topType
	}

	return renderPydanticClasses(classes), topType
}

// schemaToPydanticType recursively converts a JSON Schema node to a Python type
// string, accumulating pydanticClass definitions along the way.
func schemaToPydanticType(schema map[string]interface{}, className string, classes *[]pydanticClass) string {
	if len(schema) == 0 {
		return "dict[str, Any]"
	}

	// anyOf (no type field)
	if anyOf, ok := schema["anyOf"]; ok {
		return anyOfToPydantic(anyOf, className, classes)
	}

	// enum (independent of type)
	if enum, ok := schema["enum"]; ok {
		return enumToPydantic(enum)
	}

	typ, _ := schema["type"].(string)
	switch typ {
	case "string":
		return "str"
	case "number":
		return "float"
	case "integer":
		return "int"
	case "boolean":
		return "bool"
	case "object":
		return objectToPydantic(schema, className, classes)
	case "array":
		return arrayToPydantic(schema, className, classes)
	default:
		return "Any"
	}
}

func enumToPydantic(enum interface{}) string {
	values, ok := enum.([]interface{})
	if !ok || len(values) == 0 {
		return "Any"
	}

	allStrings := true
	allNumbers := true
	allBools := true
	for _, v := range values {
		switch v.(type) {
		case string:
			allNumbers = false
			allBools = false
		case float64:
			allStrings = false
			allBools = false
		case bool:
			allStrings = false
			allNumbers = false
		default:
			return "Any"
		}
	}

	items := make([]string, len(values))
	switch {
	case allStrings:
		for i, v := range values {
			items[i] = fmt.Sprintf("%q", v.(string))
		}
	case allNumbers:
		for i, v := range values {
			items[i] = formatNumberLiteral(v)
		}
	case allBools:
		for i, v := range values {
			if v.(bool) {
				items[i] = "True"
			} else {
				items[i] = "False"
			}
		}
	default:
		return "Any"
	}
	return fmt.Sprintf("Literal[%s]", strings.Join(items, ", "))
}

func formatNumberLiteral(v interface{}) string {
	n := v.(float64)
	if n == float64(int64(n)) {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

func objectToPydantic(schema map[string]interface{}, className string, classes *[]pydanticClass) string {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return "dict[str, Any]"
	}

	requiredSet := make(map[string]bool)
	if req, ok := schema["required"].([]interface{}); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}

	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var fields []pydanticField
	for _, k := range keys {
		v, _ := props[k].(map[string]interface{})
		childClassName := "_" + className + "_" + toPascalCase(k)
		fieldType := schemaToPydanticType(v, childClassName, classes)

		isRequired := requiredSet[k]
		if !isRequired {
			fieldType = "Optional[" + fieldType + "]"
		}

		fields = append(fields, pydanticField{
			Name:     k,
			Type:     fieldType,
			Required: isRequired,
		})
	}

	cls := pydanticClass{
		Name:   className,
		Fields: fields,
	}
	*classes = append(*classes, cls)
	return className
}

func arrayToPydantic(schema map[string]interface{}, className string, classes *[]pydanticClass) string {
	items, ok := schema["items"].(map[string]interface{})
	if !ok {
		return "list[Any]"
	}
	inner := schemaToPydanticType(items, className, classes)
	return "list[" + inner + "]"
}

func anyOfToPydantic(anyOf interface{}, className string, classes *[]pydanticClass) string {
	variants, ok := anyOf.([]interface{})
	if !ok || len(variants) == 0 {
		return "Any"
	}

	var nonNull []map[string]interface{}
	hasNull := false
	for _, v := range variants {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t == "null" {
			hasNull = true
		} else {
			nonNull = append(nonNull, m)
		}
	}

	var base string
	switch len(nonNull) {
	case 0:
		return "Any"
	case 1:
		base = schemaToPydanticType(nonNull[0], className, classes)
	default:
		parts := make([]string, len(nonNull))
		for i, s := range nonNull {
			parts[i] = schemaToPydanticType(s, className, classes)
		}
		base = "Union[" + strings.Join(parts, ", ") + "]"
	}

	if hasNull {
		return "Optional[" + base + "]"
	}
	return base
}

// renderPydanticClasses joins class definitions with blank lines.
func renderPydanticClasses(classes []pydanticClass) string {
	var b strings.Builder
	for i, cls := range classes {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("class ")
		b.WriteString(cls.Name)
		b.WriteString("(BaseModel):\n")
		for _, f := range cls.Fields {
			b.WriteString("    ")
			b.WriteString(f.Name)
			b.WriteString(": ")
			b.WriteString(f.Type)
			if !f.Required {
				b.WriteString(" = None")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}
