package toolschema

import (
	"strings"
	"testing"
)

func obj(props map[string]any, required ...any) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func TestValidateInput_AcceptsTheSupportedSubset(t *testing.T) {
	schema := obj(map[string]any{
		"query":  map[string]any{"type": "string", "description": "What to search for"},
		"limit":  map[string]any{"type": "integer", "default": float64(10)},
		"ratio":  map[string]any{"type": "number"},
		"deep":   map[string]any{"type": "boolean"},
		"tags":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"tone":   map[string]any{"type": "string", "enum": []any{"formal", "casual"}},
		"nested": obj(map[string]any{"inner": map[string]any{"type": "string"}}, "inner"),
		"either": map[string]any{"anyOf": []any{
			map[string]any{"type": "string"},
			map[string]any{"type": "null"},
		}},
		"open": map[string]any{"type": "object", "additionalProperties": true},
	}, "query")

	if issues := ValidateInput(schema); len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestValidateInput_RequiresAnObjectRoot(t *testing.T) {
	issues := ValidateInput(map[string]any{"type": "string"})
	if len(issues) == 0 {
		t.Fatal("expected an issue for a non-object root")
	}
	if !strings.Contains(issues[0].Message, `"type": "object"`) {
		t.Errorf("message should say what the root must be: %q", issues[0].Message)
	}
}

func TestValidateInput_NilIsAnError(t *testing.T) {
	if len(ValidateInput(nil)) == 0 {
		t.Error("expected an issue for a missing schema")
	}
}

func TestValidate_RejectsUnsupportedConstructs(t *testing.T) {
	cases := map[string]struct {
		schema map[string]any
		path   string
		want   string
	}{
		"$ref": {
			schema: obj(map[string]any{"a": map[string]any{"$ref": "#/$defs/Thing"}}),
			path:   "/properties/a",
			want:   "$ref",
		},
		"allOf": {
			schema: obj(map[string]any{"a": map[string]any{"allOf": []any{}}}),
			path:   "/properties/a",
			want:   "allOf",
		},
		"not": {
			schema: obj(map[string]any{"a": map[string]any{"not": map[string]any{}}}),
			path:   "/properties/a",
			want:   "not",
		},
		"patternProperties": {
			schema: obj(map[string]any{"a": map[string]any{"patternProperties": map[string]any{}}}),
			path:   "/properties/a",
			want:   "patternProperties",
		},
		"tuple items": {
			schema: obj(map[string]any{"a": map[string]any{
				"type":  "array",
				"items": []any{map[string]any{"type": "string"}},
			}}),
			path: "/properties/a/items",
			want: "tuple",
		},
		"schema-valued additionalProperties": {
			schema: obj(map[string]any{"a": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			}}),
			path: "/properties/a/additionalProperties",
			want: "additionalProperties",
		},
		"mixed enum": {
			schema: obj(map[string]any{"a": map[string]any{"enum": []any{"x", float64(1)}}}),
			path:   "/properties/a/enum",
			want:   "same type",
		},
		"unknown type": {
			schema: obj(map[string]any{"a": map[string]any{"type": "date"}}),
			path:   "/properties/a",
			want:   `unknown type "date"`,
		},
		"list of types": {
			schema: obj(map[string]any{"a": map[string]any{"type": []any{"string", "null"}}}),
			path:   "/properties/a",
			want:   "anyOf",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			issues := ValidateInput(c.schema)
			if len(issues) == 0 {
				t.Fatalf("expected an issue for %s", name)
			}
			var found bool
			for _, issue := range issues {
				if issue.Path == c.path && strings.Contains(issue.Message, c.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("expected an issue at %q mentioning %q, got %v", c.path, c.want, issues)
			}
		})
	}
}

// A required name with no matching property generates a field no caller can
// supply, so it is worth catching even though the server accepts it.
func TestValidate_RequiredMustMatchAProperty(t *testing.T) {
	schema := obj(map[string]any{"a": map[string]any{"type": "string"}}, "a", "missing")

	issues := ValidateInput(schema)
	if len(issues) != 1 {
		t.Fatalf("expected exactly one issue, got %v", issues)
	}
	if !strings.Contains(issues[0].Message, `"missing"`) {
		t.Errorf("issue should name the undeclared property: %q", issues[0].Message)
	}
}

func TestValidate_RecursesIntoBranchesAndItems(t *testing.T) {
	schema := obj(map[string]any{
		"list": map[string]any{
			"type":  "array",
			"items": map[string]any{"$ref": "#/x"},
		},
		"union": map[string]any{"oneOf": []any{
			map[string]any{"type": "string"},
			map[string]any{"allOf": []any{}},
		}},
	})

	issues := ValidateInput(schema)
	paths := map[string]bool{}
	for _, i := range issues {
		paths[i.Path] = true
	}
	for _, want := range []string{"/properties/list/items", "/properties/union/oneOf/1"} {
		if !paths[want] {
			t.Errorf("expected an issue at %s, got %v", want, issues)
		}
	}
}

// A property named with a slash must not read as two pointer segments.
func TestValidate_EscapesPointerSegments(t *testing.T) {
	schema := obj(map[string]any{"a/b": map[string]any{"$ref": "#/x"}})

	issues := ValidateInput(schema)
	if len(issues) != 1 {
		t.Fatalf("expected one issue, got %v", issues)
	}
	if issues[0].Path != "/properties/a~1b" {
		t.Errorf("expected an escaped pointer, got %q", issues[0].Path)
	}
}

// The output schema types a value your own code returns, so any shape is fine
// at the root — only unsupported constructs matter.
func TestValidateOutput_AllowsANonObjectRoot(t *testing.T) {
	if issues := ValidateOutput(map[string]any{"type": "array", "items": map[string]any{"type": "string"}}); len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
	if issues := ValidateOutput(nil); len(issues) != 0 {
		t.Errorf("a missing output schema is allowed, got %v", issues)
	}
	if issues := ValidateOutput(map[string]any{"$ref": "#/x"}); len(issues) == 0 {
		t.Error("expected $ref to be rejected in an output schema too")
	}
}

func TestIssue_String(t *testing.T) {
	if got := (Issue{Message: "bad"}).String(); !strings.Contains(got, "/") {
		t.Errorf("a root-level issue should still show a path: %q", got)
	}
	if got := (Issue{Path: "/properties/a", Message: "bad"}).String(); !strings.Contains(got, "/properties/a") {
		t.Errorf("unexpected rendering: %q", got)
	}
}
