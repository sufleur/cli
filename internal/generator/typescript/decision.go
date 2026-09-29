package typescript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sufleur/cli/internal/generator"
)

// decisionQuestionData is one question template of a decision prompt.
type decisionQuestionData struct {
	ID string
	// Type is "noul" | "choice" | "score".
	Type string
	// InputsType is the TypeScript type of the question's template inputs
	// (instructions + criteria); "Record<string, never>" when it has none.
	InputsType string
	// OptionInputsType is set only for open choices: the inputs of one added option.
	OptionInputsType string
	// AnswerSchemaName / AnswerZod: the Zod schema of one answer.
	AnswerSchemaName string
	AnswerZod        string
}

// decisionTemplateData is one SYSTEM_ONE decision prompt.
type decisionTemplateData struct {
	Name        string
	PascalName  string
	Description string
	Version     string
	Status      string
	Questions   []decisionQuestionData
	DefJSON     string
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
	Metadata  decisionMetadata      `json:"metadata"`
	Questions []decisionQuestionDef `json:"questions"`
	Files     orderedJSON           `json:"files"`
	Partials  orderedJSON           `json:"partials"`
}

type decisionMetadata struct {
	Version     string                 `json:"version"`
	ModelConfig map[string]interface{} `json:"modelConfig"`
}

type decisionQuestionDef struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Criteria       json.RawMessage `json:"criteria,omitempty"`
	OptionCriteria json.RawMessage `json:"optionCriteria,omitempty"`
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

func buildDecisionData(p generator.PromptData) (decisionTemplateData, error) {
	dn := displayName(p)
	spec := p.DecisionSpec
	td := decisionTemplateData{
		Name:        dn,
		PascalName:  toPascalCase(dn),
		Description: p.Description,
		Version:     p.Version,
		Status:      p.Status,
	}

	filesByName := make(map[string]generator.PromptFile, len(p.Files))
	for _, f := range p.Files {
		filesByName[f.Name] = f
	}
	answerSchemas, _ := p.OutputSchema["properties"].(map[string]interface{})

	modelConfig := p.ModelConfig
	if modelConfig == nil {
		modelConfig = map[string]interface{}{}
	}
	def := decisionDef{Metadata: decisionMetadata{Version: p.Version, ModelConfig: modelConfig}}

	for _, q := range spec.Questions {
		f, ok := filesByName[q.ID]
		if !ok {
			return td, fmt.Errorf("%s: question file %q is missing from the version", dn, q.ID)
		}
		if f.Format == generator.FormatYAML {
			tree, err := generator.ParseStructured(f.Content)
			if err != nil {
				return td, fmt.Errorf("%s: %s: %w", dn, q.ID, err)
			}
			raw, err := tree.MarshalJSON()
			if err != nil {
				return td, err
			}
			def.Files.add(q.ID, decisionYAMLFile{Kind: "yaml", Tree: raw})
		} else {
			def.Files.add(q.ID, decisionTextFile{Kind: "text", Template: f.Content})
		}
		def.Questions = append(def.Questions, decisionQuestionDef{
			ID: q.ID, Type: q.Type, Criteria: q.Criteria, OptionCriteria: q.OptionCriteria,
		})

		qd := decisionQuestionData{
			ID:               q.ID,
			Type:             q.Type,
			InputsType:       "Record<string, never>",
			AnswerSchemaName: td.PascalName + toPascalCase(q.ID) + "AnswerSchema",
			AnswerZod:        "z.unknown()",
		}
		if hasProperties(f.InputSchema) {
			qd.InputsType = schemaToTSType(f.InputSchema, 2)
		}
		if q.IsOpenChoice() {
			qd.OptionInputsType = "Record<string, never>"
			if hasProperties(f.OptionInputSchema) {
				qd.OptionInputsType = schemaToTSType(f.OptionInputSchema, 2)
			}
		}
		if schema, ok := answerSchemas[q.ID].(map[string]interface{}); ok {
			qd.AnswerZod = jsonSchemaToZod(schema, 0)
		}
		td.Questions = append(td.Questions, qd)
	}

	var partialNames []string
	for _, f := range p.Files {
		if !f.IsEntrypoint {
			partialNames = append(partialNames, f.Name)
		}
	}
	sort.Strings(partialNames)
	for _, name := range partialNames {
		def.Partials.add(name, filesByName[name].Content)
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
	ids := []string{d.PascalName + "Questions"}
	for _, q := range d.Questions {
		ids = append(ids, q.AnswerSchemaName)
	}
	return ids
}

var decisionTemplate = `
// ─── Decision Prompts (System-One) ────────────────────────────────────────────
//
// Decision prompts are question templates for System-One models such as
// TypeSafe Jev: typed noul / choice / score questions. Render a question with
// question(), or collect several with batch(); send them to your provider
// together with your own state; read the answers back with parseAnswer() or
// the batch's read(). No provider request or response shape lives here.

export type DecisionQuestionType = 'noul' | 'choice' | 'score';
type _Criteria = string | Record<string, unknown> | unknown[] | null;

/** One rendered question: what a System-One API takes under questions.<key>. */
export type RenderedQuestion<T extends DecisionQuestionType = DecisionQuestionType> = {
  noul: { type: 'noul'; instructions: unknown; criteria?: { true?: _Criteria; false?: _Criteria } };
  choice: { type: 'choice'; instructions: unknown; criteria: Record<string, _Criteria> };
  score: { type: 'score'; instructions: unknown; criteria: _Criteria[] };
}[T];

export type DecisionParseResult<T> =
  | { success: true; data: T }
  | { success: false; error: string; code: 'schema-validation' };

export interface DecisionMetadata {
  version: string;
  /** The recommended model config, same shape as getPrompt(...).metadata.modelConfig. */
  modelConfig: { provider?: string; model?: string; parameters?: Record<string, unknown> };
}

interface _QuestionSpec {
  type: DecisionQuestionType;
  inputs: object;
  answer: unknown;
  optionInputs?: object;
}
type _QuestionMap<M> = { [K in keyof M]: _QuestionSpec };
type _NoInputs = Record<string, never>;
type _OptionInputs<S> = S extends { optionInputs: infer O } ? O : never;
type _WithOptions<M> = { [K in keyof M]: M[K] extends { optionInputs: object } ? K : never }[keyof M];
type _QuestionOptions<S> = [_OptionInputs<S>] extends [never] ? { options?: never } : { options?: Record<string, _OptionInputs<S>> };

/** Returned by batch.ask(); reads the same question's answer back. A is its answer type. */
export interface DecisionHandle<A> {
  readonly key: string;
  readonly questionId: string;
  readonly question: RenderedQuestion;
  /** Type-level only: carries the answer type. */
  readonly __answer?: A;
}

export interface DecisionAskOptions<O> {
  /** The key the question is sent (and answered) under. Default: its question id. */
  key?: string;
  /** Options to add to an open choice: option key → that option's template inputs. */
  options?: [O] extends [never] ? never : Record<string, O>;
}

export type DecisionAnswerResult<A> = { success: true; data: A } | { success: false; error: string };

/** Partial read: every answer that validated, plus every problem. */
export interface DecisionAnswers {
  errors: string[];
  get<A>(handle: DecisionHandle<A>): DecisionAnswerResult<A>;
  /** The answer, or throws when it is missing or invalid. */
  getOrThrow<A>(handle: DecisionHandle<A>): A;
}

export type DecisionReadAllResult =
  | { success: true; answers: { get<A>(handle: DecisionHandle<A>): A } }
  | { success: false; errors: string[]; code: 'schema-validation' };

/** Collects questions under keys and matches answers back. Never sees a request or response. */
export interface DecisionBatch<M extends _QuestionMap<M>> {
  ask<K extends keyof M & string>(
    questionId: K,
    ...args: M[K]['inputs'] extends _NoInputs
      ? [inputs?: _NoInputs, opts?: DecisionAskOptions<_OptionInputs<M[K]>>]
      : [inputs: M[K]['inputs'], opts?: DecisionAskOptions<_OptionInputs<M[K]>>]
  ): DecisionHandle<M[K]['answer']>;
  /** Every asked question, in order. Turn these into a provider request. */
  items(): ReadonlyArray<{ key: string; questionId: keyof M & string; question: RenderedQuestion }>;
  /** Match answers back by key; keeps every answer that validated. */
  read(answers: Record<string, unknown>): DecisionAnswers;
  /** Match answers back by key; fails unless every answer validated. */
  readAll(answers: Record<string, unknown>): DecisionReadAllResult;
}

export interface Decision<M extends _QuestionMap<M>> {
  metadata: DecisionMetadata;
  questionIds: ReadonlyArray<keyof M & string>;
  /** Render one question with its inputs (and, for an open choice, added options). */
  question<K extends keyof M & string>(
    id: K,
    ...args: M[K]['inputs'] extends _NoInputs
      ? [inputs?: _NoInputs, opts?: _QuestionOptions<M[K]>]
      : [inputs: M[K]['inputs'], opts?: _QuestionOptions<M[K]>]
  ): RenderedQuestion<M[K]['type']>;
  /** Validate one raw answer against question id's answer schema. */
  parseAnswer<K extends keyof M & string>(id: K, raw: unknown): DecisionParseResult<M[K]['answer']>;
  /** Start a batch of questions. */
  batch(): DecisionBatch<M>;
}

type _DecisionFile = { kind: 'text'; template: string } | { kind: 'yaml'; tree: unknown };

interface _DecisionDef {
  metadata: DecisionMetadata;
  questions: ReadonlyArray<{
    id: string;
    type: DecisionQuestionType;
    criteria?: unknown;
    optionCriteria?: unknown;
  }>;
  files: Record<string, _DecisionFile>;
  partials: Record<string, string>;
}

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

// Criteria: every string is a template; keys and other values pass through.
const _renderEntry = (entry: unknown, view: Record<string, unknown>, render: (t: string) => string): unknown => {
  if (typeof entry === 'string') return render(entry);
  if (Array.isArray(entry)) return entry.map((item) => _renderEntry(item, view, render));
  if (entry !== null && typeof entry === 'object') {
    return Object.fromEntries(
      Object.entries(entry as Record<string, unknown>).map(([k, v]) => [k, _renderEntry(v, view, render)]),
    );
  }
  return entry;
};

function _renderQuestion(
  def: _DecisionDef,
  id: string,
  inputs: Record<string, unknown> = {},
  options?: Record<string, Record<string, unknown>>,
): RenderedQuestion {
  const q = def.questions.find((item) => item.id === id);
  const file = def.files[id];
  if (!q || !file) throw new Error('[sufleur] unknown question "' + id + '"');
  const render = (view: Record<string, unknown>) => (template: string): string =>
    Mustache.render(template, view, def.partials);
  const instructions =
    file.kind === 'text' ? render(inputs)(file.template) : _renderTree(file.tree, inputs, render(inputs));
  const rendered: Record<string, unknown> = { type: q.type, instructions };
  if (q.criteria !== undefined) rendered.criteria = _renderEntry(q.criteria, inputs, render(inputs));

  const added = Object.entries(options ?? {});
  if (q.type !== 'choice') {
    if (added.length > 0) throw new Error('[sufleur] "' + id + '" is a ' + q.type + ' question: it takes no options');
    return rendered as RenderedQuestion;
  }
  if (added.length > 0 && q.optionCriteria === undefined) {
    throw new Error('[sufleur] "' + id + '" has a fixed set of options: add optionCriteria to let callers add options');
  }
  const criteria = { ...((rendered.criteria as Record<string, unknown>) ?? {}) };
  for (const [key, optionInputs] of added) {
    if (key.trim() === '' || key.length > 255) {
      throw new Error('[sufleur] "' + id + '": option keys must be non-blank and at most 255 characters');
    }
    if (key in criteria) throw new Error('[sufleur] "' + id + '": option "' + key + '" is already one of the fixed options');
    criteria[key] = _renderEntry(q.optionCriteria, optionInputs ?? {}, render(optionInputs ?? {}));
  }
  const total = Object.keys(criteria).length;
  if (total < 2 || total > 255) {
    throw new Error('[sufleur] "' + id + '": a choice needs between 2 and 255 options (this one has ' + total + ')');
  }
  rendered.criteria = criteria;
  return rendered as RenderedQuestion;
}

function _createDecision<M extends _QuestionMap<M>>(
  name: string,
  def: _DecisionDef,
  schemas: Record<string, z.ZodType>,
  isDraft: boolean,
): Decision<M> {
  const parseAnswer = (id: string, raw: unknown): DecisionParseResult<unknown> => {
    const schema = schemas[id];
    if (!schema) throw new Error('[sufleur] unknown question "' + id + '"');
    const validated = schema.safeParse(raw);
    return validated.success
      ? { success: true, data: validated.data }
      : { success: false, error: '[' + id + '] ' + validated.error.message, code: 'schema-validation' };
  };

  const batch = () => {
    type AnyHandle = DecisionHandle<unknown>;
    const asked = new Map<string, AnyHandle>();

    const ask = (
      questionId: string,
      inputs: Record<string, unknown> = {},
      opts: { key?: string; options?: Record<string, Record<string, unknown>> } = {},
    ): AnyHandle => {
      const key = opts.key ?? questionId;
      if (asked.has(key)) {
        throw new Error('[sufleur] key "' + key + '" is already used in this batch: give repeated questions distinct keys');
      }
      const handle: AnyHandle = Object.freeze({
        key,
        questionId,
        question: _renderQuestion(def, questionId, inputs, opts.options),
      });
      asked.set(key, handle);
      return handle;
    };

    const items = () => [...asked.values()].map(({ key, questionId, question }) => ({ key, questionId, question }));

    const collect = (answers: Record<string, unknown>) => {
      const values = new Map<AnyHandle, DecisionAnswerResult<unknown>>();
      const errors: string[] = [];
      for (const [key, handle] of asked) {
        if (!answers || !(key in answers)) {
          const error = '[' + key + '] no answer';
          errors.push(error);
          values.set(handle, { success: false, error });
          continue;
        }
        const parsed = parseAnswer(handle.questionId, answers[key]);
        if (parsed.success) values.set(handle, { success: true, data: parsed.data });
        else {
          const error = key === handle.questionId ? parsed.error : key + ' ' + parsed.error;
          errors.push(error);
          values.set(handle, { success: false, error });
        }
      }
      return { values, errors };
    };
    const lookup = (values: Map<AnyHandle, DecisionAnswerResult<unknown>>, handle: AnyHandle) => {
      const result = values.get(handle);
      if (!result) throw new Error('[sufleur] this handle was not asked in this batch');
      return result;
    };

    const read = (answers: Record<string, unknown>): DecisionAnswers => {
      const { values, errors } = collect(answers);
      return {
        errors,
        get: (handle) => lookup(values, handle) as DecisionAnswerResult<never>,
        getOrThrow: (handle) => {
          const result = lookup(values, handle);
          if (!result.success) throw new Error(result.error);
          return result.data as never;
        },
      };
    };

    const readAll = (answers: Record<string, unknown>): DecisionReadAllResult => {
      const { values, errors } = collect(answers);
      if (errors.length > 0) return { success: false, errors, code: 'schema-validation' };
      return {
        success: true,
        answers: { get: (handle) => (lookup(values, handle) as { data: unknown }).data as never },
      };
    };

    return { ask, items, read, readAll } as unknown as DecisionBatch<M>;
  };

  return {
    get metadata() {
      if (isDraft) console.warn('[sufleur] Warning: decision prompt "' + name + '" is a draft version');
      return def.metadata;
    },
    questionIds: def.questions.map((q) => q.id),
    question: (id: string, inputs?: Record<string, unknown>, opts?: { options?: Record<string, Record<string, unknown>> }) =>
      _renderQuestion(def, id, inputs, opts?.options),
    parseAnswer,
    batch,
  } as unknown as Decision<M>;
}
{{range .Decisions}}
// ─── {{.Name}} ────────────────────────────────────────────────────────────────
{{range .Questions}}
export const {{.AnswerSchemaName}} = {{.AnswerZod}};
{{end}}
export type {{.PascalName}}Questions = {
{{- range .Questions}}
  {{.ID}}: {
    type: '{{.Type}}';
    inputs: {{.InputsType}};
{{- if .OptionInputsType}}
    optionInputs: {{.OptionInputsType}};
{{- end}}
    answer: z.infer<typeof {{.AnswerSchemaName}}>;
  };
{{- end}}
};

const _{{.PascalName}}Def: _DecisionDef = {{.DefJSON}};
{{end}}
export type DecisionName ={{range .Decisions}} | '{{.Name}}'{{end}};

const _decisions = {
{{- range .Decisions}}
  '{{.Name}}': _createDecision<{{.PascalName}}Questions>('{{.Name}}', _{{.PascalName}}Def, {
{{- range .Questions}}
    '{{.ID}}': {{.AnswerSchemaName}},
{{- end}}
  }, {{if eq .Status "DRAFT"}}true{{else}}false{{end}}),
{{- end}}
};
{{range .Decisions}}
{{- if .Description}}
/**
 * {{jsDocComment .Description}}
 *
 * Version: {{.Version}}
 */
{{- end}}
export function getDecision(name: '{{.Name}}'): Decision<{{.PascalName}}Questions>;
{{- end}}
export function getDecision(name: DecisionName): Decision<any>;
export function getDecision(name: DecisionName) {
  return _decisions[name];
}
`
