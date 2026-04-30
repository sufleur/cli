package python

import (
	"strings"
	"testing"
)

func TestJsonSchemaToPydantic(t *testing.T) {
	tests := []struct {
		name             string
		schema           map[string]interface{}
		className        string
		expectedModels   string
		expectedTopClass string
	}{
		{
			name:             "nil schema",
			schema:           nil,
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "dict[str, Any]",
		},
		{
			name:             "empty map",
			schema:           map[string]interface{}{},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "dict[str, Any]",
		},
		{
			name:             "string type",
			schema:           map[string]interface{}{"type": "string"},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "str",
		},
		{
			name:             "number type",
			schema:           map[string]interface{}{"type": "number"},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "float",
		},
		{
			name:             "integer type",
			schema:           map[string]interface{}{"type": "integer"},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "int",
		},
		{
			name:             "boolean type",
			schema:           map[string]interface{}{"type": "boolean"},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "bool",
		},
		{
			name: "string enum",
			schema: map[string]interface{}{
				"type": "string",
				"enum": []interface{}{"red", "green", "blue"},
			},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: `Literal["red", "green", "blue"]`,
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
			className: "Output",
			expectedModels: "class Output(BaseModel):\n" +
				"    age: Optional[int] = None\n" +
				"    name: str\n",
			expectedTopClass: "Output",
		},
		{
			name: "object with no properties",
			schema: map[string]interface{}{
				"type": "object",
			},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "dict[str, Any]",
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
			className: "Output",
			expectedModels: "class _Output_Address(BaseModel):\n" +
				"    city: Optional[str] = None\n" +
				"    street: str\n" +
				"\n\n" +
				"class Output(BaseModel):\n" +
				"    address: _Output_Address\n",
			expectedTopClass: "Output",
		},
		{
			name: "array with items",
			schema: map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "list[str]",
		},
		{
			name: "array of objects",
			schema: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{"type": "string"},
					},
					"required": []interface{}{"name"},
				},
			},
			className: "Output",
			expectedModels: "class Output(BaseModel):\n" +
				"    name: str\n",
			expectedTopClass: "list[Output]",
		},
		{
			name: "anyOf with null (nullable)",
			schema: map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "null"},
				},
			},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "Optional[str]",
		},
		{
			name: "anyOf without null (union)",
			schema: map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "number"},
				},
			},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "Union[str, float]",
		},
		{
			name: "anyOf with only null",
			schema: map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{"type": "null"},
				},
			},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "Any",
		},
		{
			name: "$ref unsupported",
			schema: map[string]interface{}{
				"$ref": "#/definitions/Foo",
			},
			className:        "Output",
			expectedModels:   "",
			expectedTopClass: "Any",
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
			className: "Output",
			expectedModels: "class _Output_Users(BaseModel):\n" +
				"    email: str\n" +
				"    name: str\n" +
				"\n\n" +
				"class Output(BaseModel):\n" +
				"    users: list[_Output_Users]\n",
			expectedTopClass: "Output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models, topClass := jsonSchemaToPydantic(tt.schema, tt.className)
			if models != tt.expectedModels {
				t.Errorf("models =\n%s\nwant:\n%s", models, tt.expectedModels)
			}
			if topClass != tt.expectedTopClass {
				t.Errorf("topClass = %q, want %q", topClass, tt.expectedTopClass)
			}
		})
	}
}

func TestRenderPydanticClasses(t *testing.T) {
	classes := []pydanticClass{
		{
			Name: "Inner",
			Fields: []pydanticField{
				{Name: "value", Type: "str", Required: true},
			},
		},
		{
			Name: "Outer",
			Fields: []pydanticField{
				{Name: "inner", Type: "Inner", Required: true},
				{Name: "label", Type: "Optional[str]", Required: false},
			},
		},
	}

	result := renderPydanticClasses(classes)

	if !strings.Contains(result, "class Inner(BaseModel):") {
		t.Error("missing Inner class")
	}
	if !strings.Contains(result, "class Outer(BaseModel):") {
		t.Error("missing Outer class")
	}
	if !strings.Contains(result, "label: Optional[str] = None") {
		t.Error("missing optional field default")
	}

	// Inner should come before Outer (leaf-first)
	innerIdx := strings.Index(result, "Inner")
	outerIdx := strings.Index(result, "Outer")
	if innerIdx > outerIdx {
		t.Error("expected leaf-first ordering (Inner before Outer)")
	}
}
