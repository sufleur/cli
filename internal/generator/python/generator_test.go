package python

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/generator"
)

func generateAndRead(t *testing.T, prompts []generator.PromptData) string {
	t.Helper()
	outFile := filepath.Join(t.TempDir(), "prompts.py")
	g := &Generator{}
	if err := g.Generate(outFile, prompts); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	return string(data)
}

func TestSinglePromptFullSchema(t *testing.T) {
	userInputSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "User's name",
					},
					"age": map[string]interface{}{
						"type":        "integer",
						"description": "User's age in years",
					},
				},
			},
		},
	}
	systemInputSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tone": map[string]interface{}{
				"type": "string",
			},
		},
	}

	prompts := []generator.PromptData{
		{
			Ref:         "@wtomas/email-subject-generator",
			Name:        "email-subject-generator",
			Version:     "1.4.2",
			Description: "Generates email subject lines",
			Status:      "PUBLISHED",
			Metadata: map[string]interface{}{
				"model":       map[string]interface{}{"type": "string", "value": "gpt-4o"},
				"temperature": map[string]interface{}{"type": "integer", "value": float64(0)},
			},
			Files: []generator.PromptFile{
				{
					Name:         "userPrompt",
					Content:      "Hello {{user.name}}, you are {{user.age}} years old.",
					IsEntrypoint: true,
					InputSchema:  userInputSchema,
				},
				{
					Name:         "systemPrompt",
					Content:      "You are a helpful assistant with {{tone}} tone.",
					IsEntrypoint: true,
					InputSchema:  systemInputSchema,
				},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// TypedDict classes use PascalCase from full ref + entrypoint name
	assertContains(t, output, "class WtomasEmailSubjectGenerator_UserPromptInput(TypedDict):")
	assertContains(t, output, "class WtomasEmailSubjectGenerator_SystemPromptInput(TypedDict):")
	// Nested user object should be private (prefixed with _)
	assertContains(t, output, "class _WtomasEmailSubjectGenerator_UserPromptInput_User(TypedDict):")
	assertContains(t, output, "name: str")
	assertContains(t, output, "age: int")
	assertContains(t, output, "tone: str")

	// PromptName literal uses full ref
	assertContains(t, output, "\"@wtomas/email-subject-generator\"")

	// Templates contain raw Mustache content under entrypoint keys
	assertContains(t, output, "Hello {{user.name}}, you are {{user.age}} years old.")
	assertContains(t, output, "You are a helpful assistant with {{tone}} tone.")

	// Metadata
	assertContains(t, output, "\"model\": \"gpt-4o\"")
	assertContains(t, output, "\"temperature\": 0")
	assertContains(t, output, "\"version\": \"1.4.2\"")

	// render overloads per entrypoint
	assertContains(t, output, "def render(self, entrypoint: Literal[\"userPrompt\"], input: WtomasEmailSubjectGenerator_UserPromptInput) -> PromptOutput:")
	assertContains(t, output, "def render(self, entrypoint: Literal[\"systemPrompt\"], input: WtomasEmailSubjectGenerator_SystemPromptInput) -> PromptOutput:")

	// get_prompt entry point
	assertContains(t, output, "def get_prompt(prompt_name: PromptName)")

	// Imports
	assertContains(t, output, "import chevron")
	assertContains(t, output, "from typing import Any, Literal, TypedDict, overload")
}

func TestMultiplePrompts(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@acme/beta-prompt",
			Name:    "beta-prompt",
			Version: "2.0.0",
			Status:  "PUBLISHED",
			Files:   []generator.PromptFile{{Name: "userPrompt", Content: "Hello", IsEntrypoint: true}},
		},
		{
			Ref:     "@acme/alpha-prompt",
			Name:    "alpha-prompt",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files:   []generator.PromptFile{{Name: "userPrompt", Content: "World", IsEntrypoint: true}},
		},
	}

	output := generateAndRead(t, prompts)

	// Both appear in PromptName literal
	assertContains(t, output, "\"@acme/alpha-prompt\"")
	assertContains(t, output, "\"@acme/beta-prompt\"")

	// Both appear in templates
	assertContains(t, output, "\"@acme/alpha-prompt\": {")
	assertContains(t, output, "\"@acme/beta-prompt\": {")

	// Sorted alphabetically — alpha should come first
	alphaIdx := strings.Index(output, "\"@acme/alpha-prompt\"")
	betaIdx := strings.Index(output, "\"@acme/beta-prompt\"")
	if alphaIdx > betaIdx {
		t.Error("expected prompts to be sorted alphabetically (alpha before beta)")
	}
}

func TestPromptWithNoSystemPrompt(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/simple-prompt",
			Name:    "simple-prompt",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{
					Name:         "userPrompt",
					Content:      "Hello {{name}}",
					IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// User prompt input type emitted
	assertContains(t, output, "class TestSimplePrompt_UserPromptInput(TypedDict):")
	// No system prompt entrypoint, so no input type for it
	assertNotContains(t, output, "TestSimplePrompt_SystemPromptInput")

	// render overload only for userPrompt
	assertContains(t, output, "def render(self, entrypoint: Literal[\"userPrompt\"], input: TestSimplePrompt_UserPromptInput) -> PromptOutput:")
	assertNotContains(t, output, "Literal[\"systemPrompt\"]")
}

func TestEntrypointWithoutInput(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/no-input",
			Name:    "no-input",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Hi there", IsEntrypoint: true},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// No input type
	assertNotContains(t, output, "TestNoInput_UserPromptInput")
	// Overload has no input parameter
	assertContains(t, output, "def render(self, entrypoint: Literal[\"userPrompt\"]) -> PromptOutput:")
}

func TestCustomEntrypointName(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/multi-entry",
			Name:    "multi-entry",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{
					Name:         "assistantPrompt",
					Content:      "Assist with {{topic}}",
					IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"topic": map[string]interface{}{"type": "string"},
						},
					},
				},
				{
					Name:         "toolCallPrompt",
					Content:      "Call tool",
					IsEntrypoint: true,
				},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Custom entrypoint produces input TypedDict named after the entrypoint
	assertContains(t, output, "class TestMultiEntry_AssistantPromptInput(TypedDict):")
	assertContains(t, output, "topic: str")

	// Both render overloads present
	assertContains(t, output, "def render(self, entrypoint: Literal[\"assistantPrompt\"], input: TestMultiEntry_AssistantPromptInput) -> PromptOutput:")
	assertContains(t, output, "def render(self, entrypoint: Literal[\"toolCallPrompt\"]) -> PromptOutput:")

	// Both keys present in templates
	assertContains(t, output, `"assistantPrompt": "Assist with {{topic}}"`)
	assertContains(t, output, `"toolCallPrompt": "Call tool"`)
}

func TestPartialsAreNonEntrypoints(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/with-partial",
			Name:    "with-partial",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Use {{>greeting}}", IsEntrypoint: true},
				{Name: "greeting", Content: "Hello!"},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Partial appears in _partials, not _templates
	templatesSection := extractSection(output, "_templates: dict[str, dict[str, str]]", "}")
	assertNotContains(t, templatesSection, "\"greeting\":")
	partialsSection := extractSection(output, "_partials: dict[str, dict[str, str]]", "}")
	assertContains(t, partialsSection, `"greeting": "Hello!"`)
}

func TestDraftPrompt(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/draft-prompt",
			Name:    "draft-prompt",
			Version: "draft",
			Status:  "DRAFT",
			Files:   []generator.PromptFile{{Name: "userPrompt", Content: "Draft content", IsEntrypoint: true}},
		},
	}

	output := generateAndRead(t, prompts)

	assertContains(t, output, "_draft_prompts")
	draftSection := extractSection(output, "_draft_prompts", "}")
	assertContains(t, draftSection, "\"@test/draft-prompt\"")
}

func TestSchemaTypeMappings(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]interface{}
		expected string
	}{
		{
			name:     "string",
			schema:   map[string]interface{}{"type": "string"},
			expected: "str",
		},
		{
			name:     "integer",
			schema:   map[string]interface{}{"type": "integer"},
			expected: "int",
		},
		{
			name:     "number",
			schema:   map[string]interface{}{"type": "number"},
			expected: "float",
		},
		{
			name:     "boolean",
			schema:   map[string]interface{}{"type": "boolean"},
			expected: "bool",
		},
		{
			name:     "empty schema falls through to Any",
			schema:   map[string]interface{}{},
			expected: "Any",
		},
		{
			name: "array of strings",
			schema: map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			expected: "list[str]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var classes []typedDictClass
			result := collectTypedDicts(tt.schema, "Test", &classes, false)
			if result != tt.expected {
				t.Errorf("collectTypedDicts() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestStringEscaping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"double quote", `Hello "world"`, `Hello \"world\"`},
		{"backslash", `path\to\file`, `path\\to\\file`},
		{"newline", "Hello\nworld", `Hello\nworld`},
		{"tab", "Hello\tworld", `Hello\tworld`},
		{"carriage return", "Hello\rworld", `Hello\rworld`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeForPythonString(tt.input)
			if result != tt.expected {
				t.Errorf("escapeForPythonString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPascalCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"email-subject-generator", "EmailSubjectGenerator"},
		{"simple", "Simple"},
		{"my_prompt", "MyPrompt"},
		{"already-PascalCase", "AlreadyPascalCase"},
		{"a-b-c", "ABC"},
		{"@wtomas/my-prompt", "WtomasMyPrompt"},
		{"@acme-corp/support-reply", "AcmeCorpSupportReply"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toPascalCase(tt.input)
			if result != tt.expected {
				t.Errorf("toPascalCase(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFieldDescriptions(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/desc-prompt",
			Name:    "desc-prompt",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{
					Name:         "userPrompt",
					Content:      "Hi {{name}}",
					IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{
								"type":        "string",
								"description": "The user's full name",
							},
						},
					},
				},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Attribute docstrings should appear after field annotation
	assertContains(t, output, "name: str")
	assertContains(t, output, "\"\"\"The user's full name\"\"\"")
}

func TestExtractMetadataValues(t *testing.T) {
	input := map[string]interface{}{
		"model":       map[string]interface{}{"type": "string", "value": "gpt-4o"},
		"temperature": map[string]interface{}{"type": "integer", "value": float64(0)},
		"plain":       "already-flat",
	}

	result := extractMetadataValues(input)

	if result["model"] != "gpt-4o" {
		t.Errorf("model = %v, want gpt-4o", result["model"])
	}
	if result["temperature"] != float64(0) {
		t.Errorf("temperature = %v, want 0", result["temperature"])
	}
	if result["plain"] != "already-flat" {
		t.Errorf("plain = %v, want already-flat", result["plain"])
	}
}

func TestExtractMetadataValuesNil(t *testing.T) {
	result := extractMetadataValues(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestGenerateCreatesParentDirs(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "nested", "output", "prompts.py")
	g := &Generator{}
	err := g.Generate(outFile, []generator.PromptData{
		{
			Ref:     "@test/test",
			Name:    "test",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files:   []generator.PromptFile{{Name: "userPrompt", Content: "Hello", IsEntrypoint: true}},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("output file not created: %v", err)
	}
}

func TestRefUsedAsKey(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@wtomas/my-prompt",
			Name:    "my-prompt",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{
					Name:         "userPrompt",
					Content:      "Hello {{name}}",
					IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Ref should be used as the key everywhere
	assertContains(t, output, "\"@wtomas/my-prompt\"")
	// PascalCase should derive from ref
	assertContains(t, output, "WtomasMyPrompt_UserPromptInput")
}

func TestFallbackToNameWhenNoRef(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Name:    "legacy-prompt",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{
					Name:         "userPrompt",
					Content:      "Hello {{name}}",
					IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}

	output := generateAndRead(t, prompts)

	assertContains(t, output, "\"legacy-prompt\"")
	assertContains(t, output, "LegacyPrompt_UserPromptInput")
}

func TestNestedObjectTypedDict(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/nested",
			Name:    "nested",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{
					Name:         "userPrompt",
					Content:      "{{outer.inner.value}}",
					IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"outer": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"inner": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"value": map[string]interface{}{
												"type": "string",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Top-level TypedDict (no _ prefix)
	assertContains(t, output, "class TestNested_UserPromptInput(TypedDict):")
	// Nested objects get _ prefix
	assertContains(t, output, "class _TestNested_UserPromptInput_Outer(TypedDict):")
	assertContains(t, output, "class _TestNested_UserPromptInput_Outer_Inner(TypedDict):")
	// Deepest field
	assertContains(t, output, "value: str")
}

func TestOverloadSignatures(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@acme/prompt-a",
			Name:    "prompt-a",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files:   []generator.PromptFile{{Name: "userPrompt", Content: "A", IsEntrypoint: true}},
		},
		{
			Ref:     "@acme/prompt-b",
			Name:    "prompt-b",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files:   []generator.PromptFile{{Name: "userPrompt", Content: "B", IsEntrypoint: true}},
		},
	}

	output := generateAndRead(t, prompts)

	assertContains(t, output, "@overload")
	assertContains(t, output, "def get_prompt(prompt_name: Literal[\"@acme/prompt-a\"]) -> _AcmePromptAResult: ...")
	assertContains(t, output, "def get_prompt(prompt_name: Literal[\"@acme/prompt-b\"]) -> _AcmePromptBResult: ...")
}

func TestResultTypeClasses(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/with-input",
			Name:    "with-input",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{
					Name:         "userPrompt",
					Content:      "Hello {{name}}",
					IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Result class with typed render overload
	assertContains(t, output, "class _TestWithInputResult:")
	assertContains(t, output, "def render(self, entrypoint: Literal[\"userPrompt\"], input: TestWithInput_UserPromptInput) -> PromptOutput:")
}

func TestTypedMetadata(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/meta-prompt",
			Name:    "meta-prompt",
			Version: "2.0.0",
			Status:  "PUBLISHED",
			Metadata: map[string]interface{}{
				"model":       map[string]interface{}{"type": "string", "value": "gpt-4o"},
				"temperature": map[string]interface{}{"type": "integer", "value": float64(0)},
				"stream":      map[string]interface{}{"type": "boolean", "value": true},
			},
			Files: []generator.PromptFile{{Name: "userPrompt", Content: "Hello", IsEntrypoint: true}},
		},
	}

	output := generateAndRead(t, prompts)

	// Metadata TypedDict should be generated with types from hints
	assertContains(t, output, "class _TestMetaPromptMetadata(TypedDict):")
	assertContains(t, output, "model: str")
	assertContains(t, output, "stream: bool")
	assertContains(t, output, "temperature: int")
	assertContains(t, output, "version: str")

	// Result class should reference the typed metadata
	assertContains(t, output, "metadata: _TestMetaPromptMetadata")
}

func TestPyTypeFromMetaEntry(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		// Wrapped entries use the "type" hint
		{"wrapped string", map[string]interface{}{"type": "string", "value": "gpt-4o"}, "str"},
		{"wrapped integer", map[string]interface{}{"type": "integer", "value": float64(0)}, "int"},
		{"wrapped float", map[string]interface{}{"type": "float", "value": float64(0.7)}, "float"},
		{"wrapped boolean", map[string]interface{}{"type": "boolean", "value": true}, "bool"},
		// Flat values fall back to runtime type inference
		{"flat string", "hello", "str"},
		{"flat float64", float64(42), "int | float"},
		{"flat bool", true, "bool"},
		{"flat nil", nil, "None"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pyTypeFromMetaEntry(tt.input)
			if result != tt.expected {
				t.Errorf("pyTypeFromMetaEntry(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPyMetadataValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"string", "gpt-4o", "\"gpt-4o\""},
		{"integer", float64(42), "42"},
		{"float", 0.7, "0.7"},
		{"bool true", true, "True"},
		{"bool false", false, "False"},
		{"nil", nil, "None"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pyMetadataValue(tt.input)
			if result != tt.expected {
				t.Errorf("pyMetadataValue(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestOutputSchema_SinglePrompt(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/structured-output",
			Name:    "structured-output",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"summary": map[string]interface{}{"type": "string"},
					"score":   map[string]interface{}{"type": "number"},
				},
				"required": []interface{}{"summary", "score"},
			},
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Summarize this", IsEntrypoint: true},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Pydantic model class
	assertContains(t, output, "class TestStructuredOutputOutput(BaseModel):")
	assertContains(t, output, "score: float")
	assertContains(t, output, "summary: str")

	// Pydantic import
	assertContains(t, output, "from pydantic import BaseModel, ValidationError")

	// json/re imports
	assertContains(t, output, "import json")
	assertContains(t, output, "import re")

	// Per-prompt ParseSuccess TypedDict + shared ParseFailure TypedDict
	assertContains(t, output, "class _TestStructuredOutputParseSuccess(TypedDict):")
	assertContains(t, output, "data: TestStructuredOutputOutput")
	assertContains(t, output, "success: Literal[True]")
	assertContains(t, output, "class ParseFailure(TypedDict):")
	assertContains(t, output, "success: Literal[False]")

	// parse_output method on per-prompt result class
	assertContains(t, output, "def parse_output(self, raw: str)")
	assertContains(t, output, "TestStructuredOutputOutput.model_validate(parsed)")

	// _output_models dict
	assertContains(t, output, "_output_models")
	assertContains(t, output, `"@test/structured-output": TestStructuredOutputOutput`)

	// metadata output_schema
	assertContains(t, output, `"output_schema": json.loads(`)

	// typing imports include Optional, Union
	assertContains(t, output, "Optional, Union")

	// Typed parse_output return uses per-prompt TypedDict
	assertContains(t, output, "_TestStructuredOutputParseSuccess | ParseFailure")
}

func TestOutputSchema_MixedPrompts(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/with-schema",
			Name:    "with-schema",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"result": map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"result"},
			},
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Do something", IsEntrypoint: true},
			},
		},
		{
			Ref:     "@test/without-schema",
			Name:    "without-schema",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Do something else", IsEntrypoint: true},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// With-schema prompt has Pydantic model
	assertContains(t, output, "class TestWithSchemaOutput(BaseModel):")
	// Without-schema prompt has no Pydantic model
	assertNotContains(t, output, "TestWithoutSchemaOutput")

	// _output_models only has the schema prompt
	outputModelsSection := extractSection(output, "_output_models", "}")
	assertContains(t, outputModelsSection, `"@test/with-schema": TestWithSchemaOutput`)
	assertNotContains(t, outputModelsSection, `"@test/without-schema"`)

	// Only with-schema prompt has output_schema in metadata
	metadataSection := extractSection(output, "_metadata:", "}")
	withSchemaSection := extractSection(metadataSection, `"@test/with-schema"`, "},")
	assertContains(t, withSchemaSection, "output_schema")
	withoutSchemaSection := extractSection(metadataSection, `"@test/without-schema"`, "},")
	assertNotContains(t, withoutSchemaSection, "output_schema")

	// Only with-schema result class has parse_output
	withSchemaResult := extractSection(output, "class _TestWithSchemaResult:", "class _TestWithoutSchemaResult:")
	assertContains(t, withSchemaResult, "def parse_output")
	withoutSchemaResult := extractSection(output, "class _TestWithoutSchemaResult:", "# ─── Overloads")
	assertNotContains(t, withoutSchemaResult, "def parse_output")
}

func TestOutputSchema_NoPromptHasSchema(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/plain-prompt",
			Name:    "plain-prompt",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Hello world", IsEntrypoint: true},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// No pydantic import
	assertNotContains(t, output, "from pydantic import")
	// No ParseSuccess/ParseFailure
	assertNotContains(t, output, "ParseSuccess")
	assertNotContains(t, output, "class ParseFailure")
	// No parse_output
	assertNotContains(t, output, "parse_output")
	// No _output_models
	assertNotContains(t, output, "_output_models")
	// No json import (not needed without output schema)
	assertNotContains(t, output, "import json")
	// Backward compatible — still has standard imports
	assertContains(t, output, "import chevron")
}

func TestOutputSchema_NestedObject(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/nested-output",
			Name:    "nested-output",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"user": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{"type": "string"},
							"age":  map[string]interface{}{"type": "integer"},
						},
						"required": []interface{}{"name"},
					},
				},
				"required": []interface{}{"user"},
			},
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Get user info", IsEntrypoint: true},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Multiple Pydantic classes with leaf-first ordering
	assertContains(t, output, "class _TestNestedOutputOutput_User(BaseModel):")
	assertContains(t, output, "class TestNestedOutputOutput(BaseModel):")

	// Leaf class should come before root class
	innerIdx := strings.Index(output, "_TestNestedOutputOutput_User")
	outerIdx := strings.Index(output, "class TestNestedOutputOutput(BaseModel):")
	if innerIdx > outerIdx {
		t.Error("expected leaf-first ordering (_TestNestedOutputOutput_User before TestNestedOutputOutput)")
	}

	// Fields in nested class
	innerSection := extractSection(output, "class _TestNestedOutputOutput_User(BaseModel):", "class TestNestedOutputOutput")
	assertContains(t, innerSection, "name: str")
	assertContains(t, innerSection, "age: Optional[int] = None")
}

func TestOutputSchema_DirectiveResolution(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/directive-prompt",
			Name:    "directive-prompt",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"answer": map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"answer"},
			},
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Respond with this schema: {{@outputSchema}}", IsEntrypoint: true},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Directive should be replaced with actual schema JSON
	assertNotContains(t, output, "{{@outputSchema}}")
	assertContains(t, output, `answer`)
}

func TestUnknownEntrypointError(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/unknown-entry",
			Name:    "unknown-entry",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Hello", IsEntrypoint: true},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Per-prompt result class: explicit guard + branded KeyError, prompt name baked in.
	assertContains(t, output, "template = self._templates.get(entrypoint)")
	assertContains(t, output, `raise KeyError(f'[sufleur] Unknown entrypoint "{entrypoint}" for prompt "@test/unknown-entry"')`)

	// Inner _PromptResult inside get_prompt: same guard, prompt_name from closure.
	assertContains(t, output, "template = templates.get(entrypoint)")
	assertContains(t, output, `raise KeyError(f'[sufleur] Unknown entrypoint "{entrypoint}" for prompt "{prompt_name}"')`)

	// The old unchecked lookups are gone from both render bodies.
	assertNotContains(t, output, "chevron.render(self._templates[entrypoint]")
	assertNotContains(t, output, "chevron.render(templates[entrypoint]")
}

// Helpers

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("output does not contain %q\n\nFull output:\n%s", substr, s)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("output should not contain %q", substr)
	}
}

func extractSection(s, startMarker, endMarker string) string {
	startIdx := strings.Index(s, startMarker)
	if startIdx == -1 {
		return ""
	}
	endIdx := strings.Index(s[startIdx:], endMarker)
	if endIdx == -1 {
		return s[startIdx:]
	}
	return s[startIdx : startIdx+endIdx+len(endMarker)]
}
