package python

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/generator"
)

// keywordKeysPrompts has JSON keys that are Python keywords or not identifiers
// in an input schema (nested in an array), an output schema and a decision
// question's inputs.
func keywordKeysPrompts() []generator.PromptData {
	str := map[string]interface{}{"type": "string"}
	link := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"from": str, "to": str, "needs-review": str, "0": str, "why": str},
		"required":   []interface{}{"from", "to"},
	}
	input := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"links": map[string]interface{}{"type": "array", "items": link},
			"class": str,
		},
		"required": []interface{}{"links", "class"},
	}
	output := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"edges": map[string]interface{}{"type": "array", "items": link},
		},
		"required": []interface{}{"edges"},
	}
	simple := map[string]interface{}{"type": "object", "properties": map[string]interface{}{"text": str}, "required": []interface{}{"text"}}
	spec := &generator.DecisionSpec{Questions: []generator.DecisionQuestion{{ID: "same", Type: "noul"}}}
	return []generator.PromptData{
		{
			Ref: "@acme/tutor-path", Name: "tutor-path", Version: "1.0.0", Status: "PUBLISHED", OutputSchema: output,
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "{{class}}{{#links}}{{from}} → {{to}}{{/links}}", IsEntrypoint: true, InputSchema: input},
				{Name: "systemPrompt", Content: "{{text}}", IsEntrypoint: true, InputSchema: simple},
			},
		},
		{
			Ref: "@acme/summarise", Name: "summarise", Version: "1.0.0", Status: "PUBLISHED",
			Files: []generator.PromptFile{
				{Name: "userPrompt", Content: "{{text}}", IsEntrypoint: true, InputSchema: simple},
				{Name: "systemPrompt", Content: "{{text}}", IsEntrypoint: true, InputSchema: simple},
			},
		},
		{
			Ref: "@acme/pair", Name: "pair", Version: "1.0.0", Status: "PUBLISHED",
			Kind: generator.KindSystemOne, DecisionSpec: spec,
			Files: []generator.PromptFile{{
				Name: "same", Content: "Is {{{from}}} the same as {{{to}}}?", IsEntrypoint: true,
				InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"from": str, "to": str}, "required": []interface{}{"from", "to"}},
			}},
		},
	}
}

func TestKeywordKeysUseFunctionalTypedDicts(t *testing.T) {
	output := generateAndRead(t, keywordKeysPrompts())
	assertContains(t, output, `_AcmeTutorPath_UserPromptInput_Links = TypedDict('_AcmeTutorPath_UserPromptInput_Links', {
    '0': NotRequired[Optional[str]],
    'from': str,
    'needs-review': NotRequired[Optional[str]],
    'to': str,
    'why': NotRequired[Optional[str]],
})`)
	assertContains(t, output, "AcmeTutorPath_UserPromptInput = TypedDict('AcmeTutorPath_UserPromptInput', {\n    'class': str,\n    'links': list[_AcmeTutorPath_UserPromptInput_Links],\n})")
	assertContains(t, output, "AcmePairSameInput = TypedDict('AcmePairSameInput', {\n    'from': str,\n    'to': str,\n})")
	assertContains(t, output, "class AcmeSummarise_UserPromptInput(TypedDict):\n    text: str")
}

// TestGeneratedKeywordKeysRuntime checks that the generated module compiles,
// type-checks under mypy --strict, renders keyword-keyed inputs and validates a
// keyword-keyed output payload.
func TestGeneratedKeywordKeysRuntime(t *testing.T) {
	python := runtimePython(t)
	work := t.TempDir()
	outFile := filepath.Join(work, "prompts.py")
	if err := (&Generator{}).Generate(outFile, keywordKeysPrompts()); err != nil {
		t.Fatal(err)
	}
	program := `import json
from prompts import AcmeTutorPath_UserPromptInput, AcmePairSameInput, get_decision, get_prompt

payload: AcmeTutorPath_UserPromptInput = {"class": "c", "links": [{"from": "a", "to": "b", "needs-review": "no"}]}
bad: AcmePairSameInput = {"from": "a"}  # type-error
rendered = get_prompt("@acme/tutor-path").render("userPrompt", payload)["prompt"]
question = get_decision("@acme/pair").question("same", {"from": "x", "to": "y"})
parsed = get_prompt("@acme/tutor-path").parse_output('{"edges": [{"from": "a", "to": "b"}]}')
print(json.dumps({"rendered": rendered, "question": question["instructions"], "parsed": parsed["success"]}, ensure_ascii=False))
`
	if err := os.WriteFile(filepath.Join(work, "program.py"), []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}

	compile := exec.Command(python, "-m", "py_compile", "prompts.py")
	compile.Dir = work
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("generated module does not compile: %v\n%s", err, out)
	}

	mypy := exec.Command(python, "-m", "mypy", "--strict", "--no-error-summary", "--ignore-missing-imports", "--warn-unused-ignores", "program.py", "prompts.py")
	mypy.Dir = work
	report, _ := mypy.CombinedOutput()
	// parse_output's inferred error code is a known, separate --strict finding.
	var errors []string
	for _, line := range strings.Split(strings.TrimSpace(string(report)), "\n") {
		if strings.Contains(line, "error:") && !strings.Contains(line, `TypedDict item "code"`) {
			errors = append(errors, line)
		}
	}
	if len(errors) != 1 || !strings.HasPrefix(errors[0], "program.py:5:") {
		t.Errorf("mypy should flag only program.py:5, got:\n%s", report)
	}

	run := exec.Command(python, "program.py")
	run.Dir = work
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("python: %v\n%s", err, out)
	}
	want := `{"rendered": "ca → b", "question": "Is x the same as y?", "parsed": true}`
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}
