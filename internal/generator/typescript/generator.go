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
		"tsMetadataValue": tsMetadataValue,
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

		tds = append(tds, td)
	}

	return templateContext{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Prompts:      tds,
		AnyHasOutput: anyHasOutput,
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
// (type/properties/items/required), so we read `type` here.
func schemaToTSType(schema map[string]interface{}, indent int) string {
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
			b.WriteString(desc)
			b.WriteString(" */\n")
		}

		b.WriteString(innerIndent)
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(schemaToTSType(v, indent+1))
		b.WriteString(";\n")
	}

	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString("}")
	return b.String()
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

// tsMetadataValue formats a Go value as a TypeScript literal.
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
export type ParseResult<T> = { success: true; data: T } | { success: false; error: string };

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
// ` + "`void`" + ` means the entrypoint has no input schema and is callable with no
// arguments (other than the entrypoint name).

export interface EntrypointMapping {
{{- range $p := .Prompts}}
  '{{$p.Name}}': {
    {{- range $p.Entrypoints}}
    '{{.Name}}': {{if .HasInput}}{{$p.PascalName}}_{{.PascalEntry}}Input{{else}}void{{end}};
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
    ...input: EntrypointMapping[N][E] extends void ? [] : [EntrypointMapping[N][E]]
  ) => PromptOutput;
  metadata: (typeof _metadata)[N];
} & (OutputMapping[N] extends never ? {} : {
  parseOutput(raw: string): ParseResult<OutputMapping[N]>;
});
{{- else}}

interface PromptResult<N extends PromptName> {
  render: <E extends keyof EntrypointMapping[N] & string>(
    entrypoint: E,
    ...input: EntrypointMapping[N][E] extends void ? [] : [EntrypointMapping[N][E]]
  ) => PromptOutput;
  metadata: (typeof _metadata)[N];
}
{{- end}}
{{range .Prompts}}
{{- if .Description}}
/**
 * {{.Description}}
 * @version {{.Version}}
 */
{{- end}}
{{- end}}
{{- if .AnyHasOutput}}
export function getPrompt<N extends PromptName>(promptName: N): PromptResult<N> {
  if (_draftPrompts.has(promptName)) {
    console.warn(` + "`" + `[sufleur] Warning: prompt "${promptName}" is a draft version` + "`" + `);
  }

  const templates = _templates[promptName];
  const partials = _partials[promptName] ?? {};

  const render = <E extends keyof EntrypointMapping[N] & string>(
    entrypoint: E,
    ...input: EntrypointMapping[N][E] extends void ? [] : [EntrypointMapping[N][E]]
  ): PromptOutput => {
    const template = templates[entrypoint];
    if (template === undefined) {
      throw new Error(` + "`" + `[sufleur] Unknown entrypoint "${entrypoint}" for prompt "${promptName}"` + "`" + `);
    }
    return { prompt: Mustache.render(template, input[0] ?? {}, partials) };
  };

  const metadata = _metadata[promptName];
  const schema = _outputSchemas[promptName];

  if (schema) {
    const parseOutput = (raw: string): ParseResult<OutputMapping[N]> => {
      let text = raw.trim();
      const fenceMatch = text.match(/^` + "```" + `(?:\w*)\s*\n?([\s\S]*?)\n?\s*` + "```" + `$/);
      if (fenceMatch) {
        const inner = fenceMatch[1];
        if (inner === undefined) {
          throw new Error("[sufleur] Code fence matched but capture group is missing");
        }
        text = inner.trim();
      }
      try {
        const parsed = JSON.parse(text);
        const validated = schema.safeParse(parsed);
        if (validated.success) {
          return { success: true, data: validated.data as OutputMapping[N] };
        }
        return { success: false, error: validated.error.message };
      } catch (e: unknown) {
        return { success: false, error: e instanceof Error ? e.message : String(e) };
      }
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
    ...input: EntrypointMapping[N][E] extends void ? [] : [EntrypointMapping[N][E]]
  ): PromptOutput => {
    const template = templates[entrypoint];
    if (template === undefined) {
      throw new Error(` + "`" + `[sufleur] Unknown entrypoint "${entrypoint}" for prompt "${promptName}"` + "`" + `);
    }
    return { prompt: Mustache.render(template, input[0] ?? {}, partials) };
  };

  return { render, metadata: _metadata[promptName] };
}
{{- end}}
`
