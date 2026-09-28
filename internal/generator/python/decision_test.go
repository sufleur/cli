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
	if err := json.Unmarshal([]byte(`{"stateFile":"state","questions":{"isUrgent":{"type":"noul"},"department":{"type":"choice","criteria":{"technical":null,"billing":"Payments"}}}}`), &spec); err != nil {
		t.Fatal(err)
	}
	return generator.PromptData{
		Ref:          "@acme/triage",
		Name:         "triage",
		Version:      "1.0.0",
		Status:       "PUBLISHED",
		Kind:         generator.KindSystemOne,
		DecisionSpec: &spec,
		ModelConfig:  map[string]interface{}{"provider": "TYPESAFE", "model": "jev-latest"},
		OutputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"isUrgent":   map[string]interface{}{"type": "object", "properties": map[string]interface{}{"noul": map[string]interface{}{"type": "number"}}, "required": []interface{}{"noul"}},
				"department": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"choice": map[string]interface{}{"type": "string", "enum": []interface{}{"technical", "billing"}}}, "required": []interface{}{"choice"}},
			},
			"required": []interface{}{"isUrgent", "department"},
		},
		Files: []generator.PromptFile{
			{Name: "isUrgent", Content: "Is {{@field ticket.body}} urgent for a {{tier}} customer?", IsEntrypoint: true,
				InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"tier": map[string]interface{}{"type": "string"}}, "required": []interface{}{"tier"}}},
			{Name: "department", Content: "question: \"Which team owns {{@field ticket.body}}?\"\npolicy: \"{{> policy}}\"", IsEntrypoint: true, Format: generator.FormatYAML},
			{Name: "state", Content: "ticket: \"{{ticket}}\"", IsEntrypoint: true, Format: generator.FormatYAML,
				InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"ticket": map[string]interface{}{}}, "required": []interface{}{"ticket"}}},
			{Name: "policy", Content: "Refunds within 30 days."},
		},
	}
}

func TestDecisionPromptGeneratesGetDecision(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{decisionFixture(t)})
	for _, want := range []string{
		"PromptName = Literal[None]",
		"from pydantic import BaseModel, ValidationError",
		`DecisionName = Literal["@acme/triage"]`,
		"class AcmeTriageStateInputs(TypedDict):",
		"class AcmeTriageQuestionInputs(TypedDict):",
		"question_inputs: AcmeTriageQuestionInputs,",
		"state_inputs: AcmeTriageStateInputs,",
		`def get_decision(name: Literal["@acme/triage"]) -> _AcmeTriageDecision: ...`,
		"question_ids: tuple[str, ...] = ('isUrgent', 'department',)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated output missing %q", want)
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
