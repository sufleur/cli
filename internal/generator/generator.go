// Package generator defines the interface and types for code generation.
package generator

import (
	"encoding/json"
	"strings"
)

// PromptData holds the resolved prompt information needed for code generation.
type PromptData struct {
	Ref                     string // full "@workspace/prompt-name" reference
	Name                    string
	Version                 string
	Description             string
	Status                  string                 // "PUBLISHED" or "DRAFT"
	Metadata                map[string]interface{} // model, provider, temperature, etc.
	UserPromptInputSchema   map[string]interface{}
	SystemPromptInputSchema map[string]interface{}
	OutputSchema            map[string]interface{} // nullable JSON Schema for structured output
	Files                   []PromptFile
}

// PromptFile represents a single file within a prompt version.
type PromptFile struct {
	Name    string
	Content string
}

// Generator produces language-specific code from prompt data.
type Generator interface {
	// Generate writes generated code to the given file path.
	Generate(outFile string, prompts []PromptData) error
}

// ResolveDirectives substitutes platform directives ({{@...}}) in template content
// at codegen time. The engineer's runtime never sees these directives.
func ResolveDirectives(content string, pd PromptData) string {
	if !strings.Contains(content, "{{@") {
		return content
	}

	var replacement string
	if pd.OutputSchema != nil {
		rendered, err := json.MarshalIndent(pd.OutputSchema, "", "  ")
		if err == nil {
			replacement = string(rendered)
		}
	}

	content = strings.ReplaceAll(content, "{{@outputSchema}}", replacement)
	return content
}
