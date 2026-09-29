package python

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
	Name         string
	PascalName   string
	Description  string
	Version      string
	Status       string
	AnswersModel string // pydantic class definitions
	AnswersClass string
	TypedDicts   []typedDictClass
	HasStateFile bool
	// StateType types the caller's raw state (from the inferred state schema);
	// StateInputsType types the state file's template inputs.
	StateType            string
	StateInputsType      string
	QuestionInputsType   string // "" when no question has template variables
	QuestionIDsLiteral   string
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
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Criteria json.RawMessage `json:"criteria,omitempty"`
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

type decisionDef struct {
	Model     string                `json:"model"`
	StateFile *string               `json:"stateFile"`
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

func prepareDecisionContent(content string, p generator.PromptData) string {
	return generator.ResolveFieldDirectives(generator.ResolveDirectives(content, p))
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
	td.AnswersModel, td.AnswersClass = jsonSchemaToPydantic(p.OutputSchema, td.PascalName+"Answers")

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

	var ids []string
	var questionInputFields []typedDictField
	for _, q := range spec.Questions {
		ids = append(ids, pyStringLiteral(q.ID))
		def.Questions = append(def.Questions, decisionQuestionDef{ID: q.ID, Type: q.Type, Criteria: q.Criteria})
		if err := emitFile(q.ID); err != nil {
			return td, err
		}
		if schema := filesByName[q.ID].InputSchema; hasProperties(schema) {
			var classes []typedDictClass
			typeName := collectTypedDicts(schema, td.PascalName+"_"+toPascalCase(q.ID)+"Input", &classes, true, analysis)
			td.TypedDicts = append(td.TypedDicts, classes...)
			questionInputFields = append(questionInputFields, typedDictField{Name: q.ID, Type: typeName})
		}
	}
	td.QuestionIDsLiteral = "(" + strings.Join(ids, ", ") + ",)"
	if len(questionInputFields) > 0 {
		name := td.PascalName + "QuestionInputs"
		td.TypedDicts = append(td.TypedDicts, typedDictClass{Name: name, Fields: questionInputFields})
		td.QuestionInputsType = name
	}

	if spec.StateFile != "" {
		td.HasStateFile = true
		if err := emitFile(spec.StateFile); err != nil {
			return td, err
		}
		td.StateInputsType = "dict[str, Any]"
		if schema := filesByName[spec.StateFile].InputSchema; hasProperties(schema) {
			var classes []typedDictClass
			td.StateInputsType = collectTypedDicts(schema, td.PascalName+"StateInputs", &classes, true, analysis)
			td.TypedDicts = append(td.TypedDicts, classes...)
		}
	} else {
		td.StateType = "Any"
		if hasProperties(p.StateSchema) {
			var classes []typedDictClass
			td.StateType = collectTypedDicts(p.StateSchema, td.PascalName+"State", &classes, true, analysis)
			td.TypedDicts = append(td.TypedDicts, classes...)
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

var decisionTemplate = `

# ─── Decision Prompts (System-One) ────────────────────────────────────────────
#
# Decision prompts target System-One models such as TypeSafe Jev: typed
# noul / choice / score questions evaluated against one state. get_decision()
# builds the request body (POST /v1/systemone) and validates the answers.


class DecisionParseFailure(TypedDict):
    error: str
    code: Literal["schema-validation"]
    success: Literal[False]


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


def _render_decision_file(file: dict[str, Any], view: dict[str, Any], partials: dict[str, str]) -> Any:
    def render(template: str) -> str:
        return chevron.render(template, view, partials_dict=partials)

    if file["kind"] == "text":
        return render(file["template"])
    return _render_tree(file["tree"], view, render)


def _build_decision_request(
    definition: dict[str, Any],
    state: Any,
    state_inputs: Any,
    question_inputs: Any,
) -> dict[str, Any]:
    state_file = definition["stateFile"]
    if state_file is not None:
        state = _render_decision_file(definition["files"][state_file], dict(state_inputs or {}), definition["partials"])
    questions: dict[str, Any] = {}
    for q in definition["questions"]:
        view = dict((question_inputs or {}).get(q["id"]) or {})
        entry: dict[str, Any] = {
            "type": q["type"],
            "instructions": _render_decision_file(definition["files"][q["id"]], view, definition["partials"]),
        }
        if "criteria" in q:
            entry["criteria"] = q["criteria"]
        questions[q["id"]] = entry
    return {"model": definition["model"], "state": state, "questions": questions}
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

{{.AnswersModel}}

class _{{.PascalName}}DecisionParseSuccess(TypedDict):
    data: {{.AnswersClass}}
    success: Literal[True]


_{{.PascalName}}_DECISION: dict[str, Any] = json.loads({{.DefJSONStringLiteral}})


class _{{.PascalName}}Decision:
    {{- if .Description}}
    """{{pyDocstring .Description}}

    Version: {{.Version}}
    """
    {{- end}}

    question_ids: tuple[str, ...] = {{.QuestionIDsLiteral}}
    model: str = _{{.PascalName}}_DECISION["model"]

    def build_request(
        self,
        *,
{{- if .HasStateFile}}
        state_inputs: {{.StateInputsType}},
{{- else}}
        state: {{.StateType}},
{{- end}}
{{- if .QuestionInputsType}}
        question_inputs: {{.QuestionInputsType}},
{{- end}}
    ) -> dict[str, Any]:
        """Build the request body for the System-One API (POST /v1/systemone)."""
        return _build_decision_request(
            _{{.PascalName}}_DECISION,
            {{if .HasStateFile}}None{{else}}state{{end}},
            {{if .HasStateFile}}state_inputs{{else}}None{{end}},
            {{if .QuestionInputsType}}question_inputs{{else}}None{{end}},
        )

    def parse_response(self, raw: Any) -> _{{.PascalName}}DecisionParseSuccess | DecisionParseFailure:
        """Validate a response (or its answers object) against the typed answers model."""
        candidate = raw["answers"] if isinstance(raw, dict) and "answers" in raw else raw
        try:
            validated = {{.AnswersClass}}.model_validate(candidate)
        except ValidationError as e:
            return {"error": str(e), "code": "schema-validation", "success": False}
        return {"data": validated, "success": True}
{{end}}
DecisionName = Literal[{{range $i, $d := .Decisions}}{{if $i}}, {{end}}"{{$d.Name}}"{{end}}]

_decisions: dict[str, Any] = {
{{- range .Decisions}}
    "{{.Name}}": _{{.PascalName}}Decision,
{{- end}}
}

_draft_decisions: set[str] = set([
{{- range .Decisions}}
{{- if eq .Status "DRAFT"}}
    "{{.Name}}",
{{- end}}
{{- end}}
])
{{range .Decisions}}

@overload
def get_decision(name: Literal["{{.Name}}"]) -> _{{.PascalName}}Decision: ...
{{end}}

def get_decision(name: DecisionName) -> Any:
    """Get a typed decision prompt: build_request(...) and parse_response(raw)."""
    if name in _draft_decisions:
        warnings.warn(f'[sufleur] Warning: decision prompt "{name}" is a draft version', stacklevel=2)
    return _decisions[name]()
`
