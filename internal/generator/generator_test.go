package generator

import (
	"strings"
	"testing"
)

func TestResolveDirectives_WithOutputSchema(t *testing.T) {
	pd := PromptData{
		OutputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sentiment": map[string]interface{}{
					"type": "string",
					"enum": []interface{}{"positive", "neutral", "negative"},
				},
				"confidence": map[string]interface{}{
					"type": "number",
				},
			},
			"required": []interface{}{"sentiment", "confidence"},
		},
	}

	content := "Return JSON matching this schema:\n{{@outputSchema}}\nEnd."
	result := ResolveDirectives(content, pd)

	if strings.Contains(result, "{{@outputSchema}}") {
		t.Error("directive was not replaced")
	}
	if !strings.Contains(result, `"type": "object"`) {
		t.Error("expected rendered JSON Schema in output")
	}
	if !strings.Contains(result, `"sentiment"`) {
		t.Error("expected 'sentiment' property in rendered schema")
	}
	if !strings.Contains(result, "Return JSON matching this schema:") {
		t.Error("surrounding content should be preserved")
	}
	if !strings.Contains(result, "End.") {
		t.Error("surrounding content should be preserved")
	}
}

func TestResolveDirectives_WithNullOutputSchema(t *testing.T) {
	pd := PromptData{
		OutputSchema: nil,
	}

	content := "Return JSON matching this schema:\n{{@outputSchema}}\nEnd."
	result := ResolveDirectives(content, pd)

	if strings.Contains(result, "{{@outputSchema}}") {
		t.Error("directive was not replaced")
	}
	expected := "Return JSON matching this schema:\n\nEnd."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestResolveDirectives_NoDirective(t *testing.T) {
	pd := PromptData{
		OutputSchema: map[string]interface{}{
			"type": "object",
		},
	}

	content := "Just a regular template with {{name}} variable."
	result := ResolveDirectives(content, pd)

	if result != content {
		t.Errorf("expected content unchanged, got %q", result)
	}
}

func TestResolveDirectives_MultipleOccurrences(t *testing.T) {
	pd := PromptData{
		OutputSchema: map[string]interface{}{
			"type": "string",
		},
	}

	content := "Schema: {{@outputSchema}} and again: {{@outputSchema}}"
	result := ResolveDirectives(content, pd)

	if strings.Contains(result, "{{@outputSchema}}") {
		t.Error("not all occurrences were replaced")
	}
	count := strings.Count(result, `"type": "string"`)
	if count != 2 {
		t.Errorf("expected 2 replacements, got %d", count)
	}
}

func TestResolveDirectives_WhitespaceTolerant(t *testing.T) {
	pd := PromptData{
		OutputSchema: map[string]interface{}{"type": "string"},
	}

	for _, content := range []string{
		"a {{ @outputSchema }} b",
		"a {{@outputSchema }} b",
		"a {{  @outputSchema  }} b",
		"a {{\t@outputSchema\t}} b",
	} {
		result := ResolveDirectives(content, pd)
		if strings.Contains(result, "@outputSchema") {
			t.Errorf("directive not replaced in %q -> %q", content, result)
		}
		if !strings.Contains(result, `"type": "string"`) {
			t.Errorf("schema not injected in %q -> %q", content, result)
		}
	}
}

func TestResolveDirectives_DollarKeysLiteral(t *testing.T) {
	// $-prefixed JSON Schema keys ($ref/$defs/$schema) must be injected verbatim;
	// a non-literal replace would treat them as submatch expansions and drop them.
	pd := PromptData{
		OutputSchema: map[string]interface{}{"$ref": "#/$defs/User"},
	}

	result := ResolveDirectives("{{@outputSchema}}", pd)
	if !strings.Contains(result, `"$ref": "#/$defs/User"`) {
		t.Errorf("$-keys not injected literally: %q", result)
	}
}
