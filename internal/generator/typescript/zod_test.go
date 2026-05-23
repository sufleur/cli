package typescript

import (
	"testing"
)

func TestJsonSchemaToZod(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]interface{}
		expected string
	}{
		{
			name:     "nil schema",
			schema:   nil,
			expected: "z.record(z.unknown())",
		},
		{
			name:     "empty map",
			schema:   map[string]interface{}{},
			expected: "z.record(z.unknown())",
		},
		{
			name:     "string type",
			schema:   map[string]interface{}{"type": "string"},
			expected: "z.string()",
		},
		{
			name:     "number type",
			schema:   map[string]interface{}{"type": "number"},
			expected: "z.number()",
		},
		{
			name:     "integer type",
			schema:   map[string]interface{}{"type": "integer"},
			expected: "z.number().int()",
		},
		{
			name:     "boolean type",
			schema:   map[string]interface{}{"type": "boolean"},
			expected: "z.boolean()",
		},
		{
			name: "string enum",
			schema: map[string]interface{}{
				"type": "string",
				"enum": []interface{}{"red", "green", "blue"},
			},
			expected: `z.enum(["red", "green", "blue"])`,
		},
		{
			name: "string enum without type",
			schema: map[string]interface{}{
				"enum": []interface{}{"add", "update", "complete", "merge"},
			},
			expected: `z.enum(["add", "update", "complete", "merge"])`,
		},
		{
			name: "single-value string enum",
			schema: map[string]interface{}{
				"enum": []interface{}{"only"},
			},
			expected: `z.enum(["only"])`,
		},
		{
			name: "integer enum",
			schema: map[string]interface{}{
				"type": "integer",
				"enum": []interface{}{float64(1), float64(2), float64(3)},
			},
			expected: "z.union([z.literal(1), z.literal(2), z.literal(3)])",
		},
		{
			name: "number enum",
			schema: map[string]interface{}{
				"enum": []interface{}{float64(0.5), float64(1.5)},
			},
			expected: "z.union([z.literal(0.5), z.literal(1.5)])",
		},
		{
			name: "single-value number enum",
			schema: map[string]interface{}{
				"enum": []interface{}{float64(42)},
			},
			expected: "z.literal(42)",
		},
		{
			name: "boolean enum",
			schema: map[string]interface{}{
				"enum": []interface{}{true, false},
			},
			expected: "z.union([z.literal(true), z.literal(false)])",
		},
		{
			name: "single-value boolean enum",
			schema: map[string]interface{}{
				"enum": []interface{}{true},
			},
			expected: "z.literal(true)",
		},
		{
			name: "mixed-kind enum falls back to unknown",
			schema: map[string]interface{}{
				"enum": []interface{}{"a", float64(1)},
			},
			expected: "z.unknown() /* unsupported */",
		},
		{
			name: "empty enum falls back to unknown",
			schema: map[string]interface{}{
				"enum": []interface{}{},
			},
			expected: "z.unknown() /* unsupported */",
		},
		{
			name: "object with required and optional properties",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
					"age":  map[string]interface{}{"type": "integer"},
				},
				"required": []interface{}{"name"},
			},
			expected: "z.object({\n  age: z.number().int().optional(),\n  name: z.string(),\n})",
		},
		{
			name: "object with no properties",
			schema: map[string]interface{}{
				"type": "object",
			},
			expected: "z.record(z.unknown())",
		},
		{
			name: "nested objects",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"address": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"city":   map[string]interface{}{"type": "string"},
							"street": map[string]interface{}{"type": "string"},
						},
						"required": []interface{}{"street"},
					},
				},
				"required": []interface{}{"address"},
			},
			expected: "z.object({\n  address: z.object({\n    city: z.string().optional(),\n    street: z.string(),\n  }),\n})",
		},
		{
			name: "array with items",
			schema: map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			expected: "z.array(z.string())",
		},
		{
			name: "anyOf with null (nullable)",
			schema: map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "null"},
				},
			},
			expected: "z.string().nullable()",
		},
		{
			name: "anyOf without null (union)",
			schema: map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "number"},
				},
			},
			expected: "z.union([z.string(), z.number()])",
		},
		{
			name: "$ref unsupported",
			schema: map[string]interface{}{
				"$ref": "#/definitions/Foo",
			},
			expected: "z.unknown() /* unsupported */",
		},
		{
			name: "allOf unsupported",
			schema: map[string]interface{}{
				"allOf": []interface{}{
					map[string]interface{}{"type": "string"},
				},
			},
			expected: "z.unknown() /* unsupported */",
		},
		{
			name: "deeply nested schema",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"users": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"email": map[string]interface{}{"type": "string"},
								"name":  map[string]interface{}{"type": "string"},
							},
							"required": []interface{}{"email", "name"},
						},
					},
				},
				"required": []interface{}{"users"},
			},
			expected: "z.object({\n  users: z.array(z.object({\n    email: z.string(),\n    name: z.string(),\n  })),\n})",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jsonSchemaToZod(tt.schema, 0)
			if result != tt.expected {
				t.Errorf("jsonSchemaToZod() =\n%s\nwant:\n%s", result, tt.expected)
			}
		})
	}
}
