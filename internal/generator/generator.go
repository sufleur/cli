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
	// Tools are the tool contracts this version pins. Tagged so a version that
	// pins none marshals exactly as it did before tool support existed —
	// existing cache files and lockfile integrity hashes stay valid on upgrade.
	// The other fields are deliberately left untagged for the same reason.
	Tools []ToolPin `json:"tools,omitempty"`
}

// ToolPin is one prompt-version -> tool-version pin, with the tool's contract
// inlined. The backend resolves the pin to a concrete version and returns the
// contract alongside the prompt, so there is no separate tool to fetch, cache
// or lock.
//
// It is a value type: two prompts pinning the same tool version each carry
// their own copy through the prompt cache file. Emitting one set of types per
// distinct contract is a codegen-time concern, keyed on Ref+"@"+Version.
type ToolPin struct {
	// Alias is the wire name the model sees. It belongs to the pin, not the
	// tool: the same contract can be pinned under different names by different
	// prompts, which is how two tools sharing a bare name are told apart.
	Alias            string
	Ref              string                 // "@workspace/tool-name"
	Version          string                 // "1.2.0", or "draft"
	Status           string                 // "PUBLISHED" or "DRAFT"
	ModelDescription string                 // the model-facing text, versioned
	InputSchema      map[string]interface{} // arguments the model emits; always an object schema
	OutputSchema     map[string]interface{} // what the implementation returns; nullable
	Metadata         map[string]interface{} // carried for inspection; not emitted
}

// AliasRe is the wire-name rule every major provider enforces. It mirrors the
// backend's TOOL_ALIAS_PATTERN; the CLI re-checks it because a hand-edited
// cache file would otherwise produce a file the provider rejects at runtime.
var AliasRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// WireDescription returns the description sent to the model for a pinned tool.
//
// A tool's output schema is not part of any provider's tool definition, but the
// model still has to know what it will get back — so it is appended to the
// description, which is the only free-text channel available.
func WireDescription(p ToolPin) string {
	if p.OutputSchema == nil {
		return p.ModelDescription
	}
	compact, err := json.Marshal(p.OutputSchema)
	if err != nil {
		return p.ModelDescription
	}
	return p.ModelDescription + "\n\nReturns JSON matching: " + string(compact)
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
