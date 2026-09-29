package typescript

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

// runtimeDir returns a directory whose node_modules has mustache, zod, tsx and
// typescript, or skips the test. Generated code only runs against real
// dependencies, so these tests are opt-in:
//
//	SUFLEUR_TS_RUNTIME_DIR=/path/with/node_modules go test ./internal/generator/typescript/
func runtimeDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("SUFLEUR_TS_RUNTIME_DIR")
	if dir == "" {
		t.Skip("set SUFLEUR_TS_RUNTIME_DIR to a directory with mustache, zod, tsx and typescript installed")
	}
	work, err := os.MkdirTemp(dir, "decision-runtime-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(work) })
	return work
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
// shared rendering case, runs question() in Node, and compares the output with
// the case's expected bytes — the same file the backend and Go render use.
func TestGeneratedDecisionRuntimeMatchesSharedCases(t *testing.T) {
	work := runtimeDir(t)
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
			options = "undefined"
		}
		fmt.Fprintf(&calls, "run(() => (getDecision as any)('@t/%s').question('q', %s, { options: %s }));\n", name, inputs, options)
	}

	if err := (&Generator{}).Generate(filepath.Join(work, "prompts.ts"), prompts); err != nil {
		t.Fatal(err)
	}
	script := `import { getDecision } from './prompts';
const out: unknown[] = [];
const run = (fn: () => unknown) => {
  try { out.push({ ok: JSON.stringify(fn()) }); } catch (e) { out.push({ error: (e as Error).message }); }
};
` + calls.String() + `console.log(JSON.stringify(out));
`
	if err := os.WriteFile(filepath.Join(work, "run.ts"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(work), "node_modules", ".bin", "tsx"), "run.ts")
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
				if got.Instructions != c.Escaped["mustache.js"] {
					t.Fatalf("got %q, want %q", got.Instructions, c.Escaped["mustache.js"])
				}
			default:
				var want bytes.Buffer
				if err := json.Compact(&want, c.Expected); err != nil {
					t.Fatal(err)
				}
				if r.Ok == nil || *r.Ok != want.String() {
					t.Fatalf("\n got %v (error %v)\nwant %s", deref(r.Ok), deref(r.Error), want.String())
				}
			}
		})
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// TestGeneratedDecisionTypesAndBatch compiles the generated code under strict
// TypeScript with type tests (each expect-error must fail to compile), then
// runs a batch end to end: keys, options, partial read and readAll.
func TestGeneratedDecisionTypesAndBatch(t *testing.T) {
	work := runtimeDir(t)
	if err := (&Generator{}).Generate(filepath.Join(work, "prompts.ts"), []generator.PromptData{decisionFixture(t)}); err != nil {
		t.Fatal(err)
	}
	program := `import { getDecision, type RenderedQuestion } from './prompts';

const triage = getDecision('@acme/triage');
const batch = triage.batch();
const urgent = batch.ask('isUrgent', { tier: 'gold' });
const again = batch.ask('isUrgent', { tier: 'free' }, { key: 'isUrgent:free' });
const team = batch.ask('department');
const about = batch.ask('about', {}, { options: { k01: { name: 'functor' }, k02: { name: 'monad' } } });

if (Math.random() > 2) {
  // Type-only checks: each line must fail to compile, and never runs.
  // @ts-expect-error — isUrgent requires its tier input
  batch.ask('isUrgent');
  // @ts-expect-error — department is a closed choice: no options
  batch.ask('department', {}, { options: { x: {} } });
  // @ts-expect-error — unknown question
  batch.ask('priority');
  // @ts-expect-error — option inputs are typed
  triage.question('about', {}, { options: { k01: { nam: 'x' } } });
}

const questions: Record<string, RenderedQuestion> = Object.fromEntries(
  batch.items().map((item) => [item.key, item.question]),
);

const read = batch.read({
  isUrgent: { type: 'noul', noul: 0.9 },
  'isUrgent:free': { type: 'noul', noul: 'high' },
  department: { type: 'choice', choice: 'billing' },
  about: { type: 'choice', choice: 'k02' },
});
const u = read.getOrThrow(urgent);
const n: number = u.noul;
const c: 'technical' | 'billing' = read.getOrThrow(team).choice;
const a: string = read.getOrThrow(about).choice;
if (Math.random() > 2) {
  // @ts-expect-error — a noul answer has no choice
  read.getOrThrow(urgent).choice;
}
const bad = read.get(again);

const all = batch.readAll({ isUrgent: { type: 'noul', noul: 0.9 } });

let duplicate = '';
try {
  batch.ask('isUrgent', { tier: 'x' });
} catch (e) {
  duplicate = (e as Error).message;
}

const errorOf = (fn: () => unknown): string => {
  try {
    fn();
    return '';
  } catch (e) {
    return (e as Error).message;
  }
};
const missingInput = errorOf(() => (triage.question as any)('isUrgent', {}));
const missingOptionInput = errorOf(() => (triage.question as any)('about', {}, { options: { k01: {} } }));

console.log(JSON.stringify({
  missingInput,
  missingOptionInput,
  keys: Object.keys(questions),
  aboutCriteria: questions.about.criteria,
  urgentCriteria: questions['isUrgent:free'].criteria,
  n, c, a,
  badOk: bad.success,
  readErrors: read.errors.length,
  allOk: all.success,
  allErrors: all.success ? [] : all.errors.length,
  duplicate,
  model: triage.metadata.modelConfig.model,
}));
`
	if err := os.WriteFile(filepath.Join(work, "program.ts"), []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(filepath.Dir(work), "node_modules", ".bin")
	tsc := exec.Command(filepath.Join(bin, "tsc"), "--noEmit", "--strict", "--target", "es2022",
		"--module", "esnext", "--moduleResolution", "bundler", "--skipLibCheck", "--types", "node", "program.ts")
	tsc.Dir = work
	if out, err := tsc.CombinedOutput(); err != nil {
		t.Fatalf("tsc: %v\n%s", err, out)
	}
	run := exec.Command(filepath.Join(bin, "tsx"), "program.ts")
	run.Dir = work
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("tsx: %v\n%s", err, out)
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
		"duplicate":          `[sufleur] key "isUrgent" is already used in this batch: give repeated questions distinct keys`,
		"model":              "jev-latest",
		"missingInput":       `[sufleur] "isUrgent" is missing required input(s): tier`,
		"missingOptionInput": `[sufleur] "about": option "k01" is missing required input(s): name`,
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("\n got %s\nwant %s", gotJSON, wantJSON)
	}
}
