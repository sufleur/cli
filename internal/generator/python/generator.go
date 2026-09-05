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

	"github.com/sufleur/cli/internal/generator"
	"github.com/sufleur/cli/internal/generator/parser"
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
	ModelConfigRaw      string
	HasTools            bool
	ToolsTypeName       string
	ToolBindings        []toolBindingData
	ToolDefsRaw         string
	DraftTools          []string
}

type partialData struct {
	Name    string
	Content string
}

// toolTemplateData is one distinct pinned contract, emitted once however many
// prompts pin it. Input comes from the model so it is a validating pydantic
// model; output comes from the engineer's own code so a TypedDict is enough.
type toolTemplateData struct {
	BaseName       string
	InputModel     string // pydantic class source
	InputClassName string
	OutputDicts    []typedDictClass
	OutputTypeName string
	Ref            string
	Version        string
}

// toolBindingData is one pin as seen by a single prompt.
type toolBindingData struct {
	Alias          string
	SafeAlias      string // a valid Python identifier, for local variable names
	BaseName       string
	InputClassName string
	IsDraft        bool
}

type templateContext struct {
	Timestamp         string
	Prompts           []promptTemplateData
	AnyHasOutput      bool
	AnyHasOptional    bool
	AnyHasUnion       bool
	AnyHasModelConfig bool
	AnyHasTools       bool
	AnyDraftTools     bool
	AnyDraftPrompts   bool
	Tools             []toolTemplateData
	FencePattern      string
}

// inputAnalysis is accumulated during input-schema traversal so the template
// knows which extra imports to emit (NotRequired, Optional, Union).
type inputAnalysis struct {
	HasOptional bool
	HasUnion    bool
}

func (g *Generator) Generate(outFile string, prompts []generator.PromptData) error {
	if dir := filepath.Dir(outFile); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}

	data, err := buildTemplateData(prompts)
	if err != nil {
		return err
	}

	tmpl, err := template.New("output").Funcs(template.FuncMap{
		"pyMetadataValue": pyMetadataValue,
		"pyDocstring":     pyDocstring,
		"pyStringLiteral": pyStringLiteral,
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

func buildTemplateData(prompts []generator.PromptData) (templateContext, error) {
	sort.Slice(prompts, func(i, j int) bool {
		return displayName(prompts[i]) < displayName(prompts[j])
	})

	plan, err := generator.PlanTools(prompts)
	if err != nil {
		return templateContext{}, err
	}

	var tds []promptTemplateData
	anyHasOutput := false
	anyHasModelConfig := false
	anyDraftTools := false
	anyDraftPrompts := false
	anyAnalysis := inputAnalysis{}
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
					typeName := collectTypedDicts(f.InputSchema, td.PascalName+"_"+toPascalCase(f.Name)+"Input", &classes, true, &anyAnalysis)
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

		// modelConfig → raw JSON, emitted verbatim via json.loads (parameters
		// stay camelCase; runtime dict key is "modelConfig" to match the TS output).
		if p.ModelConfig != nil {
			if raw, err := json.MarshalIndent(p.ModelConfig, "", "  "); err == nil {
				td.ModelConfigRaw = string(raw)
				anyHasModelConfig = true
			}
		}

		// Build typed metadata TypedDict (use raw metadata to read type hints)
		metaClassName := "_" + td.PascalName + "Metadata"
		td.MetadataTypeName = metaClassName
		metaFields := buildMetadataFields(p.Metadata)
		if p.OutputSchema != nil {
			metaFields = append(metaFields, typedDictField{Name: "outputSchema", Type: "dict[str, Any]"})
		}
		if td.ModelConfigRaw != "" {
			metaFields = append(metaFields, typedDictField{Name: "modelConfig", Type: "dict[str, Any]"})
		}
		// Always include version
		metaFields = append(metaFields, typedDictField{Name: "version", Type: "str"})
		td.TypedDicts = append(td.TypedDicts, typedDictClass{
			Name:   metaClassName,
			Fields: metaFields,
		})

		// Tool pins, sorted by wire name so the emitted bindings and dispatch
		// branches do not depend on the order the backend returned them in.
		for _, pin := range p.Tools {
			key := generator.ToolKey(pin)
			td.ToolBindings = append(td.ToolBindings, toolBindingData{
				Alias:          pin.Alias,
				SafeAlias:      safePyIdent(pin.Alias),
				BaseName:       plan.BaseNames[key],
				InputClassName: plan.BaseNames[key] + "Input",
				IsDraft:        pin.Status == "DRAFT",
			})
		}
		sort.Slice(td.ToolBindings, func(i, j int) bool {
			return td.ToolBindings[i].Alias < td.ToolBindings[j].Alias
		})
		if p.Status == "DRAFT" {
			anyDraftPrompts = true
		}
		td.HasTools = len(td.ToolBindings) > 0
		td.ToolsTypeName = td.PascalName + "Tools"
		td.DraftTools = generator.DraftToolAliases(p)
		if len(td.DraftTools) > 0 {
			anyDraftTools = true
		}
		if td.HasTools {
			td.ToolDefsRaw = toolDefsLiteral(p.Tools)
		}

		tds = append(tds, td)
	}

	tools := make([]toolTemplateData, 0, len(plan.Keys))
	for _, key := range plan.Keys {
		pin := plan.Pins[key]
		base := plan.BaseNames[key]

		inputModel, inputClass := jsonSchemaToPydantic(pin.InputSchema, base+"Input")

		outputTypeName := "Any"
		var outputDicts []typedDictClass
		if pin.OutputSchema != nil {
			outputTypeName = collectTypedDicts(pin.OutputSchema, base+"Output", &outputDicts, true, &anyAnalysis)
		}

		tools = append(tools, toolTemplateData{
			BaseName:       base,
			InputModel:     inputModel,
			InputClassName: inputClass,
			OutputDicts:    outputDicts,
			OutputTypeName: outputTypeName,
			Ref:            pin.Ref,
			Version:        pin.Version,
		})
	}

	ctx := templateContext{
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		Prompts:           tds,
		AnyHasOutput:      anyHasOutput,
		AnyHasOptional:    anyAnalysis.HasOptional,
		AnyHasUnion:       anyAnalysis.HasUnion,
		AnyHasModelConfig: anyHasModelConfig,
		AnyHasTools:       len(tools) > 0,
		AnyDraftTools:     anyDraftTools,
		AnyDraftPrompts:   anyDraftPrompts,
		Tools:             tools,
		FencePattern:      parser.FencePattern,
	}
	if err := assertNoIdentifierCollisions(ctx); err != nil {
		return templateContext{}, err
	}
	return ctx, nil
}

// toolDefsLiteral renders the provider-neutral definitions the model is offered,
// as a JSON string the generated module parses once at import time. A Python
// literal would need every JSON true/false/null rewritten; json.loads does not.
func toolDefsLiteral(pins []generator.ToolPin) string {
	sorted := make([]generator.ToolPin, len(pins))
	copy(sorted, pins)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Alias < sorted[j].Alias })

	defs := make([]map[string]interface{}, 0, len(sorted))
	for _, pin := range sorted {
		defs = append(defs, map[string]interface{}{
			"name":         pin.Alias,
			"description":  generator.WireDescription(pin),
			"input_schema": pin.InputSchema,
		})
	}
	raw, err := json.Marshal(defs)
	if err != nil {
		return "[]"
	}
	return pyStringLiteral(string(raw))
}

// safePyIdent turns a wire name into a valid Python identifier for use as a
// local variable. Wire names may be kebab-case, which identifiers may not be.
func safePyIdent(alias string) string {
	var b strings.Builder
	for i, r := range alias {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			b.WriteRune(r)
		case r >= '0' && r <= '9' && i > 0:
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "_tool"
	}
	return b.String()
}

// assertNoIdentifierCollisions catches a tool whose generated names clash with
// a prompt's. `@ws/web-search` yields `WsWebSearchToolOutput`, and so does a
// prompt named `@ws/web-search-tool`; emitting both produces a module whose
// later definition silently wins.
func assertNoIdentifierCollisions(ctx templateContext) error {
	owner := map[string]string{}
	claim := func(ident, by string) error {
		if prev, taken := owner[ident]; taken && prev != by {
			return fmt.Errorf(
				"%s and %s both generate the identifier %q; rename one of them, or install the prompt under a different alias (sufleur add --alias)",
				prev, by, ident)
		}
		owner[ident] = by
		return nil
	}

	for _, p := range ctx.Prompts {
		by := "prompt " + p.Name
		for _, dict := range p.TypedDicts {
			if err := claim(dict.Name, by); err != nil {
				return err
			}
		}
		if p.HasOutputSchema {
			if err := claim(p.OutputClassName, by); err != nil {
				return err
			}
		}
		if p.HasTools {
			if err := claim(p.ToolsTypeName, by); err != nil {
				return err
			}
		}
	}

	for _, tool := range ctx.Tools {
		by := "tool " + tool.Ref + "@" + tool.Version
		if err := claim(tool.BaseName, by); err != nil {
			return err
		}
		if err := claim(tool.InputClassName, by); err != nil {
			return err
		}
		for _, dict := range tool.OutputDicts {
			if err := claim(dict.Name, by); err != nil {
				return err
			}
		}
	}
	return nil
}

// collectTypedDicts recursively walks a JSON Schema and emits TypedDict classes.
// It returns the Python type string for this schema node. The backend serves
// standard JSON Schema (type/properties/items), so we read `type` here. Input
// schemas may also carry `oneOf`/`anyOf` (union types) and `optional: true` on
// properties; both are handled here. `analysis` is updated to record which
// extra imports the generated file needs.
func collectTypedDicts(schema map[string]interface{}, namePrefix string, classes *[]typedDictClass, isTopLevel bool, analysis *inputAnalysis) string {
	if v, ok := schema["oneOf"]; ok {
		return unionToPythonType(v, namePrefix, classes, analysis)
	}
	if v, ok := schema["anyOf"]; ok {
		return unionToPythonType(v, namePrefix, classes, analysis)
	}

	t, _ := schema["type"].(string)

	switch t {
	case "string":
		return "str"
	case "integer":
		return "int"
	case "number":
		return "float"
	case "boolean":
		return "bool"
	case "null":
		return "None"
	case "array":
		items, ok := schema["items"].(map[string]interface{})
		if !ok {
			return "list[Any]"
		}
		inner := collectTypedDicts(items, namePrefix, classes, false, analysis)
		return "list[" + inner + "]"
	case "object":
		props, ok := schema["properties"].(map[string]interface{})
		if !ok || len(props) == 0 {
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

		// A property is optional iff it is not listed in the parent object's
		// `required` array (standard JSON Schema). When `required` is absent
		// entirely, every property is optional — that is the case the backend
		// uses to mean "no required properties".
		requiredSet := requiredSetFromSchema(schema)

		var fields []typedDictField
		for _, k := range keys {
			v, ok := props[k].(map[string]interface{})
			if !ok {
				continue
			}
			// Child name prefix never includes _; that gets added by the recursive call.
			childName := namePrefix + "_" + toPascalCase(k)
			fieldType := collectTypedDicts(v, childName, classes, false, analysis)
			if optional := !requiredSet[k]; optional {
				// Accept None-shaped caller data (DBs, APIs) without forcing
				// callers to coerce. mustache treats None as falsy, so runtime
				// is unaffected. Skip the Optional[] wrap when the inner type
				// already accepts None (Any, or already-Optional via a oneOf
				// with a null variant).
				if fieldType != "Any" && !strings.HasPrefix(fieldType, "Optional[") {
					fieldType = "Optional[" + fieldType + "]"
				}
				fieldType = "NotRequired[" + fieldType + "]"
				analysis.HasOptional = true
			}
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
	}
	// Empty schema or untyped property — treat as opaque.
	return "Any"
}

// requiredSetFromSchema reads schema["required"] as a JSON Schema string array
// and returns the membership set. Returns a nil map when `required` is absent;
// callers rely on the fact that indexing a nil map yields the zero value, so
// the "absent" case naturally means "no properties are required".
func requiredSetFromSchema(schema map[string]interface{}) map[string]bool {
	raw, ok := schema["required"].([]interface{})
	if !ok {
		return nil
	}
	set := make(map[string]bool, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			set[s] = true
		}
	}
	return set
}

// unionToPythonType emits a Python Union from a JSON Schema oneOf/anyOf array.
// Mirrors anyOfToPydantic: a {type:"null"} variant becomes a wrapping Optional[].
func unionToPythonType(union interface{}, namePrefix string, classes *[]typedDictClass, analysis *inputAnalysis) string {
	variants, ok := union.([]interface{})
	if !ok || len(variants) == 0 {
		return "Any"
	}

	var nonNull []map[string]interface{}
	hasNull := false
	for _, v := range variants {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t == "null" {
			hasNull = true
		} else {
			nonNull = append(nonNull, m)
		}
	}

	var base string
	switch len(nonNull) {
	case 0:
		return "Any"
	case 1:
		base = collectTypedDicts(nonNull[0], namePrefix, classes, false, analysis)
	default:
		parts := make([]string, len(nonNull))
		for i, s := range nonNull {
			// Each object variant gets its own class name suffix so nested
			// TypedDicts don't collide.
			parts[i] = collectTypedDicts(s, namePrefix+"_Variant"+fmt.Sprintf("%d", i+1), classes, false, analysis)
		}
		base = "Union[" + strings.Join(parts, ", ") + "]"
		analysis.HasUnion = true
	}

	if hasNull {
		analysis.HasUnion = true
		return "Optional[" + base + "]"
	}
	return base
}

// toPascalCase delegates to the shared implementation so prompt and tool
// identifiers are derived identically by both generators.
func toPascalCase(s string) string { return generator.ToPascalCase(s) }

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
// pyDocstring makes an arbitrary string safe to embed inside a """...""" docstring.
// User-authored descriptions (e.g. from {{@doc "..."}}) may contain double quotes
// or backslashes; escaping both is the simplest transform that can never produce a
// """ run or leave a trailing " that merges with the closing delimiter.
func pyDocstring(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// pyStringLiteral renders s as a single-quoted Python string literal whose value
// equals s exactly. Backslashes are escaped first so JSON escapes like \" survive
// Python's own string-literal decoding, then single quotes (and any stray newlines)
// are escaped so they can't terminate the literal. Used to embed marshalled output
// schema JSON — which may contain ' or \" from field descriptions — into a
// json.loads('...') call without producing a syntax error or corrupting the JSON.
func pyStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return "'" + s + "'"
}

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
#
# Runtime peer dependencies (install in your project):
#   pip install chevron
{{- if or .AnyHasOutput .AnyHasTools}}
#   pip install pydantic
{{- end}}

from __future__ import annotations

import warnings
from typing import Any, Literal, TypedDict, overload{{if or .AnyHasOutput .AnyHasUnion .AnyHasOptional .AnyHasTools}}, Optional, Union{{end}}{{if .AnyHasTools}}, Protocol{{end}}
{{- if .AnyHasOptional}}
from typing_extensions import NotRequired
{{- end}}

import chevron
{{- if or .AnyHasOutput .AnyHasModelConfig .AnyHasTools}}
import json
{{- end}}
{{- if .AnyHasOutput}}
import re
{{- end}}
{{- if or .AnyHasOutput .AnyHasTools}}
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
    """{{pyDocstring .Description}}"""
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
    code: Literal["fence-extraction", "json-parse", "schema-validation"]
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
# Note: modelConfig.parameters keys are camelCase (matching the platform API);
# adapt them to your target SDK's parameter naming if needed.

_metadata: dict[str, dict[str, Any]] = {
{{- range .Prompts}}
    "{{.Name}}": {
        {{- range $k, $v := .Metadata}}
        "{{$k}}": {{pyMetadataValue $v}},
        {{- end}}
        {{- if .HasOutputSchema}}
        "outputSchema": json.loads({{pyStringLiteral .OutputSchemaRaw}}),
        {{- end}}
        "version": "{{.Version}}",
        {{- if .ModelConfigRaw}}
        "modelConfig": json.loads({{pyStringLiteral .ModelConfigRaw}}),
        {{- end}}
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


_fence_re = re.compile(r"{{.FencePattern}}")


def _extract_balanced_braces(s: str) -> str | None:
    obj_start = s.find("{")
    arr_start = s.find("[")
    if obj_start == -1 and arr_start == -1:
        return None
    if obj_start != -1 and (arr_start == -1 or obj_start < arr_start):
        start, opener, closer = obj_start, "{", "}"
    else:
        start, opener, closer = arr_start, "[", "]"
    depth = 0
    in_string = False
    escape = False
    for i in range(start, len(s)):
        c = s[i]
        if escape:
            escape = False
            continue
        if in_string and c == "\\":
            escape = True
            continue
        if c == '"':
            in_string = not in_string
            continue
        if in_string:
            continue
        if c == opener:
            depth += 1
        elif c == closer:
            depth -= 1
            if depth == 0:
                return s[start:i + 1]
    return None


def _extract_json_candidate(raw: str) -> tuple[str, bool]:
    """Best-effort JSON extraction. Returns (candidate_text, found_fence)."""
    trimmed = raw.strip()
    fences = [
        (m.group(1).lower(), m.group(2).strip())
        for m in _fence_re.finditer(trimmed)
    ]
    if fences:
        json_fence = next((body for lang, body in fences if lang == "json"), None)
        return (json_fence if json_fence is not None else fences[0][1], True)
    bare = _extract_balanced_braces(trimmed)
    if bare is not None:
        return (bare, False)
    return (trimmed, False)
{{- end}}

{{if .AnyHasTools}}# ─── Tool Contracts ───────────────────────────────────────────────────────────
#
# The trust boundary runs the opposite way from prompt I/O: a tool's arguments
# are written by the model, so they are validated at runtime, while a tool's
# result comes from your own code, so a static type is enough.
{{range .Tools}}

{{.InputModel}}
{{- range .OutputDicts}}

class {{.Name}}(TypedDict):
{{- range .Fields}}
    {{.Name}}: {{.Type}}
    {{- if .Description}}
    """{{pyDocstring .Description}}"""
    {{- end}}
{{- end}}
{{- end}}

class {{.BaseName}}(Protocol):
    """Implement this to bind {{.Ref}}@{{.Version}}."""

    def __call__(self, input: {{.InputClassName}}) -> {{.OutputTypeName}}: ...
{{end}}

class ToolExecutionError(Exception):
    """Raise from a tool implementation to report a failure the model may see."""


class _DispatchSuccess(TypedDict):
    content: str
    success: Literal[True]


class DispatchFailure(TypedDict):
    error: str
    code: Literal["unknown-tool", "input-validation", "execution"]
    success: Literal[False]

# Bindings per prompt, in functional syntax because wire names may be
# kebab-case, which is not a valid identifier. Every key is required — the model
# is offered every pinned tool, so an implementation for each has to be supplied.
{{range .Prompts}}
{{- if .HasTools}}
{{.ToolsTypeName}} = TypedDict("{{.ToolsTypeName}}", {
{{- range .ToolBindings}}
    "{{.Alias}}": {{.BaseName}},
{{- end}}
})

{{end}}
{{- end}}
# ─── Tool Definitions ─────────────────────────────────────────────────────────
#
# Provider-neutral {name, description, input_schema}; adapt them if your SDK's
# tool format differs.

_tool_defs: dict[str, list[dict[str, Any]]] = {
{{- range .Prompts}}
{{- if .HasTools}}
    "{{.Name}}": json.loads({{.ToolDefsRaw}}),
{{- end}}
{{- end}}
}

# Argument validators, looked up at dispatch time. The typed per-prompt classes
# below exist for the type checker; get_prompt returns the dynamic result object,
# so this is what actually validates what the model sent.
_tool_input_models: dict[str, dict[str, type[BaseModel]]] = {
{{- range .Prompts}}
{{- if .HasTools}}
    "{{.Name}}": {
{{- range .ToolBindings}}
        "{{.Alias}}": {{.InputClassName}},
{{- end}}
    },
{{- end}}
{{- end}}
}
{{- if .AnyDraftTools}}

# Pins on unpublished tool versions: the contract can still change under you.
_draft_tools: dict[str, list[str]] = {
{{- range .Prompts}}
{{- if .DraftTools}}
    "{{.Name}}": [{{range $i, $a := .DraftTools}}{{if $i}}, {{end}}"{{$a}}"{{end}}],
{{- end}}
{{- end}}
}
{{- end}}

{{end -}}
# ─── Draft Prompts ────────────────────────────────────────────────────────────

{{if .AnyDraftPrompts}}_draft_prompts: set[str] = {
{{- range .Prompts}}
{{- if eq .Status "DRAFT"}}
    "{{.Name}}",
{{- end}}
{{- end}}
}
{{- else}}
# No draft prompts installed. Spelled set() rather than {}, which is a dict.
_draft_prompts: set[str] = set()
{{- end}}

# ─── Per-prompt result types ─────────────────────────────────────────────────
{{range .Prompts}}

class _{{.PascalName}}Result:
    {{- if .Description}}
    """{{pyDocstring .Description}}

    Version: {{.Version}}
    """
    {{- end}}

    def __init__(self, templates: dict[str, str], partials: dict[str, str], metadata: {{.MetadataTypeName}}) -> None:
        self._templates = templates
        self._partials = partials
        self.metadata = metadata
    {{- range .Entrypoints}}

    @overload
    def render(self, entrypoint: Literal["{{.Name}}"], input: {{if .HasInput}}{{.InputTypeName}}{{else}}dict[str, Any]{{end}}) -> PromptOutput: ...
    {{- end}}

    def render(self, entrypoint: str, input: Any = None) -> PromptOutput:
        """Render the named entrypoint template with the given input."""
        template = self._templates.get(entrypoint)
        if template is None:
            raise KeyError(f'[sufleur] Unknown entrypoint "{entrypoint}" for prompt "{{.Name}}"')
        return {"prompt": chevron.render(template, input or {}, partials_dict=self._partials)}
    {{- if .HasOutputSchema}}

    def parse_output(self, raw: str) -> _{{.PascalName}}ParseSuccess | ParseFailure:
        """Parse and validate LLM output against the output schema."""
        candidate, found_fence = _extract_json_candidate(raw)
        try:
            parsed = json.loads(candidate)
        except json.JSONDecodeError as e:
            code = "fence-extraction" if found_fence else "json-parse"
            return {"error": str(e), "code": code, "success": False}
        try:
            validated = {{.OutputClassName}}.model_validate(parsed)
        except ValidationError as e:
            return {"error": str(e), "code": "schema-validation", "success": False}
        return {"data": validated, "success": True}
    {{- end}}
    {{- if .HasTools}}

    def tool_defs(self) -> list[dict[str, Any]]:
        """Provider-neutral definitions for every tool this prompt pins."""
        return list(_tool_defs["{{.Name}}"])

    def dispatch_tool(
        self, name: str, raw_input: Any, tools: {{.ToolsTypeName}}
    ) -> _DispatchSuccess | DispatchFailure:
        """Validate the model's arguments and invoke the bound implementation.

        Deliberately not a loop: call it once per tool_use block the model emits.
        """
        {{- range .ToolBindings}}
        if name == "{{.Alias}}":
            try:
                {{.SafeAlias}}_input = {{.InputClassName}}.model_validate(raw_input)
            except ValidationError as e:
                return {"error": str(e), "code": "input-validation", "success": False}
            try:
                {{.SafeAlias}}_output = tools["{{.Alias}}"]({{.SafeAlias}}_input)
            except ToolExecutionError as e:
                # Only ToolExecutionError is reported back to the model; anything
                # else is a bug in the implementation and keeps its traceback.
                return {"error": str(e), "code": "execution", "success": False}
            return {"content": json.dumps({{.SafeAlias}}_output), "success": True}
        {{- end}}
        return {
            "error": f'Unknown tool "{name}" for prompt "{{.Name}}"',
            "code": "unknown-tool",
            "success": False,
        }
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
{{- if .AnyDraftTools}}
    _pinned_drafts = _draft_tools.get(prompt_name)
    if _pinned_drafts:
        warnings.warn(
            f'[sufleur] Warning: prompt "{prompt_name}" pins draft tool version(s): '
            + ", ".join(_pinned_drafts),
            stacklevel=2,
        )
{{- end}}

    templates = _templates[prompt_name]
    partials = _partials.get(prompt_name, {})
    metadata = _metadata[prompt_name]

    class _PromptResult:
        def __init__(self) -> None:
            self.metadata = metadata

        def render(self, entrypoint: str, input: Any = None) -> PromptOutput:
            template = templates.get(entrypoint)
            if template is None:
                raise KeyError(f'[sufleur] Unknown entrypoint "{entrypoint}" for prompt "{prompt_name}"')
            return {"prompt": chevron.render(template, input or {}, partials_dict=partials)}
{{- if .AnyHasOutput}}

        def parse_output(self, raw: str) -> dict[str, Any]:
            model = _output_models.get(prompt_name)
            if model is None:
                return {"error": f"No output schema for prompt \"{prompt_name}\"", "code": "schema-validation", "success": False}
            candidate, found_fence = _extract_json_candidate(raw)
            try:
                parsed = json.loads(candidate)
            except json.JSONDecodeError as e:
                code = "fence-extraction" if found_fence else "json-parse"
                return {"error": str(e), "code": code, "success": False}
            try:
                validated = model.model_validate(parsed)
            except ValidationError as e:
                return {"error": str(e), "code": "schema-validation", "success": False}
            return {"data": validated, "success": True}
{{- end}}{{- if .AnyHasTools}}

        def tool_defs(self) -> list[dict[str, Any]]:
            return list(_tool_defs.get(prompt_name, []))

        def dispatch_tool(self, name: str, raw_input: Any, tools: Any) -> dict[str, Any]:
            model = _tool_input_models.get(prompt_name, {}).get(name)
            impl = tools.get(name) if isinstance(tools, dict) else None
            if model is None or impl is None:
                return {
                    "error": f'Unknown tool "{name}" for prompt "{prompt_name}"',
                    "code": "unknown-tool",
                    "success": False,
                }
            try:
                validated = model.model_validate(raw_input)
            except ValidationError as e:
                return {"error": str(e), "code": "input-validation", "success": False}
            try:
                output = impl(validated)
            except ToolExecutionError as e:
                # Only ToolExecutionError is reported back to the model; anything
                # else is a bug in the implementation and keeps its traceback.
                return {"error": str(e), "code": "execution", "success": False}
            return {"content": json.dumps(output), "success": True}
{{- end}}

    return _PromptResult()
`
