package typescript

import (
	"encoding/json"
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
		"export type PromptName = never;",
		"import { z } from 'zod';",
		"export type DecisionName = | '@acme/triage';",
		"export const AcmeTriageDepartmentAnswerSchema = z.object({",
		`choice: z.enum(["technical", "billing"])`,
		"export type AcmeTriageQuestions = {",
		"    type: 'choice';",
		"    optionInputs: {",
		"answer: z.infer<typeof AcmeTriageAboutAnswerSchema>;",
		`"template": "Is ` + "`ticket.body`" + ` urgent?"`,
		`"question": "Which team owns ` + "`ticket.body`" + `?"`,
		`"policy": "{{> policy}}"`,
		`"policy": "Refunds within 30 days."`,
		`"optionCriteria": {`,
		`"model": "jev-latest"`,
		"export function getDecision(name: '@acme/triage'): Decision<AcmeTriageQuestions>;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated output missing %q", want)
		}
	}
	for _, gone := range []string{"buildRequest", "parseResponse", "stateFile", "StateInputs"} {
		if strings.Contains(out, gone) {
			t.Errorf("generated output still mentions %q", gone)
		}
	}
	if strings.Index(out, `"id": "isUrgent"`) > strings.Index(out, `"id": "department"`) {
		t.Error("questions must be emitted in authored order")
	}
	if strings.Contains(out, "\\u003e") {
		t.Error("partial tags must not be HTML-escaped")
	}
}

func TestDecisionPromptsLeaveLLMSectionUntouched(t *testing.T) {
	llm := generator.PromptData{
		Ref: "@acme/summarise", Name: "summarise", Version: "1.0.0", Status: "PUBLISHED",
		Files: []generator.PromptFile{{Name: "userPrompt", Content: "Summarise {{text}}", IsEntrypoint: true}},
	}
	alone := generateAndRead(t, []generator.PromptData{llm})
	mixed := generateAndRead(t, []generator.PromptData{llm, decisionFixture(t)})

	// Everything up to the decision section is identical (modulo the timestamp
	// and the zod import the decision section needs).
	strip := func(s string) string {
		lines := strings.Split(s, "\n")
		var kept []string
		for _, l := range lines {
			if strings.HasPrefix(l, "// Generated at:") || strings.Contains(l, "zod") {
				continue
			}
			kept = append(kept, l)
		}
		return strings.Join(kept, "\n")
	}
	prefix := strings.SplitN(mixed, "// ─── Decision Prompts", 2)[0]
	if strings.TrimSpace(strip(prefix)) != strings.TrimSpace(strip(alone)) {
		t.Fatal("adding a decision prompt changed the LLM getPrompt() section")
	}
}
