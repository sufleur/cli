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
		"export type PromptName = never;",
		"import { z } from 'zod';",
		"export type DecisionName = | '@acme/triage';",
		`choice: z.enum(["technical", "billing"])`,
		"export type AcmeTriageStateInputs = {",
		"export type AcmeTriageQuestionInputs = {",
		"questionInputs: AcmeTriageQuestionInputs;",
		`"template": "Is ` + "`ticket.body`" + ` urgent for a {{tier}} customer?"`,
		`"question": "Which team owns ` + "`ticket.body`" + `?"`,
		`"policy": "{{> policy}}"`,
		`"policy": "Refunds within 30 days."`,
		"export function getDecision(name: '@acme/triage'): DecisionResult<'@acme/triage'>;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated output missing %q", want)
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
