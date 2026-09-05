package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/sufleur/cli/internal/toolschema"
	"github.com/sufleur/cli/internal/userapi"
)

func TestParseToolRef(t *testing.T) {
	cases := []struct {
		arg             string
		requireVersion  bool
		wantErrContains string
	}{
		{arg: "@acme/web-search"},
		{arg: "@acme/web_search"},
		{arg: "@acme/web-search@1.0.0", requireVersion: true},
		{arg: "@acme/web-search@draft", requireVersion: true},
		{arg: "@acme/+pack", wantErrContains: "+ marker"},
		// Stricter than a prompt name: no dots, must start with a letter.
		{arg: "@acme/web.search", wantErrContains: "wire name"},
		{arg: "@acme/9lives", wantErrContains: "wire name"},
		{arg: "@acme/Web-Search", wantErrContains: "wire name"},
		{arg: "@acme/web-search", requireVersion: true, wantErrContains: "version is required"},
	}

	for _, c := range cases {
		t.Run(c.arg, func(t *testing.T) {
			ref, err := parseToolRef(c.arg, c.requireVersion)
			if c.wantErrContains == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if ref.Workspace != "acme" {
					t.Errorf("workspace = %q", ref.Workspace)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", c.wantErrContains)
			}
			if !strings.Contains(err.Error(), c.wantErrContains) {
				t.Errorf("error = %v, want it to mention %q", err, c.wantErrContains)
			}
		})
	}
}

func TestWriteToolDump(t *testing.T) {
	dir := t.TempDir()
	tool := &userapi.Tool{
		Name: "web-search", Description: "Catalog blurb", Visibility: "PRIVATE", Tags: []string{"search"},
	}
	version := &userapi.ToolVersion{
		Version: "1.2.0", Status: "PUBLISHED",
		ModelDescription: "Searches the web.",
		InputSchema:      map[string]any{"type": "object"},
		OutputSchema:     map[string]any{"type": "object"},
		Metadata:         map[string]any{"owner": "platform"},
		Readme:           "# Web search\n",
	}

	written, err := writeToolDump(dir, tool, version)
	if err != nil {
		t.Fatalf("writeToolDump: %v", err)
	}
	if written != 6 {
		t.Errorf("wrote %d files, want 6", written)
	}

	// The model-facing description and the catalog blurb are different things
	// and must not be conflated in the dump.
	description := readDumpFile(t, dir, "description.md")
	if description != "Searches the web." {
		t.Errorf("description.md = %q", description)
	}
	var toolYAML map[string]any
	if err := yaml.Unmarshal([]byte(readDumpFile(t, dir, "tool.yaml")), &toolYAML); err != nil {
		t.Fatalf("parsing tool.yaml: %v", err)
	}
	if toolYAML["description"] != "Catalog blurb" {
		t.Errorf("tool.yaml description = %v", toolYAML["description"])
	}

	if readDumpFile(t, dir, "README.md") != "# Web search\n" {
		t.Errorf("README.md round-trip failed")
	}
	var inputSchema map[string]any
	if err := json.Unmarshal([]byte(readDumpFile(t, dir, "input-schema.json")), &inputSchema); err != nil {
		t.Fatalf("parsing input-schema.json: %v", err)
	}
	if inputSchema["type"] != "object" {
		t.Errorf("input-schema.json = %v", inputSchema)
	}
}

func TestWriteToolDump_OmitsAbsentOutputSchema(t *testing.T) {
	dir := t.TempDir()
	version := &userapi.ToolVersion{Version: "draft", Status: "DRAFT", InputSchema: map[string]any{"type": "object"}}

	written, err := writeToolDump(dir, &userapi.Tool{Name: "t"}, version)
	if err != nil {
		t.Fatalf("writeToolDump: %v", err)
	}
	if written != 5 {
		t.Errorf("wrote %d files, want 5 without an output schema", written)
	}
	if _, err := os.Stat(filepath.Join(dir, "output-schema.json")); !os.IsNotExist(err) {
		t.Error("output-schema.json must be omitted when the version has none")
	}
	// Empty metadata still round-trips as an object, not "null".
	if got := readDumpFile(t, dir, "metadata.yaml"); strings.TrimSpace(got) != "{}" {
		t.Errorf("metadata.yaml = %q, want {}", got)
	}
}

func readDumpFile(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(raw)
}

// A schema the generators cannot express is refused locally, before the command
// ever needs credentials — so the author gets the real problem, not "not logged in".
func TestToolSchemaSet_RejectsUnsupportedSchemaBeforeAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // guarantees "not logged in"

	dir := t.TempDir()
	path := filepath.Join(dir, "schema.json")
	schema := `{"type":"object","properties":{"a":{"$ref":"#/$defs/Thing"}}}`
	if err := os.WriteFile(path, []byte(schema), 0o644); err != nil {
		t.Fatalf("writing schema: %v", err)
	}

	toolSchemaSetCmd.SetContext(context.Background())
	_ = toolSchemaSetCmd.Flags().Set("file", path)
	_ = toolSchemaSetCmd.Flags().Set("output", "false")
	_ = toolSchemaSetCmd.Flags().Set("clear", "false")
	t.Cleanup(func() { _ = toolSchemaSetCmd.Flags().Set("file", "") })

	err := toolSchemaSetCmd.RunE(toolSchemaSetCmd, []string{"@acme/web-search@draft"})
	if err == nil {
		t.Fatal("expected the schema to be refused")
	}
	if !strings.Contains(err.Error(), "$ref") {
		t.Errorf("error should name the unsupported construct: %v", err)
	}
	if !strings.Contains(err.Error(), "/properties/a") {
		t.Errorf("error should point at the property: %v", err)
	}
	if strings.Contains(err.Error(), "not logged in") {
		t.Errorf("schema problems must be reported before credentials are needed: %v", err)
	}
}

func TestToolSchemaSet_ClearOnlyAppliesToTheOutputSchema(t *testing.T) {
	toolSchemaSetCmd.SetContext(context.Background())
	_ = toolSchemaSetCmd.Flags().Set("clear", "true")
	_ = toolSchemaSetCmd.Flags().Set("output", "false")
	_ = toolSchemaSetCmd.Flags().Set("file", "")
	t.Cleanup(func() { _ = toolSchemaSetCmd.Flags().Set("clear", "false") })

	err := toolSchemaSetCmd.RunE(toolSchemaSetCmd, []string{"@acme/web-search@draft"})
	if err == nil || !strings.Contains(err.Error(), "input schema is required") {
		t.Errorf("expected --clear to be refused for the input schema, got %v", err)
	}
}

func TestSplitMetadataKV(t *testing.T) {
	if k, v, err := splitMetadataKV("owner=platform"); err != nil || k != "owner" || v != "platform" {
		t.Errorf("got %q=%q err=%v", k, v, err)
	}
	// A value containing "=" keeps everything after the first separator.
	if _, v, err := splitMetadataKV("expr=a=b"); err != nil || v != "a=b" {
		t.Errorf("value = %q err=%v", v, err)
	}
	if _, _, err := splitMetadataKV("noseparator"); err == nil {
		t.Error("expected an error without a separator")
	}
	if _, _, err := splitMetadataKV("=novalue"); err == nil {
		t.Error("expected an error with an empty key")
	}
}

func TestCollectMetadataPatches(t *testing.T) {
	_ = toolVersionSetMetadataCmd.Flags().Set("string", "owner=platform")
	_ = toolVersionSetMetadataCmd.Flags().Set("integer", "retries=3")
	_ = toolVersionSetMetadataCmd.Flags().Set("float", "weight=0.5")
	_ = toolVersionSetMetadataCmd.Flags().Set("boolean", "beta=true")
	_ = toolVersionSetMetadataCmd.Flags().Set("delete", "stale")
	t.Cleanup(func() {
		for _, f := range []string{"string", "integer", "float", "boolean", "delete"} {
			_ = toolVersionSetMetadataCmd.Flags().Set(f, "")
			toolVersionSetMetadataCmd.Flags().Lookup(f).Changed = false
		}
	})

	patches, err := collectMetadataPatches(toolVersionSetMetadataCmd)
	if err != nil {
		t.Fatalf("collectMetadataPatches: %v", err)
	}

	byKey := map[string]metadataPatch{}
	for _, p := range patches {
		byKey[p.key] = p
	}
	if byKey["owner"].value != "platform" {
		t.Errorf("owner = %v", byKey["owner"].value)
	}
	if byKey["retries"].value != int64(3) {
		t.Errorf("retries = %v (%T)", byKey["retries"].value, byKey["retries"].value)
	}
	if byKey["weight"].value != 0.5 {
		t.Errorf("weight = %v", byKey["weight"].value)
	}
	if byKey["beta"].value != true {
		t.Errorf("beta = %v", byKey["beta"].value)
	}
	if !byKey["stale"].remove {
		t.Error("stale should be a removal")
	}
}

func TestCollectMetadataPatches_RejectsBadValues(t *testing.T) {
	_ = toolVersionSetMetadataCmd.Flags().Set("integer", "retries=many")
	t.Cleanup(func() {
		_ = toolVersionSetMetadataCmd.Flags().Set("integer", "")
		toolVersionSetMetadataCmd.Flags().Lookup("integer").Changed = false
	})

	if _, err := collectMetadataPatches(toolVersionSetMetadataCmd); err == nil {
		t.Error("expected a non-integer value to be rejected")
	}
}

func TestToolUpdate_RequiresDescription(t *testing.T) {
	toolUpdateCmd.SetContext(context.Background())
	toolUpdateCmd.Flags().Lookup("description").Changed = false

	err := toolUpdateCmd.RunE(toolUpdateCmd, []string{"@acme/web-search"})
	if err == nil || !strings.Contains(err.Error(), "--description is required") {
		t.Errorf("expected --description to be required, got %v", err)
	}
}

// `tool dump` writes metadata.yaml, so --from-file has to read YAML — parsing
// JSON here broke the documented dump → edit → push loop on the first push.
func TestParseToolMetadataFile(t *testing.T) {
	fromDump := "owner: search-team\nretries: 3\n"
	parsed, err := parseToolMetadataFile([]byte(fromDump))
	if err != nil {
		t.Fatalf("parsing dumped YAML: %v", err)
	}
	if parsed["owner"] != "search-team" || parsed["retries"] != 3 {
		t.Errorf("YAML did not round-trip: %v", parsed)
	}

	// YAML is a superset of JSON, so a hand-written JSON file still works.
	parsed, err = parseToolMetadataFile([]byte(`{"owner":"json-team"}`))
	if err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	if parsed["owner"] != "json-team" {
		t.Errorf("JSON did not parse: %v", parsed)
	}

	// An empty file is an empty object, which clears the metadata.
	parsed, err = parseToolMetadataFile(nil)
	if err != nil || len(parsed) != 0 {
		t.Errorf("empty file should clear metadata, got %v (%v)", parsed, err)
	}

	if _, err := parseToolMetadataFile([]byte("- not\n- an object\n")); err == nil {
		t.Error("expected a list to be rejected")
	}
}

// The dump writes exactly what the push commands read back.
func TestToolDumpRoundTripsThroughTheSetters(t *testing.T) {
	dir := t.TempDir()
	version := &userapi.ToolVersion{
		Version: "draft", Status: "DRAFT",
		ModelDescription: "Searches the web.",
		InputSchema:      map[string]any{"type": "object", "properties": map[string]any{}},
		Metadata:         map[string]any{"owner": "search-team", "retries": 3},
		Readme:           "# Tool\n",
	}
	if _, err := writeToolDump(dir, &userapi.Tool{Name: "t"}, version); err != nil {
		t.Fatalf("writeToolDump: %v", err)
	}

	metadata, err := parseToolMetadataFile([]byte(readDumpFile(t, dir, "metadata.yaml")))
	if err != nil {
		t.Fatalf("metadata.yaml is not readable by set-metadata --from-file: %v", err)
	}
	if metadata["owner"] != "search-team" {
		t.Errorf("metadata lost a key on the round trip: %v", metadata)
	}

	var inputSchema map[string]any
	if err := json.Unmarshal([]byte(readDumpFile(t, dir, "input-schema.json")), &inputSchema); err != nil {
		t.Fatalf("input-schema.json is not readable by schema set: %v", err)
	}
	if issues := toolschema.ValidateInput(inputSchema); len(issues) != 0 {
		t.Errorf("a dumped schema must pass the local check: %v", issues)
	}
}
