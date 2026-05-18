package typescript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/generator"
	"github.com/sufleur/cli/internal/generator/parser"
)

func generateAndRead(t *testing.T, prompts []generator.PromptData) string {
	t.Helper()
	outFile := filepath.Join(t.TempDir(), "prompts.ts")
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

	// Per-entrypoint input types use PascalCase from full ref + entrypoint name
	assertContains(t, output, "export type WtomasEmailSubjectGenerator_UserPromptInput =")
	assertContains(t, output, "export type WtomasEmailSubjectGenerator_SystemPromptInput =")
	assertContains(t, output, "name: string;")
	assertContains(t, output, "age: number;")
	assertContains(t, output, "tone: string;")

	// PromptName union uses full ref
	assertContains(t, output, "'@wtomas/email-subject-generator'")

	// EntrypointMapping wires entrypoints to inputs
	assertContains(t, output, "export interface EntrypointMapping {")
	assertContains(t, output, "'userPrompt': WtomasEmailSubjectGenerator_UserPromptInput;")
	assertContains(t, output, "'systemPrompt': WtomasEmailSubjectGenerator_SystemPromptInput;")

	// Templates contain raw Mustache content under entrypoint keys
	assertContains(t, output, "Hello {{user.name}}, you are {{user.age}} years old.")
	assertContains(t, output, "You are a helpful assistant with {{tone}} tone.")

	// Metadata
	assertContains(t, output, "model: 'gpt-4o'")
	assertContains(t, output, "temperature: 0")
	assertContains(t, output, "version: '1.4.2'")

	// render method on the result type
	assertContains(t, output, "render: <E extends keyof EntrypointMapping[N] & string>")

	// getPrompt entry point
	assertContains(t, output, "export function getPrompt")

	// Mustache import
	assertContains(t, output, "import Mustache from 'mustache'")
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

	// Both appear in PromptName union
	assertContains(t, output, "'@acme/alpha-prompt'")
	assertContains(t, output, "'@acme/beta-prompt'")

	// Both appear in templates
	assertContains(t, output, "'@acme/alpha-prompt': {")
	assertContains(t, output, "'@acme/beta-prompt': {")

	// Sorted alphabetically — alpha should come first
	alphaIdx := strings.Index(output, "'@acme/alpha-prompt'")
	betaIdx := strings.Index(output, "'@acme/beta-prompt'")
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
							"name": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// User prompt input type emitted
	assertContains(t, output, "export type TestSimplePrompt_UserPromptInput =")
	// No system prompt entrypoint, so no input type for it
	assertNotContains(t, output, "TestSimplePrompt_SystemPromptInput")
	// EntrypointMapping has no systemPrompt entry (only userPrompt)
	assertNotContains(t, output, "'systemPrompt':")
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

	// No input interface — entrypoint maps to void
	assertNotContains(t, output, "TestNoInput_UserPromptInput")
	assertContains(t, output, "'userPrompt': void;")
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

	// Custom entrypoint names produce input types named after the entrypoint
	assertContains(t, output, "export type TestMultiEntry_AssistantPromptInput =")
	assertContains(t, output, "topic: string;")

	// Both entrypoints appear in EntrypointMapping
	assertContains(t, output, "'assistantPrompt': TestMultiEntry_AssistantPromptInput;")
	assertContains(t, output, "'toolCallPrompt': void;")

	// Both keys present in templates
	assertContains(t, output, "'assistantPrompt': `Assist with {{topic}}`,")
	assertContains(t, output, "'toolCallPrompt': `Call tool`,")
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
	templatesSection := extractSection(output, "const _templates", "};")
	assertNotContains(t, templatesSection, "'greeting':")
	partialsSection := extractSection(output, "const _partials", "};")
	assertContains(t, partialsSection, "'greeting': `Hello!`")
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

	assertContains(t, output, "_draftPrompts")
	assertContains(t, output, "'@test/draft-prompt'")
	draftSection := extractSection(output, "_draftPrompts", "]);")
	assertContains(t, draftSection, "'@test/draft-prompt'")
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
			expected: "string",
		},
		{
			name:     "integer",
			schema:   map[string]interface{}{"type": "integer"},
			expected: "number",
		},
		{
			name:     "number",
			schema:   map[string]interface{}{"type": "number"},
			expected: "number",
		},
		{
			name:     "boolean",
			schema:   map[string]interface{}{"type": "boolean"},
			expected: "boolean",
		},
		{
			name:     "empty schema falls through to unknown",
			schema:   map[string]interface{}{},
			expected: "unknown",
		},
		{
			name: "array of strings",
			schema: map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			expected: "string[]",
		},
		{
			name: "nested object",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"inner": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"value": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
			expected: "inner: {\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := schemaToTSType(tt.schema, 0)
			if !strings.Contains(result, tt.expected) {
				t.Errorf("schemaToTSType() = %q, want it to contain %q", result, tt.expected)
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
		{"backtick", "Hello `world`", "Hello \\`world\\`"},
		{"dollar brace", "Hello ${name}", "Hello \\${name}"},
		{"backslash", "path\\to\\file", "path\\\\to\\\\file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeForTSTemplateLiteral(tt.input)
			if result != tt.expected {
				t.Errorf("escapeForTSTemplateLiteral(%q) = %q, want %q", tt.input, result, tt.expected)
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

func TestJSDocAnnotations(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:         "@test/documented-prompt",
			Name:        "documented-prompt",
			Version:     "3.0.0",
			Description: "A well documented prompt",
			Status:      "PUBLISHED",
			Files:       []generator.PromptFile{{Name: "userPrompt", Content: "Hello", IsEntrypoint: true}},
		},
	}

	output := generateAndRead(t, prompts)

	assertContains(t, output, "* A well documented prompt")
	assertContains(t, output, "@version 3.0.0")
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

	assertContains(t, output, "/** The user's full name */")
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
	outFile := filepath.Join(t.TempDir(), "nested", "output", "prompts.ts")
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
	assertContains(t, output, "'@wtomas/my-prompt'")
	// PascalCase derives from ref + entrypoint name
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

	assertContains(t, output, "'legacy-prompt'")
	assertContains(t, output, "LegacyPrompt_UserPromptInput")
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

	// Zod import
	assertContains(t, output, "import { z } from 'zod'")

	// Zod schema constant
	assertContains(t, output, "export const TestStructuredOutputOutputSchema = z.object({")
	assertContains(t, output, "score: z.number(),")
	assertContains(t, output, "summary: z.string(),")

	// Inferred type
	assertContains(t, output, "export type TestStructuredOutputOutput = z.infer<typeof TestStructuredOutputOutputSchema>")

	// ParseResult type
	assertContains(t, output, "export type ParseResult<T>")

	// OutputMapping
	assertContains(t, output, "OutputMapping")
	assertContains(t, output, "'@test/structured-output': TestStructuredOutputOutput")

	// parseOutput in PromptResult type
	assertContains(t, output, "parseOutput(raw: string): ParseResult<OutputMapping[N]>")

	// metadata.outputSchema
	assertContains(t, output, "outputSchema:")

	// _outputSchemas lookup
	assertContains(t, output, "_outputSchemas")

	// parseOutput implementation
	assertContains(t, output, "const parseOutput = (raw: string): ParseResult<OutputMapping[N]>")
	assertContains(t, output, "schema.safeParse")
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

	// With-schema prompt has output type
	assertContains(t, output, "TestWithSchemaOutputSchema")
	assertContains(t, output, "TestWithSchemaOutput")

	// Without-schema prompt mapped to never
	assertContains(t, output, "'@test/without-schema': never")
	assertContains(t, output, "'@test/with-schema': TestWithSchemaOutput")

	// Zod import present
	assertContains(t, output, "import { z } from 'zod'")

	// No output schema constant for the without-schema prompt
	assertNotContains(t, output, "TestWithoutSchemaOutputSchema")

	// Only the with-schema prompt has outputSchema in metadata
	metadataSection := extractSection(output, "const _metadata", "as const;")
	withSchemaSection := extractSection(metadataSection, "'@test/with-schema'", "},")
	assertContains(t, withSchemaSection, "outputSchema:")
	withoutSchemaSection := extractSection(metadataSection, "'@test/without-schema'", "},")
	assertNotContains(t, withoutSchemaSection, "outputSchema:")
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

	// No zod import
	assertNotContains(t, output, "import { z }")
	// No output-related types
	assertNotContains(t, output, "OutputSchema")
	assertNotContains(t, output, "ParseResult")
	assertNotContains(t, output, "OutputMapping")
	assertNotContains(t, output, "parseOutput")
	assertNotContains(t, output, "_outputSchemas")
	// Original interface keyword used (not type alias)
	assertContains(t, output, "interface PromptResult<N extends PromptName>")
}

func TestOutputSchema_ArrayTopLevel(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/array-output",
			Name:    "array-output",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			OutputSchema: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{"type": "string"},
					},
					"required": []interface{}{"name"},
				},
			},
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "List items", IsEntrypoint: true},
			},
		},
	}

	output := generateAndRead(t, prompts)

	assertContains(t, output, "TestArrayOutputOutputSchema = z.array(")
	assertContains(t, output, "TestArrayOutputOutput")
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
	assertContains(t, output, `"answer"`)
	assertContains(t, output, `"type"`)
}

func TestStrictModeCompliance(t *testing.T) {
	t.Run("with output schema branch", func(t *testing.T) {
		prompts := []generator.PromptData{
			{
				Ref:     "@test/strict-with-output",
				Name:    "strict-with-output",
				Version: "1.0.0",
				Status:  "PUBLISHED",
				OutputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"value": map[string]interface{}{"type": "string"},
					},
					"required": []interface{}{"value"},
				},
				Files: []generator.PromptFile{
					{Name: "userPrompt", Content: "Hello", IsEntrypoint: true},
				},
			},
		}

		output := generateAndRead(t, prompts)

		// Runtime throw guards the unknown-entrypoint case.
		assertContains(t, output, "if (template === undefined)")
		assertContains(t, output, "throw new Error(`[sufleur] Unknown entrypoint")

		// The output parser routes fence-extraction failures through the `code`
		// discriminator instead of throwing (see TestParseOutputResilience).

		// Old `any`-based shape is gone.
		assertNotContains(t, output, "const result: any")
		assertNotContains(t, output, "as any")
		assertNotContains(t, output, "input?: any")
		assertNotContains(t, output, "ParseResult<any>")

		// Typed inner render and parseOutput take the place of the old result-cast pattern.
		assertContains(t, output, "const render = <E extends keyof EntrypointMapping[N] & string>")
		assertContains(t, output, "const parseOutput = (raw: string): ParseResult<OutputMapping[N]>")
	})

	t.Run("no output schema branch", func(t *testing.T) {
		prompts := []generator.PromptData{
			{
				Ref:     "@test/strict-no-output",
				Name:    "strict-no-output",
				Version: "1.0.0",
				Status:  "PUBLISHED",
				Files: []generator.PromptFile{
					{Name: "userPrompt", Content: "Hello", IsEntrypoint: true},
				},
			},
		}

		output := generateAndRead(t, prompts)

		// Throw guard still present in the no-output branch.
		assertContains(t, output, "if (template === undefined)")
		assertContains(t, output, "throw new Error(`[sufleur] Unknown entrypoint")

		// No-output branch must not contain `any`-shaped fallbacks.
		assertNotContains(t, output, "as any")
		assertNotContains(t, output, "input?: any")
		assertNotContains(t, output, "parseOutput")
	})
}

func TestParseOutputResilience(t *testing.T) {
	prompts := []generator.PromptData{
		{
			Ref:     "@test/parse-resilience",
			Name:    "parse-resilience",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"value": map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"value"},
			},
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "Hello", IsEntrypoint: true},
			},
		},
	}

	output := generateAndRead(t, prompts)

	// Old anchored regex is gone, new unanchored form is present.
	assertNotContains(t, output, "/^`")
	assertNotContains(t, output, "`$/")
	assertContains(t, output, "const _fenceRe = /"+parser.FencePattern+"/g")

	// Pattern-emission: the Go const value appears verbatim in the output.
	assertContains(t, output, parser.FencePattern)

	// Helpers are emitted at module scope (under the AnyHasOutput gate).
	assertContains(t, output, "const _extractBalancedBraces = (s: string): string | null =>")
	assertContains(t, output, "const _extractJsonCandidate = (raw: string): { text: string; foundFence: boolean } =>")

	// ParseResult union carries the `code` discriminator.
	assertContains(t, output, "code: 'fence-extraction' | 'json-parse' | 'schema-validation'")

	// All three code literals appear at the call site in parseOutput.
	assertContains(t, output, "candidate.foundFence ? 'fence-extraction' : 'json-parse'")
	assertContains(t, output, "code: 'schema-validation'")

	// parseOutput uses the new helper, not the old inline regex.
	assertContains(t, output, "const candidate = _extractJsonCandidate(raw)")
}

// Helpers

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("output does not contain %q", substr)
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
