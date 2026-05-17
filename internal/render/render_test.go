package render

import (
	"os"
	"path/filepath"
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
		"entry":   "Hello {{name}}! {{>greeting}}",
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
