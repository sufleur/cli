package python

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/generator"
)

func decisionFixture(t *testing.T) generator.PromptData {
	t.Helper()
	var spec generator.DecisionSpec
	if err := json.Unmarshal([]byte(`{"questions":{"isUrgent":{"type":"noul","criteria":{"true":"Urgent for a {{{tier}}} customer"}},"department":{"type":"choice","criteria":{"technical":null,"billing":"Payments"}},"about":{"type":"choice","criteria":{"none":null},"optionCriteria":{"what":"about {{{name}}}"}}}}`), &spec); err != nil {
		t.Fatal(err)
	}
	obj := func(props map[string]interface{}, required ...interface{}) map[string]interface{} {
		return map[string]interface{}{"type": "object", "properties": props, "required": required}
	}
	str := map[string]interface{}{"type": "string"}
	prob := map[string]interface{}{"type": "number"}
	return generator.PromptData{
		Ref:          "@acme/triage",
		Name:         "triage",
		Version:      "1.0.0",
		Status:       "PUBLISHED",
		Kind:         generator.KindSystemOne,
		DecisionSpec: &spec,
		ModelConfig:  map[string]interface{}{"provider": "TYPESAFE", "model": "jev-latest"},
		OutputSchema: obj(map[string]interface{}{
			"isUrgent":   obj(map[string]interface{}{"type": map[string]interface{}{"type": "string", "enum": []interface{}{"noul"}}, "noul": prob}, "type", "noul"),
			"department": obj(map[string]interface{}{"type": map[string]interface{}{"type": "string", "enum": []interface{}{"choice"}}, "choice": map[string]interface{}{"type": "string", "enum": []interface{}{"technical", "billing"}}}, "type", "choice"),
			"about":      obj(map[string]interface{}{"type": map[string]interface{}{"type": "string", "enum": []interface{}{"choice"}}, "choice": str}, "type", "choice"),
		}),
		Files: []generator.PromptFile{
			{Name: "isUrgent", Content: "Is `ticket.body` urgent?", IsEntrypoint: true,
				InputSchema: obj(map[string]interface{}{"tier": str}, "tier")},
			{Name: "department", Content: "question: \"Which team owns `ticket.body`?\"\npolicy: \"{{> policy}}\"", IsEntrypoint: true, Format: generator.FormatYAML},
			{Name: "about", Content: "Which concept is it about?", IsEntrypoint: true,
				OptionInputSchema: obj(map[string]interface{}{"name": str}, "name")},
			{Name: "policy", Content: "Refunds within 30 days."},
		},
	}
}

func TestDecisionPromptGeneratesGetDecision(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{decisionFixture(t)})
	for _, want := range []string{
		"PromptName = Literal[None]",
		"from pydantic import TypeAdapter",
		`DecisionName = Literal["@acme/triage"]`,
		"class AcmeTriageIsUrgentInput(TypedDict):",
		"class AcmeTriageAboutOptionInput(TypedDict):",
		"class AcmeTriageDepartmentAnswer(BaseModel):",
		"question_id: Literal['isUrgent'],",
		"inputs: AcmeTriageIsUrgentInput,",
		"options: Optional[Mapping[str, AcmeTriageAboutOptionInput]] = None,",
		") -> DecisionHandle[AcmeTriageAboutAnswer]: ...",
		`def get_decision(name: Literal["@acme/triage"]) -> _AcmeTriageDecision:`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated output missing %q", want)
		}
	}
	for _, gone := range []string{"build_request", "parse_response", "stateFile", "StateInputs"} {
		if strings.Contains(out, gone) {
			t.Errorf("generated output still mentions %q", gone)
		}
	}
}

// The generated module must at least be valid Python (compile-only; no deps
// needed). Skipped when no python3 is on PATH.
func TestDecisionPromptCompiles(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	out := generateAndRead(t, []generator.PromptData{decisionFixture(t)})
	file := filepath.Join(t.TempDir(), "decisions.py")
	if err := os.WriteFile(file, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	if msg, err := exec.Command(python, "-m", "py_compile", file).CombinedOutput(); err != nil {
		t.Fatalf("generated Python does not compile: %v\n%s", err, msg)
	}
}
