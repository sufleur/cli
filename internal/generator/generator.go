// Package generator defines the interface and types for code generation.
package generator

import (
	"encoding/json"
	"regexp"
	"strings"
)

// PromptData holds the resolved prompt information needed for code generation.
type PromptData struct {
	Ref          string // full "@workspace/prompt-name" reference
	Name         string
	Version      string
	Description  string
	Status       string                 // "PUBLISHED" or "DRAFT"
	Metadata     map[string]interface{} // model, provider, temperature, etc.
	OutputSchema map[string]interface{} // nullable JSON Schema for structured output
	ModelConfig  map[string]interface{} // provider, model, parameters (nullable)
	Files        []PromptFile
}

// PromptFile represents a single file within a prompt version.
// Entrypoint files are rendered with Mustache; non-entrypoint files are partials.
type PromptFile struct {
	Name           string
	Content        string
	IsEntrypoint   bool
	InputSchema    map[string]interface{} // nullable; populated only for entrypoints
	SchemaWarnings []SchemaWarning        // empty for non-entrypoints
}

// SchemaWarning is a single warning produced during input-schema inference.
type SchemaWarning struct {
	Path    string
	Message string
}

// Generator produces language-specific code from prompt data.
type Generator interface {
	// Generate writes generated code to the given file path.
	Generate(outFile string, prompts []PromptData) error
}

// outputSchemaDirective matches the {{@outputSchema}} directive, tolerating
// whitespace inside the braces ({{ @outputSchema }}).
var outputSchemaDirective = regexp.MustCompile(`\{\{\s*@outputSchema\s*\}\}`)

// ReplaceOutputSchemaDirective replaces every {{@outputSchema}} directive with
// replacement, inserted literally (no $-expansion). Shared with internal/render
// so local rendering and codegen resolve the directive identically.
func ReplaceOutputSchemaDirective(content, replacement string) string {
	return outputSchemaDirective.ReplaceAllLiteralString(content, replacement)
}

// ResolveDirectives substitutes platform directives ({{@...}}) in template content
// at codegen time. The engineer's runtime never sees these directives.
func ResolveDirectives(content string, pd PromptData) string {
	if !strings.Contains(content, "@outputSchema") {
		return content
	}

	var replacement string
	if pd.OutputSchema != nil {
		rendered, err := json.MarshalIndent(pd.OutputSchema, "", "  ")
		if err == nil {
			replacement = string(rendered)
		}
	}

	return ReplaceOutputSchemaDirective(content, replacement)
}
