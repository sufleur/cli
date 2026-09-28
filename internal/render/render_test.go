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

func TestRenderDecision_StateFileAndYAMLQuestion(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"decision.yaml":             "stateFile: state\nquestions:\n  team:\n    type: choice\n    criteria:\n      technical: null\n      billing: Payments\n  spam:\n    type: noul\n",
		"model-config.yaml":         "provider: typesafe\nmodel: jev-latest\nparameters: {}\n",
		"files/state.yaml.mustache": "ticket: \"{{ticket}}\"\npolicy: \"{{> policy}}\"\n",
		"files/team.yaml.mustache":  "question: \"Which team owns {{@field ticket.subject}}?\"\n",
		"files/spam.mustache":       "Is {{@field ticket.body}} spam for {{tier}}?",
		"files/policy.mustache":     "Refunds within {{days}} days.",
	})
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.RenderDecision(nil, map[string]map[string]any{
		"state": {"ticket": map[string]any{"subject": "Charged \"twice\"", "body": "x"}, "days": 30},
		"spam":  {"tier": "gold"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"model": "jev-latest",
		"state": map[string]any{
			"ticket": map[string]any{"subject": "Charged \"twice\"", "body": "x"},
			"policy": "Refunds within 30 days.",
		},
		"questions": map[string]any{
			"team": map[string]any{
				"type":         "choice",
				"instructions": map[string]any{"question": "Which team owns `ticket.subject`?"},
				"criteria":     map[string]any{"technical": nil, "billing": "Payments"},
			},
			"spam": map[string]any{"type": "noul", "instructions": "Is `ticket.body` spam for gold?"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request mismatch:\n got %s", out)
	}
	if strings.Index(string(out), `"team"`) > strings.Index(string(out), `"spam"`) {
		t.Error("questions must keep decision.yaml order")
	}

	if _, err := p.RenderDecision("raw", nil); err == nil {
		t.Error("a raw state must be rejected when the prompt has a state file")
	}
}
