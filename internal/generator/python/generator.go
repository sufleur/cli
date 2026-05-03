package python

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/WTomas/sufleur-cli/internal/generator"
)

func init() {
	generator.Register("python", func() generator.Generator { return &Generator{} })
}

// Generator produces Python code from prompt data.
type Generator struct{}

// typedDictField represents a single field in a TypedDict class.
type typedDictField struct {
	Name        string
	Type        string
	Description string
}

// typedDictClass represents a TypedDict class to be emitted.
// Name already includes the _ prefix for non-public (nested) classes.
type typedDictClass struct {
	Name   string
	Fields []typedDictField
}

// entrypointData describes a single render target within a prompt.
// Name is the runtime key (file name); InputTypeName is the TypedDict class name
// (empty when the entrypoint has no input schema).
type entrypointData struct {
	Name          string
	Template      string
	HasInput      bool
	InputTypeName string
}

// promptTemplateData is the data passed to the Go text/template.
type promptTemplateData struct {
	Name                string
	PascalName          string
	Description         string
	Version             string
	Status              string
	Metadata            map[string]interface{}
	MetadataTypeName    string
	Entrypoints         []entrypointData
	Partials            []partialData
	TypedDicts          []typedDictClass
	HasOutputSchema     bool
	OutputPydanticModel string
	OutputClassName     string
	OutputSchemaRaw     string
}

type partialData struct {
	Name    string
	Content string
}

type templateContext struct {
	Timestamp    string
	Prompts      []promptTemplateData
	AnyHasOutput bool
}

func (g *Generator) Generate(outFile string, prompts []generator.PromptData) error {
	if dir := filepath.Dir(outFile); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}

	data := buildTemplateData(prompts)

	tmpl, err := template.New("output").Funcs(template.FuncMap{
		"pyMetadataValue": pyMetadataValue,
	}).Parse(pythonTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	f, err := os.Create(outFile)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	return nil
}

// displayName returns the Ref when non-empty, otherwise falls back to Name.
func displayName(p generator.PromptData) string {
	if p.Ref != "" {
		return p.Ref
	}
	return p.Name
}

func buildTemplateData(prompts []generator.PromptData) templateContext {
	sort.Slice(prompts, func(i, j int) bool {
		return displayName(prompts[i]) < displayName(prompts[j])
	})

	var tds []promptTemplateData
	anyHasOutput := false
	for _, p := range prompts {
		dn := displayName(p)
		td := promptTemplateData{
			Name:        dn,
			PascalName:  toPascalCase(dn),
			Description: p.Description,
			Version:     p.Version,
			Status:      p.Status,
			Metadata:    extractMetadataValues(p.Metadata),
		}

		// Classify files by IsEntrypoint; resolve directives before escaping.
		for _, f := range p.Files {
			content := generator.ResolveDirectives(f.Content, p)
			escaped := escapeForPythonString(content)
			if f.IsEntrypoint {
				ep := entrypointData{
					Name:     f.Name,
					Template: escaped,
				}
				if f.InputSchema != nil {
					var classes []typedDictClass
					typeName := collectTypedDicts(f.InputSchema, td.PascalName+"_"+toPascalCase(f.Name)+"Input", &classes, true)
					ep.HasInput = true
					ep.InputTypeName = typeName
					td.TypedDicts = append(td.TypedDicts, classes...)
				}
				td.Entrypoints = append(td.Entrypoints, ep)
			} else {
				td.Partials = append(td.Partials, partialData{
					Name:    f.Name,
					Content: escaped,
				})
			}
		}

		// Sort entrypoints and partials for deterministic output.
		sort.Slice(td.Entrypoints, func(i, j int) bool {
			return td.Entrypoints[i].Name < td.Entrypoints[j].Name
		})
		sort.Slice(td.Partials, func(i, j int) bool {
			return td.Partials[i].Name < td.Partials[j].Name
		})

		// Output schema → Pydantic + raw JSON
		if p.OutputSchema != nil {
			td.HasOutputSchema = true
			models, topClass := jsonSchemaToPydantic(p.OutputSchema, td.PascalName+"Output")
			td.OutputPydanticModel = models
			td.OutputClassName = topClass
			if raw, err := json.Marshal(p.OutputSchema); err == nil {
				td.OutputSchemaRaw = string(raw)
			}
			anyHasOutput = true
		}

		// Build typed metadata TypedDict (use raw metadata to read type hints)
		metaClassName := "_" + td.PascalName + "Metadata"
		td.MetadataTypeName = metaClassName
		metaFields := buildMetadataFields(p.Metadata)
		if p.OutputSchema != nil {
			metaFields = append(metaFields, typedDictField{Name: "output_schema", Type: "dict[str, Any]"})
		}
		// Always include version
		metaFields = append(metaFields, typedDictField{Name: "version", Type: "str"})
		td.TypedDicts = append(td.TypedDicts, typedDictClass{
			Name:   metaClassName,
			Fields: metaFields,
		})

		tds = append(tds, td)
	}

	return templateContext{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Prompts:      tds,
		AnyHasOutput: anyHasOutput,
	}
}

// collectTypedDicts recursively walks a schema and emits TypedDict classes.
// It returns the Python type string for this schema node.
func collectTypedDicts(schema map[string]interface{}, namePrefix string, classes *[]typedDictClass, isTopLevel bool) string {
	kind, _ := schema["kind"].(string)

	switch kind {
	case "primitive":
		return primitiveToPython(schema)
	case "array":
		elementType, ok := schema["elementType"].(map[string]interface{})
		if !ok {
			return "list[Any]"
		}
		inner := collectTypedDicts(elementType, namePrefix, classes, false)
		return "list[" + inner + "]"
	case "object":
		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			return "dict[str, Any]"
		}

		// Sort keys for deterministic output
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// The class name for this object: top-level keeps the name as-is,
		// nested objects get a _ prefix to mark them private.
		className := namePrefix
		if !isTopLevel {
			className = "_" + namePrefix
		}

		var fields []typedDictField
		for _, k := range keys {
			v, ok := props[k].(map[string]interface{})
			if !ok {
				continue
			}
			// Child name prefix never includes _; that gets added by the recursive call.
			childName := namePrefix + "_" + toPascalCase(k)
			fieldType := collectTypedDicts(v, childName, classes, false)
			desc, _ := v["description"].(string)
			fields = append(fields, typedDictField{
				Name:        k,
				Type:        fieldType,
				Description: desc,
			})
		}

		cls := typedDictClass{
			Name:   className,
			Fields: fields,
		}
		*classes = append(*classes, cls)
		return className
	default:
		return "Any"
	}
}

func primitiveToPython(schema map[string]interface{}) string {
	t, _ := schema["type"].(string)
	switch t {
	case "string":
		return "str"
	case "int":
		return "int"
	case "float":
		return "float"
	case "boolean":
		return "bool"
	default:
		return "Any"
	}
}

// toPascalCase converts kebab-case, snake_case, or @workspace/name to PascalCase.
func toPascalCase(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		if r == '-' || r == '_' || r == '@' || r == '/' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(toUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

// escapeForPythonString escapes special characters for use inside a Python double-quoted string.
func escapeForPythonString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// buildMetadataFields creates typed fields from raw (unwrapped) metadata.
// It reads the "type" hint from {type, value} wrappers when available,
// falling back to runtime type inference for flat values.
func buildMetadataFields(meta map[string]interface{}) []typedDictField {
	if meta == nil {
		return nil
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var fields []typedDictField
	for _, k := range keys {
		fields = append(fields, typedDictField{
			Name: k,
			Type: pyTypeFromMetaEntry(meta[k]),
		})
	}
	return fields
}

// pyTypeFromMetaEntry determines the Python type for a metadata entry.
// If the entry is a {type, value} wrapper, the "type" hint is used.
// Otherwise the runtime Go type is used as a fallback.
func pyTypeFromMetaEntry(v interface{}) string {
	if wrapper, ok := v.(map[string]interface{}); ok {
		if hint, ok := wrapper["type"].(string); ok {
			return pyTypeFromHint(hint)
		}
	}
	return pyTypeFromValue(v)
}

// pyTypeFromHint maps a metadata type hint string to a Python type annotation.
func pyTypeFromHint(hint string) string {
	switch hint {
	case "string":
		return "str"
	case "integer":
		return "int"
	case "float":
		return "float"
	case "boolean":
		return "bool"
	default:
		return "Any"
	}
}

// pyTypeFromValue infers a Python type annotation from a Go interface{} value.
func pyTypeFromValue(v interface{}) string {
	switch v.(type) {
	case string:
		return "str"
	case float64:
		return "int | float"
	case bool:
		return "bool"
	case nil:
		return "None"
	default:
		return "Any"
	}
}

// extractMetadataValues unwraps the { type, value } wrappers from metadata fields.
func extractMetadataValues(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		if wrapper, ok := v.(map[string]interface{}); ok {
			if val, hasValue := wrapper["value"]; hasValue {
				result[k] = val
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}
	return result
}

// pyMetadataValue formats a Go value as a Python literal.
func pyMetadataValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("\"%s\"", strings.ReplaceAll(val, "\"", "\\\""))
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "True"
		}
		return "False"
	case nil:
		return "None"
	default:
		return fmt.Sprintf("\"%v\"", val)
	}
}

var pythonTemplate = `# ⚠️ AUTO-GENERATED by Sufleur CLI — do not edit manually
# Generated at: {{.Timestamp}}

from __future__ import annotations

import warnings
from typing import Any, Literal, TypedDict, overload{{if .AnyHasOutput}}, Optional, Union{{end}}

import chevron
{{- if .AnyHasOutput}}
import json
import re
from pydantic import BaseModel, ValidationError
{{- end}}

# ─── Types ────────────────────────────────────────────────────────────────────


class PromptOutput(TypedDict):
    prompt: str

{{range .Prompts}}
# ─── TypedDicts for {{.Name}} ────────────────────────────────────────────────
{{range .TypedDicts}}

class {{.Name}}(TypedDict):
{{- range .Fields}}
    {{.Name}}: {{.Type}}
    {{- if .Description}}
    """{{.Description}}"""
    {{- end}}
{{- end}}
{{end}}
{{- end}}
{{- if .AnyHasOutput}}
# ─── Output Models ────────────────────────────────────────────────────────────
{{range .Prompts}}
{{- if .HasOutputSchema}}

{{.OutputPydanticModel}}

class _{{.PascalName}}ParseSuccess(TypedDict):
    data: {{.OutputClassName}}
    success: Literal[True]
{{end}}
{{- end}}

class ParseFailure(TypedDict):
    error: str
    success: Literal[False]

{{end -}}
# ─── Prompt Name Literal ─────────────────────────────────────────────────────

PromptName = Literal[{{range $i, $p := .Prompts}}{{if $i}}, {{end}}"{{$p.Name}}"{{end}}]

# ─── Templates ────────────────────────────────────────────────────────────────

_templates: dict[str, dict[str, str]] = {
{{- range .Prompts}}
    "{{.Name}}": {
        {{- range .Entrypoints}}
        "{{.Name}}": "{{.Template}}",
        {{- end}}
    },
{{- end}}
}

# ─── Partials ─────────────────────────────────────────────────────────────────

_partials: dict[str, dict[str, str]] = {
{{- range .Prompts}}
    "{{.Name}}": {
        {{- range .Partials}}
        "{{.Name}}": "{{.Content}}",
        {{- end}}
    },
{{- end}}
}

# ─── Metadata ─────────────────────────────────────────────────────────────────

_metadata: dict[str, dict[str, Any]] = {
{{- range .Prompts}}
    "{{.Name}}": {
        {{- range $k, $v := .Metadata}}
        "{{$k}}": {{pyMetadataValue $v}},
        {{- end}}
        {{- if .HasOutputSchema}}
        "output_schema": json.loads('{{.OutputSchemaRaw}}'),
        {{- end}}
        "version": "{{.Version}}",
    },
{{- end}}
}
{{- if .AnyHasOutput}}

_output_models: dict[str, type[BaseModel]] = {
{{- range .Prompts}}
{{- if .HasOutputSchema}}
    "{{.Name}}": {{.OutputClassName}},
{{- end}}
{{- end}}
}
{{- end}}

# ─── Draft Prompts ────────────────────────────────────────────────────────────

_draft_prompts: set[str] = {
{{- range .Prompts}}
{{- if eq .Status "DRAFT"}}
    "{{.Name}}",
{{- end}}
{{- end}}
}

# ─── Per-prompt result types ─────────────────────────────────────────────────
{{range .Prompts}}

class _{{.PascalName}}Result:
    {{- if .Description}}
    """{{.Description}}

    Version: {{.Version}}
    """
    {{- end}}

    def __init__(self, templates: dict[str, str], partials: dict[str, str], metadata: {{.MetadataTypeName}}) -> None:
        self._templates = templates
        self._partials = partials
        self.metadata = metadata
    {{- range .Entrypoints}}

    @overload
    def render(self, entrypoint: Literal["{{.Name}}"]{{if .HasInput}}, input: {{.InputTypeName}}{{end}}) -> PromptOutput: ...
    {{- end}}

    def render(self, entrypoint: str, input: Any = None) -> PromptOutput:
        """Render the named entrypoint template with the given input."""
        return {"prompt": chevron.render(self._templates[entrypoint], input or {}, partials_dict=self._partials)}
    {{- if .HasOutputSchema}}

    def parse_output(self, raw: str) -> _{{.PascalName}}ParseSuccess | ParseFailure:
        """Parse and validate LLM output against the output schema."""
        text = raw.strip()
        fence_match = re.match(r"^` + "```" + `(?:\\w*)\\s*\\n?([\\s\\S]*?)\\n?\\s*` + "```" + `$", text)
        if fence_match:
            text = fence_match.group(1).strip()
        try:
            parsed = json.loads(text)
            validated = {{.OutputClassName}}.model_validate(parsed)
            return {"data": validated, "success": True}
        except (json.JSONDecodeError, ValidationError) as e:
            return {"error": str(e), "success": False}
    {{- end}}
{{end}}

# ─── Overloads + implementation ──────────────────────────────────────────────
{{range .Prompts}}

@overload
def get_prompt(prompt_name: Literal["{{.Name}}"]) -> _{{.PascalName}}Result: ...
{{end}}

def get_prompt(prompt_name: PromptName) -> Any:
    """Get a type-safe prompt by name.

    Returns a result object with a ` + "`render(entrypoint, input)`" + ` method.
    """
    if prompt_name in _draft_prompts:
        warnings.warn(
            f'[sufleur] Warning: prompt "{prompt_name}" is a draft version',
            stacklevel=2,
        )

    templates = _templates[prompt_name]
    partials = _partials.get(prompt_name, {})
    metadata = _metadata[prompt_name]

    class _PromptResult:
        def __init__(self) -> None:
            self.metadata = metadata

        def render(self, entrypoint: str, input: Any = None) -> PromptOutput:
            return {"prompt": chevron.render(templates[entrypoint], input or {}, partials_dict=partials)}
{{- if .AnyHasOutput}}

        def parse_output(self, raw: str) -> dict[str, Any]:
            model = _output_models.get(prompt_name)
            if model is None:
                return {"error": f"No output schema for prompt \"{prompt_name}\"", "success": False}
            text = raw.strip()
            fence_match = re.match(r"^` + "```" + `(?:\\w*)\\s*\\n?([\\s\\S]*?)\\n?\\s*` + "```" + `$", text)
            if fence_match:
                text = fence_match.group(1).strip()
            try:
                parsed = json.loads(text)
                validated = model.model_validate(parsed)
                return {"data": validated, "success": True}
            except (json.JSONDecodeError, ValidationError) as e:
                return {"error": str(e), "success": False}
{{- end}}

    return _PromptResult()
`
