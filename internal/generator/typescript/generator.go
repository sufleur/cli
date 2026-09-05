package typescript

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
	generator.Register("typescript", func() generator.Generator { return &Generator{} })
}

// Generator produces TypeScript code from prompt data.
type Generator struct{}

// entrypointData describes a single render target within a prompt.
// PascalEntry is used to form the input interface name; Name is the runtime key.
type entrypointData struct {
	Name        string // file name as-is, e.g. "userPrompt", "my-file"
	PascalEntry string // PascalCase for use in TS identifiers
	Template    string // escaped template literal body
	HasInput    bool
	InputType   string // TS type literal, "" when HasInput is false
}

// promptTemplateData is the data passed to the Go text/template.
type promptTemplateData struct {
	Name            string
	PascalName      string
	Description     string
	Version         string
	Status          string
	Metadata        map[string]interface{}
	Entrypoints     []entrypointData
	Partials        []partialData
	HasOutputSchema bool
	OutputSchemaZod string
	OutputSchemaRaw string
	ModelConfigRaw  string
}

type partialData struct {
	Name    string
	Content string
}

type templateContext struct {
	Timestamp    string
	Prompts      []promptTemplateData
	AnyHasOutput bool
	FencePattern string
}

func (g *Generator) Generate(outFile string, prompts []generator.PromptData) error {
	if dir := filepath.Dir(outFile); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}

	data := buildTemplateData(prompts)

	tmpl, err := template.New("output").Funcs(template.FuncMap{
		"tsMetadataValue": tsMetadataValue,
		"jsDocComment":    jsDocComment,
	}).Parse(indexTemplate)
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
			escaped := escapeForTSTemplateLiteral(content)
			if f.IsEntrypoint {
				ep := entrypointData{
					Name:        f.Name,
					PascalEntry: toPascalCase(f.Name),
					Template:    escaped,
				}
				if f.InputSchema != nil {
					ep.HasInput = true
					ep.InputType = schemaToTSType(f.InputSchema, 0)
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

		// Output schema → Zod + raw JSON
		if p.OutputSchema != nil {
			td.HasOutputSchema = true
			td.OutputSchemaZod = jsonSchemaToZod(p.OutputSchema, 0)
			if raw, err := json.Marshal(p.OutputSchema); err == nil {
				td.OutputSchemaRaw = string(raw)
			}
			anyHasOutput = true
		}

		// modelConfig → raw JSON, emitted verbatim (parameters stay camelCase).
		if p.ModelConfig != nil {
			if raw, err := json.MarshalIndent(p.ModelConfig, "", "  "); err == nil {
				td.ModelConfigRaw = string(raw)
			}
		}

		tds = append(tds, td)
	}

	return templateContext{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Prompts:      tds,
		AnyHasOutput: anyHasOutput,
		FencePattern: parser.FencePattern,
	}
}

// toPascalCase delegates to the shared implementation so prompt and tool
// identifiers are derived identically by both generators.
func toPascalCase(s string) string { return generator.ToPascalCase(s) }

// escapeForTSTemplateLiteral escapes backticks and ${ for use inside a JS template literal.
func escapeForTSTemplateLiteral(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "${", "\\${")
	return s
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

// schemaToTSType converts a JSON Schema node to a TypeScript type string.
// The backend serves standard JSON Schema for both input and output schemas
// (type/properties/items/required), so we read `type` here. Input schemas may
// also carry `oneOf`/`anyOf` for union types — handle those before the switch.
func schemaToTSType(schema map[string]interface{}, indent int) string {
	if v, ok := schema["oneOf"]; ok {
		return unionToTSType(v, indent)
	}
	if v, ok := schema["anyOf"]; ok {
		return unionToTSType(v, indent)
	}
	t, _ := schema["type"].(string)
	switch t {
	case "string":
		return "string"
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	case "null":
		return "null"
	case "array":
		return arrayToTS(schema, indent)
	case "object":
		return objectToTS(schema, indent)
	}
	// Empty schema or untyped property — treat as opaque.
	return "unknown"
}

func objectToTS(schema map[string]interface{}, indent int) string {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return "Record<string, unknown>"
	}

	var b strings.Builder
	b.WriteString("{\n")

	// Sort keys for deterministic output
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// A property is optional iff it is not listed in the parent object's
	// `required` array (standard JSON Schema). When `required` is absent
	// entirely, every property is optional — that is the case the backend
	// uses to mean "no required properties".
	requiredSet := requiredSetFromSchema(schema)

	innerIndent := strings.Repeat("  ", indent+1)
	for _, k := range keys {
		v, ok := props[k].(map[string]interface{})
		if !ok {
			continue
		}

		// Add JSDoc for description
		if desc, ok := v["description"].(string); ok && desc != "" {
			b.WriteString(innerIndent)
			b.WriteString("/** ")
			b.WriteString(jsDocComment(desc))
			b.WriteString(" */\n")
		}

		optional := !requiredSet[k]
		b.WriteString(innerIndent)
		b.WriteString(k)
		if optional {
			b.WriteString("?: ")
		} else {
			b.WriteString(": ")
		}
		inner := schemaToTSType(v, indent+1)
		b.WriteString(inner)
		// Accept null-shaped caller data (DB rows, API responses) without forcing
		// `?? undefined` at the call site. Mustache treats null and undefined
		// identically as falsy, so the runtime is unaffected. Skip when the inner
		// type already accepts null (unknown, or a union that includes null).
		if optional && inner != "unknown" && !strings.Contains(inner, "null") {
			b.WriteString(" | null")
		}
		b.WriteString(";\n")
	}

	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString("}")
	return b.String()
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

func arrayToTS(schema map[string]interface{}, indent int) string {
	items, ok := schema["items"].(map[string]interface{})
	if !ok {
		return "unknown[]"
	}
	ts := schemaToTSType(items, indent)
	// Wrap complex types in parens for array syntax
	if strings.Contains(ts, "\n") {
		return "(" + ts + ")[]"
	}
	return ts + "[]"
}

// unionToTSType emits a TypeScript union from a JSON Schema oneOf/anyOf array.
// Mirrors anyOfToZod: a {type:"null"} variant becomes a trailing ` | null`.
func unionToTSType(union interface{}, indent int) string {
	variants, ok := union.([]interface{})
	if !ok || len(variants) == 0 {
		return "unknown"
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
		return "unknown"
	case 1:
		base = schemaToTSType(nonNull[0], indent)
	default:
		parts := make([]string, len(nonNull))
		for i, s := range nonNull {
			parts[i] = schemaToTSType(s, indent)
		}
		base = strings.Join(parts, " | ")
	}

	if hasNull {
		return base + " | null"
	}
	return base
}

// tsMetadataValue formats a Go value as a TypeScript literal.
// jsDocComment makes an arbitrary string safe to embed inside a /** ... */ JSDoc
// comment. User-authored descriptions (e.g. from {{@doc "..."}}) may contain the
// */ sequence, which would close the comment early; breaking that sequence is the
// only escaping a block comment needs.
func jsDocComment(s string) string {
	return strings.ReplaceAll(s, "*/", "*\\/")
}

func tsMetadataValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(val, "'", "\\'"))
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return "undefined"
	default:
		return fmt.Sprintf("'%v'", val)
	}
}

var indexTemplate = `// ⚠️ AUTO-GENERATED by Sufleur CLI — do not edit manually
// Generated at: {{.Timestamp}}
//
// Runtime peer dependencies (install in your project):
//   npm i mustache
//   npm i -D @types/mustache
{{- if .AnyHasOutput}}
//   npm i zod
{{- end}}

import Mustache from 'mustache';
{{- if .AnyHasOutput}}
import { z } from 'zod';
{{- end}}

// ─── Types ────────────────────────────────────────────────────────────────────

interface PromptOutput {
  prompt: string;
}
{{range $p := .Prompts}}
{{- range $p.Entrypoints}}
{{- if .HasInput}}
export type {{$p.PascalName}}_{{.PascalEntry}}Input = {{.InputType}};
{{end}}
{{- end}}
{{- end}}
{{- if .AnyHasOutput}}
// ─── Output Schemas ──────────────────────────────────────────────────────────
{{range .Prompts}}
{{- if .HasOutputSchema}}
export const {{.PascalName}}OutputSchema = {{.OutputSchemaZod}};

export type {{.PascalName}}Output = z.infer<typeof {{.PascalName}}OutputSchema>;
{{end}}
{{- end}}
export type ParseResult<T> =
  | { success: true; data: T }
  | { success: false; error: string; code: 'fence-extraction' | 'json-parse' | 'schema-validation' };

export interface OutputMapping {
{{- range .Prompts}}
  '{{.Name}}': {{if .HasOutputSchema}}{{.PascalName}}Output{{else}}never{{end}};
{{- end}}
}

{{end -}}
// ─── Prompt Name Union ────────────────────────────────────────────────────────

export type PromptName ={{range .Prompts}} | '{{.Name}}'{{end}};

// ─── Entrypoint Mapping ───────────────────────────────────────────────────────
//
// For each prompt, lists its entrypoint files and the input type required to
// render each one. Drives type narrowing for ` + "`render(entrypoint, input)`" + `.
// Entrypoints with an empty object schema accept ` + "`Record<string, unknown>`" + `;
// entrypoints with no input schema at all accept ` + "`Record<string, never>`" + `.
// Callers pass ` + "`{}`" + ` for either case.

export interface EntrypointMapping {
{{- range $p := .Prompts}}
  '{{$p.Name}}': {
    {{- range $p.Entrypoints}}
    '{{.Name}}': {{if .HasInput}}{{$p.PascalName}}_{{.PascalEntry}}Input{{else}}Record<string, never>{{end}};
    {{- end}}
  };
{{- end}}
}

// ─── Templates ────────────────────────────────────────────────────────────────

const _templates: Record<PromptName, Record<string, string>> = {
{{- range .Prompts}}
  '{{.Name}}': {
    {{- range .Entrypoints}}
    '{{.Name}}': ` + "`" + `{{.Template}}` + "`" + `,
    {{- end}}
  },
{{- end}}
};

// ─── Partials ─────────────────────────────────────────────────────────────────

const _partials: Record<PromptName, Record<string, string>> = {
{{- range .Prompts}}
  '{{.Name}}': {
    {{- range .Partials}}
    '{{.Name}}': ` + "`" + `{{.Content}}` + "`" + `,
    {{- end}}
  },
{{- end}}
};

// ─── Metadata ─────────────────────────────────────────────────────────────────

export const _metadata = {
{{- range .Prompts}}
  '{{.Name}}': {
    {{- range $k, $v := .Metadata}}
    {{$k}}: {{tsMetadataValue $v}},
    {{- end}}
    version: '{{.Version}}',
    {{- if .ModelConfigRaw}}
    modelConfig: {{.ModelConfigRaw}},
    {{- end}}
    {{- if .HasOutputSchema}}
    outputSchema: {{.OutputSchemaRaw}},
    {{- end}}
  },
{{- end}}
} as const;
{{- if .AnyHasOutput}}

const _outputSchemas: Partial<Record<PromptName, z.ZodType>> = {
{{- range .Prompts}}
{{- if .HasOutputSchema}}
  '{{.Name}}': {{.PascalName}}OutputSchema,
{{- end}}
{{- end}}
};

const _fenceRe = /{{.FencePattern}}/g;

const _extractBalancedBraces = (s: string): string | null => {
  const objStart = s.indexOf('{');
  const arrStart = s.indexOf('[');
  let start = -1;
  let opener = '';
  let closer = '';
  if (objStart !== -1 && (arrStart === -1 || objStart < arrStart)) {
    start = objStart; opener = '{'; closer = '}';
  } else if (arrStart !== -1) {
    start = arrStart; opener = '['; closer = ']';
  } else {
    return null;
  }
  let depth = 0;
  let inString = false;
  let escape = false;
  for (let i = start; i < s.length; i++) {
    const c = s.charAt(i);
    if (escape) { escape = false; continue; }
    if (inString && c === '\\') { escape = true; continue; }
    if (c === '"') { inString = !inString; continue; }
    if (inString) continue;
    if (c === opener) depth++;
    else if (c === closer) {
      depth--;
      if (depth === 0) return s.slice(start, i + 1);
    }
  }
  return null;
};

const _extractJsonCandidate = (raw: string): { text: string; foundFence: boolean } => {
  const trimmed = raw.trim();
  const fences: Array<{ lang: string; body: string }> = [];
  _fenceRe.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = _fenceRe.exec(trimmed)) !== null) {
    fences.push({ lang: (m[1] ?? '').toLowerCase(), body: (m[2] ?? '').trim() });
  }
  if (fences.length > 0) {
    const jsonFence = fences.find(f => f.lang === 'json');
    const chosen = jsonFence ?? fences[0]!;
    return { text: chosen.body, foundFence: true };
  }
  const bare = _extractBalancedBraces(trimmed);
  if (bare !== null) return { text: bare, foundFence: false };
  return { text: trimmed, foundFence: false };
};
{{- end}}

// ─── Draft Prompts ────────────────────────────────────────────────────────────

const _draftPrompts: Set<string> = new Set([
{{- range .Prompts}}
{{- if eq .Status "DRAFT"}}
  '{{.Name}}',
{{- end}}
{{- end}}
]);

// ─── getPrompt ────────────────────────────────────────────────────────────────
{{- if .AnyHasOutput}}

type PromptResult<N extends PromptName> = {
  render: <E extends keyof EntrypointMapping[N] & string>(
    entrypoint: E,
    input: EntrypointMapping[N][E],
  ) => PromptOutput;
  metadata: (typeof _metadata)[N];
} & (OutputMapping[N] extends never ? {} : {
  parseOutput(raw: string): ParseResult<OutputMapping[N]>;
});
{{- else}}

interface PromptResult<N extends PromptName> {
  render: <E extends keyof EntrypointMapping[N] & string>(
    entrypoint: E,
    input: EntrypointMapping[N][E],
  ) => PromptOutput;
  metadata: (typeof _metadata)[N];
}
{{- end}}

// Per-prompt overloads carry each prompt's own JSDoc (description + version)
// so it surfaces on hover at the ` + "`getPrompt(\"name\")`" + ` call site — the generic
// implementation signature below is erased at the boundary and cannot carry
// per-prompt docs on its own.
{{range $p := .Prompts}}
{{- if $p.Description}}
/**
 * {{jsDocComment $p.Description}}
 * @version {{$p.Version}}
 */
{{- end}}
export function getPrompt(promptName: '{{$p.Name}}'): PromptResult<'{{$p.Name}}'>;
{{end -}}
// Generic overload — required so callers holding a dynamic PromptName
// (e.g. a variable from iterating prompt names) can still resolve a call;
// without it, only the literal per-prompt overloads above are externally
// callable and a union-typed argument fails overload resolution. TS prefers
// the more specific literal overload when the argument is itself a literal,
// so per-prompt hover docs are unaffected.
export function getPrompt<N extends PromptName>(promptName: N): PromptResult<N>;
{{- if .AnyHasOutput}}
export function getPrompt<N extends PromptName>(promptName: N): PromptResult<N> {
  if (_draftPrompts.has(promptName)) {
    console.warn(` + "`" + `[sufleur] Warning: prompt "${promptName}" is a draft version` + "`" + `);
  }

  const templates = _templates[promptName];
  const partials = _partials[promptName] ?? {};

  const render = <E extends keyof EntrypointMapping[N] & string>(
    entrypoint: E,
    input: EntrypointMapping[N][E],
  ): PromptOutput => {
    const template = templates[entrypoint];
    if (template === undefined) {
      throw new Error(` + "`" + `[sufleur] Unknown entrypoint "${entrypoint}" for prompt "${promptName}"` + "`" + `);
    }
    return { prompt: Mustache.render(template, input ?? {}, partials) };
  };

  const metadata = _metadata[promptName];
  const schema = _outputSchemas[promptName];

  if (schema) {
    const parseOutput = (raw: string): ParseResult<OutputMapping[N]> => {
      const candidate = _extractJsonCandidate(raw);
      let parsed: unknown;
      try {
        parsed = JSON.parse(candidate.text);
      } catch (e: unknown) {
        const message = e instanceof Error ? e.message : String(e);
        return {
          success: false,
          error: message,
          code: candidate.foundFence ? 'fence-extraction' : 'json-parse',
        };
      }
      const validated = schema.safeParse(parsed);
      if (validated.success) {
        return { success: true, data: validated.data as OutputMapping[N] };
      }
      return { success: false, error: validated.error.message, code: 'schema-validation' };
    };
    return { render, metadata, parseOutput } as PromptResult<N>;
  }

  return { render, metadata } as PromptResult<N>;
}
{{- else}}
export function getPrompt<N extends PromptName>(promptName: N): PromptResult<N> {
  if (_draftPrompts.has(promptName)) {
    console.warn(` + "`" + `[sufleur] Warning: prompt "${promptName}" is a draft version` + "`" + `);
  }

  const templates = _templates[promptName];
  const partials = _partials[promptName] ?? {};

  const render = <E extends keyof EntrypointMapping[N] & string>(
    entrypoint: E,
    input: EntrypointMapping[N][E],
  ): PromptOutput => {
    const template = templates[entrypoint];
    if (template === undefined) {
      throw new Error(` + "`" + `[sufleur] Unknown entrypoint "${entrypoint}" for prompt "${promptName}"` + "`" + `);
    }
    return { prompt: Mustache.render(template, input ?? {}, partials) };
  };

  return { render, metadata: _metadata[promptName] };
}
{{- end}}
`
