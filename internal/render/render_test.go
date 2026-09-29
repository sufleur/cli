package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, "files", name+".mustache"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoad_FilesAndSchema(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"entry":   "hi {{name}}",
		"partial": "shared",
	})
	if err := os.WriteFile(filepath.Join(dir, "output-schema.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Files["entry"] != "hi {{name}}" || p.Files["partial"] != "shared" {
		t.Errorf("files: %+v", p.Files)
	}
	if p.OutputSchema["type"] != "object" {
		t.Errorf("schema: %+v", p.OutputSchema)
	}
}

func TestLoad_NoSchema(t *testing.T) {
	dir := writeFiles(t, map[string]string{"entry": "x"})
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.OutputSchema != nil {
		t.Errorf("schema = %+v, want nil", p.OutputSchema)
	}
}

func TestRender_BasicVarsAndPartials(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"entry":    "Hello {{name}}! {{>greeting}}",
		"greeting": "Welcome to {{place}}.",
	})
	p, _ := Load(dir)
	out, err := p.Render("entry", map[string]any{"name": "Tom", "place": "Sufleur"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "Hello Tom! Welcome to Sufleur."
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestRender_SufleurAnnotationsRenderEmpty(t *testing.T) {
	// {{@type ...}} and {{@doc ...}} are platform directives; without matching
	// vars they should disappear via Mustache's missing-key behavior.
	dir := writeFiles(t, map[string]string{
		"entry": "Hi {{user.name}}{{@type string}}{{@doc User's name}}!",
	})
	p, _ := Load(dir)
	out, err := p.Render("entry", map[string]any{"user": map[string]any{"name": "Tom"}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out != "Hi Tom!" {
		t.Errorf("got %q, want %q", out, "Hi Tom!")
	}
}

func TestRender_OutputSchemaSubstitution(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"entry": "Schema:\n{{@outputSchema}}",
	})
	if err := os.WriteFile(filepath.Join(dir, "output-schema.json"), []byte(`{"type":"object","required":["x"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, _ := Load(dir)
	out, err := p.Render("entry", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, `"type": "object"`) || !strings.Contains(out, `"required":`) {
		t.Errorf("schema not injected as JSON: %q", out)
	}
}

func TestRender_OutputSchemaWhitespaceTolerant(t *testing.T) {
	dir := writeFiles(t, map[string]string{"entry": "a {{ @outputSchema }} b"})
	if err := os.WriteFile(filepath.Join(dir, "output-schema.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, _ := Load(dir)
	out, err := p.Render("entry", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, `"type": "object"`) {
		t.Errorf("spaced directive not substituted: %q", out)
	}
	if strings.Contains(out, "@outputSchema") {
		t.Errorf("directive left in output: %q", out)
	}
}

func TestRender_OutputSchemaEmptyWhenNil(t *testing.T) {
	dir := writeFiles(t, map[string]string{"entry": "x{{@outputSchema}}y"})
	p, _ := Load(dir)
	out, err := p.Render("entry", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out != "xy" {
		t.Errorf("got %q, want %q", out, "xy")
	}
}

func TestRender_UnknownEntrypoint(t *testing.T) {
	dir := writeFiles(t, map[string]string{"entry": "x"})
	p, _ := Load(dir)
	_, err := p.Render("missing", nil)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("err = %v, want one mentioning 'missing'", err)
	}
}

func TestRender_Sections(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"entry": "{{#yes}}Y{{/yes}}{{^no}}N{{/no}}",
	})
	p, _ := Load(dir)
	out, _ := p.Render("entry", map[string]any{"yes": true, "no": false})
	if out != "YN" {
		t.Errorf("got %q, want %q", out, "YN")
	}
}

// writeTree writes files at arbitrary relative paths (decision prompts need
// decision.yaml, model-config.yaml and .yaml.mustache files).
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRenderQuestion_TemplatedCriteriaYAMLAndOptions(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"decision.yaml":            "questions:\n  team:\n    type: choice\n    criteria:\n      technical: null\n      billing: Payments for {{{product}}}\n  about:\n    type: choice\n    criteria:\n      none: no concept fits\n    optionCriteria:\n      what: about \"{{{name}}}\"\n",
		"model-config.yaml":        "provider: typesafe\nmodel: jev-latest\nparameters: {}\n",
		"files/team.yaml.mustache": "question: \"Which team owns `ticket.subject`?\"\npolicy: \"{{> policy}}\"\n",
		"files/about.mustache":     "Which concept is {{{misconception}}} about?",
		"files/policy.mustache":    "Refunds within {{{days}}} days.",
	})
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	out, warnings, err := p.RenderQuestion("team", map[string]any{"product": "Acme & Co", "days": 30}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"type":         "choice",
		"instructions": map[string]any{"question": "Which team owns `ticket.subject`?", "policy": "Refunds within 30 days."},
		"criteria":     map[string]any{"technical": nil, "billing": "Payments for Acme & Co"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rendered question mismatch:\n got %s", out)
	}

	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	out, _, err = p.RenderQuestion("about", map[string]any{"misconception": "functors"}, []byte(`{"k02":{"name":"monad"},"k01":{"name":"functor"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"none": "no concept fits"`) ||
		strings.Index(string(out), `"k02"`) > strings.Index(string(out), `"k01"`) {
		t.Fatalf("options must follow the fixed options in the given order:\n%s", out)
	}

	if _, warnings, err := p.RenderQuestion("about", map[string]any{}, []byte(`{"k01":{"name":"functor"}}`)); err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "misconception") {
		t.Errorf("a missing input must be warned about, got %v %v", warnings, err)
	}
	if _, _, err := p.RenderQuestion("team", nil, []byte(`{"x":{}}`)); err == nil || !strings.Contains(err.Error(), "fixed set of options") {
		t.Errorf("options on a closed choice must be rejected, got %v", err)
	}
	if _, _, err := p.RenderQuestion("missing", nil, nil); err == nil {
		t.Error("an unknown question must be rejected")
	}
}
