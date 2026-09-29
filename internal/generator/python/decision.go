package python

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
	ID        string
	IDLiteral string
	// InputsType is the TypedDict of the question's template inputs, or "" when
	// it has none (the argument is then optional).
	InputsType string
	// Open reports an open choice: callers may add options.
	Open bool
	// OptionInputsType is the TypedDict of one added option's inputs
	// ("dict[str, Any]" when the option template has no variables).
	OptionInputsType string
	AnswerClass      string
}

// decisionTemplateData is one SYSTEM_ONE decision prompt.
type decisionTemplateData struct {
	Name                 string
	PascalName           string
	Description          string
	Version              string
	Status               string
	AnswerModels         string // pydantic class definitions for every question's answer
	TypedDicts           []typedDictClass
	Questions            []decisionQuestionData
	DefJSONStringLiteral string
}

type decisionContext struct {
	Decisions                []decisionTemplateData
	WholeValuePatternLiteral string
}

type decisionTextFile struct {
	Kind     string `json:"kind"`
	Template string `json:"template"`
}

type decisionYAMLFile struct {
	Kind string          `json:"kind"`
	Tree json.RawMessage `json:"tree"`
}

type decisionQuestionDef struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Criteria       json.RawMessage `json:"criteria,omitempty"`
	OptionCriteria json.RawMessage `json:"optionCriteria,omitempty"`
	// Required / OptionRequired are the top-level inputs a question (or one
	// added option) cannot render without; checked before rendering.
	Required       []string `json:"required,omitempty"`
	OptionRequired []string `json:"optionRequired,omitempty"`
}

type orderedJSON struct {
	keys   []string
	values []interface{}
}

func (o *orderedJSON) add(key string, value interface{}) {
	o.keys = append(o.keys, key)
	o.values = append(o.values, value)
}

func (o orderedJSON) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
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
	return b.Bytes(), nil
}

type decisionMetadata struct {
	Version     string                 `json:"version"`
	ModelConfig map[string]interface{} `json:"modelConfig"`
}

type decisionDef struct {
	Metadata  decisionMetadata      `json:"metadata"`
	Questions []decisionQuestionDef `json:"questions"`
	Files     orderedJSON           `json:"files"`
	Partials  orderedJSON           `json:"partials"`
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

// requiredKeys lists a schema's top-level required properties.
func requiredKeys(schema map[string]interface{}) []string {
	raw, _ := schema["required"].([]interface{})
	var keys []string
	for _, k := range raw {
		if s, ok := k.(string); ok {
			keys = append(keys, s)
		}
	}
	return keys
}

func hasProperties(schema map[string]interface{}) bool {
	props, ok := schema["properties"].(map[string]interface{})
	return ok && len(props) > 0
}

func buildDecisionData(p generator.PromptData, analysis *inputAnalysis) (decisionTemplateData, error) {
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

	var models []string
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
			Required: requiredKeys(f.InputSchema), OptionRequired: requiredKeys(f.OptionInputSchema),
		})

		qPascal := td.PascalName + toPascalCase(q.ID)
		qd := decisionQuestionData{ID: q.ID, IDLiteral: pyStringLiteral(q.ID), AnswerClass: "Any"}
		if hasProperties(f.InputSchema) {
			var classes []typedDictClass
			qd.InputsType = collectTypedDicts(f.InputSchema, qPascal+"Input", &classes, true, analysis)
			td.TypedDicts = append(td.TypedDicts, classes...)
		}
		if q.IsOpenChoice() {
			qd.Open = true
			qd.OptionInputsType = "dict[str, Any]"
			if hasProperties(f.OptionInputSchema) {
				var classes []typedDictClass
				qd.OptionInputsType = collectTypedDicts(f.OptionInputSchema, qPascal+"OptionInput", &classes, true, analysis)
				td.TypedDicts = append(td.TypedDicts, classes...)
			}
		}
		if schema, ok := answerSchemas[q.ID].(map[string]interface{}); ok {
			model, class := jsonSchemaToPydantic(schema, qPascal+"Answer")
			if model != "" {
				models = append(models, model)
			}
			qd.AnswerClass = class
		}
		td.Questions = append(td.Questions, qd)
	}
	td.AnswerModels = strings.Join(models, "\n\n")

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

	raw, err := marshalUnescaped(def)
	if err != nil {
		return td, err
	}
	td.DefJSONStringLiteral = pyStringLiteral(string(raw))
	return td, nil
}

func buildDecisionContext(prompts []generator.PromptData, analysis *inputAnalysis) (decisionContext, error) {
	ctx := decisionContext{WholeValuePatternLiteral: pyStringLiteral(generator.WholeValuePattern)}
	for _, p := range prompts {
		d, err := buildDecisionData(p, analysis)
		if err != nil {
			return ctx, err
		}
		ctx.Decisions = append(ctx.Decisions, d)
	}
	return ctx, nil
}

// decisionIdentifiers are the module-level names a decision prompt claims.
func decisionIdentifiers(d decisionTemplateData) []string {
	ids := []string{"_" + d.PascalName + "Decision", "_" + d.PascalName + "Batch"}
	for _, c := range d.TypedDicts {
		ids = append(ids, c.Name)
	}
	for _, q := range d.Questions {
		if q.AnswerClass != "Any" && !strings.Contains(q.AnswerClass, "[") {
			ids = append(ids, q.AnswerClass)
		}
	}
	return ids
}

var decisionTemplate = `

# ─── Decision Prompts (System-One) ────────────────────────────────────────────
#
# Decision prompts are question templates for System-One models such as
# TypeSafe Jev: typed noul / choice / score questions. Render a question with
# question(), or collect several with batch(); send them to your provider
# together with your own state; read the answers back with parse_answer() or
# the batch's read(). No provider request or response shape lives here.

_A = TypeVar("_A")


class DecisionHandle(Generic[_A]):
    """Returned by batch.ask(); reads the same question's answer back."""

    __slots__ = ("key", "question_id", "question")

    def __init__(self, key: str, question_id: str, question: dict[str, Any]) -> None:
        self.key = key
        self.question_id = question_id
        self.question = question


class AnswerResult(Generic[_A]):
    """One answer: success with data, or failure with an error."""

    __slots__ = ("success", "data", "error")

    def __init__(self, success: bool, data: Optional[_A] = None, error: Optional[str] = None) -> None:
        self.success = success
        self.data = data
        self.error = error


class DecisionAnswers:
    """Partial read: every answer that validated, plus every problem."""

    def __init__(self, results: dict[int, AnswerResult[Any]], errors: list[str]) -> None:
        self._results = results
        self.errors = errors

    def get(self, handle: DecisionHandle[_A]) -> AnswerResult[_A]:
        result = self._results.get(id(handle))
        if result is None:
            raise KeyError("[sufleur] this handle was not asked in this batch")
        return result

    def get_or_throw(self, handle: DecisionHandle[_A]) -> _A:
        result = self.get(handle)
        if not result.success:
            raise ValueError(result.error)
        return result.data  # type: ignore[return-value]


class DecisionReadAllResult:
    """All-or-nothing read: success only when every answer validated."""

    def __init__(self, answers: Optional[DecisionAnswers], errors: list[str]) -> None:
        self.success = answers is not None
        self.errors = errors
        self._answers = answers

    def get(self, handle: DecisionHandle[_A]) -> _A:
        if self._answers is None:
            raise ValueError("[sufleur] the answers did not validate: " + "; ".join(self.errors))
        return self._answers.get_or_throw(handle)


_WHOLE_VALUE_RE = re.compile({{.WholeValuePatternLiteral}})


def _lookup(view: Any, name: str) -> Any:
    if name == ".":
        return view
    current = view
    for key in name.split("."):
        if not isinstance(current, dict):
            return None
        current = current.get(key)
    return current


def _render_tree(tree: Any, view: dict[str, Any], render: Any) -> Any:
    if isinstance(tree, str):
        m = _WHOLE_VALUE_RE.match(tree)
        if m:
            return _lookup(view, (m.group(1) or m.group(2) or "").strip())
        return render(tree)
    if isinstance(tree, list):
        return [_render_tree(item, view, render) for item in tree]
    if isinstance(tree, dict):
        return {k: _render_tree(v, view, render) for k, v in tree.items()}
    return tree


def _render_entry(entry: Any, render: Any) -> Any:
    # Criteria: every string is a template; keys and other values pass through.
    if isinstance(entry, str):
        return render(entry)
    if isinstance(entry, list):
        return [_render_entry(item, render) for item in entry]
    if isinstance(entry, dict):
        return {k: _render_entry(v, render) for k, v in entry.items()}
    return entry


def _render_question(
    definition: dict[str, Any],
    question_id: str,
    inputs: Optional[Mapping[str, Any]] = None,
    options: Optional[Mapping[str, Mapping[str, Any]]] = None,
) -> dict[str, Any]:
    q = next((item for item in definition["questions"] if item["id"] == question_id), None)
    file = definition["files"].get(question_id)
    if q is None or file is None:
        raise KeyError('[sufleur] unknown question "' + question_id + '"')
    partials = definition["partials"]

    def missing(required: Optional[list[str]], view: Mapping[str, Any]) -> list[str]:
        return [name for name in (required or []) if view.get(name) is None]

    absent = missing(q.get("required"), inputs or {})
    if absent:
        raise ValueError('[sufleur] "' + question_id + '" is missing required input(s): ' + ", ".join(absent))

    def renderer(view: Mapping[str, Any]) -> Any:
        return lambda template: chevron.render(template, dict(view), partials_dict=partials)

    view = dict(inputs or {})
    if file["kind"] == "text":
        # Editors save files with a trailing newline; it must not reach the model.
        instructions = renderer(view)(file["template"]).rstrip(" \t\r\n")
    else:
        instructions = _render_tree(file["tree"], view, renderer(view))
    rendered: dict[str, Any] = {"type": q["type"], "instructions": instructions}
    if "criteria" in q:
        rendered["criteria"] = _render_entry(q["criteria"], renderer(view))

    added = list((options or {}).items())
    if q["type"] != "choice":
        if added:
            raise ValueError('[sufleur] "' + question_id + '" is a ' + q["type"] + " question: it takes no options")
        return rendered
    if added and "optionCriteria" not in q:
        raise ValueError(
            '[sufleur] "' + question_id + '" has a fixed set of options: add optionCriteria to let callers add options'
        )
    criteria = dict(rendered.get("criteria") or {})
    for key, option_inputs in added:
        if key.strip() == "" or len(key) > 255:
            raise ValueError('[sufleur] "' + question_id + '": option keys must be non-blank and at most 255 characters')
        if key in criteria:
            raise ValueError('[sufleur] "' + question_id + '": option "' + key + '" is already one of the fixed options')
        option_absent = missing(q.get("optionRequired"), option_inputs or {})
        if option_absent:
            raise ValueError(
                '[sufleur] "' + question_id + '": option "' + key + '" is missing required input(s): '
                + ", ".join(option_absent)
            )
        criteria[key] = _render_entry(q["optionCriteria"], renderer(option_inputs or {}))
    if len(criteria) < 2 or len(criteria) > 255:
        raise ValueError(
            '[sufleur] "' + question_id + '": a choice needs between 2 and 255 options (this one has '
            + str(len(criteria)) + ")"
        )
    rendered["criteria"] = criteria
    return rendered


class _DecisionBase:
    _name: str
    _definition: dict[str, Any]
    _answer_types: dict[str, Any]
    _draft: bool

    @property
    def metadata(self) -> dict[str, Any]:
        """The version and recommended model config, like get_prompt(...).metadata."""
        metadata: dict[str, Any] = self._definition["metadata"]
        return metadata

    @property
    def question_ids(self) -> tuple[str, ...]:
        return tuple(q["id"] for q in self._definition["questions"])

    def _parse(self, question_id: str, raw: Any) -> AnswerResult[Any]:
        answer_type = self._answer_types.get(question_id)
        if answer_type is None:
            raise KeyError('[sufleur] unknown question "' + question_id + '"')
        try:
            return AnswerResult(True, data=TypeAdapter(answer_type).validate_python(raw))
        except ValidationError as e:
            return AnswerResult(False, error="[" + question_id + "] " + str(e))


class _BatchBase:
    def __init__(self, decision: _DecisionBase) -> None:
        self._decision = decision
        self._asked: dict[str, DecisionHandle[Any]] = {}

    def _ask(
        self,
        question_id: str,
        inputs: Optional[Mapping[str, Any]],
        key: Optional[str],
        options: Optional[Mapping[str, Mapping[str, Any]]],
    ) -> DecisionHandle[Any]:
        k = key if key is not None else question_id
        if k in self._asked:
            raise ValueError(
                '[sufleur] key "' + k + '" is already used in this batch: give repeated questions distinct keys'
            )
        handle: DecisionHandle[Any] = DecisionHandle(
            k, question_id, _render_question(self._decision._definition, question_id, inputs, options)
        )
        self._asked[k] = handle
        return handle

    def items(self) -> list[tuple[str, str, dict[str, Any]]]:
        """Every asked question, in order, as (key, question_id, question)."""
        return [(h.key, h.question_id, h.question) for h in self._asked.values()]

    def _collect(self, answers: Mapping[str, Any]) -> tuple[dict[int, AnswerResult[Any]], list[str]]:
        results: dict[int, AnswerResult[Any]] = {}
        errors: list[str] = []
        for k, handle in self._asked.items():
            if k not in answers:
                error = "[" + k + "] no answer"
                errors.append(error)
                results[id(handle)] = AnswerResult(False, error=error)
                continue
            parsed = self._decision._parse(handle.question_id, answers[k])
            if not parsed.success:
                error = str(parsed.error) if k == handle.question_id else k + " " + str(parsed.error)
                errors.append(error)
                results[id(handle)] = AnswerResult(False, error=error)
            else:
                results[id(handle)] = parsed
        return results, errors

    def read(self, answers: Mapping[str, Any]) -> DecisionAnswers:
        """Match answers back by key; keeps every answer that validated."""
        results, errors = self._collect(answers)
        return DecisionAnswers(results, errors)

    def read_all(self, answers: Mapping[str, Any]) -> DecisionReadAllResult:
        """Match answers back by key; fails unless every answer validated."""
        results, errors = self._collect(answers)
        return DecisionReadAllResult(None if errors else DecisionAnswers(results, errors), errors)
{{range .Decisions}}
# ─── Decision {{.Name}} ────────────────────────────────────────────────
{{range .TypedDicts}}

class {{.Name}}(TypedDict):
{{- range .Fields}}
    {{.Name}}: {{.Type}}
    {{- if .Description}}
    """{{pyDocstring .Description}}"""
    {{- end}}
{{- end}}
{{end}}
{{if .AnswerModels}}
{{.AnswerModels}}
{{end}}

class _{{.PascalName}}Batch(_BatchBase):
{{- range .Questions}}
    @overload
    def ask(
        self,
        question_id: Literal[{{.IDLiteral}}],
        {{if .InputsType}}inputs: {{.InputsType}}{{else}}inputs: Optional[dict[str, Any]] = None{{end}},
        *,
        key: Optional[str] = None,
{{- if .Open}}
        options: Optional[Mapping[str, {{.OptionInputsType}}]] = None,
{{- end}}
    ) -> DecisionHandle[{{.AnswerClass}}]: ...
{{- end}}

    def ask(self, question_id: Any, inputs: Any = None, *, key: Optional[str] = None, options: Any = None) -> Any:
        """Ask a question under key (default: its question id); returns a typed handle."""
        return self._ask(question_id, inputs, key, options)


class _{{.PascalName}}Decision(_DecisionBase):
    {{- if .Description}}
    """{{pyDocstring .Description}}

    Version: {{.Version}}
    """
    {{- end}}

    _name = "{{.Name}}"
    _definition: dict[str, Any] = json.loads({{.DefJSONStringLiteral}})
    _answer_types: dict[str, Any] = {
{{- range .Questions}}
        {{.IDLiteral}}: {{.AnswerClass}},
{{- end}}
    }
    _draft = {{if eq .Status "DRAFT"}}True{{else}}False{{end}}
{{range .Questions}}
    @overload
    def question(
        self,
        question_id: Literal[{{.IDLiteral}}],
        {{if .InputsType}}inputs: {{.InputsType}}{{else}}inputs: Optional[dict[str, Any]] = None{{end}},
{{- if .Open}}
        *,
        options: Optional[Mapping[str, {{.OptionInputsType}}]] = None,
{{- end}}
    ) -> dict[str, Any]: ...
{{- end}}

    def question(self, question_id: Any, inputs: Any = None, *, options: Any = None) -> Any:
        """Render one question into what a System-One API takes under questions.<key>."""
        return _render_question(self._definition, question_id, inputs, options)
{{range .Questions}}
    @overload
    def parse_answer(self, question_id: Literal[{{.IDLiteral}}], raw: Any) -> AnswerResult[{{.AnswerClass}}]: ...
{{- end}}

    def parse_answer(self, question_id: Any, raw: Any) -> Any:
        """Validate one raw answer against the question's answer type."""
        return self._parse(question_id, raw)

    def batch(self) -> _{{.PascalName}}Batch:
        """Start a batch of questions."""
        return _{{.PascalName}}Batch(self)
{{end}}
DecisionName = Literal[{{range $i, $d := .Decisions}}{{if $i}}, {{end}}"{{$d.Name}}"{{end}}]

_decisions: dict[str, Any] = {
{{- range .Decisions}}
    "{{.Name}}": _{{.PascalName}}Decision,
{{- end}}
}
{{if eq (len .Decisions) 1}}{{with index .Decisions 0}}

def get_decision(name: Literal["{{.Name}}"]) -> _{{.PascalName}}Decision:
    """Get a decision prompt: question(), parse_answer() and batch()."""
    decision = _{{.PascalName}}Decision()
    if decision._draft:
        warnings.warn(f'[sufleur] Warning: decision prompt "{name}" is a draft version', stacklevel=2)
    return decision
{{end}}{{else}}{{range .Decisions}}

@overload
def get_decision(name: Literal["{{.Name}}"]) -> _{{.PascalName}}Decision: ...
{{end}}

def get_decision(name: DecisionName) -> Any:
    """Get a decision prompt: question(), parse_answer() and batch()."""
    decision = _decisions[name]()
    if decision._draft:
        warnings.warn(f'[sufleur] Warning: decision prompt "{name}" is a draft version', stacklevel=2)
    return decision
{{end}}`
