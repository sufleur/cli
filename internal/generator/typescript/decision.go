package typescript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sufleur/cli/internal/generator"
)

// decisionTemplateData is one SYSTEM_ONE decision prompt.
type decisionTemplateData struct {
	Name        string
	PascalName  string
	Description string
	Version     string
	Status      string
	AnswersZod  string
	// Raw-state prompts type the caller's state from the inferred state schema;
	// prompts with a state file type the state file's template inputs instead.
	HasStateFile   bool
	StateType      string
	StateInputType string
	// QuestionInputsType lists the questions whose instructions have Mustache
	// variables. Empty when none do.
	QuestionInputsType string
	QuestionIDs        []string
	DefJSON            string
}

type decisionTextFile struct {
	Kind     string `json:"kind"`
	Template string `json:"template"`
}

type decisionYAMLFile struct {
	Kind string          `json:"kind"`
	Tree json.RawMessage `json:"tree"`
}

// decisionDef is the runtime definition emitted per decision prompt. Field
// order is fixed so the generated file is deterministic.
type decisionDef struct {
	Model     string                  `json:"model"`
	StateFile *string                 `json:"stateFile"`
	Questions []decisionQuestionDef   `json:"questions"`
	Files     orderedJSON             `json:"files"`
	Partials  orderedJSON             `json:"partials"`
	Spec      *generator.DecisionSpec `json:"-"`
}

type decisionQuestionDef struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Criteria json.RawMessage `json:"criteria,omitempty"`
}

// orderedJSON is a JSON object whose members are emitted in the given order.
type orderedJSON struct {
	keys   []string
	values []interface{}
}

func (o orderedJSON) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := marshalUnescaped(k)
		if err != nil {
			return nil, err
		}
		value, err := marshalUnescaped(o.values[i])
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

func (o *orderedJSON) add(key string, value interface{}) {
	o.keys = append(o.keys, key)
	o.values = append(o.values, value)
}

// prepareDecisionContent resolves both platform directives the way the backend
// does before rendering: {{@outputSchema}} and {{@field path}}.
func prepareDecisionContent(content string, p generator.PromptData) string {
	return generator.ResolveFieldDirectives(generator.ResolveDirectives(content, p))
}

func buildDecisionData(p generator.PromptData) (decisionTemplateData, error) {
	dn := displayName(p)
	spec := p.DecisionSpec
	td := decisionTemplateData{
		Name:        dn,
		PascalName:  toPascalCase(dn),
		Description: p.Description,
		Version:     p.Version,
		Status:      p.Status,
		AnswersZod:  "z.record(z.string(), z.unknown())",
	}
	if p.OutputSchema != nil {
		td.AnswersZod = jsonSchemaToZod(p.OutputSchema, 0)
	}

	filesByName := make(map[string]generator.PromptFile, len(p.Files))
	for _, f := range p.Files {
		filesByName[f.Name] = f
	}

	def := decisionDef{}
	if model, ok := p.ModelConfig["model"].(string); ok {
		def.Model = model
	}
	if spec.StateFile != "" {
		stateFile := spec.StateFile
		def.StateFile = &stateFile
	}

	emitFile := func(name string) error {
		f, ok := filesByName[name]
		if !ok {
			return fmt.Errorf("%s: decision spec file %q is missing from the version", dn, name)
		}
		if f.Format == generator.FormatYAML {
			tree, err := generator.ParseStructured(f.Content)
			if err != nil {
				return fmt.Errorf("%s: %s: %w", dn, name, err)
			}
			raw, err := tree.Map(func(s string) string { return prepareDecisionContent(s, p) }).MarshalJSON()
			if err != nil {
				return err
			}
			def.Files.add(name, decisionYAMLFile{Kind: "yaml", Tree: raw})
		} else {
			def.Files.add(name, decisionTextFile{Kind: "text", Template: prepareDecisionContent(f.Content, p)})
		}
		return nil
	}

	var questionInputs []string
	for _, q := range spec.Questions {
		td.QuestionIDs = append(td.QuestionIDs, q.ID)
		def.Questions = append(def.Questions, decisionQuestionDef{ID: q.ID, Type: q.Type, Criteria: q.Criteria})
		if err := emitFile(q.ID); err != nil {
			return td, err
		}
		if schema := filesByName[q.ID].InputSchema; hasProperties(schema) {
			questionInputs = append(questionInputs, fmt.Sprintf("  %s: %s;", q.ID, schemaToTSType(schema, 1)))
		}
	}
	if len(questionInputs) > 0 {
		td.QuestionInputsType = "{\n" + strings.Join(questionInputs, "\n") + "\n}"
	}

	if spec.StateFile != "" {
		td.HasStateFile = true
		if err := emitFile(spec.StateFile); err != nil {
			return td, err
		}
		td.StateInputType = "Record<string, never>"
		if schema := filesByName[spec.StateFile].InputSchema; hasProperties(schema) {
			td.StateInputType = schemaToTSType(schema, 0)
		}
	} else {
		td.StateType = "unknown"
		if p.StateSchema != nil {
			td.StateType = schemaToTSType(p.StateSchema, 0)
		}
	}

	var partialNames []string
	for _, f := range p.Files {
		if !f.IsEntrypoint {
			partialNames = append(partialNames, f.Name)
		}
	}
	sort.Strings(partialNames)
	for _, name := range partialNames {
		def.Partials.add(name, prepareDecisionContent(filesByName[name].Content, p))
	}

	raw, err := marshalIndentUnescaped(def, "  ", "  ")
	if err != nil {
		return td, err
	}
	td.DefJSON = raw
	return td, nil
}

func marshalUnescaped(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// marshalIndentUnescaped is json.MarshalIndent without HTML escaping, so
// Mustache partial tags like {{> name}} stay readable in the generated file.
func marshalIndentUnescaped(v interface{}, prefix, indent string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent(prefix, indent)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func hasProperties(schema map[string]interface{}) bool {
	props, ok := schema["properties"].(map[string]interface{})
	return ok && len(props) > 0
}

type decisionContext struct {
	Decisions             []decisionTemplateData
	WholeValuePatternJSON string
}

func buildDecisionContext(prompts []generator.PromptData) (decisionContext, error) {
	pattern, _ := marshalUnescaped(generator.WholeValuePattern)
	ctx := decisionContext{WholeValuePatternJSON: string(pattern)}
	for _, p := range prompts {
		d, err := buildDecisionData(p)
		if err != nil {
			return ctx, err
		}
		ctx.Decisions = append(ctx.Decisions, d)
	}
	return ctx, nil
}

// decisionIdentifiers are the exported names a decision prompt claims.
func decisionIdentifiers(d decisionTemplateData) []string {
	ids := []string{d.PascalName + "Answers", d.PascalName + "AnswersSchema", d.PascalName + "RequestArgs"}
	if d.HasStateFile {
		ids = append(ids, d.PascalName+"StateInputs")
	} else {
		ids = append(ids, d.PascalName+"State")
	}
	if d.QuestionInputsType != "" {
		ids = append(ids, d.PascalName+"QuestionInputs")
	}
	return ids
}

var decisionTemplate = `
// ─── Decision Prompts (System-One) ────────────────────────────────────────────
//
// Decision prompts target System-One models such as TypeSafe Jev: typed
// noul / choice / score questions evaluated against one state. getDecision()
// builds the request body (POST /v1/systemone) and validates the answers.

export type DecisionQuestionType = 'noul' | 'choice' | 'score';

export interface DecisionRequest {
  model: string;
  state: unknown;
  questions: Record<string, { type: DecisionQuestionType; instructions: unknown; criteria?: unknown }>;
}

export type DecisionParseResult<T> =
  | { success: true; data: T }
  | { success: false; error: string; code: 'schema-validation' };
{{range .Decisions}}
export const {{.PascalName}}AnswersSchema = {{.AnswersZod}};

export type {{.PascalName}}Answers = z.infer<typeof {{.PascalName}}AnswersSchema>;
{{if .HasStateFile}}
export type {{.PascalName}}StateInputs = {{.StateInputType}};
{{else}}
export type {{.PascalName}}State = {{.StateType}};
{{end}}
{{- if .QuestionInputsType}}
export type {{.PascalName}}QuestionInputs = {{.QuestionInputsType}};
{{end}}
export type {{.PascalName}}RequestArgs = {
  {{if .HasStateFile}}stateInputs: {{.PascalName}}StateInputs;{{else}}state: {{.PascalName}}State;{{end}}
  {{if .QuestionInputsType}}questionInputs: {{.PascalName}}QuestionInputs;{{else}}questionInputs?: Record<string, never>;{{end}}
};
{{end}}
export type DecisionName ={{range .Decisions}} | '{{.Name}}'{{end}};

export interface DecisionMapping {
{{- range .Decisions}}
  '{{.Name}}': { args: {{.PascalName}}RequestArgs; answers: {{.PascalName}}Answers };
{{- end}}
}

type _DecisionFile = { kind: 'text'; template: string } | { kind: 'yaml'; tree: unknown };

interface _DecisionDef {
  model: string;
  stateFile: string | null;
  questions: ReadonlyArray<{ id: string; type: DecisionQuestionType; criteria?: unknown }>;
  files: Record<string, _DecisionFile>;
  partials: Record<string, string>;
}

const _decisions: Record<DecisionName, _DecisionDef> = {
{{- range .Decisions}}
  '{{.Name}}': {{.DefJSON}},
{{- end}}
};

const _answerSchemas: Record<DecisionName, z.ZodType> = {
{{- range .Decisions}}
  '{{.Name}}': {{.PascalName}}AnswersSchema,
{{- end}}
};

const _draftDecisions: Set<string> = new Set([
{{- range .Decisions}}
{{- if eq .Status "DRAFT"}}
  '{{.Name}}',
{{- end}}
{{- end}}
]);

// A YAML value that is exactly one variable tag passes the input through as-is.
const _wholeValueRe = new RegExp({{.WholeValuePatternJSON}});

const _lookup = (view: unknown, name: string): unknown => {
  if (name === '.') return view;
  let current: unknown = view;
  for (const key of name.split('.')) {
    if (current === null || typeof current !== 'object') return undefined;
    current = (current as Record<string, unknown>)[key];
  }
  return current;
};

const _renderTree = (tree: unknown, view: Record<string, unknown>, render: (t: string) => string): unknown => {
  if (typeof tree === 'string') {
    const m = _wholeValueRe.exec(tree);
    if (m) {
      const value = _lookup(view, (m[1] ?? m[2] ?? '').trim());
      return value === undefined ? null : value;
    }
    return render(tree);
  }
  if (Array.isArray(tree)) return tree.map((item) => _renderTree(item, view, render));
  if (tree !== null && typeof tree === 'object') {
    return Object.fromEntries(
      Object.entries(tree as Record<string, unknown>).map(([k, v]) => [k, _renderTree(v, view, render)]),
    );
  }
  return tree;
};

const _renderDecisionFile = (
  file: _DecisionFile,
  view: Record<string, unknown>,
  partials: Record<string, string>,
): unknown => {
  const render = (template: string): string => Mustache.render(template, view, partials);
  return file.kind === 'text' ? render(file.template) : _renderTree(file.tree, view, render);
};

export interface DecisionResult<N extends DecisionName> {
  /** Question ids in authored order — also the keys of the answers object. */
  questionIds: readonly string[];
  /** The System-One model this version was authored for. */
  model: string;
  /** Build the request body for the System-One API (POST /v1/systemone). */
  buildRequest(args: DecisionMapping[N]['args']): DecisionRequest;
  /** Validate a response (or its answers object) against the typed answers schema. */
  parseResponse(raw: unknown): DecisionParseResult<DecisionMapping[N]['answers']>;
}
{{range .Decisions}}
{{- if .Description}}
/**
 * {{jsDocComment .Description}}
 * @version {{.Version}}
 */
{{- end}}
export function getDecision(name: '{{.Name}}'): DecisionResult<'{{.Name}}'>;
{{end -}}
export function getDecision<N extends DecisionName>(name: N): DecisionResult<N>;
export function getDecision<N extends DecisionName>(name: N): DecisionResult<N> {
  if (_draftDecisions.has(name)) {
    console.warn('[sufleur] Warning: decision prompt "' + name + '" is a draft version');
  }
  const def = _decisions[name];
  const schema = _answerSchemas[name];

  const buildRequest = (args: DecisionMapping[N]['args']): DecisionRequest => {
    const input = args as {
      state?: unknown;
      stateInputs?: Record<string, unknown>;
      questionInputs?: Record<string, Record<string, unknown>>;
    };
    const stateFile = def.stateFile === null ? undefined : def.files[def.stateFile];
    const state = stateFile ? _renderDecisionFile(stateFile, input.stateInputs ?? {}, def.partials) : input.state;
    const questions: DecisionRequest['questions'] = {};
    for (const q of def.questions) {
      const file = def.files[q.id]!;
      questions[q.id] = {
        type: q.type,
        instructions: _renderDecisionFile(file, input.questionInputs?.[q.id] ?? {}, def.partials),
        ...(q.criteria !== undefined ? { criteria: q.criteria } : {}),
      };
    }
    return { model: def.model, state, questions };
  };

  const parseResponse = (raw: unknown): DecisionParseResult<DecisionMapping[N]['answers']> => {
    const candidate =
      raw !== null && typeof raw === 'object' && 'answers' in (raw as Record<string, unknown>)
        ? (raw as { answers: unknown }).answers
        : raw;
    const validated = schema.safeParse(candidate);
    if (validated.success) {
      return { success: true, data: validated.data as DecisionMapping[N]['answers'] };
    }
    return { success: false, error: validated.error.message, code: 'schema-validation' };
  };

  return { questionIds: def.questions.map((q) => q.id), model: def.model, buildRequest, parseResponse };
}
`
