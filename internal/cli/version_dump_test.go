package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/sufleur/cli/internal/userapi"
)

func TestWriteDump_WritesEvalYaml(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{
		Version:  "1.0.0",
		Files:    []userapi.PromptFile{{Name: "welcome", Content: "Hi"}},
		Readme:   "readme",
		Metadata: map[string]any{},
	}

	// Eval YAML without a trailing newline — writeDump must append one.
	if _, err := writeDump(dir, v, "description: hi\ndataset:\n  ref: null", nil); err != nil {
		t.Fatalf("writeDump: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "eval.yaml"))
	if err != nil {
		t.Fatalf("eval.yaml not written: %v", err)
	}
	if string(raw) != "description: hi\ndataset:\n  ref: null\n" {
		t.Errorf("eval.yaml = %q (expected trailing newline appended)", string(raw))
	}

	// The rest of the dump must still be present alongside eval.yaml.
	if _, err := os.Stat(filepath.Join(dir, "files", "welcome.mustache")); err != nil {
		t.Errorf("welcome.mustache missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Errorf("README.md missing: %v", err)
	}
}

func TestWriteDump_EmptyEvalYamlStillWritesFile(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{Version: "1.0.0", Metadata: map[string]any{}}
	if _, err := writeDump(dir, v, "", nil); err != nil {
		t.Fatalf("writeDump: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "eval.yaml"))
	if err != nil {
		t.Fatalf("eval.yaml not written for empty input: %v", err)
	}
	if string(raw) != "\n" {
		t.Errorf("eval.yaml = %q, want a single newline", string(raw))
	}
}

func TestWriteDump_WritesModelConfigYaml(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{
		Version:  "1.0.0",
		Metadata: map[string]any{},
		ModelConfig: &userapi.ModelConfig{
			Provider:   "ANTHROPIC",
			Model:      "claude-sonnet-4-5",
			Parameters: map[string]any{"temperature": 0.7},
		},
	}
	if _, err := writeDump(dir, v, "", nil); err != nil {
		t.Fatalf("writeDump: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "model-config.yaml"))
	if err != nil {
		t.Fatalf("model-config.yaml not written: %v", err)
	}
	want := "provider: anthropic\nmodel: claude-sonnet-4-5\nparameters:\n    temperature: 0.7\n"
	if string(raw) != want {
		t.Errorf("model-config.yaml = %q, want %q", string(raw), want)
	}
}

func TestWriteDump_NoModelConfigOmitsFile(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{Version: "1.0.0", Metadata: map[string]any{}, ModelConfig: nil}
	if _, err := writeDump(dir, v, "", nil); err != nil {
		t.Fatalf("writeDump: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "model-config.yaml")); !os.IsNotExist(err) {
		t.Errorf("model-config.yaml should not be written when ModelConfig is nil, stat err = %v", err)
	}
}

func TestWriteDump_WritesAllExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{
		Version:  "1.0.0",
		Files:    []userapi.PromptFile{{Name: "main", Content: "Hi"}, {Name: "helper", Content: "Help"}},
		Readme:   "# README",
		Metadata: map[string]any{},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"result": map[string]any{"type": "string"},
			},
		},
		ModelConfig: &userapi.ModelConfig{
			Provider:   "ANTHROPIC",
			Model:      "claude-opus-4-1",
			Parameters: map[string]any{"temperature": 0.7},
		},
	}

	if _, err := writeDump(dir, v, "description: test", nil); err != nil {
		t.Fatalf("writeDump: %v", err)
	}

	// Verify all expected files exist
	expectedFiles := []string{
		"files/main.mustache",
		"files/helper.mustache",
		"README.md",
		"metadata.yaml",
		"output-schema.json",
		"model-config.yaml",
		"eval.yaml",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist, got error: %v", f, err)
		}
	}
}

// tools.yaml is always written, so its absence never has to be told apart from
// "this version pins nothing".
func TestWriteDump_WritesToolsYAMLWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{Version: "1.0.0", Metadata: map[string]any{}}

	written, err := writeDump(dir, v, "", nil)
	if err != nil {
		t.Fatalf("writeDump: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "tools.yaml"))
	if err != nil {
		t.Fatalf("tools.yaml not written: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); got != "tools: []" {
		t.Errorf("tools.yaml = %q, want an empty list rather than null", got)
	}
	// README.md, metadata.yaml, eval.yaml, tools.yaml.
	if written != 4 {
		t.Errorf("wrote %d files, want 4", written)
	}
}

func TestWriteDump_WritesPins(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{Version: "1.0.0", Metadata: map[string]any{}}
	pins := []userapi.PromptToolPin{{
		Alias: "web_search",
		ToolVersion: userapi.PinnedToolVersion{
			Version: "1.2.0", Status: "PUBLISHED",
			Tool: userapi.PinnedTool{Name: "web-search"},
		},
	}}
	pins[0].ToolVersion.Tool.Workspace.Name = "vendor"

	if _, err := writeDump(dir, v, "", pins); err != nil {
		t.Fatalf("writeDump: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "tools.yaml"))
	if err != nil {
		t.Fatalf("tools.yaml not written: %v", err)
	}
	var parsed struct {
		Tools []map[string]any `yaml:"tools"`
	}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parsing tools.yaml: %v", err)
	}
	if len(parsed.Tools) != 1 {
		t.Fatalf("expected 1 pin, got %v", parsed.Tools)
	}
	// The tool's own workspace, which may differ from the prompt's.
	if parsed.Tools[0]["ref"] != "@vendor/web-search" {
		t.Errorf("ref = %v", parsed.Tools[0]["ref"])
	}
	if parsed.Tools[0]["alias"] != "web_search" || parsed.Tools[0]["version"] != "1.2.0" {
		t.Errorf("pin = %v", parsed.Tools[0])
	}
}

// The reported count comes from writeDump itself, so it cannot drift from what
// was actually written.
func TestWriteDump_CountsEveryFile(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{
		Version:      "1.0.0",
		Files:        []userapi.PromptFile{{Name: "a"}, {Name: "b"}},
		Metadata:     map[string]any{},
		OutputSchema: map[string]any{"type": "object"},
		ModelConfig:  &userapi.ModelConfig{Provider: "ANTHROPIC", Model: "m"},
	}

	written, err := writeDump(dir, v, "description: x", nil)
	if err != nil {
		t.Fatalf("writeDump: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	onDisk := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		onDisk++
	}
	onDisk += len(v.Files) // the files/ subdirectory

	if written != onDisk {
		t.Errorf("reported %d files, %d on disk", written, onDisk)
	}
}
