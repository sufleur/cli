package python

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/generator"
)

// runtimePython returns a Python interpreter with chevron, pydantic and mypy
// installed, or skips the test. Generated code only runs against real
// dependencies, so these tests are opt-in:
//
//	SUFLEUR_PY_RUNTIME=/path/to/venv/bin/python go test ./internal/generator/python/
func runtimePython(t *testing.T) string {
	t.Helper()
	python := os.Getenv("SUFLEUR_PY_RUNTIME")
	if python == "" {
		t.Skip("set SUFLEUR_PY_RUNTIME to a python with chevron, pydantic and mypy installed")
	}
	return python
}

type sharedRenderCase struct {
	Name         string          `json:"name"`
	Question     json.RawMessage `json:"question"`
	Instructions struct {
		Format  string `json:"format"`
		Content string `json:"content"`
	} `json:"instructions"`
	Partials      map[string]string `json:"partials"`
	Inputs        json.RawMessage   `json:"inputs"`
	Options       json.RawMessage   `json:"options"`
	Expected      json.RawMessage   `json:"expected"`
	ExpectedError string            `json:"expectedError"`
	Escaped       map[string]string `json:"escaped"`
}

// TestGeneratedDecisionRuntimeMatchesSharedCases generates a decision prompt per
// shared rendering case, runs question() in Python, and compares the output
// with the case's expected bytes — the same file the backend and Go render use.
func TestGeneratedDecisionRuntimeMatchesSharedCases(t *testing.T) {
	python := runtimePython(t)
	raw, err := os.ReadFile("../testdata/decision-render-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []sharedRenderCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}

	var prompts []generator.PromptData
	var calls strings.Builder
	for i, c := range file.Cases {
		var spec generator.DecisionSpec
		if err := json.Unmarshal([]byte(`{"questions":{"q":`+string(c.Question)+`}}`), &spec); err != nil {
			t.Fatal(err)
		}
		files := []generator.PromptFile{{Name: "q", Content: c.Instructions.Content, IsEntrypoint: true}}
		if c.Instructions.Format == "yaml" {
			files[0].Format = generator.FormatYAML
		}
		for name, content := range c.Partials {
			files = append(files, generator.PromptFile{Name: name, Content: content})
		}
		name := fmt.Sprintf("c%d", i)
		prompts = append(prompts, generator.PromptData{
			Ref: "@t/" + name, Name: name, Version: "1.0.0", Status: "PUBLISHED",
			Kind: generator.KindSystemOne, DecisionSpec: &spec, Files: files,
		})
		inputs, options := string(c.Inputs), string(c.Options)
		if inputs == "" {
			inputs = "{}"
		}
		if options == "" {
			options = "null"
		}
		fmt.Fprintf(&calls, "run(lambda: get_decision(%q).question('q', json.loads(%q), options=json.loads(%q)))\n", "@t/"+name, inputs, options)
	}

	work := t.TempDir()
	if err := (&Generator{}).Generate(filepath.Join(work, "prompts.py"), prompts); err != nil {
		t.Fatal(err)
	}
	script := `import json
from prompts import get_decision

out = []


def run(fn):
    try:
        out.append({"ok": json.dumps(fn(), ensure_ascii=False, separators=(",", ":"))})
    except Exception as e:
        out.append({"error": str(e)})


` + calls.String() + `print(json.dumps(out, ensure_ascii=False))
`
	if err := os.WriteFile(filepath.Join(work, "run.py"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, "run.py")
	cmd.Dir = work
	stdout, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("running generated code: %v\n%s", err, stderr)
	}
	var results []struct {
		Ok    *string `json:"ok"`
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(stdout, &results); err != nil {
		t.Fatalf("parsing output: %v\n%s", err, stdout)
	}

	for i, c := range file.Cases {
		r := results[i]
		t.Run(c.Name, func(t *testing.T) {
			switch {
			case c.ExpectedError != "":
				if r.Error == nil || !strings.Contains(*r.Error, c.ExpectedError) {
					t.Fatalf("got %+v, want an error containing %q", r, c.ExpectedError)
				}
			case c.Escaped != nil:
				var got struct{ Instructions string }
				if r.Ok == nil || json.Unmarshal([]byte(*r.Ok), &got) != nil {
					t.Fatalf("got %+v", r)
				}
				if got.Instructions != c.Escaped["chevron"] {
					t.Fatalf("got %q, want %q", got.Instructions, c.Escaped["chevron"])
				}
			default:
				var want bytes.Buffer
				if err := json.Compact(&want, c.Expected); err != nil {
					t.Fatal(err)
				}
				// Python's json.dumps escapes nothing we compare on (ensure_ascii off),
				// but re-compact to normalise number formatting (30 vs 30).
				var got bytes.Buffer
				if r.Ok == nil || json.Compact(&got, []byte(*r.Ok)) != nil || got.String() != want.String() {
					t.Fatalf("\n got %v (error %v)\nwant %s", deref(r.Ok), deref(r.Error), want.String())
				}
			}
		})
	}
}

// TestGeneratedDecisionTypesAndBatch type-checks the generated module with mypy
// (every "# type-error" line must be reported), then runs a batch end to end.
func TestGeneratedDecisionTypesAndBatch(t *testing.T) {
	python := runtimePython(t)
	work := t.TempDir()
	if err := (&Generator{}).Generate(filepath.Join(work, "prompts.py"), []generator.PromptData{decisionFixture(t)}); err != nil {
		t.Fatal(err)
	}
	program := `import json
from typing import Any

from prompts import get_decision

triage = get_decision("@acme/triage")
batch = triage.batch()
urgent = batch.ask("isUrgent", {"tier": "gold"})
again = batch.ask("isUrgent", {"tier": "free"}, key="isUrgent:free")
team = batch.ask("department")
about = batch.ask("about", options={"k01": {"name": "functor"}, "k02": {"name": "monad"}})

def _type_checks() -> None:
    batch.ask("isUrgent")  # type-error: missing inputs
    batch.ask("department", options={"x": {}})  # type-error: closed choice takes no options
    batch.ask("priority")  # type-error: unknown question
    triage.question("about", options={"k01": {"nam": "x"}})  # type-error: option inputs are typed

questions: dict[str, Any] = {key: question for key, _, question in batch.items()}

read = batch.read({
    "isUrgent": {"type": "noul", "noul": 0.9},
    "isUrgent:free": {"type": "noul", "noul": "high"},
    "department": {"type": "choice", "choice": "billing"},
    "about": {"type": "choice", "choice": "k02"},
})
n: float = read.get_or_throw(urgent).noul
c: str = read.get_or_throw(team).choice
a: str = read.get_or_throw(about).choice
def _more_type_checks() -> None:
    read.get_or_throw(urgent).choice  # type-error: a noul answer has no choice
bad = read.get(again)

all_ = batch.read_all({"isUrgent": {"type": "noul", "noul": 0.9}})

duplicate = ""
try:
    batch.ask("isUrgent", {"tier": "x"})
except ValueError as e:
    duplicate = str(e)

print(json.dumps({
    "keys": list(questions),
    "aboutCriteria": questions["about"]["criteria"],
    "urgentCriteria": questions["isUrgent:free"]["criteria"],
    "n": n, "c": c, "a": a,
    "badOk": bad.success,
    "readErrors": len(read.errors),
    "allOk": all_.success,
    "allErrors": len(all_.errors),
    "duplicate": duplicate,
    "model": triage.metadata["modelConfig"]["model"],
}))
`
	if err := os.WriteFile(filepath.Join(work, "program.py"), []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}

	// mypy must report exactly the "# type-error" lines, and nothing else.
	mypy := exec.Command(python, "-m", "mypy", "--strict", "--no-error-summary", "--follow-imports=silent", "program.py")
	mypy.Dir = work
	report, _ := mypy.CombinedOutput()
	var expected []int
	for i, line := range strings.Split(program, "\n") {
		if strings.Contains(line, "# type-error") {
			expected = append(expected, i+1)
		}
	}
	reported := map[int]bool{}
	for _, line := range strings.Split(string(report), "\n") {
		var n int
		if _, err := fmt.Sscanf(line, "program.py:%d:", &n); err == nil && strings.Contains(line, "error:") {
			reported[n] = true
		}
	}
	for _, n := range expected {
		if !reported[n] {
			t.Errorf("mypy did not flag line %d:\n%s", n, report)
		}
		delete(reported, n)
	}
	for n := range reported {
		t.Errorf("mypy flagged unexpected line %d:\n%s", n, report)
	}

	run := exec.Command(python, "program.py")
	run.Dir = work
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("python: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output: %v\n%s", err, out)
	}
	want := map[string]any{
		"keys":           []any{"isUrgent", "isUrgent:free", "department", "about"},
		"aboutCriteria":  map[string]any{"none": nil, "k01": map[string]any{"what": "about functor"}, "k02": map[string]any{"what": "about monad"}},
		"urgentCriteria": map[string]any{"true": "Urgent for a free customer"},
		"n":              0.9, "c": "billing", "a": "k02",
		"badOk": false, "readErrors": float64(1),
		"allOk": false, "allErrors": float64(3),
		"duplicate": `[sufleur] key "isUrgent" is already used in this batch: give repeated questions distinct keys`,
		"model":     "jev-latest",
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("\n got %s\nwant %s", gotJSON, wantJSON)
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
