package typescript

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// jsonSchemaToZod converts a JSON Schema (as a map) into Zod TypeScript source code.
func jsonSchemaToZod(schema map[string]interface{}, indent int) string {
	if len(schema) == 0 {
		return "z.record(z.unknown())"
	}

	// anyOf (no type field)
	if anyOf, ok := schema["anyOf"]; ok {
		return anyOfToZod(anyOf, indent)
	}

	// enum (independent of type)
	if enum, ok := schema["enum"]; ok {
		return enumToZod(enum)
	}

	typ, _ := schema["type"].(string)
	switch typ {
	case "string":
		return "z.string()"
	case "number":
		return "z.number()"
	case "integer":
		return "z.number().int()"
	case "boolean":
		return "z.boolean()"
	case "object":
		return objectToZod(schema, indent)
	case "array":
		return arrayToZod(schema, indent)
	default:
		return "z.unknown() /* unsupported */"
	}
}

func enumToZod(enum interface{}) string {
	values, ok := enum.([]interface{})
	if !ok || len(values) == 0 {
		return "z.unknown() /* unsupported */"
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
			return "z.unknown() /* unsupported */"
		}
	}

	switch {
	case allStrings:
		items := make([]string, len(values))
		for i, v := range values {
			items[i] = fmt.Sprintf("%q", v.(string))
		}
		return fmt.Sprintf("z.enum([%s])", strings.Join(items, ", "))
	case allNumbers:
		return literalsToZod(values, formatNumberLiteral)
	case allBools:
		return literalsToZod(values, formatBoolLiteral)
	default:
		return "z.unknown() /* unsupported */"
	}
}

func literalsToZod(values []interface{}, format func(interface{}) string) string {
	if len(values) == 1 {
		return fmt.Sprintf("z.literal(%s)", format(values[0]))
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("z.literal(%s)", format(v))
	}
	return fmt.Sprintf("z.union([%s])", strings.Join(parts, ", "))
}

func formatNumberLiteral(v interface{}) string {
	n := v.(float64)
	if n == float64(int64(n)) {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

func formatBoolLiteral(v interface{}) string {
	if v.(bool) {
		return "true"
	}
	return "false"
}

func objectToZod(schema map[string]interface{}, indent int) string {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return "z.record(z.unknown())"
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

	innerIndent := strings.Repeat("  ", indent+1)
	outerIndent := strings.Repeat("  ", indent)

	var b strings.Builder
	b.WriteString("z.object({\n")
	for _, k := range keys {
		v, _ := props[k].(map[string]interface{})
		b.WriteString(innerIndent)
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(jsonSchemaToZod(v, indent+1))
		if !requiredSet[k] {
			b.WriteString(".optional()")
		}
		b.WriteString(",\n")
	}
	b.WriteString(outerIndent)
	b.WriteString("})")
	return b.String()
}

func arrayToZod(schema map[string]interface{}, indent int) string {
	items, ok := schema["items"].(map[string]interface{})
	if !ok {
		return "z.array(z.unknown())"
	}
	return fmt.Sprintf("z.array(%s)", jsonSchemaToZod(items, indent))
}

func anyOfToZod(anyOf interface{}, indent int) string {
	variants, ok := anyOf.([]interface{})
	if !ok || len(variants) == 0 {
		return "z.unknown() /* unsupported */"
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
		return "z.unknown() /* unsupported */"
	case 1:
		base = jsonSchemaToZod(nonNull[0], indent)
	default:
		parts := make([]string, len(nonNull))
		for i, s := range nonNull {
			parts[i] = jsonSchemaToZod(s, indent)
		}
		base = fmt.Sprintf("z.union([%s])", strings.Join(parts, ", "))
	}

	if hasNull {
		return base + ".nullable()"
	}
	return base
}
